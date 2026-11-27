# Design: full model names in non-TTY search

## Summary

Add a writer-aware console-width result in `internal/report`, retain the existing `ConsoleWidth() int` compatibility wrapper for every other command, and pass the resulting width/mode into the search table renderer. Only a positive console result truncates NAME. Non-consoles and failed queries retain the 80-column budgets for all other cells but write NAME verbatim.

## Change boundary

The smallest behavior gap is not the existence of the 80-column fallback; it is that `renderSearchTable` cannot distinguish a successful console query from the fallback and therefore always calls `report.Truncate` for NAME (`internal/cmd/search.go:266-299`). The fix belongs at that rendering decision, supported by a richer report query. It does not belong in API decoding, JSON rendering, global flags, or a universal change to `ConsoleWidth`.

Expected implementation files:

| File | Necessary change |
|---|---|
| `internal/report/table.go` | Add the writer-aware `(width, console)` query and pure fallback normalizer; preserve `ConsoleWidth() int` through it. |
| `internal/report/width_windows.go` | Query the supplied stdout file descriptor instead of reading `os.Stdout` directly. |
| `internal/report/width_other.go` | Reuse `console.IsTerminal(fd)`: return the existing 80-column fallback for an interactive terminal and zero for non-TTY so the common normalizer supplies the mode. |
| `internal/report/table_test.go` | Pin successful/failed normalization, injected non-file writers, and pipe behavior without a TTY. |
| `internal/cmd/search.go` | Query mode only in the human branch and make NAME truncation conditional; leave `searchLayout(width)` arithmetic intact. |
| `internal/cmd/search_test.go` | Replace the unconditional 80-column human expectation with explicit console/non-console cases and add long, Unicode, other-column, row-order, validation, and post-rebase footer coverage. |

No change is expected in `internal/cmd/globals.go`, the API packages, `go.mod`, shared scripts, README, or other renderers. A test-local pipe can be assigned directly to the existing `testEnv.g.out`; no new global factory or command harness seam is needed.

## Output-width contract

Add this internal-package API in `internal/report/table.go`:

```go
func ConsoleWidthFor(w io.Writer) (width int, console bool)
```

Its contract is:

1. A non-`*os.File` writer (including `bytes.Buffer`) returns `(80, false)`. This mirrors the skill command's type-assertion rule and prevents an injected test writer from accidentally using the process's real terminal.
2. A real file writer is queried through `consoleWidth(f.Fd())`; a nil file pointer also takes the safe `(80, false)` fallback.
3. A positive platform result returns `(width, true)`.
4. A zero or negative platform result returns `(80, false)`. On Windows this covers pipes, redirects, missing procedures, and `GetConsoleScreenBufferInfo` failure without surfacing an error or warning.
5. A non-Windows interactive character device returns the existing width 80 with `console=true`; a pipe or other non-TTY returns `(80, false)`. No `TIOCGWINSZ` work is included.

Centralize `fallbackConsoleWidth = 80` in `report` and use a small pure helper such as `consoleWidthOrFallback(width int) (int, bool)` for decision tests. The helper is unexported production logic, not a test-only exported API.

Keep the current signature and behavior for existing consumers:

```go
func ConsoleWidth() int {
    width, _ := ConsoleWidthFor(os.Stdout)
    return width
}
```

Change the platform helper to `consoleWidth(fd uintptr) int`. Windows removes its direct `os` import and calls `GetConsoleScreenBufferInfo` with `fd`; its success/failure behavior otherwise stays byte-for-byte equivalent. Non-Windows imports the existing leaf `internal/console` and returns 80 only when `console.IsTerminal(fd)` is true. This reuses the project's current non-Windows heuristic instead of copying `Fstat` logic or introducing `x/sys`.

## Search rendering data flow

The human path becomes:

```text
runSearch validation -> one SearchModels request -> ParseModels
  -> g.stdout() -> report.ConsoleWidthFor
  -> (width, console)
  -> searchLayout(width) [existing arithmetic]
  -> render rows
       NAME: Truncate(name, caps.name) only when console == true
       every other cell: existing formatting/truncation unchanged
  -> tabwriter.Flush
  -> dependency task's human summary footer, preserved after rebase
```

Use an internal signature along these lines:

```go
func renderSearchTable(w io.Writer, models []api.Model, width int, console bool) error
```

`runSearch` obtains `width, console := report.ConsoleWidthFor(g.stdout())` only after human models have decoded successfully. The explicit renderer arguments are the test seam for successful wide/narrow consoles and non-console/failure decisions; tests do not need to impersonate a TTY. `searchLayout(width) searchCaps` remains unchanged so its existing arithmetic tests continue to provide a compatibility oracle.

For `console == false`, compute the same caps from width 80 but assign `name := m.Name` directly. For `console == true`, assign `name := report.Truncate(m.Name, caps.name)` exactly as before. Do not use a zero cap as an “unlimited” sentinel because `report.Truncate(s, 0)` currently means an empty string.

JSON returns before this query. Empty results preserve `No models found.` plus the dependency's zero-result summary footer, without rendering model rows or changing the footer contract. Validation and API/decode errors still occur before rendering, so the new probe cannot turn them into partial stdout or additional requests.

## Formatting and compatibility invariants

- The shared `report.NewTableWriter` remains the only table writer. Removing the NAME cap lets tabwriter widen that elastic column and align later columns at their new positions; this is expected, not a regression.
- At non-TTY width 80, TYPE/BASE/CREATOR continue to use the same fallback-derived caps as before (including the existing narrow-layout floors). Their truncation and all fixed columns remain unchanged.
- Long and Unicode NAME values are copied byte-for-byte from decoded Go strings to the tabwriter. Existing `report.Truncate` remains rune-based on consoles. Neither path adds CJK/emoji cell-width claims.
- Existing result order is retained by the same single range over `models`; no sorting or map is introduced.
- `ConsoleWidth` keeps the 80-column width fallback for `show`, `recipe`, `vet`, and `sync`; their rendered behavior is not intentionally changed.
- There is no new flag, output schema, config field, log message, request, third-party dependency, or mutable package-global seam.

## Alternatives considered

| Alternative | Decision |
|---|---|
| Make `ConsoleWidth` return an unlimited/special value for every non-console | Rejected: it would change every table consumer and provides no explicit mode bit. |
| Change `ConsoleWidth` to a two-result signature and update all callers | Rejected: the compatibility wrapper is smaller and prevents unrelated command churn. |
| Add a package-global function variable that tests replace | Rejected: writer injection plus explicit pure renderer arguments already provides deterministic coverage without new mutable package state. |
| Use only `console.IsTerminal` on Windows | Rejected: a console handle whose width query fails must follow the non-console fallback and emit full names. Windows therefore treats only a positive width query as `console=true`. |
| Remove truncation from every human column in redirected output | Rejected by product scope; only search NAME changes. |
| Add `--wide` or tell users to rerun with `--json` | Rejected: automatic non-TTY behavior solves the problem in one invocation without new surface. |

## Test design

### Report tests

Extend `internal/report/table_test.go` with table-driven checks for:

- positive width, for example 120 -> `(120, true)`;
- zero and negative widths -> `(80, false)`;
- `ConsoleWidthFor(&bytes.Buffer{})` -> `(80, false)` independent of the host terminal;
- `ConsoleWidthFor` on the write end of an `os.Pipe` -> `(80, false)`, closing both ends without swapping `os.Stdout`;
- existing `ConsoleWidth` fallback and `Truncate` rune cases remain unchanged.

Cross-compile report and cmd tests for Linux in addition to running the native Windows suite so both platform helper signatures and the non-Windows console import are checked.

### Search tests

Rework `TestSearch_TableFixture` into explicit mode coverage rather than allowing its injected buffer to masquerade as a console:

1. **Command-level non-TTY regression:** the existing fixture's `Animagine XL V3.1` appears verbatim through `runSearchCmd`; `Animagine X…` is absent. Header, IDs, creator, and statistics remain present. The old unconditional “all lines <=80” assertion is removed only from this non-TTY case.
2. **Pure layout compatibility:** retain the existing 80/100/120/200 budget test and add direct renderer cases at successful console widths 80, a narrow width such as 60, and 120. Assert exact `report.Truncate(name, searchLayout(width).name)` output and unchanged other-cell values.
3. **Exact non-TTY strings:** use one name longer than 48 runes and one Unicode name containing multibyte characters. Assert exact verbatim presence and no ellipsized NAME variant. Use ASCII rows separately for tabwriter column-alignment assertions so the test makes no CJK display-cell claim.
4. **Other columns and row order:** give TYPE/BASE/CREATOR overlong values and assert their current capped forms in both non-TTY and console output. Assert model IDs/names occur in input order and fixed formatted fields are unchanged.
5. **Real pipe/failure fallback:** route `testEnv.g.out` through an `os.Pipe`, execute the human command, close/read the pipe, and assert the full NAME. This exercises an actual non-console `*os.File`, not only a buffer.
6. **JSON and empty results:** keep all post-rebase compact/full golden and request-parity tests unchanged by this child, including the summary task's compact additions; keep `No models found.` and the dependency's zero-result summary footer unchanged.
7. **Validation:** consolidate or extend the current missing-query, limit, unknown sort/period/type, and view cases to assert zero request and zero stdout consistently, including wrong-case and explicit-empty `--view` values already covered; retain success coverage for valid case-insensitive sort/period/type aliases.

After the dependency lands, retain its summary tests verbatim where practical and extend the relevant human case to assert both the full non-TTY NAME and exactly one footer in the dependency's expected position. Do not invent a second footer format in this child.

## Rebase onto `11-27-search-base-model-summary`

This child has a hard ordering dependency because both tasks edit `runSearch`, `renderSearchTable`, and `internal/cmd/search_test.go`.

1. Do not implement until the summary task's changes are present in the child's base.
2. Rebase/merge normally, then inspect the resulting human and compact-JSON branches before editing. Treat the summary task's code and tests as the starting text, not the current commit's snapshots.
3. If the summary task changes `renderSearchTable` into a table-plus-footer sequence, add `ConsoleWidthFor` only around the table call and leave footer rendering, data derivation, and error handling intact. If its renderer signature differs, add the two layout inputs additively rather than restoring the old signature.
4. Resolve search-test conflicts hunk by hunk. Preserve all summary/footer and compact-envelope assertions, then apply the explicit console/non-TTY cases. Never discard the whole incoming `search.go` or `search_test.go` file.
5. Run the summary task's original tests plus this task's matrix. A footer duplicated, dropped, moved before the table, or omitted after a write error is a failed rebase, not an accepted behavior change.

## Parent handoff: exact shared updates

This child must not edit these files. The parent should apply and run the following after integrating the child:

1. **`scripts/integration-test.sh` — add a paired real non-TTY case.** Next to the existing search invocations (`scripts/integration-test.sh:104-107`), add:

   ```bash
   run search_names_table search anima --limit 3
   ```

   The existing `search_json` case already runs `search anima --json --limit 3 --view full`; `run` redirects stdout, so `search_names_table` exercises the real executable's non-TTY path without another test-only CLI option.

2. **`scripts/integration-test.sh` — assert names against that paired response.** Immediately after the current human-table header assertion (`scripts/integration-test.sh:316-318`), read `out_of('search_names_table')`, assert exit 0 and the NAME header, then for every entry in the already parsed `items` list assert that its string `name` occurs verbatim in the human output. Include the model id in each failure label. Document in a comment that guaranteed long/Unicode cases live in unit tests and live results provide the real-pipe smoke. Retain the existing `search_table` filter case unchanged.

3. **`scripts/integration-test.sh` — strengthen invalid-output compatibility.** Change the `usage_error` assertion at `scripts/integration-test.sh:485-487` to require both exit 1 and `out_of('usage_error').strip() == ''`. Zero-request proof remains in offline unit tests because the live runner has no request counter.

4. **`README.md` — document the output distinction.** In the human search paragraph at `README.md:270-274`, add: “When human search stdout is piped or redirected, model NAME values are written in full; interactive consoles retain width-based truncation, and the other columns keep their existing limits. A failed console-width query uses the same full-NAME fallback. JSON output is unchanged.”

5. Run `make integration` (or the documented `bash scripts/integration-test.sh <exe>` equivalent) after building the real executable. Live query composition may vary, which is why the deterministic exact-string suite remains the release gate and the live script is the drift canary.

## Rollback and operational risk

The change is local and reversible. Before commit, undo only the six child-owned implementation/test files if necessary; do not restore whole search files from an older revision because that would remove the summary footer. After landing, revert this child's commit with `git revert`, then rerun both this task's and the summary task's tests. JSON, flags, network behavior, and other command layouts are compatibility barriers for any rollback or fix.
