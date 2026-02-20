# AGENTS.md

This file is guidance for coding agents working in this repository.
It documents build/test/lint commands and project-specific coding conventions.

## Project Snapshot

- Language: Go
- Module: `github.com/AkatukiSora/vrc-vrpoker-ststs`
- GUI stack: Fyne (`fyne.io/fyne/v2`)
- Runtime focus: Linux/Wayland build for desktop app
- Current architecture direction: parser/import pipeline decoupled from persistence

## Tooling and Environment

- Go version in `go.mod`: `1.25.0`
- Preferred tool manager: `mise` (`mise.toml` specifies `go = "1.26.0"`)
- Build tags: `wayland`
- `CGO_ENABLED=1` is expected for UI builds

## Canonical Commands

Use `mise` tasks when possible; they are the source of truth for CI-local parity.

### Setup

- Install toolchain via mise:
  - `mise install`

### Build

- Standard build (Wayland):
  - `mise run build`
- Debug build (no strip):
  - `mise run build-debug`
- Direct equivalent:
  - `go build -tags wayland -ldflags "-s -w" -o vrpoker-stats .`

### Run

- Build + run:
  - `mise run run`
- Launcher script:
  - `./run.sh`

### Lint / Vet

- Preferred:
  - `mise run lint`
- Direct equivalent:
  - `go vet -tags "wayland" ./...`

### Tests

- Fast unit test set (CI-style, excludes real-log integration test):
  - `mise run test`
- All parser/stats/watcher tests (includes `TestRealLog`, which may skip if file missing):
  - `mise run test-all`
- Parser-only tests:
  - `mise run test-parser`
- Whole repo smoke test:
  - `go test ./...`

### Running a Single Test (important)

- Single test in parser package:
  - `go test -v -run '^TestParseSimpleHand$' ./internal/parser/...`
- Single test by exact name in any package:
  - `go test -v -run '^TestName$' ./path/to/pkg`
- Multiple tests by regex:
  - `go test -v -run 'TestParse(Card|ShowdownHand)' ./internal/parser/...`
- Run one test repeatedly (flake check):
  - `go test -count=20 -run '^TestParseShowdownHand$' ./internal/parser/...`

### Module hygiene

- Tidy modules:
  - `mise run tidy`
- CI expects `go mod tidy` to produce no diff in `go.mod`/`go.sum`.

## CI Notes

- CI tests parser/stats/watcher separately from UI-heavy build concerns.
- Build job installs Wayland/GL dev packages before `mise run build`.
- Do not assume headless CI can run GUI integration tests.

## Repository Rules Discovery

Agent checked for additional instruction files:

- `.cursor/rules/`: not found
- `.cursorrules`: not found
- `.github/copilot-instructions.md`: not found

If these are added later, update this file and follow them as higher-priority repository policy.

## Code Organization Conventions

- `internal/parser`: log parsing and poker-hand reconstruction
- `internal/stats`: aggregate/statistical calculations
- `internal/watcher`: file-change monitoring and tail-like reading
- `internal/application`: orchestration service layer
- `internal/persistence`: repository interfaces and implementations
- `internal/ui`: Fyne UI only (presentation + user interactions)

## Style Guidelines

### Formatting and imports

- Always run `gofmt` on changed Go files.
- Keep imports grouped by standard library, third-party, internal packages.
- Avoid unused imports/variables; keep `go test` and `go vet` clean.

### Naming

- Exported identifiers: `PascalCase` with domain-friendly names (`Hand`, `ParseResult`).
- Unexported identifiers: `camelCase` (`parseCard`, `streetBetAmount`).
- Acronyms follow existing local style in this repo (`VPIP`, `PFR`, `ThreeBet`).
- Keep enum-like constants prefixed by type context (`PosSB`, `ActionFold`, `StreetFlop`).

### Types and APIs

- Prefer small, explicit structs at package boundaries.
- Keep parser models coherent; avoid ad-hoc maps in UI code.
- Add interfaces only where they provide swappable boundaries (e.g., persistence/service).
- Return concrete slices for query results; avoid exposing mutable internal state.

### Error handling

- Return errors upward with context using `%w` when wrapping.
- Use clear messages that explain operation context (`"save imported hands"`, etc.).
- In long-running watchers/UI callbacks, surface non-fatal errors to status UI rather than panic.
- Reserve `panic` for truly unrecoverable programmer errors (rare in this codebase).

### Concurrency

- Protect shared mutable state with `sync.Mutex`/`sync.RWMutex`.
- Keep lock scope tight; avoid I/O while holding locks when possible.
- UI updates must run on Fyne main thread via `fyne.Do(...)`.
- Stop/replace watchers carefully to avoid goroutine leaks.

### Parsing and data logic

- Regexes should be compiled once as package globals.
- Parser methods should be tolerant of malformed lines (ignore and continue, not fail whole import).
- Keep side effects centralized in service/repository layers, not in rendering code.
- Preserve hand/action ordering semantics when transforming or persisting data.

### Testing expectations

- Add/adjust tests when changing parser behavior or stats semantics.
- Prefer table-driven tests for parsing and classification logic.
- Keep tests deterministic and independent of local machine files unless explicitly integration-only.
- For real-log tests, guard with skip behavior when fixture file is unavailable.

## Change Management Expectations for Agents

- Make minimal, scoped changes consistent with existing architecture direction.
- Do not introduce unrelated refactors while fixing one issue.
- Keep commits logically grouped and message intent-focused.
- Verify with targeted tests first, then broader suite when feasible.

## Practical Agent Workflow

1. Read `mise.toml`, relevant package files, and tests before editing.
2. Implement smallest viable change.
3. Run `gofmt` on touched files.
4. Run package-level tests for touched areas.
5. Run `go test ./...` when practical.
6. Summarize behavior impact and any follow-up migration notes.
