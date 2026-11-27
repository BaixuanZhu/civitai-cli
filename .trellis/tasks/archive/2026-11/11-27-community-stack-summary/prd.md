# Community recipe stack summary

Status: **Requirements converged for final implementation review; implementation not started or authorized.**

## Goal and user value

Add `civ recipe <versionId> --source community --stack-summary` in human and JSON forms so users and agents can inspect which resource identities and generation values occur in one exact community sample without reading every image observation.

The result must be compact descriptive evidence with explicit coverage. It must not become a crawler, a prompt-bearing view, a recommendation engine, or a claim that the bounded sample is representative, stable, high quality, or reproducible.

## Confirmed facts and constraints

- The existing community command already performs one bounded `GET /images` with `modelVersionId`, `withMeta=true`, `type=image`, limit 1-100 (default 20), sort `Most Reactions` (default) or `Newest`, and fixed `period=AllTime`; it sends no page/cursor or visibility overrides (`internal/api/images.go:13-82`, `internal/cmd/recipe.go:251-288`).
- Existing community coverage is exactly `requestedLimit`, `returned`, `withMeta`, `usable`, and presence-only `continuationOffered`; continuation tokens and next-page URLs are never emitted or followed (`internal/api/community.go:36-58`, `internal/api/images.go:143-173`).
- The recipe family uses `schemaVersion: "civ.recipe/v1"` with distinct kind discriminators. Its list DTOs structurally exclude prompt text, raw metadata, workflows, and media locations (`internal/api/recipe.go:13-36`, `internal/api/community.go:10-179`). Project machine-schema guidance requires that type-level redaction and versioned envelopes (`.trellis/spec/backend/design-principles.md:170-189`).
- Existing projection preserves both resource arrays, their duplicates, nullable identity fields, and normalized weights. It deliberately does not infer resources from top-level `modelVersionIds` (`internal/api/community.go:512-645`). Numeric strings and JSON numbers share a tolerant normalizer; invalid and non-finite numbers are rejected rather than defaulted (`internal/api/recipe.go:268-409`).
- The current default community view is per-image only. Its archived R4 deferred aggregation until normalized equality, resource identity, deterministic ordering, counts, evidence scope, and redaction had a separately reviewed contract (`.trellis/tasks/archive/2026-11/11-24-community-recipes/prd.md:87-100,180-196`). This child is that separate contract; it does not change the default view.
- Community edge evidence currently lives in inline synthetic bodies, including null/unsupported metadata, duplicate resources, invalid numerics, missing identity/weights, and continuation (`internal/api/community_test.go:10-76,283-568`). `testdata/FIXTURES.md:14-24` has no recorded `/images` body; the recorded version fixture is official-example evidence, not a community stack fixture. New edge cases can remain synthetic and offline without refreshing live fixtures.
- Standard-library-only code, meaningful GoDoc, offline `httptest` tests, expected-failure coverage, clean stdout on failure, and the `go vet`/test/race gates are mandatory (`.trellis/spec/backend/quality-guidelines.md:18-63,69-143`). CLI-surface changes also require parent-coordinated live integration coverage.

## Requirements

### CSS-1 Opt-in CLI and compatibility

- Support exactly `civ recipe <versionId> --source community --stack-summary [--sort <order>] [--limit <count>] [--json]`, with human output when `--json` is absent.
- `--stack-summary` is valid only for the community source and may be combined with the existing community sort and limit options. It selects the aggregate view instead of, not in addition to, the existing observations view.
- Reject it with official source (explicit or default), `--example`, `--examples`, and `civ recipe image`. Existing ID, sort, limit, and selector validation remains unchanged; all invalid combinations fail before any request with usage-class exit 1 and empty stdout.
- With the flag absent, every official, community-list, and image-detail request and output contract remains byte-for-byte compatible apart from curated help documenting the new option.

### CSS-2 One response and exact evidence scope

- Reuse `Client.ListImages` and its existing validated query unchanged. Make no version/model/resource-detail request, pagination, alternate-sort retry, image/model download, media inspection, or other additional API call.
- Preserve the existing `provenance`, `query`, and `coverage` objects and definitions. In particular, report continuation offered but not followed, including on an empty response.
- Treat each returned `items[]` element as one sampled image, identified internally by its upstream response position rather than a nullable image ID. Resource duplicates within that element must not turn into multiple sampled images.

### CSS-3 Aggregate-only and large-field-free output

- Emit aggregates only: no `observations`, per-image blocks, image/post/user identity lists, supporting-image ID arrays, raw resource entries, or per-image issue arrays.
- Never emit prompt or negative-prompt text, excerpts, raw/free-form metadata, ComfyUI/workflow payloads, media/page/thumbnail URLs, BlurHash, continuation tokens, or next-page URLs in either rendering.
- Prompt/negative-prompt availability counts and rates are permitted only as aggregate status evidence; they carry no text values.

### CSS-4 Resource identity and distinct-image usage

- Aggregate checkpoint, LoRA, and other/unknown-type resource usage by the number of distinct sampled images associated with each conservative identity, never by raw resource-entry count.
- Within each resource type/category, choose identity by precedence: positive `modelVersionId`, then non-empty `hash`, then non-empty `name`. Do not merge an ID-based identity with a name/hash-only identity, because the response does not prove their equivalence. Do not case-fold or semantically resolve names/hashes.
- Collapse duplicate entries for the same image and identity across and within `civitaiResources` and `resources`. Count decodable entries with no usable identity as unidentified, but never rank them or invent an `unknown` identity.
- Classify only the reviewed `checkpoint` and `lora` type tokens into their named groups. Preserve other and missing types in a separate aggregate rather than aliasing them to LoRAs or checkpoints. Never infer checkpoint identity from top-level `modelVersionIds`, the requested version, resource origin, or a weight.
- Return at most five identities per resource group, ordered by descending distinct-image count with deterministic total-order tie breakers. Report the number of omitted identities; do not sum their image counts because image sets can overlap.

### CSS-5 LoRA weight evidence

- Compute medians only from finite weights accepted by the existing normalizer. Missing, null, malformed, non-scalar, and non-finite weights contribute no sample and never default to 1. Do not impose an unverified weight range or clamp observed values.
- For one sampled image and one LoRA identity, repeated equal normalized weights count once; different valid observed weights remain separate samples. Thus duplicate resource entries cannot inflate weight samples, while genuinely different observed weights are not silently selected or averaged away.
- For each returned LoRA identity, expose its distinct-image count, number of those images with at least one valid weight, number of retained weight samples, and median. Sort samples numerically; for an even count, use the arithmetic mean of the two middle values. With no valid samples, the median is null and coverage is honestly zero.

### CSS-6 Parameter distributions

- Aggregate `steps`, `cfgScale`, and `sampler`; include `scheduler` because it is already a first-class normalized field in both official and community recipe projections and needs no new fetch or interpretation (`internal/api/recipe.go:110-126`, `internal/api/community.go:476-510`).
- Every distribution reports `observedImages` and the full `sampledImages` denominator. Only valid normalized values count as observed: numeric zero is observed; absent, null, empty, malformed, fractional-integer, non-scalar, and non-finite values are not.
- Numeric equality follows the existing normalizer. Sampler and scheduler equality remains exact and case-sensitive after only an emptiness check; do not trim into a different value, lowercase, or merge semantic aliases.
- Return at most five values per parameter, ordered by descending image count and deterministic value tie breakers. Always include an `other` object with the observed image count and distinct-value count outside the emitted top values, so top plus other reconciles to `observedImages`.

### CSS-7 Prompt availability without prompt content

- From the same raw response, count `provided` (non-empty string), `empty` (exact empty string), `notProvided` (absent/null, including unavailable metadata), and `unsupportedJsonShape` separately for prompt and negative prompt. Whitespace-only strings remain `provided`, matching `recipeTextValue` (`internal/api/recipe.go:391-409`).
- The four status counts sum to `coverage.returned` for each field. Expose `providedRate = provided / returned`; when returned is zero, the rate is null in JSON and `n/a` in human output.
- Unsupported metadata makes both fields unsupported; a readable metadata object classifies each field independently. No status computation may retain text in the public DTO.

### CSS-8 Honest empty, partial, and biased-sample states

- Empty or absent/null `items` succeeds with zero counts, non-nil empty top arrays, zero other counts, null availability rates, and no fabricated distributions or medians.
- Partial metadata and invalid fields reduce only their relevant observed/weight coverage. Valid sibling fields and items remain usable; malformed response envelopes remain errors with no partial stdout.
- Mark the machine result with stable caveats for a bounded non-random sample, continuation not followed, and descriptive evidence only. Human output and shared documentation must explicitly say that Most Reactions and Newest are biased discovery orders, not random samples.
- Do not derive recommendations, stability or compatibility claims, quality/engagement scores, selected defaults, or reproducibility guarantees from counts, ranks, rates, or medians.

### CSS-9 Versioned deterministic machine contract

- Use `schemaVersion: "civ.recipe/v1"` and the distinct `kind: "communityStackSummary"`; the existing family is compatible because this is a new view rather than a change to an existing kind.
- Keep all contracted fields present, arrays non-nil, counts internally reconcilable, and output deterministic regardless of Go map iteration. Use no third-party dependency and introduce no mutable global or persistent state.

### CSS-10 Verification and coordinated shared updates

- Add pure API aggregation tests and command-level `httptest` tests for success and expected failure, including request counts, exact query keys, human/JSON redaction, deterministic ties, and zero denominators.
- The child owns product code, unit tests, and recipe help. The parent integrator owns the shared integration script, README, bundled skill, backend specs, and CHANGELOG; `implement.md` defines the exact handoff and live scenarios.

## Acceptance criteria

- [ ] **AC-1 (CSS-1, CSS-2, CSS-9):** Default and custom sort/limit stack-summary invocations each make exactly one `/images` request with the existing six query keys and no version/detail/media request. JSON has `civ.recipe/v1`, `communityStackSummary`, identical query/provenance/coverage to the list projection, and no observations. Existing no-flag tests remain green.
- [ ] **AC-2 (CSS-1):** Official plus `--stack-summary`, community plus either official selector, image detail plus the flag, invalid IDs/sorts, limits 0/-1/101, and malformed flag values all fail with exit class 1, zero requests for run-level validation, and empty stdout.
- [ ] **AC-3 (CSS-2, CSS-4):** If two returned items each name checkpoint version 777 and one item duplicates it in both resource arrays, its usage count is 2 images, not 3 entries. An ID-based entry and a hash/name-only entry are not merged; top-level `modelVersionIds` creates no checkpoint; identity-free and unknown/untyped entries receive their defined coverage treatment.
- [ ] **AC-4 (CSS-4, CSS-9):** More than five tied resource identities produce exactly five deterministic rows plus the correct omitted-identity count. Repeating the same parse yields identical JSON; changing map iteration cannot change ordering.
- [ ] **AC-5 (CSS-5):** For one LoRA seen in three images with samples `[0.5, 0.5 duplicate, 1.0, missing/invalid]`, usage is 3 images, weight coverage is 2 images and 2 retained samples, and median is 0.75. A separate identity with no valid weights has null median and zero coverage. Two different valid weights in one image remain two samples; equal duplicates remain one.
- [ ] **AC-6 (CSS-6):** Numeric strings and numbers coalesce, zero remains observed, invalid numerics reduce observed coverage, and `Euler`, `euler`, and `Euler ` remain distinct sampler values. More than five tied values produce deterministic top five plus `other`; top counts plus `other.imageCount` equal `observedImages` for all four parameters.
- [ ] **AC-7 (CSS-7):** A mixed body containing non-empty, whitespace-only, empty, absent/null, and wrong-shape text plus null/unsupported metadata yields the specified per-field status counts and rates, with each status set summing to returned and no text sentinel or exact prompt/negativePrompt key in JSON.
- [ ] **AC-8 (CSS-2, CSS-7, CSS-8):** Empty, absent, and null items return successful zero states and null rates; an offered continuation keeps `coverage.continuationOffered: true`, while no cursor, continuation token, or next-page URL is exposed or followed. Non-object items and null/unsupported metadata degrade honestly, while malformed envelope JSON or non-array items fails with empty stdout.
- [ ] **AC-9 (CSS-3, CSS-8):** Human and JSON sentinel tests prove no prompt/negative text, raw unknown metadata, workflow/ComfyUI payload, media/page URL, per-image identity, continuation token/cursor, or next-page URL leaks; the boolean `coverage.continuationOffered` remains present and accurate. Neither form contains a recommendation, quality score, stability claim, or reproducibility guarantee.
- [ ] **AC-10 (CSS-10):** Parent integration covers default and Newest stack summaries, schema/kind/query/coverage invariants, bounded arrays, count/rate reconciliation, redaction, help, and offline official-source/range failures without assuming non-empty live metadata.
- [ ] **AC-11 (project contracts):** `go vet ./...`, `go test ./...`, and `go test -race ./...` pass; `go list -m all` still prints only `civitai-cli`; all new exported Go symbols have meaningful GoDoc and all new behavior has success plus expected-failure tests.

## Out of scope

- Pagination, exhaustive/random sampling, alternate-query retries, combined official/community views, or any extra network/resource-resolution operation.
- Image/video/audio download, thumbnails, visual analysis, media inspection, or visibility/NSFW overrides.
- Prompt text or excerpts, raw metadata/workflow output, prompt parsing, per-image observations or supporting-image lists in this view.
- Name/hash resolution, cross-basis or semantic resource deduplication, automatic checkpoint/LoRA resolution, and compatibility/readiness evaluation.
- User-configurable top count, arbitrary aggregation fields, statistical inference, recommendation/default selection, quality scoring, or reproducibility claims.
- Persistence, caches, background work, new dependencies, or changes to existing output when `--stack-summary` is absent.
- Direct edits by this child to parent-owned shared docs, integration scripts, backend specs, or release notes.
