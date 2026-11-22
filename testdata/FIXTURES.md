# testdata fixtures

All fixtures were fetched live on **2026-11-22** (development-time manual
fetch; unit tests and CI never touch the network). JSON has no comment
syntax, so fetch dates and source URLs live in this manifest instead of
file headers — the `.json` files are verbatim API bodies and must stay
parseable by `python -c "import json; json.load(open(...))"`.

Regenerate with the appendix D commands of
`.trellis/tasks/11-22-civitai-cli-go/research/civitai-cli-plan.md`.
When the upstream API drifts, re-fetch deliberately and review field
additions/removals before updating types.go.

## API body fixtures (verbatim)

| File | Source URL |
|---|---|
| `models_search_anima.json` | `https://civitai.com/api/v1/models?limit=2&query=anima` |
| `models_2458426.json` | `https://civitai.com/api/v1/models/2458426` |
| `version_3263843.json` | `https://civitai.com/api/v1/model-versions/3263843` |
| `byhash_anima_turbo.json` | `https://civitai.com/api/v1/model-versions/by-hash/FBA11953276B57EDF59D1DC4F1857AC05AA079C56F982B4D7C20298D57D3F7EB` |
| `err_limit_too_big.json` | `https://civitai.com/api/v1/models?limit=200` (HTTP 400 ZodError) |
| `err_not_found.json` | `https://civitai.com/api/v1/model-versions/by-hash/DEADBEEF` (HTTP 404) |
| `err_unauthorized.json` | `https://civitai.com/api/v1/models?limit=1&favorites=true` (HTTP 401) |

## Enum probe evidence (probes/)

| File | What it pins |
|---|---|
| `enum_sort.json` | Raw ZodError for `sort=__invalid__`: Hot / Highest Rated / Most Downloaded / Most Liked / Most Discussed / Most Collected / Most Images / Newest / Oldest / Recently Added |
| `enum_types.json` | Raw ZodError for `types=__invalid__` (invalid_union): 23 type values, single value OR repeated array |
| `enum_period.json` | Raw ZodError for `period=__invalid__`: Day / Week / Month / Year / AllTime |
| `base_model_verified.json` | Derived evidence: final 63-value baseModel whitelist (verified-by-filter union observed-on-live-models) |
| `base_model_harvested.json` | baseModels[] values harvested from broad live samples (10 queries, limit=100) |
| `base_model_singular_ignored.json` | Raw 200 body proving singular `baseModel=__invalid__` is silently ignored |
| `enum_base_models_raw.json` | Raw 200 body proving plural `baseModels=__invalid__` filters to empty items (no Zod validation) |

## Live API facts learned during the 2026-11-22 probe (drift vs research)

1. **Singular `baseModel` param is dead**: silently ignored for ALL values
   (even Illustrious / SDXL 1.0). The working filter is plural `baseModels`.
2. **`baseModels` multi-value**: repeated params → FIRST value wins;
   comma-joined string → treated as one unknown value → empty items.
   The CLI therefore warns and sends only the first --base value.
3. **`types` multi-value**: repeated `types=LORA&types=LoCon` works;
   comma-joined `types=LORA,LoCon` is a ZodError. No enum leak for
   baseModels (unknown value → empty items, HTTP 200).
4. **`sort=Recently Added` requires a collectionId** (HTTP 400 with a
   third error shape `{"message":"..."}`); kept in the enum table because
   the Zod values list it, but plain model search cannot use it.
5. Model items carry `baseModels[]` and embedded `modelVersions[]`;
   embedded versions may omit `trainedWords` (nil ≠ empty).
6. Version detail `model` summary is `{name,type,nsfw,poi}` (no id);
   the owning model id is the top-level `modelId` field.
7. `images[].meta.width/height` can be ComfyUI node objects, not numbers
   (decoded as json.RawMessage); `hasMeta=true` does not guarantee meta.