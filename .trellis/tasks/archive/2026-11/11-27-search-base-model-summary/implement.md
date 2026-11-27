# Search base-model/result summary — implementation plan

> Planning only. Execute these steps only after final artifact review, fresh implementation approval, and the normal task activation. This child must land before `11-27-non-tty-search-names`.

## 1. Scope and preflight

- [ ] Confirm the task is still approved for the converged `civ.search/v1` additive summary and the eight-category human display limit in `prd.md`/`design.md`.
- [ ] Inspect the current diff before editing. Preserve all parent/sibling task directories and unrelated work. Do not start or modify `11-27-non-tty-search-names`.
- [ ] Limit product changes to:
  - `internal/api/compact.go`;
  - `internal/api/compact_models.go`;
  - `internal/api/compact_test.go`;
  - `internal/cmd/search.go`;
  - `internal/cmd/search_test.go`.
- [ ] Do not edit shared integration/docs/release/spec/skill files in this child. The parent owns those updates and receives the scenarios in PRD AC8.

## 2. Add the shared typed summary

- [ ] Add GoDoc-covered `SearchBaseCoverage` and `SearchResultSummary` DTOs with the exact JSON tags in the design; add the required value-typed `Summary` field after `Items` in `CompactSearchEnvelope`.
- [ ] Implement the unexported evidence record/core in `compact_models.go`: blank-only filtering without mutating retained names, model-level precedence, all-version fallback only when model-level evidence is unusable, exact per-model deduplication, coverage counts, and both reconciliation counters.
- [ ] Add the compact raw-model adapter inside `ParseCompactModels`; initialize `BaseCoverage` to `[]` even for zero items and set `Returned` from the decoded source length.
- [ ] Add `SummarizeSearchModels` for the legacy human model slice. Keep it a thin adapter to the same core rather than a second implementation.
- [ ] Sort map-derived entries by count descending and exact base name ascending before storing them in the summary. Add no percentage, total, cursor, or primary-base field.
- [ ] Leave `Model`, `ModelVersion`, `ParseModels`, `ModelsRawItems`, `SearchModels`, and all show/full-view projection code unchanged.

**Rollback point R1:** API-only changes must compile and preserve every pre-existing API assertion except tests intentionally extended for the new required summary. If source presence or item parity regresses, stop and revert this step before command wiring.

## 3. Wire the human footer without disturbing view branches

- [ ] Keep `renderSearchTable(w, models)` as the entry point. Compute the shared summary from those exact models and append `writeSearchSummary` only after the existing table flush; on empty input, preserve `No models found.` before the footer.
- [ ] Implement the exact two-line labels and separators in `design.md`, including `Base coverage: none`, explicit zero reconciliation counters, and no named zero-count category.
- [ ] Bound only the human category prefix with `searchSummaryHumanBaseLimit = 8`; append the exact `+K more (use --json for all)` overflow text when needed. Never truncate an emitted unknown name and never bound the JSON list.
- [ ] Propagate footer writer errors through `runSearch`; keep all output on `g.stdout()` and add no logging.
- [ ] Do not modify the raw full-view branch. Compact/default output receives the summary through `ParseCompactModels`, with no additional command-side schema assembly.

**Rollback point R2:** If footer layout or a later sibling merge causes trouble, revert only the footer helper/call while leaving the API projection intact. Do not resolve a human presentation issue by changing full view or inventing a primary base.

## 4. Build semantic and compatibility tests

- [ ] Extend the API fixture test for returned `2`, `Anima=1`, `SDXL 1.0=1`, and zero reconciliation counts; assert summary keys/types and non-nil empty arrays where applicable.
- [ ] Add table-driven API cases for absent/null/empty/all-blank model-level values; absent/null/empty/blank version values; usable model-level precedence; all-version fallback; duplicates within a model; multi-base models; and no-base models.
- [ ] Add unknown exact-string cases and count-tie/input-permutation cases. Verify no canonicalization, case folding, trimming of retained values, zero categories, or map-order leakage.
- [ ] For each shared synthetic response, compare the compact parser summary with the legacy human summarizer result. Include empty results and verify no division-dependent behavior.
- [ ] Extend command tests for the human fixture footer, empty human output, compact summary JSON, default/explicit compact equality, and raw full-view byte compatibility.
- [ ] Add a human overflow case with more than eight observed categories; assert the selected prefix follows the sorted summary, the overflow count is correct, and JSON still emits every category.
- [ ] Reassert one request and unchanged parameters for human, compact, and full success paths. Strengthen all existing invalid-input cases to assert zero requests and empty stdout; retain compact decode-failure/no-fallback coverage.
- [ ] Keep existing record-order, required-item-key, unknown-field exclusion, deterministic-repeat, and compact-under-half-size assertions intact. Use inline synthetic responses; do not rewrite verbatim fixtures or their manifest.

## 5. Focused and full validation

Run from the repository root after implementation, formatting only the files changed by this child:

```bash
gofmt -w internal/api/compact.go internal/api/compact_models.go internal/api/compact_test.go internal/cmd/search.go internal/cmd/search_test.go
go test ./internal/api ./internal/cmd
go vet ./...
go test ./...
go test -race ./...
go list -m all
git diff --check
git status --short
```

Gates:

- [ ] Focused API/cmd tests pass with zero external network access.
- [ ] Full vet/test/race gates pass; `go list -m all` still lists only `civitai-cli`.
- [ ] The diff contains only this child's approved product/test files plus its own Trellis artifacts when bookkeeping is included.
- [ ] Review confirms full view is still raw, compact remains `civ.search/v1`, no new request exists, and invalid input still produces no request/stdout.
- [ ] Do not run or edit the live integration script as this child's implementation step. After the parent applies its shared updates, the parent runs the PRD-listed scenarios through its normal integration workflow.

## 6. Final review and handoff

- [ ] Review the implementation against every R1–R7 and AC1–AC8 mapping in `prd.md`; verify the exact JSON and human contracts in `design.md` rather than accepting approximate behavior.
- [ ] Inspect the actual diff for accidental changes to other commands, sibling artifacts, shared README/CHANGELOG/scripts/specs, fixtures, dependencies, or generated output.
- [ ] Report to the parent that this child is ready to land before `11-27-non-tty-search-names`, and hand off the five exact integration scenarios in `prd.md` without editing that sibling or parent artifacts.
- [ ] During later conflict resolution, preserve this child's summary footer and semantic tests while allowing the sibling to own non-TTY model-name layout. Any contract conflict returns to planning rather than being silently resolved in code.
