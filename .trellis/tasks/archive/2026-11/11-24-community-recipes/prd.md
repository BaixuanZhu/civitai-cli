# Bounded community recipe sampling

Status: **Requirements converged for implementation review; implementation not started or authorized.**

## Goal and user value

Add an optional, bounded community-observation source beneath the future recipe surface. It should supplement sparse or null official version examples with real, uploader-supplied checkpoint/LoRA stacks, explicit resource weights, and observed generation parameters for one exact model version.

The result is evidence for review, not a crawler, a persistent collection, or a recommendation engine. Users and agents must be able to tell where every observation came from, how much of the bounded sample had metadata, and which observations were actually usable.

**Parent-approved output invariant:** every list or summary omits large fields. Large fields such as prompts, negative prompts, raw/free-form metadata, and full workflow payloads are returned only for an explicitly selected single-image detail. This applies to human output, JSON, aggregates, and combined-source views.

## Confirmed facts and constraints

- The current `show -v --prompts` and `vet --prompts` paths read only the version's embedded `images[]`; nullable metadata is skipped and counted. They do not browse community posts or project resource stacks (`internal/cmd/prompts.go`, `internal/api/types.go`).
- The repository has no `/images` client yet. The sibling `11-24-recipe-context` owns the recipe command and summary/detail DTO family; its approved contract is the integration target.
- The official reference is `GET https://civitai.com/api/v1/images`, documented at <https://developer.civitai.com/site/reference/images.md> (reviewed 2026-11-24).
- The API supports exact `modelVersionId`, `withMeta=true`, `sort=Most Reactions|Newest`, `period=AllTime`, `type=image`, and positive bounded limits. Its documented limit range is 0-200, but `limit=0` has special visibility behavior and is not a valid bounded-sample choice.
- Image `meta` is uploader-supplied and free-form. Documented examples include scalar generation fields plus `resources` and `civitaiResources`; absence, nulls, unknown keys, and schema drift are expected.
- Response `stats` exposes separate reaction/comment counts. Top-level provenance includes image ID, post ID, uploader username, creation time, media type, and visibility labels.
- Anonymous and authenticated responses are constrained by Civitai visibility and browsing-level rules. An empty result can be a valid visibility outcome and does not prove that a version does not exist.
- Project hard contracts remain: Go standard library only, all outbound API traffic through `api.Client.Get`, offline `httptest`/fixture tests, English output, stdout for results only, and existing exit-code semantics.

## Dependency and review gate

The recipe core contract must be reviewed before community-source integration. That review must assign the source registration seam, the source-neutral observation contract, selected-image detail ownership, and the final recipe output envelope.

This child remains independently testable before and after that review: endpoint decoding, bounded query construction, metadata projection, coverage accounting, resource extraction, and summary redaction must all have fixture tests that do not import or require the recipe core.

The list CLI, default limit, default sort, aggregation policy, shared scalar normalizer, separate detail command, and source-neutral integration boundary are decided; see **Approved decisions**. No implementation starts until the parent convergence review and explicit task approval.

## Requirements

### CR-1 Optional source on the recipe surface

- Community sampling is opt-in; the official recipe path remains the default and does not incur `/images` traffic. The approved list form is `civ recipe <modelVersionId> --source official|community [--json]`, default `official`; combining both sources in one view is deferred.
- Every community list invocation identifies exactly one positive `modelVersionId`. There is no fallback to `modelId`, all versions, text search, or a hash lookup.
- Every separately invoked community detail identifies exactly one positive `imageId`; bulk or multi-image detail selection is not supported.
- Existing `show`, `vet`, `search`, and `sync` behavior and output remain unchanged.
- If a combined official-plus-community mode is approved, each source remains separately labeled and a community empty sample does not erase official evidence.

### CR-2 Bounded list and explicit-detail requests

- Perform one logical `GET /images` operation per community list sample, using the existing client and its normal transient-retry policy.
- A list always sends `modelVersionId`, `withMeta=true`, `type=image`, the approved positive `limit` (default 20, range 1–100), the approved sort (default `Most Reactions`; `Newest` allowed), and `period=AllTime`.
- Support only `Most Reactions` and `Newest` as community review orders. The default is `Most Reactions` with fixed `period=AllTime`; neither is a quality signal. Do not expose Random, Oldest, Most Comments, or Most Collected in this task.
- Use a CLI list limit range of 1-100 with default 20, even though the API permits up to 200. Reject zero, negatives, values above 100, invalid IDs, and unknown sorts before any request.
- Do not send `page` or `cursor` on list or detail requests, and never follow `nextCursor` or `nextPage`. Record whether a list response offered a continuation without emitting or consuming the opaque token.
- The approved `civ recipe image <imageId> [--json]` command performs one logical `GET /images`, sending exactly `imageId`, `withMeta=true`, `type=image`, and `limit=1`; it sends no list sort/period, `modelVersionId`, `modelId`, `page`, `cursor`, `nsfw`, or `browsingLevel`. Existing transient-retry policy applies, but there is no application-level retry with another ID or filter.
- A detail response must contain exactly one item whose ID matches the selection. An empty, mismatched, or ambiguous result is a detail-unavailable outcome, not a reason to page, broaden the query, or infer whether the image is absent versus hidden.
- List and detail commands are independent. List output and internal list projections provide no detail-selection handoff, and no detail or list response is persisted for another command.
- No alternate-sort retry, broadened filter, automatic detail fetch, or automatic second sample when a list is empty or has poor coverage.

### CR-3 Explicit sample coverage

Every successful sample reports at least:

- `requestedLimit`, `returned`, `withMeta`, and `usable` counts;
- `withMeta / returned` and `usable / returned` coverage in human output, with JSON counts mandatory and ratios optional but well-defined;
- `continuationOffered`, derived only from response metadata;
- the exact source query semantics: requested version, sort, period, media type, and bounded limit.

Definitions are normative:

- `returned`: number of items in this one response.
- `withMeta`: number whose `meta` is a non-null JSON object, including `{}`.
- `usable`: number with at least one supported, non-null, non-empty scalar generation observation, or at least one resource entry carrying identity evidence (`modelVersionId`, `name`, or `hash`). Prompt-only, empty, unknown-field-only, and identity-free resource-only metadata do not count as usable.
- Zero returned items produces zero counts and `null`/not-applicable ratios, never NaN or division by zero.

These are coverage facts about this bounded response only, not estimates of all community content.

### CR-4 Large-field-free lists and summaries

- Every human or JSON list/summary, including aggregates, combined-source output, and non-default modes, contains no full or excerpted `prompt`, `negativePrompt`, raw `meta`, full workflow payload, or equivalent large free-form field.
- The community source must not use raw API JSON passthrough for any list/summary `--json` output, because that would expose prompts and unprojected free-form metadata.
- List/summary rows may contain only compact explicitly projected provenance, reactions, supported observed parameters, and resource stacks.
- Tests must use unique large-field sentinels and prove that neither sentinel values nor prompt/negativePrompt/raw-meta field names leak into any list or summary rendering.

### CR-5 Explicit single details belong to recipe core

- The recipe core, not the community list renderer, owns `civ recipe image <imageId> [--json]` single-image detail rendering, including prompts and any full free-form metadata it chooses to expose.
- Large fields are returned only by that explicit single-detail command. There is no bulk detail array, "all details" option, prompt-bearing list/detail combination, or list flag that selects detail.
- The detail command uses the bounded exact `imageId` request in CR-2 and returns only the matching item. It does not load list output, recover list memory, or claim prior-list query provenance that is not present in the new response.
- Detail selection never triggers CDN or model-version requests. Large fields are never a side effect of listing, sorting, aggregation, `--json`, or verbose output.
- Public list/summary DTOs contain no detail payloads. Detail response data is used only for the current detail invocation and is never handed to another command or persisted.

### CR-6 Truthful observed parameters

- Project at least `seed`, `steps`, `cfgScale`, `sampler`, `scheduler`, `clipSkip`, `denoisingStrength`, and dimensions when present and scalar. Top-level image dimensions may supplement metadata dimensions without being described as metadata.
- Apply the approved shared scalar normalizer: valid numeric strings in known generation/resource fields become JSON numbers, seed remains an exact decimal string, numeric `20` and string `"20"` are equivalent after projection, and missing/null stays null. Do not coerce unknown fields, infer units, or supply defaults.
- Invalid numbers and non-scalar shapes become null plus a structured issue for that observation without failing the whole sample. Empty strings and missing values remain distinct from numeric zero and from invalid shapes.
- Do not parse prompts for parameters, dimensions, resource names, or weights.
- Keep upstream item order for per-image summaries. The MVP emits per-image observations only and does not aggregate or rank values; any future aggregation needs a separate reviewed contract and must use normalized-value equality with supporting image IDs.

### CR-7 Resource stacks without inference

- Project both `meta.civitaiResources` and legacy/tool-specific `meta.resources`, labeling the origin of each entry.
- Preserve observed `type`, `modelVersionId`, `name`, `hash`, and `weight` fields when present, including entry order and duplicates. Numeric-string weights normalize to numbers under R7; invalid/non-scalar weights become null plus issue and never become a default weight.
- Do not invent a missing weight of 1, infer a checkpoint from the requested version, infer identity from top-level `modelVersionIds`, resolve names implicitly, or discard an unknown resource type.
- Keep the requested version filter separate from the image's actual observed resource list. The filter is provenance, not proof of stack membership.
- Do not make per-resource `/model-versions/{id}` calls. Any later resource-name/local-resolution workflow belongs to the resource resolver or recipe core and requires its own explicit bounds.

### CR-8 Source, reactions, and provenance

Each summary observation retains, when returned:

- source name `community-images`, image ID, post ID, uploader username, creation timestamp, and a Civitai image-page reference;
- requested version/sort/period and upstream item position;
- individual `likeCount`, `dislikeCount`, `heartCount`, `laughCount`, `cryCount`, and `commentCount` values, distinguishing absent stats from observed zero where practical;
- returned NSFW/browsing labels as visibility facts, not as instructions to widen access.

Do not emit a combined engagement/quality score or relabel reaction volume as quality. `Most Reactions` is an upstream discovery order, never a recommendation.

### CR-9 Visibility and free-form metadata boundaries

- Reuse existing anonymous/token/proxy behavior. Do not add NSFW/browsing-level elevation flags or send `nsfw`/`browsingLevel` overrides in this task.
- Never request image/video/audio URLs, thumbnails, or other CDN content.
- Unknown metadata keys and resource types are tolerated and ignored by the summary projection; malformed known fields degrade only that observation, not the entire valid response.
- A syntactically invalid response envelope remains an error. A valid empty list `items` array is successful output with zero coverage, not a 404 and not proof about version existence. An empty/mismatched explicit detail response is detail-unavailable and likewise does not prove absence versus hidden visibility.
- Standard API/network/auth failures retain existing exit-code and stderr behavior; stdout remains clean on failure.

### CR-10 No collection or hidden side effects

- No automatic pagination or exhaustive sampling.
- No image downloads or media inspection.
- No embeddings, similarity search, clustering, opaque ranking, "best", "recommended", or "top pick" selection.
- No response cache, database, history file, recipe library, or other persistence; existing token/proxy configuration behavior is unchanged.
- No background jobs, scheduled refresh, resource-resolution fan-out, or NSFW bypass.

### CR-11 Offline and live verification

- Add a recorded `/images` response fixture and synthetic drift cases; unit tests never require Civitai network access.
- Pin list/detail URL query shapes, bounded request counts, no-pagination behavior, coverage definitions, universal list/summary large-field redaction, single-detail selection, raw reaction preservation, parameter/resource projection, empty results, validation, and error mapping.
- Test the pure source independently of the recipe core, then test the approved separate `civ recipe image <imageId> [--json]` exact-image request and single-detail contract through the thin core adapter.
- Update command help, user documentation, and `scripts/integration-test.sh` for the finally approved CLI surface.
- Live checks must tolerate empty/sparse metadata and must not assert nonzero community coverage or bypass account visibility.

## Acceptance criteria

- [ ] **AC-1 (CR-1, CR-2):** With community source disabled, a fake core/CLI harness performs zero `/images` calls and official-only output is unchanged. `--source community` for version `3263843` produces exactly the approved query on `/images` with default limit 20/Most Reactions, and both `Most Reactions` and `Newest` map to their exact API enum strings.
- [ ] **AC-2 (CR-2):** A 200 fixture containing both `nextCursor` and `nextPage` results in one successful HTTP request, `continuationOffered=true`, and no cursor/page request. Invalid ID, limit 0, negative/over-100 limit, and unsupported sort each fail before HTTP with usage-class exit 1.
- [ ] **AC-3 (CR-3):** In a six-item fixture with `(A) meta=null`, `(B) meta={}`, `(C) prompt-only meta`, `(D) scalar steps plus identified civitaiResources`, `(E) an identified legacy resource with weight`, and `(F) unknown-field-only meta`, output reports `returned=6`, `withMeta=5`, `usable=2`. A separate empty fixture reports `0/0/0` and non-numeric/not-applicable coverage ratios.
- [ ] **AC-4 (CR-4, CR-5, decided R6):** Every list/summary text and JSON mode contains neither unique large-field sentinels nor prompt/negativePrompt/raw-meta fields. `civ recipe image D --json` makes exactly one logical request with `imageId=D`, `withMeta=true`, `type=image`, and `limit=1`, returns only D's detail, exposes no other image prompts, accepts no list state, and persists nothing.
- [ ] **AC-5 (CR-6, CR-7, decided R7):** Fixture values `steps:"20"` and numeric `20` normalize to the same JSON number, `cfgScale:0` stays zero, `weight:"0.65"` and numeric `0.65` normalize to the same number, null fields stay null, empty strings stay empty, and object-valued fields produce null plus issue without aborting sibling items. Seed remains an exact decimal string, no default weight/value appears, and no prompt-derived value is emitted.
- [ ] **AC-6 (CR-7):** A fixture containing checkpoint and LoRA `civitaiResources` plus a named legacy LoRA preserves both origins, stack order, IDs/names/hashes, and explicit weights. An item whose top-level `modelVersionIds` exists while meta is null does not gain a fabricated resource stack or count as usable.
- [ ] **AC-7 (CR-8):** Raw per-type reactions and comments match the fixture, absent stats do not become a fabricated quality metric, and output contains no quality score or best/recommended label. Source/query/image/post/user provenance allows every observation to be traced to its image ID.
- [ ] **AC-8 (CR-9, CR-10):** Instrumented list and detail `httptest` servers observe no `nsfw`, `browsingLevel`, `page`, or `cursor` parameters and no requests to item URLs, thumbnails, or model-version endpoints. Temporary config/cache directories are unchanged after list and selected-detail invocations, and no detail survives across invocations.
- [ ] **AC-9 (CR-9):** Unknown metadata keys/types and malformed known fields follow the defined per-item degradation rules; malformed envelope JSON returns an error; empty list items succeeds; empty/mismatched/multi-item detail is unavailable without another semantic lookup; existing 401/404/429/5xx classification and stdout/stderr separation remain intact.
- [ ] **AC-10 (CR-3, CR-6, R4, decided R7):** Default list output uses limit 20 and Most Reactions with fixed AllTime; explicit Newest and valid limits work. The MVP emits per-image observations only: tests assert no frequency table, average, recommendation, rank, or quality score is present. If aggregation is ever proposed later, normalized-value equality, ordering, counts, supporting image IDs, and large-field omission must be specified first.
- [ ] **AC-11 (CR-11):** The pure source test suite passes using fixtures without a recipe-core implementation. Post-integration help/JSON/exit-code tests match the approved list CLI and detail command, and live integration accepts empty or sparse samples without retrying another sort or following a continuation. Explicit detail tests verify the exact `civ recipe image <imageId> [--json]` request policy and tolerate visibility-driven detail unavailability.
- [ ] **AC-12 (project contracts):** `go vet ./...`, `go test ./...`, and `go test -race ./...` pass; no third-party dependency or persistent artifact is added; all new public Go symbols have meaningful GoDoc.

## Out of scope

- Exhaustive community history, cursor traversal, multi-page sampling, or representativeness claims.
- Image/media download, thumbnails, visual analysis, embeddings, clustering, or semantic deduplication.
- Persistent recipe collections, caches, databases, cross-invocation detail storage, or background refresh.
- Large fields in any list/summary, bulk or multi-image detail output, and prompt-bearing aggregate views.
- Opaque quality/engagement scoring and automatic recommendation or parameter selection.
- NSFW/browse-level overrides or circumvention of Civitai/account visibility.
- Prompt parsing to recover LoRAs, weights, parameters, or resources.
- Automatic resource lookup/resolution fan-out, local compatibility checks, or downloads.
- Video/audio recipe sampling, arbitrary image endpoint filters, arbitrary sorts, and user/post/collection browsing.
- Changes to existing command behavior or replacement of the official example-image source.

## Approved decisions

### R1 — List CLI and source shape

`civ recipe <modelVersionId> --source official|community [--json]`, default `official`. Community-specific limit/sort options are valid only when `--source community` is selected. `official` does not call `/images`. A combined `both` source is deferred; the MVP keeps official and community evidence separately enumerable.

### R2 — Default list limit

Default 20, accepted range 1–100. The approved separate detail command uses `limit=1`. The default is a bounded sample, not a representative census.

### R3 — Default sort

Default `Most Reactions` with fixed `period=AllTime`; `Newest` is also supported. Neither ordering implies quality or reproducibility, and there is no automatic fallback between them.

### R4 — Aggregation

The MVP emits per-image observations in upstream order only. No averages, frequency tables, rankings, quality scores, or "best" selection. Any future aggregation requires a separate reviewed contract with normalized-value equality and supporting image IDs.

### R5 — Recipe-core integration

The community adapter registers an optional `community` source beneath the recipe surface, reuses the approved summary/detail and scalar-normalization contracts, and must not create a competing recipe DTO family. Ownership and package placement are finalized during parent integration review.

### R6 — Separate single-detail UX and request flow

`civ recipe image <imageId> [--json]` performs one bounded logical `GET /images` with exactly `imageId`, `withMeta=true`, `type=image`, and `limit=1`. It returns only the selected image's large fields through the recipe-core detail view. The command is independent of list output and has no cross-command handoff or persistence.

Every list remains large-field-free. The detail command never exhausts cursors, fetches bulk details, sends list filters or NSFW overrides, or claims prior-list provenance. Empty, mismatched, or multiple results are detail-unavailable without another semantic lookup.

### R7 — Shared scalar normalization

Valid numeric strings in known generation/resource fields become JSON numbers, including LoRA weights. Seed remains an exact decimal string. Missing/null stays null; invalid/non-scalar values become null plus structured issue. Numeric `20` and string `"20"` are equivalent after projection. The implementation shares the official recipe normalizer, does not coerce unknown fields, and does not infer units or defaults.

## Planning status

All product and output-contract decisions for this child are resolved. Implementation remains unapproved and unstarted pending parent convergence and explicit task approval.
