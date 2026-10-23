# Compatibility fixtures and golden oracle

`legacy-v1.json` records parser and application behavior from baseline commit `426d4d89cb6241b94542b7d5e2f123686d50e576`. The input logs are synthetic copies of the repository's public `sampleLog` and `showdownLog` test fixtures; `characterization_cases.txt` adds synthetic edge cases, including blind-order reversal, a local-seat change, multi-pot winners, a preflop 3-bet, and every legacy board anomaly. `legacy-rev2.json` records additional baseline service-query, cursor resume with and without `WorldContext`, fully-imported skip, idempotency, stale-source, and seek-fallback observations. No personal log, local database, real user ID, or home-directory data is included.

The generated corpus was produced once using the checked-in synthetic inputs and this command:

```sh
go run ./tools/gen_testlog --input-dir testdata/compat/generator-input --output-dir testdata/compat/corpus --count 2 --min-size 16384 --max-size 32768 --seed 20261023 --start-date 2026-02-01
```

The generator's fixed seed alone is not treated as a reproducibility guarantee. The committed corpus bytes and `SHA256SUMS` are authoritative. Compatibility tests compare against the frozen oracle and never rewrite it. Normalization is restricted to `InstanceUsers` ordering, cursor `UpdatedAt`, and temporary source paths; hand identity arrays retain repository result order. The prior `StartTime` sort was removed, and the compatibility suite passed without it.

The compatibility contract protects representative legacy-log import, saved-hand data, and the existing test suite; it does not require complete behavioral equivalence. CI allows new tests while rejecting changes, deletions, and renames of baseline tests. The LastN observation records only `TotalHands`, not selected hand IDs or all Stats fields. Non-empty partial-hand EOF/restart behavior was not observed, and exact restoration of unfinished hands across restart remains outside this oracle. These are known observation limits, not claims of verified behavior.

To verify all fixture and golden bytes from the repository root, run:

```sh
(cd testdata/compat && sha256sum --check SHA256SUMS)
go test -count=1 ./internal/compat/...
```
