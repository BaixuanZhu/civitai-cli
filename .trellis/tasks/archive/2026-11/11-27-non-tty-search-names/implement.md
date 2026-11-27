# Implementation plan: full model names in non-TTY search

## Status and hard gates

- Planning only in this phase. Do not run `task.py start`, edit product code, commit, or modify shared docs/scripts until the reviewed plan receives a separate implementation approval.
- **Hard dependency:** implement only after `11-27-search-base-model-summary` has landed in this child's base. Both tasks touch `internal/cmd/search.go` and `internal/cmd/search_test.go`.
- **Ownership boundary:** this child may change only `internal/report/table.go`, `width_windows.go`, `width_other.go`, `table_test.go`, `internal/cmd/search.go`, and `search_test.go`. The parent owns `scripts/integration-test.sh` and `README.md`; the exact handoff is in `design.md`.
- No new dependency, flag, JSON field, command, API request, package-global mutable test hook, or unrelated renderer change is authorized.

## Ordered checklist

### 1. Rebase and establish the compatibility baseline

- [ ] Confirm the summary-footer task is present. Inspect its `runSearch` human/JSON branches, table/footer write sequence, renderer signature, and all summary-related tests.
- [ ] Rebase or merge without restoring pre-summary file snapshots. Resolve `search.go` and `search_test.go` hunk by hunk, preserving every summary/footer and compact-JSON assertion.
- [ ] Run `go test ./internal/report ./internal/cmd` before child edits and record any pre-existing failure rather than attributing it to this change.
- [ ] Capture the current layout oracle: `searchLayout(80).name == 12`, NAME maximum 48, narrow-terminal floors, current other-column formatting, and the existing JSON golden tests.

**Rollback point:** if the dependency is absent or its tests are red, stop before child edits and return to the parent for sequencing. Do not implement against the old search baseline.

### 2. Add the writer-aware report seam

- [ ] In `internal/report/table.go`, centralize `fallbackConsoleWidth = 80` and add `ConsoleWidthFor(w io.Writer) (width int, console bool)` with GoDoc covering non-file writers, query failure, and the non-Windows fallback.
- [ ] Add an unexported pure normalizer for platform results: positive -> `(width, true)`; zero/negative -> `(80, false)`. Type-assert `w` to a non-nil `*os.File` before calling the platform query.
- [ ] Reimplement `ConsoleWidth() int` as the compatibility wrapper around `ConsoleWidthFor(os.Stdout)`, ignoring the mode bit. Do not change its signature or other consumers.
- [ ] In `width_windows.go`, change `consoleWidth` to accept `fd uintptr` and pass that descriptor to `GetConsoleScreenBufferInfo`. Retain all existing failure-to-zero behavior and hand-written syscalls.
- [ ] In `width_other.go`, reuse `internal/console.IsTerminal(fd)`: interactive terminal -> 80, otherwise -> 0. Do not add a Unix ioctl or copy terminal-detection code.

**Review gate A:** every existing `ConsoleWidth` caller still compiles with unchanged width semantics; Windows distinguishes successful width query from failure, and no new global seam or external module appears.

### 3. Pin report behavior without a real TTY

- [ ] Extend `internal/report/table_test.go` with table-driven normalizer cases: 120, 80, 0, and a negative value.
- [ ] Assert `ConsoleWidthFor(&bytes.Buffer{}) == (80, false)` so injected output is deterministic even on a console-hosted test process.
- [ ] Add an `os.Pipe` case returning `(80, false)`; close both pipe ends and do not replace `os.Stdout`.
- [ ] Keep the existing rune truncation and tabwriter alignment tests unchanged.

**Rollback point:** if platform compilation requires more than the fd plumbing and existing `console.IsTerminal` reuse, stop and narrow the design rather than introducing a terminal dependency.

### 4. Make only search NAME conditional

- [ ] In the post-rebase human branch of `runSearch`, call `ConsoleWidthFor(g.stdout())` only after models decode. Do not call it from compact/full JSON branches.
- [ ] Pass width and console mode explicitly to `renderSearchTable`; adapt additively if the summary task changed its signature.
- [ ] Keep `searchLayout(width)` arithmetic unchanged. In the row loop, bypass `report.Truncate` only for `m.Name` when `console == false`; retain the exact existing call when `console == true`.
- [ ] Leave all TYPE/BASE/CREATOR truncation, fixed fields, row iteration, empty-result handling, tabwriter construction, and `Flush` behavior unchanged.
- [ ] Preserve the summary task's footer exactly once after successful table rendering and preserve its error path. Do not print a footer after a failed table write if the dependency did not do so.
- [ ] Update function GoDoc/comments to explain that the mode bit controls NAME only and that non-TTY rows may exceed 80 columns.

**Review gate B:** diff inspection shows no edits to API calls, JSON rendering, other commands, flags, or summary data. No zero/negative cap is passed to `Truncate` as an “unlimited” sentinel.

### 5. Add the search regression matrix

- [ ] Convert the fixture table test into a command-level non-TTY assertion: `Animagine XL V3.1` is exact, `Animagine X…` is absent, and existing header/ID/stat/creator checks remain. Remove the <=80 assertion only for this non-TTY case.
- [ ] Retain `TestSearchLayout_WidthBudget` as the successful-console arithmetic oracle.
- [ ] Add direct renderer cases for successful consoles at widths 80, 60 (narrow), and 120. Assert the exact current NAME truncation, including the 12-rune floor, and the other columns' existing values.
- [ ] Add non-TTY exact-name cases with a name longer than 48 runes and a Unicode/multibyte name. Assert verbatim strings and absence of the ellipsized NAME variant.
- [ ] Add a non-TTY other-columns case with overlong TYPE/BASE/CREATOR values; assert their existing capped forms and all fixed values. Assert two rows remain in input order.
- [ ] Assert tabwriter expansion/alignment using ASCII content. Keep Unicode exactness as a separate assertion and make no display-cell-width claim.
- [ ] Add a real `os.Pipe` command-level fallback case by assigning the writer to `testEnv.g.out`, closing it after the run, and reading the captured result. Assert full NAME and no request/output anomalies.
- [ ] Keep post-rebase compact/full JSON expectations and request-parity checks unchanged by this child, including the summary task's compact additions. Keep empty human output equal to the dependency's `No models found.` message plus its zero-result summary footer.
- [ ] Consolidate or extend invalid-input cases for missing query, limits 0/101, unknown sort/period/type values, wrong-case or explicit-empty `--view`, and existing invalid values as applicable. Every invalid case must assert exit class 1, `lastPath == ""`, and empty stdout; retain success coverage for valid case-insensitive sort/period/type aliases. Direct `FlagSet` overflow/parse failures remain in their existing test layer if present.
- [ ] Reapply and run all summary-task tests. Extend the existing human summary test to prove full non-TTY NAME plus exactly one footer in the original position; do not weaken or replace its assertions.

**Review gate C:** each AC1–AC8 behavior maps to at least one non-tautological assertion. In particular, a test that only checks a full name is contained in itself, only checks the header, or cannot fail on the pre-change renderer is insufficient.

### 6. Format and validate

Run from the repository root after implementation:

```bash
gofmt -w internal/report/table.go internal/report/width_windows.go internal/report/width_other.go internal/report/table_test.go internal/cmd/search.go internal/cmd/search_test.go
go test ./internal/report ./internal/cmd
go vet ./...
go test ./...
go test -race ./...
go build ./...
go list -m all
```

Cross-compile the two affected test packages without executing foreign binaries or leaving artifacts in the repository:

```bash
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT
GOOS=linux GOARCH=amd64 go test -c -o "$tmpdir/report-linux.test" ./internal/report
GOOS=linux GOARCH=amd64 go test -c -o "$tmpdir/cmd-linux.test" ./internal/cmd
```

Validation checklist:

- [ ] Native Windows and cross-compiled non-Windows code use the same `ConsoleWidthFor` contract.
- [ ] `go list -m all` prints only `civitai-cli`.
- [ ] Post-rebase JSON golden/compact/full tests have no child-specific expectation changes, and the summary task's compact additions remain pinned.
- [ ] Summary-footer tests are green and the footer remains after the table.
- [ ] Invalid-input tests prove zero request and zero stdout.
- [ ] A diff audit contains only the six authorized child implementation/test files; shared README/script edits remain with the parent.

### 7. Parent integration and final review

- [ ] Hand the parent the four exact shared edits in `design.md`: paired `search_names_table` integration run, verbatim name assertions against `search_json`, empty-stdout `usage_error` assertion, and the README human-output paragraph.
- [ ] Parent builds the real executable and runs `make integration` or `bash scripts/integration-test.sh <exe>` after applying the shared updates. The child does not edit or claim completion of those parent-owned edits.
- [ ] Optionally perform a manual Windows console smoke at normal and narrow widths plus a redirected run. Automated explicit-mode tests, not availability of a development TTY, remain authoritative.
- [ ] Final full-scope review checks the PRD acceptance matrix, report/cmd specs, rebase preservation, zero dependencies, stdout/stderr discipline, and absence of unrelated renderer changes.

## Final rollback point

If a child-specific defect is found before commit, revert only this child's hunks while retaining the summary task. If found after landing, use `git revert` for this child's commit and rerun the summary tests plus the full validation gate. Never use an old whole-file checkout of `search.go` or `search_test.go` as a rollback shortcut.
