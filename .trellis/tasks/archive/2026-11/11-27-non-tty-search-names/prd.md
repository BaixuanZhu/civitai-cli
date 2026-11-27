# Full model names in non-TTY search

## Goal and user value

When human-readable `civ search` output is piped, redirected, or otherwise written to a non-TTY, every model NAME must be emitted in full. Users must be able to recover an exact long or Unicode model name from the same human command, without rerunning it with `--json`. Interactive terminal layouts and both JSON views must remain compatible.

## Background and confirmed facts

The following evidence is from repository commit `fc0553f`:

- `internal/report/table.go:31-38` maps both a non-console stdout and a failed console query to width 80. `internal/report/width_windows.go:22-44` obtains the actual width with `GetConsoleScreenBufferInfo` on `os.Stdout`; `internal/report/width_other.go:5-10` has no width query and always reaches that fallback.
- `internal/cmd/search.go:242-259` derives all nine column budgets from that width. At the 80-column fallback the NAME cap is 12 runes, so a name such as `Animagine XL V3.1` is shortened. The actual truncation occurs only at `internal/cmd/search.go:289-299`.
- `internal/report/table.go:16-29` truncates by runes and appends an ellipsis without splitting a rune. It explicitly does not model CJK display-cell widths.
- The human renderer already uses the shared tabwriter (`internal/cmd/search.go:266-301`). Compact and full JSON return through separate branches before that renderer (`internal/cmd/search.go:185-205`). Other commands also call `report.ConsoleWidth`, including `show`, `recipe`, `vet`, and `sync`, so changing its fallback semantics globally would exceed this task's scope.
- `internal/cmd/search_test.go:13-35` currently requires every captured table line to fit the 80-column fallback, while `internal/cmd/search_test.go:334-348` pins width-budget behavior. The existing fixture contains a 17-rune name and therefore provides a useful non-TTY regression case, but not a Unicode or narrow-TTY matrix.
- The integration runner redirects every `run` invocation's stdout to a file (`scripts/integration-test.sh:84-90`), but its human search assertion checks only for the NAME header (`scripts/integration-test.sh:316-318`). This is already a real non-TTY execution path that can be strengthened by the parent.

## Requirements

- **R1 — Complete non-TTY names.** In human `civ search`, when the selected stdout writer is redirected, piped, a non-file test writer, or otherwise non-TTY, write each `api.Model.Name` verbatim. Do not apply the existing NAME rune cap or add an ellipsis on that path.
- **R2 — Preserve successful-console rendering.** When the platform reports a valid interactive console width, use the existing `searchLayout(width)` budgets and NAME truncation unchanged. This includes narrow consoles, the 12-rune NAME floor, the 48-rune maximum, and the existing mild overflow below the supported width.
- **R3 — Preserve fallback semantics without truncating names.** A failed or unavailable Windows console-width query follows the existing non-console fallback: width 80 for the other column budgets, but no NAME truncation. On non-Windows, retain the 80-column layout fallback; use the existing terminal-detection facility to distinguish an interactive fallback layout from redirected output rather than adding a new width-query implementation.
- **R4 — Preserve the table contract.** Keep the header, column order, result row order, IDs, formatted statistics, dates, NSFW values, TYPE/BASE/CREATOR caps, and `text/tabwriter` alignment. Only the search model NAME cell loses its cap in non-TTY/fallback output. UTF-8 strings must survive exactly; display-cell correctness beyond current rune/tabwriter behavior is not promised.
- **R5 — Leave JSON untouched.** Relative to the post-rebase summary-task base, default compact JSON, explicit compact JSON, and `--view full` must retain identical bytes, schemas, request counts, and error behavior. Output-mode detection belongs only to the human rendering path.
- **R6 — Leave validation and failures untouched.** Invalid queries/options must still fail before network rendering with the existing error class, zero requests, and empty stdout. Empty human results must preserve `No models found.` plus the zero-result summary footer introduced by the search-summary dependency; this child adds no model rows and changes neither line.
- **R7 — Keep the mechanism small and testable.** Use only the standard library and existing internal packages. Detect mode from the actual output writer, keep width/layout decisions pure and directly testable, and do not require a real TTY or a new mutable package-global test hook.
- **R8 — Land after the summary-footer task.** Rebase onto and preserve `11-27-search-base-model-summary`, including its human footer and compact JSON additions. This child must not restore pre-summary versions of `search.go` or search tests during conflict resolution.
- **R9 — Respect parent ownership.** This child owns only its report/search implementation and unit tests. The parent owns shared integration-script and README edits; `design.md` lists the exact required handoff.

## Out of scope

- Changing truncation for `show`, `sync`, `vet`, `recipe`, or any other command.
- Removing non-TTY caps from TYPE, BASE, CREATOR, descriptions, tags, prompts, or other descriptive fields.
- Adding `--wide`, another output flag, an environment override, a configuration setting, or a public API.
- Adding a third-party terminal module, a Unix `TIOCGWINSZ` implementation, wrapping, or CJK/emoji display-cell measurement.
- Changing API queries, result ordering, JSON schemas, summary-footer content, or exit-code mappings.
- Editing shared scripts, README, CHANGELOG, specs, or another task directory from this child.

## Acceptance criteria

| ID | Requirements | Observable outcome and planned evidence |
|---|---|---|
| **AC1** | R1, R4 | A command-level test with an injected buffer emits a long fixture NAME verbatim and does not emit its former 12-rune ellipsized form. Separate renderer cases emit exact long ASCII and Unicode names, preserve row order, and leave no literal tabs after tabwriter flushing. |
| **AC2** | R2, R4 | Direct successful-console rendering at widths 80 and a narrow width below 80 produces the same caps and ellipsized NAME as the current implementation. Existing width-budget cases for 80/100/120/200 remain green; narrow-terminal behavior does not acquire a new line-width guarantee. |
| **AC3** | R3, R4 | Pure width-result tests map a positive query to `(width, true)` and zero/negative failure results to `(80, false)`. A real `os.Pipe` command/render test follows `(80, false)` and emits full names. A non-TTY table with long TYPE/BASE/CREATOR values retains exactly their current caps while only NAME expands. |
| **AC4** | R2, R3, R7 | A non-file writer is deterministically `(80, false)` even if the test process itself has a console. The non-Windows build uses the existing `console.IsTerminal` fallback with width 80 for a TTY and `(80, false)` for a pipe; no test needs to allocate or swap in a real TTY. |
| **AC5** | R5 | Post-rebase JSON compact/full tests pass without child-specific expectation changes, including verbatim full output, compact-default equality, schema checks, summary additions, decode failures, and one-request parity. Human output-mode code is not called by either JSON branch. |
| **AC6** | R6 | The invalid-input matrix covers missing query, out-of-range limits, unknown sort/period/type values, wrong-case or explicit-empty `--view` values, and other existing failure cases as applicable. Each invalid case asserts error/exit class 1, no request, and no stdout; valid case-insensitive sort/period/type aliases remain accepted. Empty human output retains `No models found.` plus the dependency's zero-result summary footer. |
| **AC7** | R8 | After rebasing, all summary-task tests still pass. Human rendering writes its summary footer exactly once after the table, compact JSON retains the summary additions, and the new NAME behavior changes neither. |
| **AC8** | R1, R6, R9 | With the parent's integration update, the real executable's redirected human search output contains every model name returned by the corresponding full-JSON search. The existing `--limit 101` integration assertion also checks empty stdout. Guaranteed long/Unicode coverage remains in deterministic unit tests because live search content is not fixed. |
| **AC9** | R7 | `go vet ./...`, `go test ./...`, `go test -race ./...`, and `go build ./...` pass; Windows and non-Windows report/cmd tests compile, and `go list -m all` still lists only `civitai-cli`. |

## Minor choices recorded for final approval

- Non-TTY lines may exceed 80 columns, and the elastic NAME column may move later columns to the right. Their values and relative order stay unchanged; an 80-column byte/rune limit is intentionally not retained on this path.
- Console probing remains silent on failure. It is a rendering fallback, not a command degradation requiring a warning on stderr.
- For final approval, the non-Windows choice is to retain the existing `console.IsTerminal` heuristic rather than add an ioctl; its trade-off is continued 80-column approximation on interactive terminals, not new Unix width accuracy.
- Live integration checks exact names from a paired full-JSON invocation, while deterministic long and Unicode edge cases stay in unit tests to avoid making upstream search composition a test requirement.

There are no blocking open questions.
