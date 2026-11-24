# Recipe context command — technical design

> **Review status:** design is converged for implementation review. The official summary/detail CLI, exact-version identity, `--examples`/`--example` selectors, sibling-owned `recipe image` syntax, global list/detail redaction invariant, shared scalar normalization, summary budget (default 3, range 0–10), schema fields, and human presentation are approved as of 2026-11-24. Implementation still requires explicit approval.

## 1. Smallest behavior gap and design boundary

### Current behavior

The existing version endpoint already supplies official version identity, `trainedWords`, and example generation metadata in one response. The missing behavior is a **bounded, stable, prompt-free summary projection**, plus a **singular explicit-detail projection** for the rare case when full prompt text is required:

- `show -v --json` supplies broad explicit version detail (`internal/cmd/show.go:77-97`).
- `show -v --prompts --json` supplies a legacy prompt-bearing compatibility detail array but removes version context, provenance, source positions for skipped images, and stable missing-field semantics (`internal/cmd/prompts.go:12-64`).
- The bundled Agent workflow teaches consumers to combine those separate views (`internal/skill/assets/civ-prompt/SKILL.md:30-49`).

The existing explicit full/prompts paths remain compatibility detail views. They are not default list behavior and must not be used as the new recipe summary DTO.

### Approved output invariant

Every list/summary output, in either human or JSON form and at every count including one, omits prompt values, negativePrompt values, description HTML, raw meta, Comfy/workflow payloads, and similar large fields. Only an explicitly selected singular detail may return large text. `--examples 1`, an incidental single search result, or a future community sample of size one remains summary output and receives no large text.

### Approved behavior

Add an additive command with two views:

1. accept one exact version ID (approved identity boundary),
2. validate summary/detail selection locally,
3. call the existing `GetVersion` exactly once per invocation,
4. decode recipe-relevant source fields with presence-aware semantics,
5. for summary, return a versioned context envelope and deterministic bounded prompt-free example summaries,
6. for detail, return the same context plus exactly one explicitly selected example with normalized prompt and negativePrompt,
7. emit no filesystem effects and no generation/download behavior.

The normalization belongs at the **API response boundary**, where source presence and JSON scalar shapes are known. It must not be reconstructed independently in command rendering or in each Agent prompt. Summary and detail should use distinct DTOs so a summary has no prompt/raw-meta field capable of leaking source text during marshaling.

The command layer owns flags, orchestration, and CLI errors; the report package remains the JSON writer and source of shared output primitives.

### Explicit non-boundaries

- No change to existing `show`, `vet`, `sync`, or `--prompts` behavior.
- No prompt-bearing summary/list array and no implicit detail when count equals one.
- No raw-meta, description-HTML, or Comfy/workflow export in either recipe view.
- No new Civitai endpoint and no call to `/models/{id}` or `/images`.
- No resource resolution, comparison, community sampling, export, workflow execution, cache, or persistent library.
- No new package-level mutable global, third-party dependency, CLI framework, or speculative interface.
- No preemptive community source flag or unused community implementation branch.

## 2. Approved official CLI shape

The command forms, selector meanings, summary budget, and field registry below are approved.

```text
civ recipe <versionId> [--examples <count>] [--json]
civ recipe <versionId> --example <sourceIndex> [--json]
```

### D1 — Exact-version identity (approved)

`<versionId>` remains one required positive integer for both views. No model ID is required because standalone version detail already carries `modelId` and the model summary (`internal/api/types.go:59-78`, `testdata/FIXTURES.md:51-54`). Existing search/show/vet or the resource-resolver sibling obtains the ID. Query/name/hash/path and implicit-latest selection remain outside this child.

### D2 — Summary selection

- `--examples` selects the first N source examples, not the first N examples that happen to contain usable metadata.
- Approved default: 3. Approved accepted range: 0–10. Zero returns context and selection counts with `examples: []`.
- Every result is a summary, including N=1. No prompt or negativePrompt value is emitted.
- Human output is concise and prompt-free by default; Agents use global `--json` for the stable summary schema.

### D3 — Explicit singular detail

- `--example` selects one one-based source index and returns a singular detail object with prompt and negativePrompt for only that example.
- `--example` is the only recipe operation that unlocks large text. It is not implied by `--examples 1`.
- Explicitly supplying both `--example` and `--examples` is a usage error, even if their values identify the same example. Implementation must track whether `--examples` was explicitly set rather than confusing its default value with user input.
- Positive malformed IDs/indexes and count-range errors fail before network access. A well-formed but out-of-range source index is detected after the one version fetch and returns a normal selection error with no partial detail output.
- Human detail may print the selected prompt/negativePrompt; JSON detail uses the stable singular schema.

Offset/range pagination and additional detail selectors are deferred. The singular selector syntax and summary budget are approved.

## 3. Module responsibilities and expected change set

| Area | Proposed responsibility | Expected implementation-time files |
|---|---|---|
| Version transport | Reuse `Client.GetVersion`; no new HTTP behavior | `internal/api/versions.go` should remain unchanged |
| Recipe source decode and normalization | Own presence-aware narrow decoding, text statuses, scalar validation, provenance, selection counts, summary/detail DTOs, and issue construction | New `internal/api/recipe.go`; tests in `internal/api/recipe_test.go` |
| Legacy API types | Remain the source of existing show/vet compatibility detail contracts; no required changes | `internal/api/types.go` and existing tests should remain unchanged unless a small shared helper is provably behavior-preserving |
| Command orchestration | Parse/validate mutually exclusive selectors, call `newAPIClient(g)`, call `GetVersion`, invoke the summary or detail projector, validate returned ID consistency, render result | New `internal/cmd/recipe.go`; tests in `internal/cmd/recipe_test.go` |
| Rendering | Use `report.WriteIndent` for JSON and existing report primitives for prompt-free summary text and explicitly selected detail text; keep `report` free of API imports | Existing `internal/report`; a small primitive may be added only if genuinely reusable |
| Public command wiring | Add dispatch, overview row, curated/named help, completion spec and all four static scripts for both selectors | `main.go`, `internal/cmd/help.go`, `internal/cmd/completion.go`, `internal/cmd/completion_scripts.go`, related tests |
| User/Agent guidance | Document summary-first discovery and explicit singular detail without overclaiming capabilities | `README.md`, `internal/skill/assets/civ-prompt/SKILL.md`, `internal/skill/assets/civ-prompt/references/civ-commands.md`, and recipe-related reference text as needed |
| Drift canary | Add live summary and detail coverage while retaining legacy assertions | `scripts/integration-test.sh` |
| Fixture evidence | Reuse the verbatim official version fixture; use focused synthetic bodies for absence/null/drift matrices | Existing `testdata/version_3263843.json`; no fixture rewrite or new live fixture required for the MVP |

### Why a dedicated narrow decoder instead of changing `ImageMeta`

The preferred implementation is `api.ParseRecipe...`/`api.NewRecipe...` in one new API file, using private source structs with `json.RawMessage` or equivalent presence tracking for recipe fields. It should **not** require changes to the existing `ImageMeta` JSON contract.

This is intentional, bounded duplication of a small number of source field declarations:

- Existing `ImageMeta` uses value fields, so absent strings and explicitly empty strings cannot be distinguished (`internal/api/types.go:152-167`).
- Adding exported fields such as denoise or changing numeric decode behavior could alter legacy `show -v --json` output and error behavior.
- The recipe contract needs raw presence, null, invalid-number, unsupported-shape, and summary-only text-status states that the legacy decoder does not expose.
- A single recipe decoder keeps all raw field access in `internal/api`; no command or renderer reads untyped payload fields locally.
- Separate summary/detail output types prevent a raw source object or detail-only text field from being marshaled accidentally into a summary.

Reuse existing endpoint, client, error, numeric, and rendering helpers when their semantics match. Do not copy the legacy prompt projector and then mutate it into a second divergent contract.

## 4. Stable JSON contracts

### 4.1 Common envelope and view discriminator

Both views use `schemaVersion: "civ.recipe/v1"`, official provenance, the same subject shape, selection metadata, and top-level issues. The `kind` and result-member shapes differ deliberately:

| Key | Summary | Singular detail |
|---|---|---|
| `kind` | `"recipeSummary"` | `"recipeExampleDetail"` |
| `examples` | Array of prompt-free example summaries; always present, including `[]` | Absent |
| `example` | Absent | One detail object; present on success |
| `provenance` | Official API source and requested version | Same |
| `subject` | Model/version identity, base-model context, and `trainedWords` | Same |
| `selection` | First-N counts and truncation | Source-index selection and source counts |
| `issues` | Envelope/subject issues; always an array | Same |

Recommended provenance for both:

```json
{
  "source": "civitai.officialApi",
  "apiPath": "/api/v1/model-versions/3263843",
  "versionId": 3263843
}
```

`versionId` here is the validated requested identity, not a claim that a missing response ID was fabricated. `apiPath` identifies the logical first-party source without embedding test-server hosts or credentials.

“Official” means returned by the first-party Civitai API and attached to that version. It does **not** mean Civitai or the model author endorses the parameters, that the image is reproducible, or that the current user possesses every referenced resource.

### 4.2 Subject

Recommended shared shape:

| Path | Type | Missing behavior |
|---|---|---|
| `subject.model.id` | number or null | Null plus `notProvided` issue if top-level `modelId` is absent/null/invalid as an identity. |
| `subject.model.name` | string or null | Null plus issue; no `/models/{id}` fallback. |
| `subject.model.type` | string or null | Preserve unknown type strings verbatim; null plus issue if absent/null. |
| `subject.version.id` | number or null | Null plus issue if omitted; a conflicting nonzero returned ID is a command error, not silently rewritten. |
| `subject.version.name` | string or null | Preserve source text; no fallback to file name. |
| `subject.version.baseModel` | string or null | Preserve source text; no inference from model name or example metadata. |
| `subject.version.baseModelType` | string or null | Preserve source text; no default such as `Standard`. |
| `subject.version.publishedAt` | string or null | Preserve source timestamp; no date reformatting in JSON. |
| `subject.trainedWords` | array of strings | Always an array. Absent/null becomes `[]` plus a `notProvided` issue; explicit `[]` stays empty without that issue. Values remain in source order and are never invented. |

An explicitly empty `trainedWords` array means only that the source returned `[]`; it must not be interpreted as proof that no activation text is needed in every engine or workflow.

### 4.3 Prompt-free example summary

Each summary `examples[]` element should have:

| Key | Type | Contract |
|---|---|---|
| `sourceIndex` | number | One-based index in the original version `images` array. It is not renumbered after missing metadata or selection. |
| `provenance` | object | `source: "civitai.versionExample"`, `versionId`, and nullable `imageUrl`. The URL is evidence location only and is never fetched. |
| `metadataStatus` | string | `"available"` when a meta object was returned; `"notProvided"` when it was absent/null. Availability does not claim completeness or correctness. |
| `promptStatus` | string | Source text availability without content: `"provided"`, `"empty"`, `"notProvided"`, or `"unsupportedJsonShape"`. |
| `negativePromptStatus` | string | Same status vocabulary and semantics for negativePrompt. |
| `parameters` | object or null | Small parameters only; null when metadata was not provided. No prompt, negativePrompt, raw-meta, or workflow member exists in this DTO. |
| `issues` | array | Field-level source issues, including text availability problems, without embedding source text. |

Text-status rules:

- `provided` means a non-empty JSON string was present. Whitespace is not trimmed and therefore remains provided content.
- `empty` means exactly `""`; it is distinct from absence and is not an error.
- `notProvided` means absent/null, including when the entire meta object is absent/null.
- `unsupportedJsonShape` means the source value was a non-string, non-null shape. The summary reports status/issue only; selected detail returns null rather than coercing the value.
- The statuses describe source state; they do not indicate that a prompt was truncated or redacted. Redaction itself is not an issue and must not generate noisy warnings.

A missing-metadata example remains in a selected first-N window with `parameters: null`, both text statuses `notProvided`, and an issue. This preserves deterministic source positions without dropping the entry.

### 4.4 Small parameters shared by summary and detail

Approved initial whitelist:

| Key | JSON type | Source and normalization |
|---|---|---|
| `steps` | number or null | Direct `meta.steps`; validate as an integer, with accepted numeric-string drift normalized by one helper. No rounding or default. |
| `cfgScale` | number or null | Direct `meta.cfgScale`; preserve a valid JSON numeric value. |
| `sampler` | string or null | Direct `meta.sampler`; preserve unknown vocabulary verbatim. |
| `scheduler` | string or null | Direct `meta.scheduler`; preserve unknown vocabulary verbatim. |
| `seed` | string or null | Direct `meta.seed` as an exact decimal integer string, preventing JavaScript consumers from losing large integer precision. No arithmetic or range invention. |
| `width` | number or null | Direct `meta.width` only when it is a valid integer scalar. Objects/lists become null plus `unsupportedJsonShape`; never substitute outer image width. |
| `height` | number or null | Same rules as width. |
| `denoise` | number or null | Direct `meta.denoise` only; never parse the similarly named value inside `meta.comfy`. |

Rules shared by scalar fields:

- The approved shared normalizer converts valid numeric strings in known fields to JSON numbers; numeric `20` and string `"20"` are equivalent after projection. Seed remains an exact decimal string. This rule is shared with community recipe projections rather than implemented as a source-specific exception.

- Absent and explicit null produce null plus `notProvided`.
- A present empty string remains `""` for small string fields and does not produce a missing issue.
- Numeric strings accepted by the reviewed normalization rules become their defined JSON representation; invalid numeric text produces null plus `invalidNumber`.
- Integer fields reject fractions rather than rounding. Non-finite values cannot appear in JSON output.
- Objects, arrays, booleans, or other incompatible shapes produce null plus `unsupportedJsonShape` for scalar fields instead of failing the entire version when a usable context can still be returned.
- Malformed top-level JSON, an incompatible fundamental container (for example `images` not being an array), or a conflicting returned version ID remains a command error.
- Unknown source fields are ignored. In particular, `comfy`, `models`, `resources`, `vaes`, `engine`, and `Model` are not dumped or promoted into unversioned recipe keys.

### 4.5 Singular example detail

Detail's top-level `example` contains every field of the prompt-free summary plus one additional member:

| Key | Type | Contract |
|---|---|---|
| `textParameters` | object or null | Null when meta was absent/null. Otherwise an object containing exactly the reviewed large text fields `prompt` and `negativePrompt`, each string or null. |

Detail text rules:

- `textParameters.prompt` is direct `meta.prompt`, verbatim including multiline text and explicit empty string.
- `textParameters.negativePrompt` is direct `meta.negativePrompt`, verbatim including explicit empty string.
- Absent/null text becomes null plus `notProvided`; explicit `""` remains empty without a missing issue; incompatible shapes become null plus `unsupportedJsonShape`.
- The summary's `promptStatus` and `negativePromptStatus` remain present in detail and must agree with these values.
- Only the selected source index is projected. There is no `examples` array and no text or raw source object from nonselected examples.
- Description HTML, raw meta, Comfy/workflow, unknown metadata bags, and other large source blobs remain excluded even in detail. Legacy explicit full/prompts commands retain their own separate compatibility contracts.

The detail DTO should reuse an embedded summary struct plus `textParameters`, while the summary DTO has no text-parameter member at all. Redaction must not depend on setting prompt fields to empty strings or on `omitempty`.

### 4.6 Selection

Recommended shared selection object:

| Key | Type | Summary behavior | Detail behavior |
|---|---|---|---|
| `mode` | string | `"first"` | `"sourceIndex"` |
| `requested` | number | Approved N; zero is meaningful | Requested one-based source index |
| `returned` | number | Length of `examples` | 1 on successful detail |
| `total` | number or null | Total source images when provided; null when absent/null | Same |
| `withMetadata` | number or null | Count across the full provided source array, not only the returned window | Same |
| `withoutMetadata` | number or null | Same full-source scope | Same |
| `truncated` | boolean or null | Whether source entries were omitted by the summary limit; null when total is unknown | Null: not applicable to singular selection |

Required edge behavior:

- Explicit `images: []`: summary returns `examples: []`, total/with/without = 0, `truncated: false`, and no not-provided issue for images. Detail returns a selection error.
- Absent/null `images`: summary returns `examples: []`, unknown-count fields null, an envelope issue for images, and no claim that the version definitively has zero examples. Detail returns a selection/unavailable error with no partial detail.
- Summary `requested: 0`: still decode/count the provided source, return no examples, and calculate truncation normally.
- Summary N greater than total: return all source entries and `truncated: false`.
- Detail may select an entry outside the default summary window; no summary call is required by the CLI itself to validate the index.
- Null meta counts as `withoutMetadata` even if `hasMeta` is true.

The summary count limit is now independent of prompt length because prompt values are never projected. Small source strings and provenance URLs can still vary in size. Detail varies only with the selected prompt/negativePrompt lengths and must not grow when nonselected large fields change without changing full-source metadata counts. A hard byte/token cap would require separate explicit truncation semantics and is not proposed.

### 4.7 Issues

Recommended compact issue shape:

```json
{
  "field": "parameters.width",
  "reason": "unsupportedJsonShape"
}
```

- Envelope/subject issues live in top-level `issues`; example issues live in the owning summary/detail example.
- `field` uses a stable dotted path relative to the owning object. Detail-only text issues use `textParameters.prompt` or `textParameters.negativePrompt`.
- Initial reason vocabulary is `notProvided`, `invalidNumber`, and `unsupportedJsonShape`.
- Summary text issues may name `prompt` or `negativePrompt` and use the status fields, but must never include the source value, snippet, length-derived preview, or raw object.
- Omission of large text from summary is a view rule, not a source-data issue. Issues are emitted only for actual source absence/shape problems.
- Issues describe source availability/shape, not operational warnings. Normal missing metadata and view redaction do not need noisy stderr warnings when represented in stdout.
- An empty issue list serializes as `[]`, never null.

## 5. Data flow and error flow

```text
argv
  -> main.go dispatch
  -> cmd recipe FlagSet + global flags
  -> local positive-ID and mutually-exclusive selector validation
  -> newAPIClient(g)
  -> Client.GetVersion(ctx, requestedID)             [one HTTP request]
  -> api recipe-only tolerant source decode
       -> source presence/null/shape tracking
       -> returned-ID consistency check
       -> full-source metadata counts
       -> branch on explicit view:
            summary: first-N source window
                     -> statuses + small parameters + issues only
            detail: exact source-index selection
                     -> summary fields + selected textParameters only
  -> report.WriteIndent (JSON) or concise cmd/report text rendering
  -> stdout
```

Error flow:

- Invalid CLI input, malformed selector, or explicitly combined summary/detail selectors: return a normal error before making a request; shared runner maps to exit 1.
- Well-formed detail index not present in the fetched source: return a selection error after the one request, with empty stdout and exit 1; do not return summary data as a fallback.
- API 404, network exhaustion, anonymous/token 401, and other API failures: preserve existing typed errors and shared 1/2/4 mapping; recipe does not use vet's exit 3.
- Malformed fundamental response or nonzero returned-ID mismatch: return a wrapped recipe/API decode error; stdout remains empty and diagnostics go to stderr.
- Missing optional source fields: return successful summary/detail with statuses, nulls, and issues; do not turn absence into a network-style or not-found failure.
- No retry or fallback to model detail, by-hash, images, or community sources. The existing `Client.Get` retry policy remains the only retry behavior.

## 6. Compatibility and future extension

### Existing commands

The new command should share transport and rendering primitives only. Existing prompt structs, prompt rendering, version types, vet model fallback, and exit overrides remain untouched. Explicit legacy full/prompts invocations remain compatibility detail and may retain their current large fields; they do not authorize large fields in new recipe summaries.

Legacy tests are compatibility tests, not candidates for expected-output rewrites. If implementation discovers that a shared decoder change is unavoidable, stop at the review gate and demonstrate before/after legacy JSON equivalence. The preferred dedicated recipe decoder makes such a change unnecessary.

### Schema evolution

Within `civ.recipe/v1`:

- Do not rename/remove keys, change a scalar's type, change first-N ordering, change source-index meaning, or change issue meaning.
- Never add prompt, negativePrompt, raw meta, description HTML, Comfy/workflow, or equivalent large content to a summary. This is an approved invariant, not merely an additive-schema question.
- Add only reviewed optional summary information that is small and source-backed. Large text additions belong only in singular detail and require contract review.
- Unknown upstream values remain strings where the contract says string; new provenance `source` values may be introduced only with documentation telling consumers not to assume every example is a version example.
- A breaking field/type/selection change requires a new schema identifier and an explicit compatibility plan; a schema bump cannot be used to violate the global summary redaction invariant.

### Community sibling

`.trellis/tasks/11-24-community-recipes` can later construct the same prompt-free `examples[]` summary element and singular detail `example` while supplying a different per-example provenance source and reviewed sampling mode/counts. The reusable seam is pure metadata/scalar/text-status normalization inside `internal/api`, not a speculative public interface or community flag added now.

The parent integration review should verify that community data:

- retains source identity and bounded-selection semantics,
- does not relabel community samples as official version examples,
- emits no prompt/negativePrompt/raw meta/Comfy/workflow content in any list/sample, including a sample of one,
- returns large text only through the same explicit singular-detail shape,
- does not bypass missing-data rules or fabricate resource/version linkage,
- does not require changing the official recipe schema version merely to add a new provenance value.

No community retrieval or sampling behavior is implemented by this design.

## 7. Trade-offs

| Decision | Benefit | Cost / rejected alternative |
|---|---|---|
| D1 exact version ID only | One deterministic request per invocation; no hidden latest-version policy | Less convenient than names/hashes; broader input can be added only after resolver semantics are reviewed |
| Prompt-free summary plus singular detail | Enforces the approved token/redaction invariant while retaining access to exact text when needed | A full prompt may require a second CLI invocation and source fetch; no cache is added to hide that cost |
| Separate `--example` selector | Makes detail intent explicit and stable across summary limits | Requires mutual-exclusion and out-of-range semantics; count `1` cannot double as detail |
| Distinct summary/detail DTOs | Makes prompt leakage unrepresentable rather than relying on renderer discipline | More types/tests than one flexible map; justified by the global invariant |
| Text availability statuses in summary | Lets Agents choose detail and distinguishes absent from explicitly empty without exposing text | Adds two small fields and status tests per example |
| Small parameters in summary | Provides useful recipe context without another call | Slightly larger summaries than provenance-only rows; fields remain whitelisted |
| Dedicated narrow recipe decoder | Presence-aware contract with no legacy `ImageMeta` output drift | Repeats a limited set of field declarations; mitigated by one owner and focused shared scalar helpers |
| Bounded first-N summary | Predictable orchestration/token use and simple deterministic tests | Later examples require larger N or direct source-index detail; pagination deferred |
| Retain missing examples in summary window | Stable source positions and explicit absence | Slightly more output than dropping them and may return fewer immediately usable small-parameter sets |
| Required nullable keys plus issues | Consumers can rely on shape and distinguish explicit empty from absent | More verbose than `omitempty`; strict issue vocabulary and tests control the cost |
| Decimal-string seed | Exact across JavaScript and other IEEE-754 consumers | Less idiomatic than a JSON number; justified because seed corruption changes reproducibility evidence |
| No model-detail fallback | One-request latency/failure profile and smaller payload | Model name/type may be missing if the version summary drifts; represented explicitly instead of hidden |
| New command instead of changing `--prompts` | Additive rollout and zero legacy migration | Two recipe-like detail surfaces coexist; docs must distinguish legacy compatibility detail from summary-first recipe flow |
| No raw extension bag in detail | Stable compact schema and no accidental Comfy/token explosion | Less commonly used large fields remain available only through existing explicit legacy detail, not recipe |
| Human summary/detail plus opt-in JSON | Matches existing CLI conventions | Two views and two formats to test; human summary remains deliberately thin and prompt-free |

## 8. Rollout and rollback shape

### Rollout

1. Review the approved summary/detail v1 contracts with the parent task and incorporate any implementation-discovery corrections without reopening settled product decisions.
2. Land pure recipe decoding/normalization and both contract test sets without changing dispatch. Prove summary redaction before implementing any renderer.
3. Add command orchestration and prove one-request, selector, bounds, error, detail-fidelity, and legacy behavior offline.
4. Wire help/completion/docs and update the bundled Agent workflow only after both output contracts pass review.
5. Add live summary/detail integration coverage and run the full project quality gates. Live integration remains the upstream-drift canary, not a substitute for offline tests.

The new command is itself the opt-in rollout boundary; no configuration feature flag or persisted migration is needed.

### Rollback

- Before public wiring, discard the isolated API recipe files/tests with no user-facing effect.
- After public wiring, remove the `recipe` dispatch/overview/help/completion/docs entries and new command/API files as one coherent revert. Do not leave a discoverable command with disabled behavior.
- If summary redaction fails, stop rollout immediately; do not ship a prompt-bearing summary temporarily and do not address it by truncation. Remove the public wiring while retaining only isolated tests for diagnosis.
- If a shared legacy type/helper was changed despite the preferred boundary, revert that change first and rerun all legacy show/vet tests before attempting another approach.
- No data rollback, cache invalidation, config migration, or library cleanup is required because the command persists nothing.
- Community work is independent; rolling back this child must not leave community code depending on an unreviewed recipe schema or redaction exception. Parent integration must check this before the sibling lands.
