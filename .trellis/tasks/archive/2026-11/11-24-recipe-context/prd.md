# Recipe context command

> **Review status:** requirements are converged. Exact-version identity, summary/detail syntax, sibling-owned community detail syntax, prompt-free list invariant, shared scalar normalization, summary budget, schema fields, and human/JSON presentation are decided as of 2026-11-24. Implementation still requires the parent planning review and fresh explicit approval; this document does not authorize `task.py start`.

## Goal and user value

Add a read-only `recipe` capability that returns compact, stable, Agent-oriented context for one Civitai model version, a bounded set of **prompt-free official example summaries**, and an explicitly selected **single-example detail** when full prompt text is actually needed.

The capability should let an Agent obtain version identity, base-model context, exact `trainedWords`, provenance, text availability, and small generation parameters in one predictable summary invocation and one version-detail API request. A second explicit detail invocation should return the complete prompt and negative prompt for only one source example. Today, the bundled Agent guidance either combines separate full-context and `--prompts` calls or requires the Agent to consume a much larger full-version payload. A normalized summary/detail flow should reduce command orchestration, default response size, and token use without hiding provenance or inventing missing data.

The output is evidence for prompt construction, not an executable generation request. It must not claim that `civ` can generate images, download models/images, or produce an engine-specific workflow.

## Approved global output invariant

- Every list or summary output must omit large fields, including prompt text, negative prompt text, description HTML, raw metadata, Comfy/workflow payloads, and similar large source blobs. This applies to human and JSON output, to every result count including one, and to future official/community summary sources.
- Large fields may be returned only for an explicitly selected single detail. A list containing one item is still a list; count `1`, a default count of one, or an incidental single result must never implicitly unlock large fields.
- Existing explicit full-model/full-version and `--prompts` invocations are **legacy compatibility detail views**, not default list behavior. They retain their current contracts and are not templates for the new recipe summary.
- Recipe detail is still a normalized view, not a raw export: the proposed detail returns prompt and negativePrompt for one selected example, while description HTML, raw meta, and Comfy/workflow remain outside this child's recipe output.

## Confirmed repository facts

- **F1 — The current CLI has no `recipe` command.** Dispatch and the public command list contain the existing `search`, `show`, `vet`, `sync`, `tags`, account/proxy/skill, upgrade, completion, version, and help surfaces (`main.go:48-115`, `main.go:134-188`). Adding a command also requires named-help registration (`internal/cmd/help.go:20-52`) and completion metadata/scripts; completion drift tests compare the command list with `main.go` (`internal/cmd/completion.go:77-90`, `internal/cmd/completion_test.go:183-195`).
- **F2 — A version-detail call already provides the required first-party source object.** `GetVersion` performs `GET /model-versions/{id}` and returns files plus images with generation metadata (`internal/api/versions.go:9-13`). `show -v` decodes that response with `ParseVersion`; without `--prompts`, JSON emits the full decoded version, and with `--prompts`, JSON switches to the prompts detail report (`internal/cmd/show.go:77-104`). Both are explicitly selected legacy detail forms.
- **F3 — The current prompts detail contract is not a versioned recipe-context contract.** `promptRecipe` contains index, URL, prompt, negativePrompt, steps, cfgScale, sampler, and scheduler; `promptsReport` adds only `prompts` and `skipped` (`internal/cmd/prompts.go:12-33`). Null-metadata images are dropped, remaining recipes are reindexed, empty fields are omitted, and no model/version identity, schema version, or provenance is emitted (`internal/cmd/prompts.go:42-64`). Seed, width, and height exist in the decoded API type but are not exposed by this view (`internal/api/types.go:152-167`). Its prompt-bearing array remains compatibility detail behavior and must not become the new default summary shape.
- **F4 — The full-version route is an available but broad explicit detail.** The decoded version includes identity, base model, trigger words, files, images, stats, and download URLs (`internal/api/types.go:59-78`). The version fixture has nine example metas and also contains large ignored source fields such as `comfy`; the current tolerant decoder deliberately ignores unknown fields (`testdata/version_3263843.json:1`, `internal/api/types.go:152-167`). The new summary must project only small fields rather than rely on consumers to discard this broad detail.
- **F5 — Official generation metadata must come from version detail, not embedded model versions.** The fixture manifest records that embedded versions may omit `trainedWords`, that standalone version identity comes from top-level `modelId`, and that `images[].meta.width/height` may be objects rather than numbers (`testdata/FIXTURES.md:51-56`). In `testdata/models_2458426.json:1`, embedded version images have no `meta` members and embedded versions omit `trainedWords`; the standalone fixture carries both (`testdata/version_3263843.json:1`).
- **F6 — Missing metadata is a real source state, not an exceptional synthetic case.** `Image.Meta` is nullable and can be null even when `hasMeta` is true (`internal/api/types.go:128-150`). Offline tests explicitly cover null/absent metadata and ComfyUI object-shaped dimensions (`internal/api/types_test.go:178-207`). A recipe contract therefore cannot treat `hasMeta`, outer image dimensions, or a Comfy workflow as a license to synthesize parameters.
- **F7 — Hash-based context currently costs additional orchestration.** `vet` resolves a version by hash and then performs a best-effort owning-model request for creator/license context (`internal/cmd/vet.go:132-154`, `internal/cmd/vet.go:181-205`). That behavior and its exit-3 semantics are existing public behavior and should not be rerouted through the new command.
- **F8 — The bundled Agent workflow demonstrates the orchestration/token problem.** It instructs Agents to run `vet` or `show -v` with `--prompts --json`, then read trigger words from full vet/show JSON and recipes from the separate `prompts` array (`internal/skill/assets/civ-prompt/SKILL.md:30-49`). It also correctly prohibits invented triggers/parameters and states that `civ` does not download or generate (`internal/skill/assets/civ-prompt/SKILL.md:51-74`).
- **F9 — CLI and output conventions are public contracts.** Global flags are registered for every command and default error mapping is 0/1/2/4, with exit 3 reserved for vet (`internal/cmd/globals.go:23-44`, `internal/cmd/globals.go:144-164`). New subcommands require curated dual-entry help, offline tests, and integration coverage (`.trellis/spec/backend/design-principles.md:151-166`, `.trellis/spec/backend/quality-guidelines.md:95-111`). Stdout is results-only; warnings and diagnostics belong on stderr (`.trellis/spec/backend/logging-guidelines.md:15-21`).
- **F10 — Existing persistence is unrelated to a model library.** The project persists token configuration and a hash cache, not a registry of model recipes (`.trellis/spec/backend/database-guidelines.md:17-30`). This task need not add any persistent store, so summary and detail invocations each independently fetch their version response.

## Requirements

### R1 — Additive capability and legacy compatibility

- Add a new `recipe` capability while retaining every existing command, flag, output contract, and exit-code behavior.
- Do not change the meaning or JSON shape of `show -v`, `show -v --prompts`, `vet`, or `vet --prompts`. Treat their explicit full/prompts forms as compatibility detail views; do not reuse their prompt-bearing arrays as recipe summary output.
- Use the existing API client, global flags, help runner, error mapping, completion framework, and stdout/stderr discipline.

### R2 — One-request official version context per invocation

- For an exact version ID, obtain recipe data from the existing `/model-versions/{id}` response, using one API request per summary or detail invocation and no owning-model or image-browse fallback request.
- Return enough identity to ground downstream prompt work: requested/source version ID, returned model/version IDs and names, model type, base model and base-model type, publication timestamp when present, and exact source `trainedWords`.
- If optional model-summary fields are unavailable, expose that absence rather than making a second request or filling values from another version.
- A summary followed by a detail may therefore require two CLI invocations and two independent version requests; do not add a persistent cache solely to hide that cost.

### R3 — Stable, versioned summary and detail contracts

- Emit a top-level schema identifier and a kind/view discriminator that clearly distinguishes bounded recipe summary from singular example detail.
- Define required keys, nullable types, array/object behavior, source provenance, selection metadata, text availability, and structured missing-data issues for both views.
- Summary uses an `examples` array containing only prompt-free summaries. Detail uses a singular `example` object and no prompt-bearing `examples` array.
- Keep JSON deterministic for a fixed decoded response and selection. Do not include volatile retrieval timestamps merely for provenance.
- Define compatibility rules: no silent field renames, removals, type changes, or summary-to-detail redaction changes within a schema version; incompatible changes require a new schema version.

### R4 — Faithful generation data and explicit missing-data semantics

- Normalize only source-backed fields. Preserve prompt text and negative prompt text **in selected detail only**, and preserve trigger words, sampler/scheduler vocabulary, and small parameter values without trimming, deduplicating, translating, sorting, or inventing them.
- Summary may include small parameters (steps, cfgScale, sampler, scheduler, seed, width, height, denoise) and prompt/negativePrompt availability statuses, but never their text values or raw metadata.
- Detail includes the same small parameters plus normalized prompt and negativePrompt for exactly one selected source example.
- Apply the approved shared scalar normalizer: valid numeric strings in known fields become JSON numbers, seed stays an exact decimal string, numeric `20` and string `"20"` are equivalent after projection, missing/null stays null, and invalid/non-scalar shapes produce null plus structured issue. Do not normalize unknown fields or infer units/defaults.
- Distinguish an explicitly empty string or empty `trainedWords` array from an absent/null source value. Summary availability statuses must distinguish provided, explicitly empty, not provided, and unsupported text shapes without exposing text; detail must preserve the corresponding value/null and issue semantics.
- Represent absent/null values, malformed numeric values, and unsupported JSON shapes distinctly through nullable values plus structured issues. Object-shaped width/height must not be converted into guessed dimensions.
- Treat `hasMeta` as a hint only. A null/absent `meta` object yields no parameters or detail text and an explicit metadata-missing state.
- Do not infer values from `comfy`, outer image dimensions, model-description prose, other examples, other versions, common engine defaults, or the requested resource identity.

### R5 — Bounded deterministic summaries and explicit singular detail

- Bound summary results with approved default 3 and accepted range 0–10; zero returns version context and selection counts with `examples: []`. Retain existing explicit legacy full-version detail as the compatibility escape hatch for broad inspection.
- Select summary examples deterministically in source order. Selection counts must identify total source examples, returned examples, metadata availability, and truncation.
- Preserve source indexes and missing-metadata entries within the selected summary window rather than silently reindexing only usable recipes.
- Use the approved explicit detail selector `--example <sourceIndex>` with a one-based source index. Summary count `1` must never act as that selector.
- Summary and detail selection flags must be mutually exclusive when both are explicitly supplied. Invalid local selectors fail before network access; an out-of-range source index discovered after the fetch returns a normal selection error with no partial detail output.
- An existing version with no examples, or with selected summary examples lacking metadata, is still a successful context summary with explicit counts/issues, not a fabricated recipe and not a usage error. Detail requires an existing selected source entry.

### R6 — Compact output with enforced large-field redaction

- Whitelist the context, availability, small-parameter, and detail fields needed by each recipe view; do not pass through the full version object.
- Exclude prompt values, negativePrompt values, description HTML, raw meta, Comfy/workflows, unknown metadata bags, file/hash inventories, stats, and download URLs from every summary, regardless of count or output format.
- Detail may include prompt and negativePrompt for the selected example only. Continue to exclude description HTML, raw meta, Comfy/workflows, unknown metadata bags, file/hash inventories, stats, download URLs, and all nonselected examples' text.
- Summary size must remain bounded by the example limit without depending on the intrinsic length of omitted prompt strings. Detail may vary with the selected prompt lengths but must not grow when nonselected examples or their large fields are added.

### R7 — Read-only behavior and honest capability language

- Perform no model/image generation, model/image download, upload, workflow execution, or NSFW-gating bypass.
- Add no standalone export command, engine-specific workflow exporter, output-file option, persistent model/recipe library, or recipe cache.
- Image and API locations may appear only as provenance; the command must not fetch those image URLs.
- Help, README, and bundled Agent guidance must describe examples as source evidence, not recommended settings, guaranteed reproducibility, an endorsement by the model author, or an executable recipe.

### R8 — Complete CLI surface and offline verification

- Add curated help, named help routing, shell completion, README guidance, and live integration assertions for both summary and selected-detail forms.
- Add offline fixture/table tests for request count/path, both output schemas, large-field redaction, selection bounds, singular detail selection, missing data, numeric drift, source-order stability, API errors, and legacy compatibility.
- Update the bundled `civ-prompt` workflow to use summary first and request singular detail only when full prompt text is needed, while retaining existing commands for discovery, local-file/hash identification, and explicit legacy detail.

### R9 — Approved community detail seam without community implementation

- Reserve the approved cross-child singular community detail syntax `civ recipe image <imageId> [--json]` for `.trellis/tasks/11-24-community-recipes`; its syntax is decided, but its retrieval behavior and detailed schema remain that sibling's implementation/review scope.
- Keep this child's implementation limited to the two official exact-version forms. Do not add the `image` route, community retrieval, crawling, sampling, persistence, or dormant community branches here.
- Use per-example provenance that can distinguish official version examples from community images through compatible summary/detail envelopes and parameter/missing-data semantics.
- Require any future community list/sample output to obey the same prompt-free summary invariant. The approved `recipe image <imageId>` form is explicitly singular detail, so it is the community path eligible for reviewed large text; raw-meta/workflow export is not implicitly approved.
- Coordinate provenance values, image-detail fields, endpoint behavior, and final `recipe image` completion/help/integration coverage through the parent and owning sibling rather than anticipating them in this child.

## Observable acceptance criteria

The official command forms, summary budget, schema fields, and human/JSON presentation used below are approved. Scalar normalization, redaction, and summary/detail selection have mandatory contract assertions.

- **AC1 — One-call grounded summary (R1, R2):** Against an `httptest` server serving `version_3263843.json`, `civ recipe 3263843 --json` requests exactly `/model-versions/3263843` once and does not request `/models/2458426` or an images endpoint. Output identifies model 2458426, version 3263843, `Anima`, `Checkpoint`, base model `Anima`, base-model type `Standard`, and the source's explicitly empty `trainedWords` array.
- **AC2 — Stable schemas and provenance (R3):** Contract tests assert the schema identifier, summary/detail kind or view, official API path, requested version ID, per-example `civitai.versionExample` provenance, source image URL/index, required keys, and required key types. Summary has an `examples` array and no singular `example`; detail has one `example` and no `examples` array. Repeating the same fixture and selection yields equivalent JSON with no timestamp-only difference.
- **AC3 — Summary redaction and small-parameter fidelity (R4, R6):** For default, explicit counts including `1`, empty results, human output, and JSON output, recursively assert absence of `prompt`, `negativePrompt`, `description`, raw `meta`, `comfy`, and workflow fields and absence of the fixture text sentinels `rx-78-2 gundam` and `chromatic aberration`. The first summary nevertheless retains steps 8, cfgScale 1, sampler `Euler`, scheduler `simple`, seed `866354325141838` without precision loss, denoise 1, prompt availability, and explicit unsupported/missing generation width/height semantics.
- **AC4 — Explicit detail is the only prompt-bearing recipe view (R4, R5, R6):** `civ recipe 3263843 --example 1 --json` makes one version request and returns only source example 1 with its exact positive prompt containing `rx-78-2 gundam`, exact negative prompt containing `chromatic aberration`, and the same small parameters/seed precision as the summary. No other example's prompt is present. A detail request for another source index does not contain example 1's unique positive-prompt sentinel `rx-78-2 gundam`; the shared negative text is not treated as a unique identifier. Neither detail nor summary contains raw meta, Comfy/workflow, description HTML, or nonselected example text.
- **AC5 — Missing/empty/invalid matrix (R4):** Table tests prove all of the following in both applicable views: absent and null `trainedWords` produce an explicit not-provided issue; explicit `[]` produces an empty array without that issue; summary text statuses distinguish absent prompt from present `""`; detail returns null plus an issue for absent prompt but `""` for explicit empty; null/absent `meta` produces null parameters, null detail text where applicable, and a metadata-not-provided state; numeric strings accepted by the decoder normalize consistently; invalid numbers and object/list scalar shapes produce structured issues and never defaults.
- **AC6 — Bounded summary and singular-detail selection (R5):** With nine source examples, default output returns exactly the first 3 in original order; explicit counts 0–10 return exactly that prompt-free prefix, zero returns context/counts with `examples: []`, and values outside 0–10 fail locally; total, returned, with-metadata, without-metadata, and truncated values are correct. A missing-metadata example inside the selected prefix keeps its source index and place. Detail returns exactly the requested source index even when it was outside the default summary window. Explicitly combining summary and detail selectors fails locally with zero requests, as do malformed counts/indexes; an out-of-range detail index fails after the one source fetch with no partial JSON.
- **AC7 — Empty-source behavior (R3, R5):** Explicit `images: []` returns a summary with `examples: []`, zero counts, and no metadata error. Absent/null `images` returns an empty summary array plus explicit not-provided semantics and nullable/otherwise reviewed unknown-count behavior; it is not reported as proof that the version has no examples. Detail on an empty/unknown/out-of-range source returns a selection error rather than an empty fabricated detail.
- **AC8 — Compactness and nonselected-data independence (R6):** Summary output is smaller than the existing full decoded version output and does not grow when examples outside the approved limit or their prompts are added. Detail output does not change when a nonselected example's prompt, negativePrompt, unknown/raw metadata content, or Comfy payload changes without changing that example's metadata-presence counts. Selected prompt strings remain verbatim only in detail.
- **AC9 — Read-only and no capability overclaim (R7):** Each command invocation observes no outbound request other than version detail and no new filesystem writes. Help/schema examples contain no generate/download action or executable workflow claim. The only URL fields are reviewed provenance locations. No config, cache, library, export, or output-file surface is added.
- **AC10 — Existing commands unchanged (R1):** Existing `show`, `show --prompts`, `vet`, and `vet --prompts` tests remain green without expected-output changes, including their explicitly selected legacy full/prompts detail behavior, prompts omission/reindex behavior, vet model fallback, and vet exit 3. New presence tracking or normalization code must not add/remove keys in legacy JSON.
- **AC11 — CLI behavior and integration (R8):** `civ recipe help`, `civ recipe -h`, and `civ help recipe` render equivalent stdout/exit-0 pages documenting summary versus detail; completion scripts include the approved official selectors `--examples` and `--example`. Live integration adds a known-version summary assertion with prompt sentinels absent and a selected-detail assertion with selected prompt text present and nonselected unique positive-prompt sentinels absent, while retaining all current show/vet assertions. API 404/network/token failures retain the shared 1/2/4 mapping; exit 3 remains vet-only. The sibling-owned `recipe image` route receives its own completion/help/integration coverage when implemented.
- **AC12 — Agent guidance and scope boundary (R7, R8, R9):** Updated Agent documentation uses summary first, selects a source index, and requests singular detail only when prompt text is needed. It instructs consumers to honor issues/missing values and retains search/vet for discovery and identification. This child makes no community request and does not claim `civ recipe image <imageId>` is available before the owning sibling lands it. Parent review verifies that the approved community detail route will reuse compatible provenance, normalized parameter/missing-data semantics, and the summary/detail redaction invariant without an unnecessary schema rewrite.

## Out of scope

- Prompt-bearing or negative-prompt-bearing summary/list arrays, implicit detail unlocked by count `1`, and large-field truncation as a substitute for explicit detail selection.
- Raw-meta, description-HTML, ComfyUI/A1111/other workflow export in either recipe view. Existing explicit legacy full/prompts paths remain compatibility detail, not a new export capability.
- Community image search/sampling, popularity ranking, pagination, crawling, NSFW expansion, and all implementation assigned to `11-24-community-recipes`.
- Query/name/hash/local-path resolution, latest-version selection, compatibility checks, local-presence checks, and resource-set resolution assigned to `11-24-resource-resolver`.
- Model/version comparison and recommendation scoring assigned to `11-24-model-version-compare`.
- Choosing a “best” recipe, merging multiple examples, deriving defaults, or recommending parameters not present in the selected source records.
- Parsing model-description HTML/Markdown or arbitrary ComfyUI workflows into generation parameters.
- Generating images, downloading models/images, invoking external engines, or writing recipe files.
- A persistent local model library, recipe database, response cache, or migration of existing config/hash-cache stores.
- Removing, renaming, or behaviorally modernizing legacy `--prompts`; that compatibility cost is not justified by this additive capability.
- A new Civitai endpoint or raw-JSON passthrough mode.

## Decided invocation and selector contract

- Official identity is exact-version-only for this child: `civ recipe <versionId>`. Query/name/hash/local-path and implicit-latest selection remain outside the MVP; existing search/show/vet obtains the ID.
- `--examples <count>` is always a prompt-free first-N summary, including when count is one.
- RT1 is approved: `--examples` defaults to 3 and accepts 0–10. Zero returns version context and selection counts with `examples: []`; no offset/range pagination is added.
- `--example <sourceIndex>` is the approved one-based singular-detail selector and returns only that official example's prompt/negativePrompt and approved small fields.
- Explicitly combining `--examples` and `--example` is a usage error before network. Invalid indices fail locally; an out-of-range index discovered after the fetch returns a selection error without partial detail.
- Sibling-owned `civ recipe image <imageId> [--json]` is the approved community singular-detail syntax and is not implemented by this child.
- Human and JSON output follow the same list/detail redaction boundary; `--json` remains opt-in for the stable machine contract.

## Approved schema and human presentation

### RT2 — Summary/detail schema

The approved schema uses:

- `schemaVersion: "civ.recipe/v1"`;
- `kind: "recipeSummary"` with an `examples` array, or `kind: "recipeExampleDetail"` with one singular `example` object;
- shared `provenance`, `subject`, `selection`, and top-level `issues`;
- summary examples containing only `sourceIndex`, provenance, metadata/text availability statuses, small parameters, and issues;
- detail containing the same summary fields plus a singular `textParameters` object with `prompt` and `negativePrompt`;
- required nullable keys, `issues: []` arrays, decimal-string seed, and the approved shared scalar normalizer.

No raw meta, description HTML, ComfyUI/workflow payload, file/hash inventory, stats, download URL, volatile timestamp, recommendation score, or nonselected example text is included.

### RT3 — Human and JSON presentation

- Human output is prompt-free summary by default and prints selected prompt/negativePrompt only for explicit `--example <sourceIndex>` detail.
- `--json` remains the stable machine contract and is opt-in through the existing global flag.
- Human summary and detail follow the same large-field boundary as JSON: no one-item summary exception.
- Human text is not a machine contract but must not overclaim availability, reproducibility, or capabilities.

## Planning status

All previously listed product/UX questions for this child are resolved. Implementation remains unapproved and unstarted pending the parent convergence review.
