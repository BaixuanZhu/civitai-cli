# Search base-model/result summary — technical design

> Status: converged design for final review. No implementation is authorized until the planning summary receives fresh explicit approval.

## 1. Smallest implementation boundary

The feature is a deterministic projection over the single `/models` response already used by `runSearch`. It needs no new endpoint, request option, persistent state, dependency, or generic aggregation framework.

```text
existing GET /models
  ├─ --json --view full
  │    └─ ModelsRawItems -> report.WriteRaw                 [unchanged legacy]
  ├─ default/explicit compact JSON
  │    └─ ParseCompactModels -> envelope with Summary
  │         -> report.WriteIndent                           [additive v1]
  └─ human
       └─ ParseModels -> existing table rows
            -> shared SearchResultSummary -> footer        [new local projection]
```

The full-view branch returns before compact parsing and remains untouched. The summary does not alter `SearchParams`, `SearchModels`, `Model`, `ModelVersion`, `ParseModels`, or `ModelsRawItems`.

## 2. Ownership and types

### 2.1 API projection owns semantics

Add the exported DTOs beside the compact search envelope in `internal/api/compact.go`:

```go
type SearchBaseCoverage struct {
    BaseModel string `json:"baseModel"`
    Models    int    `json:"models"`
}

type SearchResultSummary struct {
    Returned                int                  `json:"returned"`
    BaseCoverage            []SearchBaseCoverage `json:"baseCoverage"`
    ModelsWithMultipleBases int                  `json:"modelsWithMultipleBases"`
    ModelsWithoutBase       int                  `json:"modelsWithoutBase"`
}
```

Add the field `Summary SearchResultSummary` with JSON tag `json:"summary"` as the last field of `CompactSearchEnvelope`. It is a value, not a pointer and not `omitempty`, so every successful compact response includes the object. `BaseCoverage` is initialized to a non-nil empty slice. All exported types/fields that need documentation receive concise GoDoc explaining returned-page scope, coverage rather than occurrence counting, and multi-base semantics.

### 2.2 One policy core, two thin source adapters

Implement aggregation in `internal/api/compact_models.go` around an unexported evidence record containing model-level base strings and embedded-version base strings:

- an adapter for compact raw models preserves nullable version-base presence;
- an exported `SummarizeSearchModels(models []Model) SearchResultSummary` adapter supports the existing human decoder;
- both call one unexported summarizer that owns blank filtering, fallback, exact deduplication, counting, and sorting.

This avoids decoding the same body twice and avoids duplicating policy across legacy and compact DTOs. Adapters only extract fields; they do not make independent fallback decisions. The compact parser computes `Summary` from the already-decoded `compactRawModel` values while projecting `Items` as before.

No new package is justified. `internal/api` owns upstream evidence and typed projection; `internal/cmd` owns footer wording and writer behavior; `internal/report` needs no API-aware helper.

## 3. Aggregation algorithm

For each item, in source order:

1. Build an exact-string set from model-level bases. A value whose `strings.TrimSpace(value)` is empty is unusable; every other value is inserted using the **original untrimmed** string.
2. If that set is empty, rebuild/fill it from every embedded version's `baseModel`, applying the same blank test and exact deduplication. Null or absent compact version pointers contribute nothing; legacy empty strings contribute nothing.
3. If the final set is empty, increment `ModelsWithoutBase` and continue. Otherwise, increment the coverage counter once for each set member. If its length exceeds one, increment `ModelsWithMultipleBases` once.
4. Never merge model-level and version-level sets when at least one usable model-level value exists. A base repeated within the selected source still receives only one model contribution.

After all items, convert the coverage map to a slice and sort with:

1. `Models` descending;
2. `BaseModel` ascending via exact, case-sensitive string comparison.

Set `Returned` from the source item count, independently of evidence availability. Initialize all counts to zero and coverage to `[]`. The algorithm performs no division and has no empty-result special arithmetic.

### 3.1 Worked reconciliation

For three returned models:

- model A: `baseModels: ["SDXL 1.0", "Pony", "Pony"]`;
- model B: no model-level base, versions carry `["SDXL 1.0", "SDXL 1.0"]`;
- model C: `baseModels: []`, versions have no usable base.

The result is:

```json
{
  "returned": 3,
  "baseCoverage": [
    {"baseModel": "SDXL 1.0", "models": 2},
    {"baseModel": "Pony", "models": 1}
  ],
  "modelsWithMultipleBases": 1,
  "modelsWithoutBase": 1
}
```

Coverage sums to three only coincidentally here: A contributes twice and C contributes zero. Consumers must use the explicit counters rather than treating the categories as exclusive partitions.

## 4. JSON contract

The representative fixture produces this abbreviated envelope (the existing item objects are not abbreviated in actual output):

```json
{
  "schemaVersion": "civ.search/v1",
  "kind": "modelSearch",
  "items": [
    {"id": 2458426, "baseModels": ["Anima"], "versions": []},
    {"id": 260267, "baseModels": ["SDXL 1.0"], "versions": []}
  ],
  "summary": {
    "returned": 2,
    "baseCoverage": [
      {"baseModel": "Anima", "models": 1},
      {"baseModel": "SDXL 1.0", "models": 1}
    ],
    "modelsWithMultipleBases": 0,
    "modelsWithoutBase": 0
  }
}
```

Contract rules:

- `summary` and all four members are required on successful compact responses.
- `baseCoverage` contains observed nonzero categories only; unknown nonblank names are emitted verbatim and escaped normally by `encoding/json`.
- No `total`, `percentage`, `primaryBase`, cursor, or metadata field is added.
- Items retain their current length, order, fields, null/empty behavior, and version order. `civ show` DTOs are untouched even though they share `CompactModelSummary`.
- This is an additive field under `civ.search/v1`; no existing field is removed, renamed, or given a new type. A future incompatible change still requires schema review.

## 5. Human rendering

Keep `renderSearchTable(w, models)` as the command's rendering entry point to minimize conflict with the later non-TTY layout child. It computes `api.SummarizeSearchModels(models)` and calls a small `writeSearchSummary` helper after the existing table flush, or after `No models found.` for an empty slice.

Exact normal form:

```text
Returned: 2; multi-base models: 0; models without base: 0
Base coverage: Anima=1; SDXL 1.0=1
```

Empty form follows the existing message:

```text
No models found.
Returned: 0; multi-base models: 0; models without base: 0
Base coverage: none
```

Use a private constant `searchSummaryHumanBaseLimit = 8`. Render the sorted prefix in `name=count` form joined by `; `. If more categories exist, append `; +K more (use --json for all)`. Do not truncate or normalize an emitted name. The JSON projection always carries the full list; only human presentation is windowed.

The footer writes to the same stdout writer as the table. It emits no warning, log line, ANSI/non-ASCII decoration, or separate stderr diagnostic. Writer errors are returned through the existing command error path.

## 6. Compatibility, errors, and request behavior

- Leave the full-view branch byte path unchanged: raw items extraction, `report.WriteRaw`, and its trailing newline. No summary computation is needed on that branch.
- Default compact and explicit compact both call the same parser, so their bytes remain identical.
- Human validation and request construction remain in their current order. No summary work occurs before successful response decoding.
- Existing invalid inputs continue to return before `SearchModels`; tests assert no request and zero stdout. A decode failure returns before any JSON or human result is written; no full-view fallback is introduced.
- Success still requires exactly one `/models` request for each invocation. There is no cursor following even when metadata offers one.
- Existing compact malformed-root/items errors and numeric-drift behavior remain unchanged; the summarizer adds no new source decode errors.

## 7. Test design

### 7.1 API projection tests

Extend `internal/api/compact_test.go` with table-driven summary cases over inline response bodies:

- model-level bases absent, null, `[]`, and all blank, with and without usable versions;
- version bases absent, null, empty, and blank;
- model-level precedence when versions carry additional names;
- duplicates within model-level data and across multiple fallback versions;
- multiple distinct bases on one model;
- unknown mixed-case/whitespace-preserving nonblank names;
- equal counts and permuted item/base order to pin deterministic sorting;
- zero items and required `[]`/zero fields;
- parity between `ParseCompactModels(raw).Summary` and `SummarizeSearchModels` over `ParseModels(raw)` for every common case.

Update the existing fixture, required-key, deterministic-repeat, and size/record-preservation tests to assert the summary without weakening the existing size threshold or item/version assertions. Full-view tests remain command-level because raw passthrough intentionally bypasses compact projection.

### 7.2 Command tests

Extend `internal/cmd/search_test.go` to verify:

- the fixture table footer follows the table and matches compact summary facts;
- human empty output retains `No models found.` and adds returned zero;
- default JSON exposes typed summary values and explicit compact remains identical;
- full raw output remains byte-identical and is an array;
- more than eight categories show exactly eight names plus the correct overflow marker, with no `=0` category;
- all three output paths still make one request with identical query parameters;
- each existing invalid-input case has no request and no stdout, and compact decode failure remains no-stdout.

Use synthetic inline bodies for semantic edges rather than altering verbatim live fixtures or adding a shared test helper that the sibling task would also need to reconcile.

## 8. Tradeoffs and rollback

- **Shared core over double decoding:** two small adapters are cheaper and less behavior-changing than parsing every human response through the stricter compact decoder, while still preventing semantic drift.
- **Complete JSON, bounded human prefix:** dropping JSON categories would require a new truncation contract and hide observed evidence. Eight human entries keep the footer concise; overflow explicitly points to the complete compact projection.
- **Fallback on no usable model-level evidence:** treating absent/null/empty/all-blank uniformly matches the existing table's practical fallback condition, but using all versions removes the arbitrary first-version choice.
- **Stay on v1:** the change is strictly additive. A version bump would fragment consumers without solving an incompatibility.

Rollback can occur in two small steps: first remove the command footer wiring while retaining tested API DTOs, then remove the summary field/core if the compact addition itself must be withdrawn. Do not alter full-view code, item projections, or sibling non-TTY changes as part of rollback.
