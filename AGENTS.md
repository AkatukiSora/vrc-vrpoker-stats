# AGENTS.md

Repository guidance for contributors and coding agents. Keep changes scoped to the assigned task and follow the repository's current tasks and CI workflow.

## Project overview

- Go module: `github.com/AkatukiSora/vrc-vrpoker-ststs`
- Desktop UI: Fyne (`fyne.io/fyne/v2`)
- Domain: VRChat poker-log parsing, statistics aggregation, and local persistence

Key packages:

- `internal/parser`: parse VRChat logs into hands, players, and actions.
- `internal/stats`: aggregate metrics and hand-range summaries.
- `internal/watcher`: tail and monitor log files.
- `internal/application`: import orchestration and snapshots.
- `internal/persistence`: repository interfaces plus memory and SQLite implementations.
- `internal/ui`: Fyne tabs, settings, and visual components.

## Working with others

Before editing, inspect `git status --short`, the current branch, and the relevant diff. A worktree can contain changes made by another contributor or agent: preserve them, do not revert or overwrite them, and adapt your change around them. If the scopes conflict, stop and coordinate with the task owner.

Claim clear ownership of the files and behavior you change. Keep commits focused on one task; stage explicit paths rather than all worktree changes. In a handoff, report the files changed, checks run and their results, remaining risks, and any follow-up required.

The worker that owns a branch or pull request owns its updates, CI failures, and review follow-up. Route findings to that owner instead of making unrelated changes on its branch. Open or update a PR only when the task or project workflow calls for it; describe the behavior change and validation in the PR.

## Setup and commands

Install the declared tools with `mise install`. `mise run setup` also tidies modules and installs Lefthook hooks. On Linux, `mise run deps-linux` installs the native Wayland/CGO and MinGW prerequisites when needed.

Prefer these tasks from `mise.toml`:

- Build native Linux binary: `mise run build`
- Debug build: `mise run build-debug`
- Run native app: `mise run run`
- Cross-build Windows binary: `mise run build-windows` (requires MinGW)
- Fast parser/stats/watcher suite: `mise run test`
- Hand-history UI regression suite: `mise run test-ui`
- CI-like UI suite: `mise run test-ui-ci`
- Lint with the Wayland build tag: `mise run lint`
- Check UI localization coverage: `mise run check-i18n`
- Tidy modules: `mise run tidy`
- Local build/lint/fast-test pass: `mise run ci`

`mise run ci` is a useful local baseline, but it is not identical to GitHub Actions. CI additionally checks that `go mod tidy` leaves `go.mod` and `go.sum` unchanged, checks `gofmt -s`, runs the i18n check, runs the UI regression suite, and builds Linux and Windows artifacts. Match the affected CI jobs when practical:

```bash
go mod tidy
git diff --exit-code go.mod go.sum
gofmt -s -l .
mise run check-i18n
mise run test
mise run test-ui-ci
mise run build
mise run build-windows
```

For Go changes, run the focused package test first, for example:

```bash
go test -v -run '^TestParseSimpleHand$' ./internal/parser/...
go test -v -run '^TestName$' ./path/to/package
```

Use `go test ./...` as a broader smoke test only when the required native UI dependencies are available. Do not assume a fixed test count or test layout; inspect the relevant `*_test.go` files when changing behavior.

## Go conventions

- Run `gofmt` on every modified Go file. Keep imports grouped as standard library, third-party, and internal packages.
- Exported names use `PascalCase`; unexported names use `camelCase`. Keep poker acronyms consistent (`VPIP`, `PFR`, `WWSF`, `ThreeBet`).
- Prefer explicit structs at package boundaries and interfaces only at real swap points, such as services or repositories.
- Wrap errors with context (`fmt.Errorf("...: %w", err)`). Fail fast for setup and I/O errors; tolerate malformed log lines while parsing.
- Guard shared state with `sync.Mutex` or `sync.RWMutex`, keep lock scope small, and do not perform I/O while holding a lock.
- Fyne UI updates must run on the main thread with `fyne.Do(...)`. Guard asynchronous UI work with cancellation or generation checks.

## Domain rules

- Keep the parser model authoritative; do not re-implement hand logic in the UI.
- Preserve action order and street semantics. Keep world/session filtering explicit and persistence imports idempotent.
- Compile regular expressions once at package scope.
- Prefer opportunity-based metrics (`count / opp`). Show `n=` consistently and mark below-threshold values as `参考値` according to the metric definitions.
- For parser or statistics changes, add focused tests. Table-driven tests are preferred for classification and opportunity logic; cover relevant boundaries such as missing blinds, seat changes, and partial logs.

## UI and localization

- Keep business rules out of `internal/ui`; avoid duplicating metric definitions, labels, or thresholds.
- Prefer updating existing tab views over rebuilding containers, and keep typed widget references instead of traversing container indexes.
- All user-visible strings in `internal/ui/` use `fyne.io/fyne/v2/lang`, normally `lang.X(key, fallback)`.
- Add new translation keys to both `internal/ui/translations/en.json` and `internal/ui/translations/ja.json`. Metric labels such as VPIP, PFR, and 3Bet remain untranslated.
- Use `//i18n:ignore <reason>` only for an intentional exception.

## ADRs and repository hygiene

ADRs live in `docs/adr/`. Use `mise run adr-list` to inspect them and `mise run adr-new -- "Title"` to create one. Follow the repository ADR documentation for status, amendment, or replacement relationships.

Do not commit local logs, screenshots, generated binaries, coverage files, or runtime SQLite databases. Include `go.mod` and `go.sum` in the same commit when a dependency change requires them.
