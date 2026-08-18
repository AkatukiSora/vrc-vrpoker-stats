# UI testing and AO verification

## What AO can verify

AO's Browser panel automates Chromium pages (DOM, browser interactions and
screenshots). This application is a native Fyne desktop window, so the panel
cannot inspect, click, or screenshot that window. Do not treat a successful
`ao browser` check as native UI coverage.

The default automated UI layer is Fyne's in-memory `test` driver. It creates a
temporary app/window without a native display server and supports widget taps,
selection, typing, renderer inspection, and deterministic raster assertions.
Use `newUITestWindow` in `internal/ui/test_helpers_test.go` and wrap UI reads
and writes in `fyne.DoAndWait` (or `flushUI`) when asynchronous UI work may be
queued.

```bash
# All Fyne driver interaction and renderer regression tests.
mise run test-ui

# Same suite in the CI locale and a repeat loop.
mise run test-ui-ci

# Focus a real interaction smoke test while developing.
go test -tags wayland -run '^TestHandHistoryCanvasTapSmokeSelectsRow$' -v ./internal/ui
```

The CI `UI Regression` job runs `mise run test-ui-ci`. It deliberately runs the
complete `internal/ui` package so renderer regressions are not skipped by a
test-name filter.

## Required checks for UI changes

Choose the checks that match the change; do not add sleeps as a substitute for
an observable condition.

1. Add or update a Fyne test-driver interaction test for every new user path
   (tap/select/type, then assert the visible state or service request).
2. For a custom renderer, theme, layout, or state-transition change, add a
   deterministic renderer assertion. Prefer inspecting renderer state or a
   small pixel/raster invariant over fragile full-image golden files.
3. Run the focused test, then `mise run test-ui`. Run `mise run test-ui-ci`
   before opening a UI-heavy PR when practical.
4. Manually visual-smoke native Windows and Linux builds for changes involving
   platform drivers, scaling, file dialogs, menus, fonts, or GPU rendering.
   Record OS, resolution/scale, and the exercised path in the PR.

## Native smoke tests and screenshots

`xvfb-run` can launch a native Linux Fyne build in CI or a developer machine,
but it is only useful when the test has a deterministic exit and capture point.
The current app has neither a screenshot harness nor a scripted shutdown path,
so adding a blind Xvfb job would hang or provide weak evidence. Keep it as an
opt-in follow-up: first add a test-only startup fixture that seeds data, opens
a selected tab, waits for a readiness signal, captures a named image, and exits;
then run it under Xvfb and upload screenshots as CI artifacts. Do not compare
native screenshots across OSes until fonts, scale, and rendering backend are
controlled.

## Boundaries and migration seams

`internal/ui` depends on `application.AppService`; parser, statistics, and
persistence remain below that boundary. Put calculation, filtering, import, and
selection/query policy in `internal/application` (with unit tests), and keep
Fyne responsible for presentation and interaction wiring. A presenter or
view-model can be introduced per tab when Fyne controller state becomes hard to
test; expose plain state and commands, not Fyne widgets.

A web client or another frontend would make Chromium/AO browser automation
available, but it adds a separate UI stack, packaging/runtime concerns, and a
large migration risk. It is an architectural alternative, not a response to a
single untestable Fyne screen. Reconsider it only after the Fyne driver plus
native-smoke approach fails to cover a sustained, documented workflow.

## Staged plan

1. **Now:** keep Fyne, use the shared test-window helper, and run the complete
   Fyne driver suite in CI. Cover each changed interaction with a focused test.
2. **Next UI features:** move tab-specific state transitions and query policy
   into small presenter/view-model or application commands as they become
   difficult to exercise through widgets. Unit-test those plain Go types.
3. **When native fidelity is required:** add a deterministic native-smoke
   fixture, run it under Xvfb on Linux, and upload named screenshots. Add
   Windows smoke coverage for platform-specific behavior.
4. **Only after measured gaps remain:** evaluate a second client (including a
   web UI) against user value, packaging, accessibility, and migration cost;
   keep the application boundary as the shared contract.
