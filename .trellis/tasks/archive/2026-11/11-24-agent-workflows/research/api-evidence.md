# API and repository evidence

> Collected 2026-11-24 for planning. Official documentation describes upstream behavior; repository fixtures are historical live samples and do not guarantee future schema stability.

## Official sources

- Model versions: https://developer.civitai.com/site/reference/model-versions.md
- Images: https://developer.civitai.com/site/reference/images.md
- AIR identifiers: https://developer.civitai.com/site/guide/air.md

## Model versions and identity

- `GET /api/v1/model-versions/{id}` returns model/version identity, `air`, `baseModel`, availability, trained words, files, images, and related fields.
- A file may carry SHA256, AutoV1/2/3, BLAKE3, CRC32, format, precision, size, scan results, and download URL. A normalized feature should expose only fields required by its contract.
- AIR syntax is `urn:air:{ecosystem}:{type}:{source}:{id}[@{version}][+{fileId}][.{format}]`. For Civitai, `id` is the model ID and `version` is the numeric model-version ID, not its display name.
- Prefer an API-returned `air` over deriving ecosystem/type locally. An AIR without a version does not provide the exact-version pinning needed for reproducible resolution.

## Images and community recipes

`GET /api/v1/images` officially supports, among other filters:

- `modelVersionId`, `modelId`, `imageId`, `postId`, `username`, and cursor/page continuation.
- `withMeta=true` for complete generation metadata.
- `limit` 0–200 (upstream default 50) and sorts including `Most Reactions`, `Newest`, and `Oldest`.
- `browsingLevel`/legacy NSFW controls, subject to account, domain, region, and safety visibility rules.

Image items may contain `id`, `url`, `postId`, `username`, `baseModel`, `modelVersionIds`, reaction/comment statistics, and free-form `meta`.

Common metadata includes `prompt`, `negativePrompt`, `seed`, `steps`, `sampler`, `cfgScale`, `resources`, and `civitaiResources`. A documented resource example is:

```json
{"type":"lora","weight":0.65,"modelVersionId":249861}
```

The generic resource and metadata schemas are deliberately open. ComfyUI and other tools may add arbitrary fields; consumers must tolerate unknown shapes and must not treat missing fields as empty/false facts.

Pagination/visibility constraints relevant to the planned bounded source:

- Cursor continuation may occur even with an empty `items` page; this does not justify automatic exhaustive traversal in the planned feature.
- Broad `modelId` galleries can time out; prefer exact `modelVersionId` and bounded limits.
- Anonymous/authenticated visibility and regional/domain safety filters may yield different or empty results without representing the total upstream gallery.
- Reactions measure response to an image, not recipe correctness, reproducibility, or parameter quality.

## Bulk endpoints deliberately not required for this scope

Official documentation also provides `POST /model-versions/by-hash` (up to 100 full SHA256 values), `POST /model-versions/by-hash/ids` (up to 10,000), and `/model-versions/mini/{id}`. They would be useful for a persistent library, but that feature was explicitly excluded. Do not add these endpoints merely because they exist; the explicit-input resolver should use the minimum requests its reviewed contract requires.

## Repository evidence

- `internal/skill/assets/civ-prompt/SKILL.md:30-49` teaches Agents to chain search/show/vet and extract trigger/recipe fields themselves.
- `internal/api/types.go:152-167` normalizes basic prompt parameters but intentionally ignores unknown generation metadata until a consumer needs it.
- `internal/cmd/vet.go:105-155` resolves one file/hash and optionally obtains owning-model license context.
- `internal/cmd/sync.go:101-118` defines one-off audit rows, not a resource-set readiness result or a persistent library.
- `testdata/models_search_anima.json`: 70,216 bytes.
- `testdata/version_3263843.json`: 101,395 bytes; nine images with ComfyUI metadata. A basic projection containing prompt/negative/steps/CFG/sampler/scheduler/seed/dimensions was approximately 7,548 bytes in a local inspection, illustrating why summary/detail boundaries matter. This is a sample, not a universal size guarantee.
- That fixture contains nested ComfyUI `prompt`/`workflow` data, but workflow export and mutation are excluded from this task tree. Do not parse or execute arbitrary embedded workflows as a side effect of unrelated recipe/compare/resolve commands.

## Design implications

1. Normalization must be typed and shared by consumers; do not let each command cast free-form metadata privately.
2. Exact references, observed values, missing values, derived comparisons, and compatibility decisions need separate representations.
3. Community defaults must be bounded and summaries must avoid returning every full prompt by default.
4. New endpoints and fields require verbatim fixtures and offline tests; official documentation is not a substitute for drift-tolerant decoding.
5. No feature in this scope requires a new persistence layer or bulk inventory API.
6. The user-approved list/detail invariant requires every list or nested summary array to omit large prompt/description/raw-metadata/workflow fields. Only an explicitly selected detail (or intentional legacy full/prompts compatibility view) may return those fields; fixture tests should include unique large-field sentinels and assert they never leak into list output.
7. Official and community projections use the same approved scalar normalizer: known numeric strings become JSON numbers, seed remains an exact decimal string, missing/null stays null, invalid/non-scalar shapes become null plus issue, and arbitrary unknown fields/units are not inferred. Tests should treat numeric `20` and string `"20"` as equivalent after normalization while still testing invalid and object/array drift.
