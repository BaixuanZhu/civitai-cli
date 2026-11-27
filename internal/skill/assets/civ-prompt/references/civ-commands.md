# civ command reference

Quick reference for the `civ` commands that feed prompt creation. All read
commands are anonymous by default and accept the global flags `--json`,
`--token <key>`, `--timeout <dur>`, and `--verbose` **after** the command name.
`civ` reads registry data only; it never downloads models/images or generates images.

## search — find models

```bash
civ search <query> [--type lora] [--base "Flux.1 D"] [--sort most-downloaded] \
                   [--period month] [--tag <tag>] [--user <name>] [--limit 20] [--json]
```

- `--type` is repeatable or comma-separated (`lora`, `checkpoint`, `ti`, `locon`, ...).
- `--base` applies only the first value; quote values containing spaces.
- `--limit` is 1-100.
- `--json` defaults to the compact `civ.search/v1` envelope (`items[]` of model
  summaries with `id`, `name`, `type`, base model, stats, and every
  `modelVersions[].id`). Its `summary` reports the returned-page count and
  non-exclusive base coverage of this one response (never a match total).
  `--view full` restores the legacy raw items array.
- Use the chosen item's `id` (modelId) and a `modelVersions[].id` (versionId) with
  `recipe` (preferred) or `show`.

## show — inspect a model or version

```bash
civ show <modelId>                          # model header + version table
civ show <modelId> -v <versionId>           # version drill-down (files, hashes, trigger words)
civ show <modelId> -v <versionId> --prompts # append example-image recipes
civ show <modelId> --json                   # full model object
civ show <modelId> -v <versionId> --prompts --json
```

- `--prompts` requires `-v <versionId>`.
- `--json` defaults to compact `civ.show/v1` envelopes (`modelSummary` without
  `-v`, `versionSummary` with `-v`): identity, triggers, files/hashes/scans, and
  example counts, with no prompt/description/raw-metadata blobs.
- `--view full` restores the legacy decoded payload (full model object,
  version object including `images[].meta`, or the `--prompts` recipe view).
- `--view` requires `--json` and conflicts with `--prompts`.

## recipe — compact generation context (preferred for prompt data)

```bash
civ recipe <versionId> --json                 # prompt-free summary, 3 examples
civ recipe <versionId> --examples 0 --json    # counts only, examples: []
civ recipe <versionId> --example <sourceIndex> --json   # one example's prompt text
civ recipe <versionId> --source community [--limit 20] [--sort most-reactions] --json
civ recipe <versionId> --source community --stack-summary --json   # aggregate view
civ recipe image <imageId> --json             # one exact community image detail
```

- Summary shape (`kind: "recipeSummary"`, `schemaVersion: "civ.recipe/v1"`):
  `subject` (`model`, `version`, `trainedWords`), `provenance`,
  `selection {requested, returned, total, withMetadata, withoutMetadata, truncated}`,
  `examples[]` (`sourceIndex`, `metadataStatus`, `promptStatus`,
  `negativePromptStatus`, `parameters`), and `issues[]`. Summaries contain **no**
  prompt or negativePrompt text, at any count including 1.
- Detail shape (`kind: "recipeExampleDetail"`): a singular `example` with the
  same summary fields plus `textParameters {prompt, negativePrompt}` for the
  selected `sourceIndex` only.
- `parameters` covers steps, cfgScale, sampler, scheduler, seed (exact decimal
  string), width, height, and denoise. Numeric strings normalize to numbers;
  invalid/non-scalar values become null plus an issue.
- `--examples` defaults to 3 (range 0-10). `--example` is the 1-based
  `sourceIndex` shown in the summary and is mutually exclusive with an explicit
  `--examples`.
- Community mode performs exactly one bounded `/images` sample
  (`modelVersionId`, `withMeta=true`, `type=image`, `limit`, `sort`,
  `period=AllTime`). The default view reports per-image observations only;
  continuation is reported but never followed. No ranking or recommendation.
- `--stack-summary` (community only) re-parses that same single response into
  the aggregate-only `kind: "communityStackSummary"`: fixed top-5 resource
  usage by distinct images (identity basis modelVersionId/hash/name, no
  cross-basis merge), per-LoRA valid-weight medians with sample/image
  coverage, steps/cfgScale/sampler/scheduler distributions with per-field
  `observedImages` denominators and an `other` remainder, and
  prompt/negative-prompt availability as status counts with exact rates (null
  on an empty sample). No prompt text, raw metadata, workflows, media URLs,
  per-image observations, or cursor values. The sample is bounded, non-random,
  and visibility-biased — quote its `coverage`/`caveats`, and treat aggregates
  as evidence to inspect, never as recommended or reproducible settings.
- `civ recipe image` performs exactly one `imageId`+`withMeta=true`+
  `type=image`+`limit=1` request and returns only that image's detail.

## vet — audit a local file or hash

```bash
civ vet <path>            # streams SHA256, then looks the hash up in the registry
civ vet <sha256>          # 64 hex chars
civ vet <autov2>          # 10 hex chars
civ vet <path> --prompts --json
```

- `--json` review object keys:
  - `hash` — the queried hash (uppercase; computed SHA256 for a path input).
  - `version` — the full version object (`trainedWords`, `baseModel`, `files[]`, `images[]`, ...).
  - `model` — owning-model summary `{id,name,type,url,creator,license,stats}`, or `null`
    when the best-effort model fetch degraded.
  - `examples` — `{total,mature,withMeta}` counts.
  - `latestPublished` — informational latest-published visible-version context:
    `status` (`known`/`tied`/`unknown`), nullable `version`, sorted
    `tiedVersionIds`, and `comparison` (relationship `currentLatest` /
    `newerPublished` / `tiedLatest` / `unknown`, exact case-sensitive
    `baseModelChange`). Descriptive only — never an upgrade recommendation.
- `--prompts --json` focuses the output on the recipe view (below) and omits
  `latestPublished` and the other normal report keys.
- Exit `3` means the hash is not registered on Civitai (not a network failure).

## Recipe view (`--prompts --json` on show -v / vet)

```json
{
  "prompts": [
    {
      "index": 1,
      "url": "https://image.civitai.com/...",
      "prompt": "...",
      "negativePrompt": "...",
      "steps": 28,
      "cfgScale": 3.5,
      "sampler": "Euler a",
      "scheduler": "Karras"
    }
  ],
  "skipped": 0
}
```

- `index` is 1-based; absent fields are omitted (no blank holes).
- `steps`/`cfgScale` are numbers (string drift is normalized).
- `skipped` counts example images whose `meta` was null (no recipe available).

## sync — batch-audit a directory

```bash
civ sync <dir> [--ext .safetensors,.pt] [--format table|csv|json] [--concurrency 3] [--rehash]
```

- `--json` (or `--format json`) emits an array of rows:
  `{file,status,dupe,model,version,base,triggers,pickle,virus,modelUrl,versionId,error}`.
- `status` is `found` / `not_found` / `error`; `not_found` is a data row, not a failure.
- Useful for discovering which local files map to registry versions, then `show -v --prompts`.

## tags — tag-name completion

```bash
civ tags <query> [--limit 20] [--json]
```

- `--json` emits an array of tag name strings (prompt vocabulary inspiration).

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success / hit. |
| 1 | Usage error (bad flag, unknown command, missing/invalid argument); also anonymous 401 on personal-state parameters and 404 on search/show/tags. |
| 2 | Network failure (transport error, 429, or 5xx after retries). |
| 3 | Hash not registered — vet only. |
| 4 | Token rejected (401 with a token present). |
| 5 | Completed but not ready — resolve only (not used by prompt commands). |