# AGENTS.md

Guidance for coding agents working in this repository.
Focus: build/test/lint commands, code style, and repo rules.

## Project Snapshot

- Language: Go (module: `github.com/AkatukiSora/vrc-vrpoker-ststs`)
- UI: Fyne (`fyne.io/fyne/v2`)
- Domain: VRChat poker log parsing, stats aggregation, desktop UI
- Key packages: parser/import pipeline + persistence (memory/SQLite)

## Commands (Build/Lint/Test)

Prefer tasks from `mise.toml` when available.

### Setup
- Install toolchain: `mise install`
- Optional setup: `mise run setup`

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
- Full suite: `mise run test-all`
- Parser-only: `mise run test-parser`
- Full repo smoke: `go test ./...`

### Single Test (important)
- Exact test in package:
  - `go test -v -run '^TestParseSimpleHand$' ./internal/parser/...`
- Exact test in any package:
  - `go test -v -run '^TestName$' ./path/to/pkg`
- Regex subset:
  - `go test -v -run 'TestParse(Card|ShowdownHand)' ./internal/parser/...`
- Flake loop:
  - `go test -count=20 -run '^TestParseShowdownHand$' ./internal/parser/...`

### Module Hygiene
- Tidy modules: `mise run tidy`
- Pre-commit enforces `go mod tidy`; include `go.mod`/`go.sum` updates in the same commit.

## Repository Rule Files

Checked in this repo:
- `.cursor/rules/`: not found
- `.cursorrules`: not found
- `.github/copilot-instructions.md`: not found

If these appear later, they override guidance here.

## Package Responsibilities

- `internal/parser`: parse VRChat logs into hand/player/action data
- `internal/stats`: metrics aggregation and hand-range summaries
- `internal/watcher`: tail and monitor log files
- `internal/application`: import orchestration and snapshot service
- `internal/persistence`: repository interfaces + memory/SQLite backends
- `internal/ui`: Fyne tabs, settings, visual components

## Code Style and Conventions

### Formatting and Imports
- Always run `gofmt` on modified Go files.
- Imports grouped as: stdlib, third-party, internal.
- Avoid unused imports/vars and dead code.

### Naming
- Exported: `PascalCase` (`ParseResult`, `HandRangeTable`).
- Unexported: `camelCase` (`parseCards`, `metricRegistry`).
- Keep acronyms consistent (`VPIP`, `PFR`, `WWSF`, `ThreeBet`).
- Domain constants should stay explicit (`ActionFold`, `PosBTN`).

### Types and APIs
- Prefer explicit structs at package boundaries.
- Add interfaces only at real swap points (service/repository/storage).
- Keep parser model authoritative; avoid re-encoding hand logic in UI.
- Avoid exposing mutable internals; copy when needed.

### Error Handling
- Wrap errors with context: `fmt.Errorf("...: %w", err)`.
- Fail fast on setup/IO errors; tolerate malformed log lines while parsing.
- In long-running loops/watchers/UI callbacks, surface non-fatal errors in status UI.
- Avoid `panic` except for programmer errors.

### Concurrency and UI Threading
- Guard shared state with `sync.Mutex`/`sync.RWMutex`.
- Keep lock scope minimal; avoid I/O while holding locks.
- Fyne UI updates must run on main thread: `fyne.Do(...)`.
- Ensure watcher replacement/shutdown does not leak goroutines.

### UI Architecture
- Keep business rules out of `internal/ui`.
- Single source of truth for metric definitions; do not duplicate labels/thresholds.
- Prefer update-in-place tab views over full container rebuilds.
- Guard async UI ops with cancellation or generation checks.
- Avoid deep container index traversal; keep typed widget references.

### Stats and Metric Logic
- Prefer opportunity-based metrics (`count / opp`).
- Thresholds:
  - Hand-frequency metrics: confidence at `n >= 200`.
  - Situational metrics: confidence at `n >= 50`.
- Show `n=` consistently on metric cards/tables.
- Mark below-threshold values as reference (`参考値`).

### Parsing and Data Integrity
- Compile regex once as package globals.
- Preserve action ordering and street semantics.
- Keep world/session filtering explicit to avoid cross-world contamination.
- Maintain idempotent import expectations at persistence boundaries.

## i18n Rules (UI)

- All user-visible strings in `internal/ui/` must use `fyne.io/fyne/v2/lang`.
- Prefer `lang.X(key, fallback)`.
- For runtime vars: `lang.X("key", "Hello {{.Name}}", map[string]any{"Name": name})`.
- Translation files: `internal/ui/translations/en.json` and `internal/ui/translations/ja.json`.
- Add new keys to both JSON files.
- Metric labels (VPIP/PFR/3Bet) remain untranslated.
- Allow exceptions with `//i18n:ignore <reason>`.

## Testing Expectations

- Add/adjust tests when parser or stat semantics change.
- Prefer table-driven tests for classification/opportunity logic.
- Validate edge cases: missing blinds, seat changes, partial logs.

## Practical Workflow

1. Read `mise.toml` and touched package files/tests first.
2. Make minimal, scoped changes aligned with current architecture.
3. Run `gofmt` on touched files.
4. Run focused tests for changed packages.
5. Run `go test ./...` when practical.
6. Summarize behavior impact, risks, and follow-up work.

## Commit Hygiene

- Keep commits logically grouped by concern.
- Avoid committing local logs, screenshots, or runtime SQLite DB files.
- Use intent-focused messages.
- If a feature branch has merged, start new work from latest `origin/main`.
