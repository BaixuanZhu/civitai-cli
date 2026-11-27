# Search base-model/result summary

> Status: converged planning artifact for final review. This document does not authorize implementation or `task.py start`.

## Goal and user value

Give people and coding agents an immediate, accurate summary of the models returned by one `civ search` response. The summary must show how many items were returned and which base models they cover without another request, pagination, inferred totals, or an implicit primary base.

## Confirmed repository facts

- **F1 — One response, three output paths.** `runSearch` performs one existing `SearchModels` call, then branches to raw full JSON, compact JSON, or human table output (`internal/cmd/search.go:135-205`). The compact decoder already retains model-level `baseModels` and every embedded version's `baseModel` (`internal/api/compact_models.go:89-123`, `internal/api/compact_models.go:143-187`).
- **F2 — Existing human fallback is narrower than the new aggregation.** The table uses model-level bases when non-empty, otherwise only the first embedded version's base (`internal/cmd/search.go:266-301`). The new summary must not copy that first-version choice because doing so would hide other observed bases.
- **F3 — The compact contract is already versioned.** The default envelope is `civ.search/v1`, `kind: "modelSearch"`, with `items` in source order (`internal/api/compact.go:20-48`). Full view uses `ModelsRawItems` and writes the legacy raw array rather than a compact-plus-extras object (`internal/cmd/search.go:185-199`; `internal/api/types.go:225-238`).
- **F4 — Upstream metadata does not establish a match total.** The checked-in response contains two items and metadata with `nextCursor`/`nextPage`; its model-level bases are `Anima` and `SDXL 1.0`, repeated across their embedded versions (`testdata/models_search_anima.json`; `testdata/FIXTURES.md:14-24`, `testdata/FIXTURES.md:38-56`). A cursor is not a total.
- **F5 — Output and failure behavior are public contracts.** Existing tests pin compact/default equality, raw full-view bytes, empty arrays, one-request view parity, and pre-request view validation (`internal/cmd/search_test.go:37-217`). Project policy requires deterministic offline tests, empty stdout on failures, and integration coverage for changed CLI output (`.trellis/spec/backend/quality-guidelines.md:69-121`; `.trellis/spec/backend/logging-guidelines.md:7-25`).
- **F6 — A sibling will touch the same command files.** `11-27-non-tty-search-names` owns later non-TTY name-layout work. This child must not edit that sibling's artifacts or absorb its scope.

## Requirements

### R1 — Describe only this returned result set

- Add a local summary to both human output and the default compact JSON output.
- `summary.returned` is exactly the number of items in the current response, including zero. Never call it a total number of matches and never derive a total from a cursor, next-page URL, requested limit, or another response.
- Use only the already-fetched response. Do not add an API call, pagination, enrichment, cache, or retry specific to the summary.

### R2 — Use one explicit base-evidence policy

For each returned model, independently:

1. Read model-level `baseModels`, discard empty or whitespace-only entries as unusable evidence, and deduplicate the remaining strings by their exact original values.
2. If at least one usable model-level base remains, use only that set; do not add version-only bases.
3. If no usable model-level base remains—including absent, null, empty, or all-blank model-level data—collect `baseModel` from **all** embedded versions, discard absent/null/empty/whitespace-only values, and deduplicate exact original strings.
4. If the selected set is still empty, count the model as without base evidence. Do not select a first/latest/default base and do not infer a family from another field.

This fallback applies only to summary accounting. It must not rewrite the existing item's `baseModels`, version references, or absent/null/empty presentation semantics. Except for rejecting blank-only values, names are not trimmed, case-folded, canonicalized, or checked against the registered base filter whitelist.

### R3 — Count model coverage, not occurrences or exclusive buckets

- Each distinct observed base contributes at most one coverage count per model, even if repeated at model level or across several fallback versions.
- One model with multiple distinct bases contributes one to each corresponding base count. Consequently, coverage counts need not sum to `returned`.
- Report `modelsWithMultipleBases`: the number of returned models whose selected set contains more than one distinct base.
- Report `modelsWithoutBase`: the number of returned models whose selected set is empty. Unknown nonblank upstream strings remain named observed categories; they are not relabeled as unknown or discarded.
- Do not emit percentages, shares, a primary base, or any other value that implies mutually exclusive base membership.

### R4 — Additive, typed output contract

- Keep `schemaVersion: "civ.search/v1"`, `kind: "modelSearch"`, and the existing `items` array unchanged in meaning, length, order, and item fields. Add a required top-level `summary` object after `items`; this is an additive v1 change, not evidence for a schema-version bump.
- The compact summary has exactly these required members:
  - `returned`: non-negative integer;
  - `baseCoverage`: array, always present and `[]` when empty, of `{ "baseModel": string, "models": integer }` objects with nonzero `models`;
  - `modelsWithMultipleBases`: non-negative integer;
  - `modelsWithoutBase`: non-negative integer.
- Human output keeps the existing table and `No models found.` message, then appends two footer lines:
  - `Returned: N; multi-base models: M; models without base: U`
  - `Base coverage: name=count; ...`, or `Base coverage: none` when no observed base category exists.
- The reconciliation counters remain explicit even when zero. No zero-count named base category such as `Anima=0` may be fabricated.

### R5 — Deterministic, concise, and empty-safe

- Sort `baseCoverage` by `models` descending, then exact `baseModel` ascending using deterministic, case-sensitive Go string ordering. Input order and map iteration must not affect the result.
- JSON includes every observed category. Human output shows at most the first eight sorted categories, followed by `; +K more (use --json for all)` when categories remain. The cap never changes `returned`, either reconciliation count, or JSON coverage.
- Empty input produces `items: []`, `returned: 0`, `baseCoverage: []`, and zero reconciliation counters in compact JSON. Human empty output still includes `No models found.` and a footer with returned `0` and no named categories. No division or percentage calculation is needed.

### R6 — Compatibility and failure invariants

- `civ search --json --view full` remains the verbatim legacy raw items array with no injected envelope or summary. Its existing byte-for-byte passthrough and trailing newline behavior must remain unchanged.
- Omitted `--view` and explicit `--view compact` remain byte-identical. Existing table columns, row order, flags, exit codes, stdout/stderr separation, and non-search commands remain unchanged apart from the approved human footer.
- Existing usage validation stays ahead of HTTP. Every invalid input remains zero-request and no-stdout. Existing request/decode failures must not emit a partial summary or silently fall back to another view.

### R7 — Offline verification and ordered delivery

- Unit tests must cover absent, null, and empty base evidence; duplicate bases within one model; all-version fallback; model-level precedence over versions; multi-base coverage; unknown strings; count ties; empty results; human bounding; and compact/full-view compatibility.
- Add parity coverage proving human and compact summaries agree for the same response. Preserve the existing compact size-reduction and record-preservation tests.
- Land this child before `11-27-non-tty-search-names`. Keep changes limited to this child's search/API implementation and tests; do not modify the sibling's artifacts.

## Acceptance criteria

- [ ] **AC1 — Representative fixture (R1, R3, R4):** For `models_search_anima.json`, compact JSON reports `returned: 2`, `baseCoverage: [{baseModel: "Anima", models: 1}, {baseModel: "SDXL 1.0", models: 1}]`, and zero multi-base/without-base counts. Human output retains both table rows and reports the same facts after the table. No total-match claim or percentage appears.
- [ ] **AC2 — Evidence selection and deduplication (R2, R3):** Table-driven tests show that absent/null/empty/all-blank model-level bases fall back to all embedded versions; usable model-level bases suppress version-only additions; duplicate model or version bases count once per model; and a model with no usable evidence increments only `modelsWithoutBase`.
- [ ] **AC3 — Multi-base and unknown values (R2, R3):** A multi-base model increments every distinct observed base once and `modelsWithMultipleBases` once. Duplicate copies of one base do not make it multi-base. Unknown mixed-case or otherwise unregistered nonblank names survive verbatim in JSON and in any human category that is displayed.
- [ ] **AC4 — Ordering and bounds (R5):** Tests pin count-descending/name-ascending ordering, including tied counts and input permutations. More than eight categories produce exactly eight human entries plus the correct overflow count, while JSON retains all categories and the full returned count.
- [ ] **AC5 — Empty results (R1, R4, R5):** Both output modes succeed for `{"items":[],"metadata":{}}`; compact preserves `items: []` and reports returned `0`, coverage `[]`, and zero counters, while human output preserves `No models found.` and reports returned `0` without fabricated categories or numeric errors.
- [ ] **AC6 — View compatibility (R4, R6):** Default and explicit compact output are identical and remain `civ.search/v1`; full output remains byte-for-byte equal to the extracted raw items array plus its existing newline and contains no injected summary. Existing show compact/full contracts and compact item presence semantics remain green.
- [ ] **AC7 — Requests and failures (R1, R6):** Human, compact, and full success paths each make exactly one `/models` request with unchanged query parameters and no pagination. Existing invalid query/flag/limit/sort/period/type cases fail before HTTP with empty stdout; decode and transport failures retain their current errors and no-stdout behavior.
- [ ] **AC8 — Surface handoff (R7):** The parent has the exact live integration scenarios below. This child does not edit the shared integration script, README, CHANGELOG, specs, or skill assets.

## Out of scope

- Match-total estimation, cursor changes, pagination, extra endpoint calls, or server-side aggregation.
- Base-family inference, canonicalization, compatibility scoring, primary/default/latest base selection, percentages, or recommendations.
- Changes to search filters, table columns, non-TTY model-name layout, other commands, persistence, downloads, or export.
- New flags, a new schema version without compatibility evidence, third-party dependencies, and edits to parent-owned shared documentation/spec/skill files.

## Parent-owned integration scenarios

The parent should update `scripts/integration-test.sh` and related shared documentation after this child lands, using these exact scenarios:

1. **Strengthen existing `search_compact`** (`civ search anima --json --limit 3`): require the existing v1 envelope plus a `summary` object; assert `summary.returned == len(items)`, independently recompute coverage and both reconciliation counts from each compact item's `baseModels`/`versions[].baseModel` using R2–R3, assert exact count-descending/name-ascending order, and assert every named coverage count is positive. Do not assert a match total or a fixed live base distribution.
2. **Strengthen existing `search_table`** (`civ search stocking --type lora --limit 5`): retain the header assertion and require the `Returned: N` and `Base coverage:` footer labels with a non-negative N; verify no `=0` named category appears. Specific live base names and percentages must not be hard-coded.
3. **Retain and strengthen `search_json` and `search_filters`** (both `--view full`): assert the top-level value remains a raw array, not an object with `items` or `summary`. Keep their existing request/filter assertions unchanged.
4. **Add `search_compact_empty`** using an intentionally unregistered base such as `civ search summary-empty --base __civ_search_summary_no_such_base__ --json --limit 1`: expect exit 0, `items: []`, `returned: 0`, `baseCoverage: []`, and zero reconciliation counts. The unknown-base warning belongs on stderr and must not pollute stdout.
5. **Keep existing invalid-input checks offline-assertable:** `usage_error` and `search_nope` must remain exit 1 with empty stdout; zero-request proof remains in unit tests. No live integration case should expect an additional request made by summarization itself.
