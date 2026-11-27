# Implementation plan: community recipe stack summary

Status: **Execution plan converged for final implementation review; no task start or product edit is authorized by this artifact.**

## 1. Scope and ownership

### Child-owned implementation files

Planned product/test changes, after explicit approval and `task.py start` by the main workflow:

- Modify `internal/cmd/recipe.go` for the boolean flag, pre-network validation, post-fetch branch, and curated help.
- Add `internal/api/community_stack.go` for aggregate-only DTOs and pure parsing/aggregation.
- Add `internal/cmd/recipe_stack.go` for human rendering.
- Add `internal/api/community_stack_test.go` and `internal/cmd/recipe_stack_test.go`; make only minimal compatibility-signature updates to existing recipe tests/help assertions.
- Reuse existing helpers in `internal/api/community.go`, `internal/api/images.go`, `internal/api/recipe.go`, and the current test packages. Do not change their default output contracts.

No live `/images` fixture is required. Extend the existing inline synthetic-body style so every drift/duplicate/tie case is precise and offline. Do not alter verbatim fixtures or `testdata/FIXTURES.md` unless a separate fixture review deliberately adds one.

### Implementation deviation record (2026-11-27)

During implementation, registering the `--stack-summary` flag tripped the
repository's completion drift guard (`TestCompletionSpec_CoversRegisteredFlags`),
whose failure message instructs updating the completion spec and all four
embedded shell scripts. The implementer therefore also modified
`internal/cmd/completion.go` and `internal/cmd/completion_scripts.go` — two
files outside the original ownership list above, owned by no sibling. The
change is the minimal mechanical completion of the owned flag addition (one
`{"stack-summary", nil}` spec entry plus one line per shell script) and was
verified green by the final full-scope check. Recorded here so the ownership
list and the delivered diff reconcile.

### Parent-integrator-owned shared files

The child implementer must not edit these. Section 5 is the mandatory handoff for the parent:

- `scripts/integration-test.sh`
- `README.md`
- `internal/skill/assets/civ-prompt/SKILL.md`
- `internal/skill/assets/civ-prompt/references/civ-commands.md`
- `.trellis/spec/backend/design-principles.md`
- `.trellis/spec/backend/directory-structure.md`
- `CHANGELOG.md`

The parent should audit `internal/skill/assets/civ-prompt/references/prompt-patterns.md` and `.trellis/spec/backend/quality-guidelines.md`; update them only if the final integration reveals guidance that is not already covered. No speculative shared-spec rewrite is part of the child.

## 2. Ordered implementation checklist

### Gate 0 — Review before execution

- [ ] Present the final PRD/design/plan summary. Do not run `task.py start` until the user explicitly approves that latest summary in a subsequent message.
- [ ] Confirm both JSONL manifests contain only real repo-relative spec/research entries and that the task remains the only active implementation target.
- [ ] Snapshot unrelated dirty paths; do not include parent/sibling task directories or user changes in child work.

### Step 1 — Pure aggregate contract in `internal/api`

- [ ] Add the versioned DTOs from `design.md` with GoDoc on every exported type and `ParseCommunityStackSummary`. Initialize every slice and preserve nullable median/rate states.
- [ ] Implement `ParseCommunityStackSummary(raw, versionID, limit, sort)` by calling `ParseCommunitySample` first, copying its provenance/query/coverage, and returning its validation/envelope errors unchanged.
- [ ] Decode the same body locally only for status-only prompt/negative-prompt classification. Reuse `decodeImagesEnvelope`, `decodeImagesItems`, `decodeCommunityMeta`, `communityMetadataStatus`, and `recipeTextValue`; return only statuses and discard text immediately.
- [ ] Aggregate observations by `UpstreamPosition`. Implement canonical type/identity selection, per-image identity sets, resource entry coverage, and type-qualified `otherResources` without reading top-level `modelVersionIds` for stack membership.
- [ ] Implement per-image distinct LoRA weight samples, coverage, deterministic median, normalized scalar parameter keys, exact sampler/scheduler values, final total-order sorts, fixed top 5, and other counts.
- [ ] Keep the returned DTO structurally free of observations, raw source, prompt/negative strings, workflows, media/page URLs, continuation tokens/cursors/next-page URLs, and per-image issue/identity lists. Preserve the existing boolean `coverage.continuationOffered` fact.
- [ ] Use only the Go standard library and keep all policy out of `cmd`/renderers.

### Step 2 — API unit scenarios in `internal/api/community_stack_test.go`

Reuse `communityOneItem`, `communitySixItemBody`, `marshalRecipe`, `assertNoJSONKeys`, `assertNoSourceText`, and related same-package helpers. Add focused helpers only for building stack bodies and finding aggregate rows.

| Test | Exact scenarios and assertions |
|---|---|
| `TestParseCommunityStackSummary_ContractAndCoverage` | Parse the six-item body with continuation; assert schema/kind/caveats/topLimit, exact provenance/query, and coverage deeply equal to `ParseCommunitySample`. Assert no observations member and non-nil top arrays. |
| `TestCommunityStackResources_DuplicatesUseDistinctImages` | Two items use checkpoint ID 777; one repeats it within and across both arrays. Assert `imageCount=2`, entry counts still expose duplicates, group identified-image count is 2, and raw entry count is not used as usage. Include repeated image IDs with different response positions and missing image IDs to prove positions are the unit. |
| `TestCommunityStackResources_IdentityPrecedenceAndNoCrossBasisMerge` | Table positive ID > hash > name; zero/negative ID falls back to hash/name; same ID with conflicting secondary labels merges; same hash with different names merges; same name with different hashes does not; ID-based and hash/name-only entries with matching labels remain separate. Assert only the selected basis field is non-null. |
| `TestCommunityStackResources_NoInferenceAndUnidentifiedCoverage` | Top-level `modelVersionIds`, requested version, origin, and weight-only entries create no identity. Empty/null IDs/names/hashes increment unidentified counts without a fake row. Null/non-object/malformed resource entries do not abort valid siblings. |
| `TestCommunityStackResources_TypeGroups` | Canonical checkpoint/lora tokens group correctly; `loCon`/`lycoris`/custom types are not aliases; unknown types retain exact type-qualified identities; absent/whitespace type yields `type:null`; unknown/untyped weights get no LoRA median. |
| `TestCommunityStackResources_DeterministicTopAndOther` | Construct at least seven identities with tied image counts in unsorted source order. Assert count descending, then type/basis/value tie rules, exactly five rows, correct `otherIdentityCount`, no omitted image-count sum, and byte-identical repeated marshaling. |
| `TestCommunityStackLoRAWeights_ValidSamplesCoverageAndMedian` | AC-5 body: three images, repeated numeric/string 0.5 duplicates, 1.0, missing/null/empty/object/`NaN`/`Inf` weights. Assert 3 usage images, 2 weight images, 2 samples, median 0.75. Add an all-invalid identity with null median/zero coverage, one image with different weights 0.4/0.8 yielding 2 samples and median 0.6, odd-count median, observed zero, and finite negative/>1 values without clamping/defaulting. |
| `TestCommunityStackParameters_ObservedDenominatorsAndNormalization` | Mix numeric/string/integral-float steps, fractional/empty/object/bool/`NaN` steps, zero/negative values, cfg numeric/string/1.0/signed-zero/invalid values, and exact `Euler`, `euler`, `Euler `, empty/whitespace sampler/scheduler. Assert observed versus returned, numeric coalescing, no alias folding, and valid siblings surviving drift. |
| `TestCommunityStackParameters_DeterministicTopAndOther` | More than five tied values for each numeric and string distribution. Assert numeric/lexical tie order, top length 5, `other.imageCount`, `other.distinctValueCount`, and `sum(top)+other==observedImages`; zero observed values produce empty top and zero other counts. |
| `TestCommunityStackTextAvailability_StatusCountsAndRates` | Include non-empty, whitespace-only, exact empty, absent, null, number/object/bool fields; absent/null/object/wrong-shape metadata; and a non-object item. Assert each field’s four counts sum to returned, provided rates are exact, whitespace is provided, and unsupported metadata makes both statuses unsupported. |
| `TestCommunityStackTextAvailability_ZeroDenominator` | Empty, absent, and null items all produce four zero counts and nil rates; human-facing `n/a` is tested in cmd. |
| `TestCommunityStackSummary_EmptyPartialAndContinuation` | Empty/absent/null items plus empty-with-cursor; sparse items with null/unsupported metadata and one valid sibling. Assert all zero/empty/null states, no artificial median rows, sampled denominators, continuation true, and no cursor/page value. |
| `TestCommunityStackSummary_ValidationAndMalformedEnvelope` | Table zero/negative version, limits 0/-1/101, unsupported sort, non-JSON/null/array root, truncated body, and object/string items. Assert errors rather than clamped/fake success. |
| `TestCommunityStackSummary_RedactionAndDeterminism` | Unique sentinels in prompt/negative/unknown metadata/workflow/ComfyUI/media/thumbnail/BlurHash/cursor/nextPage plus image/post/user values. Recursively forbid observation/detail/raw/media keys and exact `prompt`/`negativePrompt`; assert no sentinel values, recommendation/quality/stability claims, or supporting-image arrays. Repeat parse/marshal and compare bytes. |

### Step 3 — CLI wiring and human output

- [ ] Register `--stack-summary` with standard boolean semantics and update `recipeHelp` Usage/Options/Examples/Notes. False behaves as absent; true is accepted only for community version sampling.
- [ ] Add all true-flag combination checks before client construction. Preserve the existing validation order/messages for old inputs and shared exit mapping.
- [ ] After the existing one `ListImages` call, select `ParseCommunityStackSummary` only when true; otherwise execute the untouched list path. No extra request or alternate fallback is permitted.
- [ ] Add the aggregate human renderer in `recipe_stack.go` with the section order and zero/`n/a` behavior in `design.md`. Reuse `communityCoverageText`, numeric formatting, and stdout writer conventions; do not pass raw bodies or observations to it.
- [ ] Keep `--json` on `report.WriteIndent` and all diagnostics on stderr. No persistence or new log statements containing response values.

### Step 4 — Command unit scenarios in `internal/cmd/recipe_stack_test.go`

Reuse `newRecipeTestEnv`, `runRecipeCmd`, `assertRecipeExactQuery`, request counters, and redaction helpers.

| Test | Exact scenarios and assertions |
|---|---|
| `TestRecipe_CommunityStackSummaryRequestJSON` | Default invocation against a sentinel mixed body: exactly one `/images` request, no version/detail/media path, exact six query keys (default 20/Most Reactions/AllTime), versioned aggregate JSON, copied coverage, no observations or large-field/media/source sentinels. |
| `TestRecipe_CommunityStackSummaryOptionMatrix` | Default, Newest, API spelling Most Reactions, limit 1, and limit 100. Assert exact outgoing sort/limit and one request per case; default no-flag community tests remain unchanged. |
| `TestRecipe_CommunityStackSummaryHuman` | Assert query/source/coverage/caveats, text rates, resource identity rows and weight coverage/median, parameter observed denominators/top/other, deterministic section order, empty-group wording, and no per-image IDs/users/positions/prompt/workflow/media text. |
| `TestRecipe_CommunityStackSummaryUsageErrorsBeforeNetwork` | Table default official + flag, explicit official + flag, community + examples, community + example, image + flag, invalid/zero/negative/overflow version, unsupported/empty sort, and limits 0/-1/101. Every run-level case: error, exit class 1, zero requests, zero stdout. |
| `TestRecipe_CommunityStackSummaryMalformedFlagValues` | Direct `FlagSet` parse for `--stack-summary=not-bool`, non-integer/overflow limit, and leading-dash positional; assert parse failure before run/network. Also parse `--stack-summary=false` and prove the existing list path is selected. |
| `TestRecipe_CommunityStackSummaryEmptyContinuation` | Empty body with nextCursor/nextPage in human and JSON: one request, zero counts/top arrays, null JSON rates, human `n/a`, continuation offered-not-followed, no token/URL or follow-up request. |
| `TestRecipe_CommunityStackSummaryErrorMapping` | 404 anonymous/token 401 mapping as applicable, malformed JSON, and non-array items. Assert existing exit classes, one non-retried deterministic request where applicable, and empty stdout with no aggregate fallback. |
| `TestRecipe_CommunityStackSummaryHelpAndCompatibility` | Assert help contains the new Usage/option/JSON example, top-5/aggregate-only/prompt-free/non-random notes, and no duplicated global flags. Run existing official/community/image tests unchanged to prove absent-flag compatibility. |

## 5. Mandatory parent-integrator handoff

### 5.1 `scripts/integration-test.sh`

Add these exact runs near the existing recipe cases:

```bash
run recipe_community_stack \
  recipe 3263843 --source community --stack-summary --limit 5 --json
run recipe_community_stack_newest \
  recipe 3263843 --source community --stack-summary --sort newest --limit 5 --json
run recipe_stack_official \
  recipe 3263843 --stack-summary
run recipe_stack_bad_limit \
  recipe 3263843 --source community --stack-summary --limit 0
run recipe_stack_selector_conflict \
  recipe 3263843 --source community --stack-summary --example 1
```

Add/extend assertions to verify:

- Both successful runs exit 0 and decode to an object with `schemaVersion == "civ.recipe/v1"`, `kind == "communityStackSummary"`, `topLimit == 5`, the exact caveat array, and no `observations`, `examples`, `image`, or `textParameters` member.
- Default query is version 3263843/type image/limit 5/sort Most Reactions/period AllTime; Newest differs only in sort. Provenance is `community-images`, `/api/v1/images`, version 3263843. Coverage contains all five existing keys and `requestedLimit == 5`; continuation is boolean and never followed by script design.
- Resources contain checkpoints/loras/otherResources; every top array has length 0-5, identity basis is one of modelVersionId/hash/name, counts are non-negative, and no resource row contains weight coverage outside the LoRA group.
- Parameters contain exactly the approved steps/cfgScale/sampler/scheduler distributions; each `sampledImages == coverage.returned`, top length is at most 5, and top image counts plus `other.imageCount == observedImages`. Do not require any particular value or non-empty metadata from the live service.
- Each text status object’s four counts sum to returned. `providedRate` is null when returned is zero; otherwise it equals provided/returned within a small numeric tolerance. Do not assert prompt text or a specific availability rate.
- For every emitted LoRA row, `sampleCount == 0` implies `median === null`; `sampleCount > 0` implies a finite numeric median and `0 <= imagesWithWeight <= imageCount`.
- Recursively reject exact prompt/negativePrompt/raw meta/workflow/ComfyUI/media keys and any cursor, continuation-token, or nextPage key/value, while preserving the boolean `coverage.continuationOffered`; ensure no per-image observation shape appears. Live output need not contain sentinels to prove type/key absence.
- `recipe_stack_official`, `recipe_stack_bad_limit`, and `recipe_stack_selector_conflict` each exit 1 with empty stdout. Unit tests, not the live script, prove zero requests.
- Add `--stack-summary` to the recipe `help_sigs` alternatives. Keep existing `recipe_community` assertions on `communityRecipeSample` and observations unchanged as the compatibility control.

Run the parent’s online integration only through the existing target/preconditions; tolerate empty/sparse/visibility-limited responses and never retry another sort to make assertions pass.

### 5.2 `README.md`

Update the recipe examples, flag table, community narrative, and schema paragraph together:

- Document the exact new command, community-only validation, human/JSON behavior, unchanged one-response query, `communityStackSummary` kind, fixed top 5, and aggregate-only/no-observations boundary.
- Replace the current unqualified “no aggregation” statement at `README.md:323-328` with “default community output is per-image; `--stack-summary` explicitly selects descriptive aggregates.”
- Define distinct-image resource counts, ID/hash/name precedence and no cross-basis merge, unidentified/other-type treatment, LoRA valid-weight median and duplicate rule/coverage, four parameter denominators/top-plus-other, and text status/rate zero-denominator behavior.
- State prompt/raw/workflow/media omission and bounded non-random/visibility-biased/descriptive-only caveats. Do not call a top value or median recommended, stable, best, or reproducible.

### 5.3 Bundled skill

- `internal/skill/assets/civ-prompt/SKILL.md`: add optional use of the aggregate for reviewing bounded community evidence, point agents to `--json`, and require preserving coverage/caveats. Do not turn top values/medians into prompt recommendations or implied defaults.
- `internal/skill/assets/civ-prompt/references/civ-commands.md`: add command syntax and the new kind/major fields. Scope the current “per-image only/no aggregation” sentence at lines 67-70 to the default view and document the aggregate-only alternative, identity/weight/parameter/text coverage semantics, and no extra requests.
- Audit `references/prompt-patterns.md`: if it references community aggregation after the above updates, explicitly say observed frequencies/medians are evidence to inspect, not values to adopt automatically. Avoid unrelated prompt-guidance changes.

### 5.4 Backend specs and release note

- `.trellis/spec/backend/design-principles.md`: after the final code contract is verified, add a concise machine-aggregate rule beside the schema recipe: aggregate-only DTOs must exclude per-image/raw/text/media fields; observed denominators, deterministic bounded top-plus-other, conservative identity/deduplication, and zero-denominator null states are public contracts.
- `.trellis/spec/backend/directory-structure.md`: add `api/community_stack.go` and the command/test file placement to the actual layout if those files land as planned. Keep API pure projection and cmd rendering responsibilities distinct.
- Review `.trellis/spec/backend/quality-guidelines.md` under the normal spec-update step; only add a general lesson if implementation exposes one not already covered by expected-failure, offline-test, and integration-parity rules.
- `CHANGELOG.md`: have the parent add the feature to the appropriate upcoming release’s `Added` section without inventing or altering the machine-managed version heading. Mention the opt-in flag, one-response aggregate-only view, new kind, prompt-free output, and bounded descriptive semantics.

## 6. Validation commands and review gates

Run only after implementation begins; none of these product commands are part of this planning turn.

```bash
# Focused formatting and offline tests
gofmt -w internal/api/community_stack.go internal/api/community_stack_test.go \
  internal/cmd/recipe.go internal/cmd/recipe_stack.go internal/cmd/recipe_stack_test.go
go test ./internal/api ./internal/cmd -run 'CommunityStack|Recipe_CommunityStack'

# Required full project gates
go vet ./...
go test ./...
go test -race ./...
go list -m all
git diff --check

# Parent integrator, online/manual only
make integration
```

Expected `go list -m all` output is only `civitai-cli`. Before handoff, inspect the complete diff for:

- exactly one existing `ListImages` call in either community view and zero requests on all run-level validation failures;
- unchanged default/official/image output and error classes;
- deterministic sorting independent of maps, correct top/other reconciliation, no overlap-summed resource other count;
- null/zero/empty behavior, valid-weight-only medians, and no default weights/parameters;
- recursive human/JSON redaction and no observations/supporting-image lists;
- GoDoc on every new exported symbol, all-English output/comments, no dependency or persistence;
- integration/README/skill/spec/release-note parity completed by the parent rather than silently skipped.

## 7. Rollback points

1. **Pure aggregator defect:** revert `internal/api/community_stack.go` and its test while leaving CLI unwired; no existing behavior changes.
2. **CLI/rendering defect:** revert the flag branch/help/renderer and cmd tests; the pure aggregator can remain unused temporarily or be removed with point 1.
3. **Shared integration/docs mismatch:** parent reverts only its shared-file hunk for this feature; no data migration, cache invalidation, schema conversion, or state cleanup exists.
