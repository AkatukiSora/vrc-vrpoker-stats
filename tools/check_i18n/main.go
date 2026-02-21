package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	targetDir    = "internal/ui"
	ignoreMarker = "i18n:ignore"
)

type funcRule struct {
	pkg  string
	name string
	args []int
}

type ignoreTag struct {
	line      int
	hasReason bool
}

var functionRules = []funcRule{
	{pkg: "widget", name: "NewLabel", args: []int{0}},
	{pkg: "widget", name: "NewLabelWithStyle", args: []int{0}},
	{pkg: "widget", name: "NewButton", args: []int{0}},
	{pkg: "widget", name: "NewCheck", args: []int{0}},
	{pkg: "widget", name: "NewSelect", args: []int{0}},
	{pkg: "widget", name: "NewRadioGroup", args: []int{0}},
	{pkg: "widget", name: "NewAccordionItem", args: []int{0}},
	{pkg: "container", name: "NewTabItem", args: []int{0}},
	{pkg: "canvas", name: "NewText", args: []int{0}},
	{pkg: "dialog", name: "ShowInformation", args: []int{0, 1}},
	{pkg: "dialog", name: "ShowError", args: []int{0}},
	{pkg: "dialog", name: "ShowConfirm", args: []int{0, 1}},
	{pkg: "dialog", name: "ShowCustom", args: []int{0}},
}

var methodRules = map[string][]int{
	"SetPlaceHolder": {0},
	"SetText":        {0},
	"SetTitle":       {0},
}

func main() {
	files, err := collectGoFiles(targetDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to collect UI files: %v\n", err)
		os.Exit(1)
	}

	fset := token.NewFileSet()
	violations := 0
	warned := map[string]struct{}{}

	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse %s: %v\n", path, err)
			os.Exit(1)
		}

		commentsByLine := collectIgnoreTags(fset, file)
		relPath := filepath.ToSlash(path)

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			argIndexes := targetArgIndexes(call)
			if len(argIndexes) == 0 {
				return true
			}

			for _, idx := range argIndexes {
				if idx >= len(call.Args) {
					continue
				}
				arg := unwrapExpr(call.Args[idx])
				if !isBareStringLiteral(arg) || isAllowedExpr(arg) {
					continue
				}

				line := fset.Position(arg.Pos()).Line
				ignored, warnLine := ignoreStatus(commentsByLine, line)
				if ignored {
					if warnLine > 0 {
						key := fmt.Sprintf("%s:%d", relPath, warnLine)
						if _, exists := warned[key]; !exists {
							fmt.Printf("WARN %s:%d: //i18n:ignore without reason\n", relPath, warnLine)
							warned[key] = struct{}{}
						}
					}
					continue
				}

				pos := fset.Position(arg.Pos())
				fmt.Printf("%s:%d:%d: bare UI string literal (use lang.X or //i18n:ignore <reason>)\n", relPath, pos.Line, pos.Column)
				violations++
			}

			return true
		})
	}

	if violations > 0 {
		os.Exit(1)
	}
}

func collectGoFiles(root string) ([]string, error) {
	files := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

func collectIgnoreTags(fset *token.FileSet, file *ast.File) map[int][]ignoreTag {
	tagsByLine := make(map[int][]ignoreTag)
	for _, group := range file.Comments {
		for _, c := range group.List {
			baseLine := fset.Position(c.Slash).Line
			for i, lineText := range commentLines(c.Text) {
				hasIgnore, hasReason := parseIgnoreComment(lineText)
				if !hasIgnore {
					continue
				}
				line := baseLine + i
				tagsByLine[line] = append(tagsByLine[line], ignoreTag{line: line, hasReason: hasReason})
			}
		}
	}
	return tagsByLine
}

func commentLines(text string) []string {
	if strings.HasPrefix(text, "//") {
		return []string{strings.TrimSpace(strings.TrimPrefix(text, "//"))}
	}
	if strings.HasPrefix(text, "/*") {
		trimmed := strings.TrimPrefix(text, "/*")
		trimmed = strings.TrimSuffix(trimmed, "*/")
		return strings.Split(trimmed, "\n")
	}
	return []string{text}
}

func parseIgnoreComment(text string) (bool, bool) {
	idx := strings.Index(text, ignoreMarker)
	if idx < 0 {
		return false, false
	}
	reason := strings.TrimSpace(text[idx+len(ignoreMarker):])
	return true, reason != ""
}

func ignoreStatus(tagsByLine map[int][]ignoreTag, targetLine int) (bool, int) {
	lines := []int{targetLine, targetLine - 1}
	hasIgnore := false
	hasReason := false
	noReasonLine := 0

	for _, line := range lines {
		tags := tagsByLine[line]
		for _, t := range tags {
			hasIgnore = true
			if t.hasReason {
				hasReason = true
				continue
			}
			if noReasonLine == 0 {
				noReasonLine = t.line
			}
		}
	}

	if !hasIgnore {
		return false, 0
	}
	if hasReason {
		return true, 0
	}
	return true, noReasonLine
}

func targetArgIndexes(call *ast.CallExpr) []int {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}

	if id, ok := sel.X.(*ast.Ident); ok {
		for _, rule := range functionRules {
			if id.Name == rule.pkg && sel.Sel.Name == rule.name {
				return rule.args
			}
		}
	}

	if args, ok := methodRules[sel.Sel.Name]; ok {
		return args
	}

	return nil
}

func unwrapExpr(expr ast.Expr) ast.Expr {
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = paren.X
	}
}

func isAllowedExpr(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Name != "lang" {
		return false
	}
	switch sel.Sel.Name {
	case "X", "L", "N", "XN":
		return true
	default:
		return false
	}
}

func isBareStringLiteral(expr ast.Expr) bool {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	unquoted, err := strconv.Unquote(lit.Value)
	if err != nil {
		return true
	}
	return unquoted != ""
}
