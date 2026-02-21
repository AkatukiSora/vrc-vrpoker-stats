# AGENTS.md

Guidance for coding agents operating in this repository.
Scope: build/test commands, workflow expectations, and code style rules.

## Project Overview

- Language: Go
- Module: `github.com/AkatukiSora/vrc-vrpoker-ststs`
- UI framework: Fyne (`fyne.io/fyne/v2`)
- Main domain: VRChat poker log parsing, stats aggregation, and desktop visualization
- Architecture trend: parser/import pipeline + repository boundary (memory/SQLite)

## Environment and Tooling

- `go.mod` Go version: `1.25.0`
- Local toolchain via `mise.toml`: `go = "1.25.0"`
- Build tags: `wayland`
- Expected env for build: `CGO_ENABLED=1`
- Task runner source of truth: `mise` tasks

## Build, Run, Lint, Test Commands

Use these first (from `mise.toml`).

### Setup

- Install tools: `mise install`
- Optional project setup: `mise run setup`

### Build and Run

- Release-style build: `mise run build`
- Debug build: `mise run build-debug`
- Build + run: `mise run run`
- Direct build equivalent:
  - `go build -tags "$BUILD_TAGS" -ldflags "-s -w" -o "$BINARY" .`

### Lint

- Preferred: `mise run lint`
- Direct equivalent: `go vet -tags wayland ./...`

### Tests

- Fast suite (parser/stats/watcher): `mise run test`
- Full parser/stats/watcher suite: `mise run test-all`
- Parser-only: `mise run test-parser`
- Whole repo smoke test: `go test ./...`

### Single Test Execution (important)

- Exact test in package:
  - `go test -v -run '^TestParseSimpleHand$' ./internal/parser/...`
- Any package exact match:
  - `go test -v -run '^TestName$' ./path/to/pkg`
- Regex subset:
  - `go test -v -run 'TestParse(Card|ShowdownHand)' ./internal/parser/...`
- Flake check loop:
  - `go test -count=20 -run '^TestParseShowdownHand$' ./internal/parser/...`

### Module Hygiene

- Tidy modules: `mise run tidy`
- CI expects no diff in `go.mod`/`go.sum` after tidy.
- `go mod tidy` is also enforced by pre-commit hooks (lefthook).
- If pre-commit updates `go.mod`/`go.sum`, include those updates in the same commit.

## Repository Rule Files

Checked in this repository:

- `.cursor/rules/`: not found
- `.cursorrules`: not found
- `.github/copilot-instructions.md`: not found

If these files appear later, treat them as higher-priority local policy.

## Package Responsibilities

- `internal/parser`: parse VRChat logs and construct hand/player/action data
- `internal/stats`: aggregate metrics and hand-range summaries
- `internal/watcher`: tail and monitor log files
- `internal/application`: import orchestration and snapshot service
- `internal/persistence`: repository interfaces + memory/SQLite backends
- `internal/ui`: Fyne tabs, settings, and visual components

## Coding Style and Conventions

### Formatting and Imports

- Always run `gofmt` on modified Go files.
- Keep imports standard-grouped: stdlib, third-party, internal.
- Do not leave unused imports, vars, or dead code.

### Naming

- Exported: `PascalCase` (`ParseResult`, `HandRangeTable`).
- Unexported: `camelCase` (`parseCards`, `metricRegistry`).
- Keep acronym style consistent (`VPIP`, `PFR`, `WWSF`, `ThreeBet`).
- Action/position constants should remain domain-specific (`ActionFold`, `PosBTN`).

### Types and API Design

- Prefer explicit structs at package boundaries.
- Add interfaces only for real swap points (service/repository/storage).
- Keep parser model authoritative; avoid re-encoding hand logic in UI.
- Avoid exposing mutable internals directly when a copy is safer.

### Error Handling

- Return errors with context (`fmt.Errorf("...: %w", err)`).
- Fail fast on setup/IO errors; tolerate malformed log lines while parsing.
- In long-running loops/watchers/UI callbacks, surface non-fatal errors in status UI.
- Avoid `panic` except for programmer errors that should never happen.

### Concurrency and UI Threading

- Guard shared mutable state with `sync.Mutex` / `sync.RWMutex`.
- Keep lock scope minimal; avoid blocking I/O while holding locks.
- Fyne UI updates must run on main thread (`fyne.Do(...)`).
- Ensure watcher replacement/shutdown does not leak goroutines.

### Stats and Metric Logic

- Prefer opportunity-based metrics (`count / opp`) over ad-hoc ratios.
- Keep threshold semantics consistent:
  - Hand-frequency metrics: confidence at `n >= 200`
  - Situational metrics: confidence at `n >= 50`
- Show `n=` consistently for user-facing metric cards/tables.
- Mark below-threshold values as reference (`参考値`).

### Parsing and Data Integrity

- Compile regex once as package globals.
- Preserve action ordering and street semantics.
- Keep world/session filtering explicit to avoid cross-world contamination.
- Maintain idempotent import expectations at persistence boundaries.

## Testing Expectations

- Add/adjust tests when parser or stat semantics change.
- Prefer table-driven tests for classification/opportunity logic.
- Validate both happy-path and edge cases (missing blinds, seat changes, partial logs).
- For integration-style real-log tests, allow skip behavior when fixture is unavailable.

## Practical Agent Workflow

1. Read `mise.toml`, touched package files, and related tests first.
2. Make minimal, scoped changes aligned with existing architecture.
3. Run `gofmt` on touched files.
4. Run focused tests for changed packages.
5. Run `go test ./...` when practical.
6. Summarize behavior impact, risks, and any follow-up work.

## Commit Hygiene for Agents

- Keep commits logically grouped by concern.
- Avoid mixing generated/local runtime artifacts with source changes.
- Do not commit local logs, screenshots, or SQLite runtime DB files.
- Use intent-focused messages (why-focused, not raw file lists).

## Subagent Usage

- Proactively use subagents (Task tool) for large or parallelizable tasks.
- Use the `general` subagent type for code writing and multi-step execution tasks.
- Use the `explore` subagent type for codebase research (file searches, keyword lookups, structural questions).
- When tasks are independent of each other, launch multiple subagents in parallel in a single message.
- Delegate focused, self-contained work to subagents (e.g., "edit these 3 files", "run these checks and fix errors").

## Internationalisation (i18n)

- All user-visible strings in `internal/ui/` **must** be wrapped with `fyne.io/fyne/v2/lang` functions.
- Prefer `lang.X(key, fallback)` as the primary API. Use `lang.L(fallback)` only for strings with no disambiguation need.
- For strings with runtime variables, use Go template syntax: `lang.X("key", "Hello {{.Name}}", map[string]any{"Name": name})`.
- Translation files live in `internal/ui/translations/en.json` (English, fallback) and `internal/ui/translations/ja.json` (Japanese).
- When adding a new UI string: (1) wrap it with `lang.X`, (2) add the key to both `en.json` and `ja.json`.
- Metric labels (e.g., "VPIP", "PFR", "3Bet") are intentionally left untranslated as international poker terminology.
- The pre-commit hook `check-i18n` and CI step enforce that no bare string literals appear in UI display APIs.
- Allow exceptions with `//i18n:ignore <reason>`.
- `//i18n:ignore` applies to the UI string on the same line or the immediately preceding line.
- `//i18n:ignore` without `<reason>` is treated as a warning by the checker.
- Do not use an allow list for i18n checker exceptions.
