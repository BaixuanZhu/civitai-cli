# Implementation plan: bounded community recipe sampling

Status: **Converged planning draft; all steps unstarted. Do not run `task.py start` without explicit final approval.**

## Wave scope override (2026-11-24 parallel wave, wave 2)

The recipe core (`internal/api/recipe.go`, `internal/cmd/recipe.go`) is implemented and committed at HEAD; this child now adapts to it. These overrides win over any conflicting step below.

- **Package placement decision (confirmed now):** put browse decoding, the pure community projector, and the public community DTOs in `internal/api/images.go` + `internal/api/community.go` (plus their `_test.go` files), beside `recipe.go` so the approved shared scalar normalizer is reused directly (same package, package-private helpers). Do not create a separate `internal/community` package and do not duplicate the normalizer.
- **Owned files (only these may be created/edited):** `internal/api/images.go`, `internal/api/images_test.go`, `internal/api/community.go`, `internal/api/community_test.go`, new fixture files under `testdata/` only if unavoidable, and modifications to `internal/cmd/recipe.go` + `internal/cmd/recipe_test.go` for the `--source/--sort/--limit` list options and the `civ recipe image <imageId>` subcommand. If a recipe.go helper must change to serve both sources, keep it package-private and keep all existing recipe tests green.
- **Forbidden (parent integration pass owns them):** `main.go`, `internal/cmd/help.go`, help tests, all `internal/cmd/completion*.go`, `scripts/integration-test.sh`, `README.md`, `CHANGELOG.md`, `internal/skill/assets/**`, `testdata/FIXTURES.md` (report required provenance entries instead), `go.mod`/`go.sum`, `internal/api/types.go`, `internal/api/compact*.go`, `internal/api/compare*.go`, every other cmd/api file, and every `.trellis/tasks/` directory except this one.
- **Parent integration pass takes over:** recipe help text additions (if any beyond the core page), dispatch/help-map/completion wiring, docs/Skill, FIXTURES.md, integration-script cases. Do not treat their absence as a failure.
- **No git commands:** never run `git add`, `git commit`, or `git push`.
- **Sibling coexistence:** compare/resolve/search-show agents may still be editing `internal/api` and `internal/cmd`. Ignore build/test failures that trace to files you do not own, and say so in the report; never edit them.
- **Verification substitute:** run focused tests for your own files. If `go test ./...` fails only because a sibling's in-progress file is broken, report that instead of editing it.

## Pre-start review gates

- [x] Review the sibling `11-24-recipe-context` core contract: source registration, neutral observation/detail types, shared summary/detail output, use of the approved R7 shared normalizer, and ownership boundaries.
- [x] R1 list CLI/source, R2 default limit, R3 default sort, R4 no-aggregation MVP, and R5 core integration are decided in the PRD. Implement without reopening them unless implementation evidence exposes a real contradiction.
- [x] Confirm the parent-approved invariant: no large fields in any list/summary; prompts and other large fields only from the approved `civ recipe image <imageId> [--json]` single-detail command.
- [ ] Confirm the pure projector's final package placement without making it import the recipe core.
- [x] Confirm each list uses one logical bounded `/images` operation with `type=image` and `period=AllTime`; the separate detail command uses one exact `imageId` request with `withMeta=true`, `type=image`, and `limit=1`. Neither command uses cursor/page, NSFW overrides, cross-command handoff, persistence, or resource fan-out.
- [ ] Before any later implementation start, the main/session owner must curate this task's spec/research context through the normal Trellis process. This planning pass deliberately does not edit JSONL manifests or `task.json`.

## Ordered implementation checklist

### 1. Pin endpoint behavior with offline fixtures

- [ ] Add one deliberately fetched, verbatim `GET /images?...&withMeta=true` list response fixture with real provenance/reactions/resource examples and a separate exact-image detail fixture; record fetch dates and exact URLs in `testdata/FIXTURES.md`. Do not require live data during unit tests.
- [ ] Add small synthetic table fixtures for null/empty/prompt-only/unknown-only metadata, scalar drift, absent stats, malformed known fields, both resource arrays, empty items, continuation metadata, and empty/mismatched/multi-item detail responses. Keep unique prompt sentinel values for redaction tests.
- [ ] Implement browse-specific API envelope/item decoding plus bounded list and explicit single-detail methods in `internal/api`, routed only through `Client.Get` with `url.Values`.
- [ ] Add list API tests asserting exact path/query for both sort values, required `modelVersionId`, `withMeta=true`, `type=image`, `period=AllTime`, reviewed limit, and absence of `modelId`, `page`, `cursor`, `nsfw`, and `browsingLevel`.
- [ ] Add detail API tests asserting exactly `imageId`, `withMeta=true`, `type=image`, and `limit=1`, with no list sort/period, model/version filters, cursor/page, or visibility overrides. Pin empty, mismatched, and multiple-item responses to detail-unavailable behavior without another query.
- [ ] Test tolerant browse decoding independently of any recipe-core type. Verify reaction fields, null/object metadata distinctions, unknown fields, textual NSFW values, and empty `items`.

**Independent checkpoint:** API fixture tests run offline and need no core contract implementation. Rollback is confined to the new endpoint/DTOs and fixture manifest entries.

### 2. Implement the pure community projector

- [ ] Add the child-owned source input/output types with a public-safe observation DTO separate from internal detail storage.
- [ ] Implement per-field scalar projection for the allowlist in `design.md`, using the approved R7 normalizer: numeric strings become numbers, seed stays decimal string, zero/null/empty distinctions survive, invalid/non-scalar values produce null plus issue, and no unit/default inference occurs.
- [ ] Implement origin-labeled `civitaiResources` and `resources` projection with exact order/duplicates and optional type/version/name/hash/normalized weight. Do not infer weights, preserve numeric-string weight distinctions, or use top-level `modelVersionIds` as a stack.
- [ ] Implement `returned`, `withMeta`, `usable`, and `continuationOffered` exactly as defined in the PRD, including zero-denominator behavior and non-nil empty output arrays.
- [ ] Keep list results compact: do not create a list detail map or retain raw list metadata for another command. Project the independent exact-image response directly into one detail result. Ensure no public list/summary JSON tag or method can reach raw metadata, prompts, or other large fields.
- [ ] Implement the approved R4 policy: upstream-order per-image observations only, with no frequency tables, averages, rankings, quality scores, or recommendation fields.

**Independent checkpoint:** table-driven projector tests cover AC-3 through AC-8 and AC-10 without HTTP, CLI, or recipe-core imports. If this checkpoint fails, change projection logic rather than weakening coverage definitions.

### 3. Verify boundedness and absence of side effects

- [ ] Use `httptest` handlers that fail on unexpected endpoints and count requests. A successful list response with `nextCursor`/`nextPage` must cause exactly one request and no continuation.
- [ ] Prove the independent `civ recipe image <imageId> [--json]` command causes exactly one logical exact `imageId` request and returns only that item's detail; it neither reuses list state nor fetches details for other list items.
- [ ] Assert no request to CDN item URLs, thumbnails, `/model-versions/{id}`, alternate sorts, broadened model queries, or cursor/page URLs from either command.
- [ ] Assert invalid IDs, limits, and sorts fail before HTTP with usage-class errors; empty list items succeeds without inferring version absence; empty/mismatched/multi-item detail is unavailable without broadening or another semantic request.
- [ ] Reuse existing client error tests/mappers for 401, 404, 429/5xx retry exhaustion, cancellation, and malformed envelope behavior. Add only community-specific assertions needed for stdout/stderr and coverage.
- [ ] Run list and detail commands against temporary config/cache roots and assert no new files or modifications. Confirm no persistence package, cross-command handoff, or detail store exists.
- [ ] Add a body-limit/error-hint test if the command introduces a "reduce the community limit" hint; do not raise the existing 5 MB cap.

**Rollback point:** stop before core/CLI wiring if the source needs pagination, resource fan-out, large fields in a list/summary, bulk detail, or persistence to satisfy tests; that indicates a scope/design defect requiring re-review.

### 4. Adapt to the reviewed recipe core

- [ ] Implement only the thin mapping assigned by the core review. Keep source identity, query provenance, image IDs, upstream positions, observations, and reactions intact.
- [ ] Register community as an optional source with official-only behavior as the default. For approved combined mode, keep official and community results separately labeled.
- [ ] Wire `civ recipe image <imageId> [--json]` through the core using one bounded exact `imageId` request. Add no community list `--prompts` shortcut, list-detail selector, cross-command handoff, or bulk detail path.
- [ ] Add adapter tests with a fake/minimal core contract fixture. Prove detail selection returns only the requested image's original metadata, every list/summary stays large-field-free, source IDs cannot collide by position, and no core implementation is needed by the lower-level projector tests.
- [ ] Assert exactly one logical bounded exact-image call per detail invocation, no list-state input or reuse, and no detail state after completion.
- [ ] If the core contract cannot represent required provenance/coverage/resource evidence or the approved independent single-detail invariant, return to planning and revise the core contract rather than dropping fields silently or leaking details into a list.

**Dependency checkpoint:** this is the first step that requires the reviewed core. All previous fixture tests remain runnable if the adapter is reverted.

### 5. Add the reviewed CLI/report surface

- [ ] Implement the approved R1 list syntax `civ recipe <modelVersionId> --source official|community [--json]`, default official, with community-only limit default 20 (range 1–100) and sort default Most Reactions/AllTime (Newest supported), plus the approved independent `civ recipe image <imageId> [--json]` command using the existing `flag`/pure `run*`/`execute` patterns and `newAPIClient` seam. Do not implement a combined `both` source. Do not modify existing command flags or output.
- [ ] Add curated list help with legal source/sort/limit values, bounded-list wording, visibility caveat, and the invariant that every list/summary omits large fields. Add detail help pinning the exact bounded `imageId` request, single-detail-only large fields, and at least one `--json` example. Keep shared global flag rows in the existing global help block only.
- [ ] Render normalized human/JSON lists/summaries through public compact DTOs. Include source/query, counts/coverage, continuation flag, provenance, separate reactions, observed parameters, and resource stacks. Never use raw API passthrough or include prompts/raw metadata in any list mode.
- [ ] Render large fields only from the independent exact-image detail response for the requested image. Do not emit other details, a detail array, or list-state provenance.
- [ ] Add command tests for defaults, explicit options, invalid combinations, source-disabled zero `/images` calls, empty success, stable empty arrays, large-field-sentinel absence from every list/summary, output/exit-code mapping, and dual help entry points.
- [ ] Add detail request/output tests: exact `imageId` query, only the selected detail in human/JSON output, no list-state input, no cross-command handoff, and no persistence.
- [ ] Add tests proving the MVP emits no aggregation, frequency, ranking, quality-score, or recommendation fields.
- [ ] Update dispatch/help/completion registration only where the reviewed core task assigns ownership; avoid conflicting edits to sibling-owned files during parallel work.

**Rollback point:** reverting source registration and command wiring restores official-only behavior; there is no state or migration to unwind.

### 6. Documentation and live integration parity

- [ ] Update README command/API notes for the final list surface and approved `civ recipe image <imageId> [--json]` command, one-response list behavior plus one exact-image detail request, universal list/summary large-field omission, coverage definitions, normalized-value semantics, no-recommendation language, no cross-command handoff or persistence, and anonymous/account visibility constraints.
- [ ] Update `scripts/integration-test.sh` in the same implementation change for every added command/flag/output shape and help route.
- [ ] Add offline integration assertions for usage errors, help, and any locally fakeable behavior.
- [ ] Add bounded live list cases for both approved sort options with a small explicit limit. Validate JSON shape, source/query/coverage fields, and absence of large fields in every list/summary; accept returned/withMeta/usable = 0 and either continuation value.
- [ ] Add the approved explicit single-detail case: `civ recipe image <imageId> [--json]` must use one exact bounded `imageId` request and return only that detail. Validate detail-unavailable behavior where safely fakeable and no handoff/persistence; do not assume a live image is visible.
- [ ] Assert live command request behavior by construction/logs where practical: no pagination, alternate-sort retry, automatic/bulk detail, media download, NSFW override, or per-resource lookup. Do not make live success depend on a particular reaction count or resource combination.

### 7. Full quality verification

- [ ] Format only changed Go files with `gofmt`; confirm meaningful GoDoc on every new exported symbol and all text in English.
- [ ] Run focused offline tests for API, projector, adapter, and command packages after each chunk.
- [ ] Run `go vet ./...` and `go test ./...`.
- [ ] Run `go test -race ./...`.
- [ ] Run `go list -m all` and confirm only `civitai-cli` is listed.
- [ ] Review the final diff against every CR/AC, especially universal list/summary large-field redaction, explicit single-detail-only prompts, coverage denominator, reviewed scalar semantics, no inferred resource weight, bounded list/detail request counts, independent detail isolation, and unchanged existing commands.
- [ ] Run the updated integration script only under its normal manual/CI network preconditions; never make it a unit-test dependency.
- [ ] Perform the Trellis spec-update judgment and record any new reusable `/images` free-form-metadata or source-adapter convention through the normal finish workflow.

## Validation matrix

| Area | Required proof |
|---|---|
| Exact list request | URL query tests for both sorts; forbidden params absent |
| Exact detail command | `civ recipe image <imageId> [--json]` uses only imageId/withMeta/type/limit=1 and returns one selected detail |
| Boundedness | nextCursor/nextPage list fixture yields one request; detail is independent with no fallback/fan-out/CDN calls |
| Coverage | Six-item fixture gives returned/withMeta/usable = 6/5/2; empty gives 0/0/0 |
| Redaction | Large-field keys and unique values absent from every list/summary text and JSON mode |
| Detail ownership | Core-owned detail command returns one image's large fields; no bulk/unselected detail or list selector |
| Detail isolation | Detail uses only its exact response; no list-state input, cross-command handoff, or persisted output state |
| Parameters | Numeric strings/numbers normalize equivalently; seed string, zero/null/empty distinctions, and invalid/non-scalar null-plus-issue behavior are correct |
| Resources | Both origins, order, IDs/names/hashes, explicit weights; no top-level-ID inference |
| Provenance | Source/query/image/post/user and raw per-type reactions preserved without inventing prior-list provenance |
| Aggregation | Conditional on R4; exact normalized-value equality/order/count/supporting IDs and large-field omission tested |
| Visibility | No NSFW/browse override; empty list succeeds and unavailable detail does not broaden |
| Side effects | No file changes, persistence, image downloads, or background work |
| Compatibility | Official-only and all pre-existing command tests remain green |

## Risky files and coordination notes

- `main.go`, shared help/dispatch, and any future recipe-core files may be sibling-owned. Coordinate ownership before editing; do not resolve overlap by duplicating command registration or core types.
- `internal/api/types.go` already models embedded version images. Browse DTO additions must not silently change that endpoint's NSFW or metadata semantics.
- `testdata/FIXTURES.md` and `scripts/integration-test.sh` are shared files: append/update only the community-specific entries during implementation and preserve unrelated concurrent work.
- The recipe JSON envelope is a compatibility contract owned by the recipe core. Community integration must conform to the approved summary/detail shapes and must not become raw `/images` passthrough.

## Final review questions to carry forward

1. Do the approved list CLI, default limit 20, Most Reactions/AllTime default, no-aggregation policy, and shared scalar semantics match help, tests, and README, and is the approved R6 detail request flow implemented exactly?
2. Does every reported aggregate or observation trace to source image IDs without claiming best/recommended?
3. Can the endpoint and projector test suites still run solely from fixtures if the recipe core is unavailable?
4. Are prompts and all other large fields visible only in one explicitly selected core detail, with no large-field leakage into any list or summary?
5. Does independent `civ recipe image <imageId> [--json]` use exactly one bounded `imageId` request, with no same-invocation alternative, list-state input, or cross-command handoff?
6. Is there no hidden persistence, cross-invocation detail storage, media fetch, cursor traversal, resource fan-out, automatic/bulk detail, or visibility bypass?
