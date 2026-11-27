# Design: community recipe stack summary

Status: **Design converged for final implementation review; implementation not started or authorized.**

## 1. Architecture and ownership

The feature is a second projection of the existing bounded community response, not another sampler or recipe engine. It reuses the API request, tolerant decode, normalized observations, and coverage from the current community list path.

| Boundary | Responsibility | Must not do |
|---|---|---|
| `internal/api/images.go` | Existing one-response `ListImages` operation | New query parameters, pagination, detail/resource fetches |
| `internal/api/community.go` | Existing validated sample projection, scalar/resource normalization, query/provenance/coverage | Aggregate policy or prompt-bearing list fields |
| New `internal/api/community_stack.go` | Public aggregate DTOs and pure `ParseCommunityStackSummary` aggregation | HTTP, flags, printing, persistence, raw/text fields in output |
| `internal/cmd/recipe.go` | Register/validate `--stack-summary`; select the aggregate parser after the existing single fetch | Duplicate query construction or aggregation policy |
| New `internal/cmd/recipe_stack.go` | Human rendering of the aggregate-only DTO | Raw response access, per-image rendering, network, exit calls |
| `internal/report` | Existing generic indented JSON writer and formatting helpers | Community field interpretation |

No new package, dependency, mutable global, API client seam, persistence, or goroutine is needed. All new exported DTOs/functions require meaningful GoDoc; implementation helpers stay unexported where possible.

### Data flow

```text
recipe args
  -> existing source/ID/sort/limit validation + stack-summary combination validation
  -> Client.ListImages(exact existing query)                  [one logical operation]
  -> if flag is false: existing ParseCommunitySample path     [unchanged]
  -> if flag is true:
       ParseCommunityStackSummary(raw, versionID, limit, sort)
         -> ParseCommunitySample(raw, ...)                    [reuse normalized observations + coverage]
         -> decode the same bounded body again locally for status-only text availability
         -> aggregate by response position and canonical identity/value keys
         -> sort, take top 5, compute other counts/rates/medians
         -> aggregate-only DTO with no raw/text/media/observation members
       -> report.WriteIndent OR renderCommunityStackSummary
  -> release response/sample memory; no persistence or follow-up
```

The second local JSON decode is deliberate. It reuses the stable list parser rather than refactoring its proven behavior, while a narrow status-only helper classifies prompt fields and immediately discards any decoded string. At the existing 100-item/5 MB bounds, this is preferable to changing the default list path or duplicating parameter/resource normalization. Profiling could justify a shared internal decode later, but that is not needed for this contract.

## 2. CLI, compatibility, and failure behavior

Add `stackSummary bool` to `recipeOptions` and register it with `fs.BoolVar` as `--stack-summary`, default false. Normal Go boolean-flag semantics apply: `--stack-summary=false` means the view was not requested and behaves as if absent; a non-boolean value is a flag-parse error. This avoids a new presence-tracking flag type for a view selector that only matters when true.

Validation occurs before `newAPIClient`:

| Invocation/option | Result |
|---|---|
| Community + true stack summary | Valid with existing sort/limit/json options |
| Default or explicit official + true stack summary | Usage error: stack summary requires `--source community` |
| Community stack summary + `--example`/`--examples` | Existing cross-source selector usage error |
| `civ recipe image` + true stack summary | Usage error; image detail remains independent |
| Invalid version, sort, or limit with stack summary | Existing usage error, zero requests/stdout |
| `--stack-summary=<non-boolean>` | `flag` parse error before run/network |
| False/absent stack summary | Existing behavior, including explicit official source |

`runRecipeCommunity` keeps one `ListImages` call and branches only after the body arrives. API/auth/network/decode failures retain shared exit mapping and stderr behavior; no new exit code or partial output exists. Help adds the new Usage line, option semantics, one JSON example, aggregate-only/redaction wording, top-5 bound, and non-random/descriptive caveats without duplicating global flags.

## 3. Versioned machine contract

### 3.1 Envelope

`CommunityStackSummary` has no `observations`, `examples`, `image`, `textParameters`, raw source, or generic issue/observation map.

| JSON field | Contract |
|---|---|
| `schemaVersion` | Always `"civ.recipe/v1"` |
| `kind` | Always `"communityStackSummary"` |
| `provenance` | Existing `RecipeProvenance`: `community-images`, `/api/v1/images`, requested version ID |
| `query` | Existing `CommunityQuery`: requested version, `image`, API sort enum, `AllTime`, limit |
| `coverage` | Existing `CommunityCoverage`, copied by value from `ParseCommunitySample` without changed definitions |
| `caveats` | Fixed ordered array: `["boundedNonRandomSample", "continuationNotFollowed", "descriptiveEvidenceOnly"]` |
| `topLimit` | Constant `5`; applies independently to every resource group and parameter distribution |
| `resources` | Aggregate resource coverage and three top-identity groups defined below |
| `parameters` | `steps`, `cfgScale`, `sampler`, and `scheduler` distributions |
| `textAvailability` | `promptStatus` and `negativePromptStatus` aggregate coverage objects; neither contains text |

Using the existing schema family with a new kind is additive and consistent with `communityRecipeSample` versus `communityImageDetail`. An incompatible change to this new contract requires a new schema identifier. All arrays are initialized to `[]`; no field uses `omitempty` to hide zero/empty states.

### 3.2 Resource coverage and usage rows

The `resources` object has this shape:

```text
resources
  entryCount                 all decodable object entries projected from both arrays, including duplicates/unidentified
  identifiedEntryCount       entries that yield a conservative identity in any group
  unidentifiedEntryCount     entryCount - identifiedEntryCount
  imagesWithEntries          distinct response positions with at least one projected resource entry
  checkpoints                ResourceGroup
  loras                      LoRAGroup
  otherResources             ResourceGroup for all other or missing types

ResourceGroup / LoRAGroup
  entryCount                 projected entries assigned to this group, including unidentified entries
  identifiedImageCount       distinct response positions contributing at least one identity in this group
  top                        []ResourceUsage or []LoRAUsage, maximum topLimit
  otherIdentityCount         unique identities outside top, not an image count

ResourceUsage
  identity                   ResourceIdentity
  imageCount                 distinct response positions for this exact identity

LoRAUsage
  identity                   ResourceIdentity
  imageCount                 distinct response positions for this exact identity
  weight                     WeightSummary

ResourceIdentity
  type                       canonical checkpoint/lora, exact other type, or null for untyped
  basis                      modelVersionId | hash | name
  modelVersionId             number only for that basis, otherwise null
  hash                       exact string only for that basis, otherwise null
  name                       exact string only for that basis, otherwise null

WeightSummary
  median                     number, or null when sampleCount is zero
  imagesWithWeight           distinct images for this identity with at least one valid weight
  sampleCount                retained distinct per-image weight values
```

Malformed/non-object resource elements already dropped by the tolerant projector cannot contribute entries or identity; they are not recast as zero-weight resources. A projected object with only a type and/or weight remains in `entryCount` and `unidentifiedEntryCount`, but not in `top`.

### 3.3 Resource identity and duplicate rules

For each `ObservedResource`, apply the following rules in order:

1. **Response-position unit:** the image key is `CommunityObservation.UpstreamPosition`, not nullable `imageId`. Two array elements are two sampled items even if upstream repeats an image ID; no cross-item semantic deduplication is attempted.
2. **Type category:** trim and case-fold only to recognize the exact reviewed tokens `checkpoint` and `lora`, then emit that canonical type. No aliases (`loCon`, `lycoris`, `embedding`, etc.) are mapped. Any other non-empty type goes to `otherResources` with its exact original string; absent/null/whitespace-only type goes there with `type: null`. Type is part of identity, so the same ID used under different types is not merged.
3. **Identity basis:** use `modelVersionId` only when present and greater than zero. Otherwise use the first non-empty `hash`, then non-empty `name`. `strings.TrimSpace` is only an emptiness test; the retained key/output string is exact, case-sensitive, and not trimmed. A non-positive ID does not suppress a valid hash/name fallback.
4. **No cross-basis merge:** keys are `(canonical/exact type, basis, typed value)`. An ID entry and hash-only entry remain separate even when names match. A hash entry with different display names remains one hash identity; equal names with different hashes remain different. Secondary labels are not emitted because choosing one among conflicting duplicates would be arbitrary.
5. **No inference:** top-level `modelVersionIds`, the requested version, origin array, resource weight, and prompt text never create identity or type.
6. **Per-image sets:** each image contributes at most one to `imageCount` for a key, regardless of duplicates across `civitaiResources` and `resources`. Group `identifiedImageCount` is also a set of positions, not a sum of row counts.

Sort each complete identity set by:

1. `imageCount` descending;
2. `type` ascending, with null before a string and exact bytewise string comparison (relevant to `otherResources`);
3. basis precedence `modelVersionId`, then `hash`, then `name`;
4. numeric ID ascending for ID identities, otherwise exact bytewise identity string ascending.

Emit the first five and set `otherIdentityCount` to the number remaining. Do not provide an aggregate image count for omitted resource identities: one image can use several omitted identities, so summing would overstate distinct images. The full sort makes ties and the selected top set deterministic rather than dependent on map order.

### 3.4 LoRA weight samples and median

Only identities in the canonical `lora` group receive `WeightSummary`; unknown/untyped resources with weights are not assumed to be LoRAs.

For every valid normalized resource weight, form `(imagePosition, LoRAIdentity, normalizedFloat64)`. Numeric/string forms that normalize to the same float are equal. Canonicalize signed zero to numeric zero for stable grouping/output. Then:

- collapse repeated equal weights within one image/identity;
- retain different valid weights within that image/identity as separate samples;
- `imagesWithWeight` counts positions with at least one retained sample;
- `sampleCount` counts retained samples, which may exceed `imagesWithWeight` only when different weights coexist in one image;
- missing/invalid weights contribute no sample and never become 1;
- do not reject finite negative or greater-than-one values without an evidenced range rule.

Sort retained float64 values ascending. For odd `sampleCount`, median is the middle value. For even `sampleCount`, median is the arithmetic mean of the two middle values, calculated with overflow-safe half-sum arithmetic. If `sampleCount == 0`, `median` is null and both coverage counts are zero. Outer `imageCount` is the denominator for weight-image coverage; human output displays `imagesWithWeight/imageCount` plus `sampleCount`.

### 3.5 Parameter distributions

Each member of `parameters` uses:

```text
steps | cfgScale | sampler | scheduler
  observedImages            valid normalized observations
  sampledImages             coverage.returned, including sparse/invalid items
  top                       []ValueCount, maximum topLimit
  other
    imageCount              observed images whose values are outside top
    distinctValueCount      unique values outside top

ValueCount
  value                     int64 for steps, float64 for cfgScale, string for sampler/scheduler
  imageCount                images with this exact normalized value
```

Read only the `origin == "meta"` observation with the matching name. Do not infer values from prompts, top-level dimensions, seeds, defaults, or other images.

- `steps` accepts the existing normalized int64, including observed zero or unusual negative values; no new semantic range is invented. Fractional/invalid integer inputs remain unobserved.
- `cfgScale` accepts finite normalized float64, including zero. Numeric `1`, string `"1"`, and numeric `1.0` coalesce; signed zero canonicalizes to zero.
- `sampler` and `scheduler` accept strings whose trimmed form is non-empty, but retain the exact original string as key/output. Empty and whitespace-only strings are unobserved. Case, surrounding whitespace, and semantic aliases remain distinct.
- Every sampled image has at most one value for each parameter, so distribution counts reconcile exactly: `sum(top.imageCount) + other.imageCount == observedImages`. `other.distinctValueCount` counts only omitted unique values.

Sort by descending `imageCount`, then numeric value ascending for steps/cfgScale or exact bytewise string ascending for sampler/scheduler. Emit five and aggregate the rest into `other`. Empty observed sets yield `top: []` and zero other counts; `sampledImages` still records the response denominator.

### 3.6 Text availability and zero denominators

`textAvailability.promptStatus` and `.negativePromptStatus` each contain:

| Field | Meaning |
|---|---|
| `provided` | Non-empty JSON string; whitespace-only is non-empty, matching `recipeTextValue` |
| `empty` | Exact `""` string |
| `notProvided` | Field absent/null, or metadata absent/null, or item provides no readable metadata object |
| `unsupportedJsonShape` | Field is non-string/non-null, or the containing metadata itself has an unsupported shape |
| `providedRate` | `provided / coverage.returned`; pointer null when returned is zero |

For a non-object item, both fields are `notProvided`. For non-object non-null metadata, both are `unsupportedJsonShape`. For a metadata object, call the existing `recipeTextValue` independently for each field, keep only its status, and discard the returned string immediately. Counts always satisfy `provided + empty + notProvided + unsupportedJsonShape == coverage.returned`. Rates are unrounded JSON numbers; human percentages use the existing integer coverage formatting and show `n/a` at zero denominator.

Using `promptStatus`/`negativePromptStatus` as aggregate member names preserves the existing summary vocabulary and lets exact-key redaction tests continue to forbid `prompt` and `negativePrompt` themselves.

### 3.7 Empty and partial outputs

For zero returned images:

- existing coverage remains `requestedLimit=<query limit>`, `returned/withMeta/usable=0`, and continuation reflects response metadata;
- resource entry/image/identity counts and parameter observed/other counts are zero;
- every `top` array is `[]` and `otherIdentityCount` is zero;
- no LoRA usage rows exist, so no artificial null-median row is created;
- both text status objects have four zero counts and `providedRate: null`;
- `sampledImages` is zero for every parameter.

Partial responses never replace missing values with defaults. Invalid numbers reduce observed/weight coverage while valid siblings still aggregate. Malformed envelope JSON or a non-array `items` value follows the existing error path and writes no stdout.

## 4. Public Go types and helper decomposition

The following exported types in `internal/api/community_stack.go` mirror the JSON blocks and carry focused GoDoc: `CommunityStackSummary`, `CommunityResourceSummary`, `CommunityResourceGroup`, `CommunityLoRAGroup`, `CommunityResourceUsage`, `CommunityLoRAUsage`, `CommunityResourceIdentity`, `CommunityWeightSummary`, `CommunityParameterSummary`, `CommunityValueDistribution`, `CommunityValueCount`, `CommunityOtherValues`, `CommunityTextAvailability`, and `CommunityTextStatusCoverage`.

Export `ParseCommunityStackSummary` beside `ParseCommunitySample`; its arguments and validation are identical (`raw`, positive `versionID`, bounded `limit`, approved API `sort`). Constants for kind/top limit/caveats remain unexported unless a real external consumer appears.

Unexported helpers should separate:

- status-only per-item text classification;
- canonical resource identity construction and type classification;
- per-image resource/weight accumulator sets;
- canonical scalar keys for numeric/string values;
- final sorting/top-5/other projection;
- median calculation.

The aggregator must sort only at final projection, not rely on upstream order or randomized map iteration. Its returned DTO copies `Provenance`, `Query`, and `Coverage` from the parsed sample but does not embed the sample or its observations.

## 5. Human output

Use a dedicated `renderCommunityStackSummary(io.Writer, *api.CommunityStackSummary) error` in `internal/cmd/recipe_stack.go`, reusing existing scalar/coverage formatting where it does not change semantics.

Section order:

1. `COMMUNITY STACK` with exact version/sort/period/limit and `SOURCE` provenance;
2. existing `COVERAGE` line, including `continuation yes (not followed)` when offered;
3. `CAVEATS` explaining bounded non-random descriptive evidence and no recommendation/reproducibility claim;
4. `PROMPT AVAILABILITY` and `NEGATIVE PROMPT AVAILABILITY`, with provided/returned percentage or `n/a` and all four status counts;
5. `CHECKPOINTS`, `LORAS`, and `OTHER RESOURCES`, each with entry/identified-image coverage, at most five identity rows, and omitted-identity count;
6. LoRA rows include median plus `imagesWithWeight/imageCount` and retained sample count, with `-` for a null median;
7. `PARAMETERS` with steps/cfgScale/sampler/scheduler observed/sampled coverage, top value rows, and other image/distinct-value counts;
8. explicit empty-group markers such as `(none observed)` instead of fabricated rows.

The renderer never receives raw JSON or observations. It prints no image IDs, users, source positions, resource-entry positions, prompts, raw metadata, workflows, or media/page locations. Stdout contains only this result; warnings/errors remain on stderr.

## 6. Compatibility, privacy, and operational behavior

- The default official recipe, default community observations, exact-image detail, and existing `civ.recipe/v1` kinds are unchanged. Existing tests are compatibility baselines, not tests to rewrite into aggregate expectations.
- The existing transient retry policy is unchanged; “one response” means one logical `ListImages` operation, not disabling client retries. No application-level second operation or alternate sort is allowed.
- Existing token/proxy/timeout/User-Agent/body-limit behavior remains. Debug logging may contain the existing bounded API URL but no new response values.
- No media URL is fetched or emitted, and no response or aggregate is persisted. Resource names/hashes are the only resource metadata labels emitted; they are selected identity values, not raw metadata dumps.
- Supporting-image IDs are deliberately not emitted. The archived future-aggregation note requested an evidence design; this approved aggregate-only contract supplies exact query/coverage and deterministic counts but excludes per-image evidence lists. Users can invoke the existing default community view separately if they need observations; this command never does so automatically.

## 7. Tradeoffs

- **Conservative identities versus complete deduplication:** ID-first/hash/name fallback avoids false merges but can split one real resource when upstream provides different identity evidence in different entries. That is safer than asserting unproven equivalence.
- **Distinct per-image weight values versus one arbitrary weight:** equal duplicates cannot skew a median, while different observed values remain visible through `sampleCount`. Selecting one value or manufacturing a per-image average would hide conflicting evidence.
- **Fixed top five versus exhaustive aggregates:** output remains compact and deterministic. Parameter `other` preserves exact count reconciliation; resources expose only omitted identity count because their image sets overlap.
- **Other-resource group versus silent loss:** unknown and missing types cannot be treated as LoRAs, but their usage should not disappear. A type-qualified `otherResources` group retains evidence without alias speculation.
- **Scheduler included:** it is already normalized in both recipe projections and is generation-relevant, so excluding it would discard available evidence without reducing requests or privacy exposure.
- **Local double decode versus stable-path refactor:** parsing the bounded body twice costs CPU but reuses the exact existing observation/coverage implementation and avoids a risky default-output refactor. No network cost is added.
- **No supporting-image lists versus audit detail:** aggregate-only output stays compact and satisfies the new view boundary; exact replay of the existing list command remains the optional human-driven audit path, not an implicit extra call.

## 8. Rollout and rollback

Implementation starts only after explicit approval of this final plan. Land the pure DTO/aggregator and tests first, then CLI wiring/help/human tests, then the parent-owned integration/docs/skill/spec/release-note updates and final full-scope check.

Rollback removes the opt-in flag branch, aggregate renderer, DTO/parser, and their tests. Existing list/official/detail paths need no migration or compatibility rollback because they are untouched and no state is persisted.
