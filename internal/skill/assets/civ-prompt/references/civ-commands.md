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
- `--json` passes through the raw API `items` array unchanged. Each item carries
  `id`, `name`, `type`, `nsfw`, `tags`, `baseModels`, `stats`, `creator`, and
  `modelVersions[]` (each with `id`, `name`, `baseModel`, and sometimes `trainedWords`).
- Use the chosen item's `id` (modelId) and a `modelVersions[].id` (versionId) with `show`.

## show — inspect a model or version

```bash
civ show <modelId>                          # model header + version table
civ show <modelId> -v <versionId>           # version drill-down (files, hashes, trigger words)
civ show <modelId> -v <versionId> --prompts # append example-image recipes
civ show <modelId> --json                   # full model object
civ show <modelId> -v <versionId> --prompts --json
```

- `--prompts` requires `-v <versionId>`.
- `--json` without `-v`: the full model object (`modelVersions[]` embedded).
- `--json` with `-v` (no `--prompts`): the full version object, including
  `trainedWords`, `baseModel`, `files[]`, `images[]` (each `images[].meta` may be null),
  `model` summary, and `stats`.
- `--json` with `-v --prompts`: the recipe view (see below).

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
- `--prompts --json` focuses the output on the recipe view (below).
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