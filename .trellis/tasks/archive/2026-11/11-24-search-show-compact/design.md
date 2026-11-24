# Compact search and show JSON — approved technical design

> Status: D1 and D2 are approved on 2026-11-24, including the concrete field registry below and the parent-wide prompt-free list/detail invariant. No implementation is authorized until the final planning summary receives fresh explicit approval.

## 1. Approved behavior and smallest implementation boundary

The three scoped `--json` forms default to compact output:

```text
civ search <query> --json
civ show <modelId> --json
civ show <modelId> -v <versionId> --json
```

Each accepts `--view compact|full`, default `compact`. Full restores the exact legacy behavior:

- search full: upstream `items` bytes passed through unchanged;
- model/version show full: the current decoded `api.Model`/`api.ModelVersion` object rendered with `report.WriteIndent`.

Human output and the independent `show -v --prompts --json` view do not change.

The implementation should branch **after the same existing API response arrives**. It adds no endpoint, request, fallback, local storage, or generic output framework.

## 2. Command and flag contract

### 2.1 View selection

- `--view compact` selects the normalized projections in Section 4.
- `--view full` selects the legacy paths without reimplementing their output.
- An omitted view is identical to explicit `--view compact`.
- View values are documented in lowercase; unknown values are usage errors before network activity.
- An explicitly supplied `--view` requires `--json`; otherwise it is a usage error rather than a silent no-op over the already-curated human output.
- An explicitly supplied `--view` combined with `--prompts` is a usage error. `--prompts` remains its own established view; without an explicit view, its behavior is unchanged.
- Flag validation uses the parsed `FlagSet` (for example `Visit` to detect explicit view) and existing command options. No global mutable flag state is added.

Examples:

```bash
civ search anima --json                         # compact search
civ search anima --json --view full             # legacy raw items
civ show 2458426 --json                         # compact model summary
civ show 2458426 -v 3263843 --json --view full # legacy decoded version
```

### 2.2 Failure behavior

- Invalid view/flag combinations fail before any request with existing usage-class exit 1.
- Existing 404/network/token failures retain exits 1/2/4 and empty stdout.
- Compact decode failure is a command error; do not silently fall back to full output, because that would flood the exact Agent path the feature is meant to bound.
- Human invocations without an explicit view continue through the current rendering paths byte-for-byte.

## 3. Projection ownership

Add focused compact decoding/projection in `internal/api`, because source presence, nulls, numeric drift, and unknown-field tolerance belong at the API boundary:

- `ParseCompactModels(raw)` for search;
- `ParseCompactModel(raw)` for model show;
- `ParseCompactVersion(raw)` for version show;
- exported compact DTOs required by `cmd`, with GoDoc explaining stable schema and missing-data semantics.

The command layer owns envelope construction only if it is trivial and type-safe; otherwise the API projection returns the complete versioned envelope. No renderer reads `map[string]any` or redefines field meaning.

Keep the existing `Model`, `ModelVersion`, `ParseModel(s)`, `ParseVersion`, `ModelsRawItems`, and prompt projector untouched for full/legacy and other commands. Dedicated compact source structs may repeat a limited set of field declarations to preserve absent/null/empty distinctions without changing legacy JSON types.

Dependency direction remains:

```text
main -> cmd -> api compact projection -> report.WriteIndent
```

No new package is needed for three projections. If recipe/compare later needs an identical field, extraction requires a separate reuse review rather than importing this CLI schema blindly.

## 4. Approved field registry (D2)

### 4.1 Shared `modelSummary`

Search items and model-show use the **same** summary object. This is the deliberate reuse boundary.

| Key | JSON type | Rule |
|---|---|---|
| `id` | number/null | Model ID; required key, null if source identity is absent |
| `name` | string/null | Source display name, verbatim |
| `url` | string/null | Canonical `https://civitai.com/models/{id}` only when ID is known |
| `type` | string/null | Unknown type strings preserved verbatim |
| `creator` | string/null | Creator username only |
| `tags` | array/null | Source order; absent/null -> null, explicit `[]` -> `[]` |
| `baseModels` | array/null | Source order; no family inference or truncation |
| `nsfw` | boolean/null | Source flag only; detailed browsing levels remain in full |
| `availability` | string/null | Model availability only, never inferred from versions |
| `license` | object/null | Four model-level grants below; null if the source object/grants are unavailable under the reviewed presence rules |
| `stats` | object/null | Selection counters below; absent stats are not converted into observed zero |
| `versions` | array/null | Every embedded version as `versionRef`; no latest-only selection |

`license` contains:

- `allowNoCredit`: boolean/null;
- `allowCommercialUse`: array/null;
- `allowDerivatives`: boolean/null;
- `allowDifferentLicense`: boolean/null.

`stats` contains only `downloadCount`, `thumbsUpCount`, `thumbsDownCount`, and `commentCount`, each number/null. Tips, ratios, and derived scores are omitted; human formatting can derive a ratio where already done.

Each `versionRef` contains:

| Key | JSON type | Rule |
|---|---|---|
| `id` | number/null | Exact version ID needed by show/recipe/compare/resolve |
| `name` | string/null | Source version name |
| `baseModel` | string/null | Source base model |
| `baseModelType` | string/null | Source subtype; no compatibility inference |
| `publishedAt` | string/null | Original timestamp, no date rewriting |
| `availability` | string/null | Embedded-version availability only |
| `stats` | object/null | `downloadCount`, `thumbsUpCount`, `thumbsDownCount` only |

The summary deliberately excludes `trainedWords`, files, images, prompts, hashes, and per-version download URLs. Embedded versions may omit triggers/files or carry incomplete generation data; basic model selection must not present those partial fields as canonical detail.

### 4.2 Search envelope

```json
{
  "schemaVersion": "civ.search/v1",
  "kind": "modelSearch",
  "items": [
    {"id": 2458426, "name": "Anima", "versions": []}
  ]
}
```

The actual item is a complete `modelSummary`; the abbreviated example only illustrates the envelope.

Rules:

- `items` has the same length and order as the upstream items array.
- Explicit empty upstream items produces `items: []`; absent/null items is a decode error because it is not a valid search envelope for this command.
- No query echo, fetch timestamp, raw response metadata, description HTML, files, images, workflows, or download URLs are added.
- This intentionally changes the old top-level array into a versioned object as part of the approved default-contract break. Full view preserves the old array exactly.

### 4.3 Model-show envelope

```json
{
  "schemaVersion": "civ.show/v1",
  "kind": "modelSummary",
  "model": {"id": 2458426, "versions": []}
}
```

`model` is the identical `modelSummary` used in search items. The command makes the same `GET /models/{id}` request as before and does not enrich it with standalone versions or images.

### 4.4 Version-show envelope

```json
{
  "schemaVersion": "civ.show/v1",
  "kind": "versionSummary",
  "version": {
    "id": 3263843,
    "model": {"id": 2458426, "name": "Anima", "type": "Checkpoint"},
    "files": [],
    "examples": {"total": 9, "withMetadata": 9, "withoutMetadata": 0}
  }
}
```

The complete `versionSummary` registry is:

| Area | Fields | Rules |
|---|---|---|
| Identity | `id`, `modelId`, `name`, `baseModel`, `baseModelType`, `publishedAt`, `availability` | Required nullable keys; no latest-version or model-availability substitution |
| Owning summary | `model: {id,name,type}` | Use top-level `modelId` plus the standalone version's model summary; no `/models/{id}` request and no creator/license fabrication |
| Adoption | `stats` | `downloadCount`, `thumbsUpCount`, `thumbsDownCount`, each nullable; no comment/tip invention |
| Triggers | `trainedWords` | Absent/null -> null plus documented not-provided meaning; explicit `[]` -> `[]`; preserve exact strings/order |
| Files | `files` | Every visible file in source order; absent/null -> null, explicit `[]` -> `[]` |
| Example coverage | `examples` | Null when images were not provided; otherwise `{total,withMetadata,withoutMetadata}` based on actual non-null meta objects |

Each compact file retains:

- `id`, `name`, `sizeKB`, `type`, `primary`;
- `metadata: {format, fp, size}` with nullable members;
- `pickleScanResult`, `virusScanResult`, `scannedAt`;
- `hashes`: the complete supplied algorithm/value object, including SHA256/AutoV2 and other algorithms, with absent/null versus explicit empty preserved.

File `downloadUrl` is excluded. There is no primary/first-file selection: all visible files remain because non-primary Text Encoder/VAE files can be important and hashes/scans are central to version inspection.

`examples.withMetadata` counts non-null metadata objects, not `hasMeta: true`. It contains no prompt text, image URLs, seeds, dimensions, ComfyUI data, or arbitrary generation fields. `show -v --prompts` and recipe remain the paths for actual generation records.

Explicitly excluded from version compact output: description HTML, raw `images`, `downloadUrl`, training details/status objects, paid/early-access details, usage controls, arbitrary unknown fields, and Orchestration data. These remain available only where the existing full view exposes them.

## 5. Presence, null, and empty rules

- Required keys are present in normalized DTOs even when their value is null; this avoids consumers guessing whether a key was forgotten.
- Absent and explicit null may share a documented `null` not-provided state where the current source semantics do not require distinguishing them. Explicit empty string/array/object/zero/false remains a supplied value.
- Arrays are null for not provided and `[]` for supplied empty. No array is silently truncated.
- Numeric fields retain `json.Number`-style tolerant decoding; invalid numeric text produces a focused decode error rather than a zero. No generation seeds are included in these basic views.
- Unknown enum strings are preserved. Unknown objects/fields are ignored rather than emitted.
- No current-time, random, score, or recommendation fields are introduced.

Any future reduction of required nullable keys is a contract change requiring schema/version, documentation, and test review; the decoder must not quietly mix omitted and null semantics by field.

## 6. Data flow

### Search

```text
GET /models (existing SearchModels)
  -> --view full: ModelsRawItems -> WriteRaw          [legacy]
  -> default compact: ParseCompactModels -> WriteIndent [new]
```

### Model show

```text
GET /models/{id} (existing GetModel)
  -> --view full: ParseModel -> WriteIndent           [legacy]
  -> default compact: ParseCompactModel -> WriteIndent [new]
```

### Version show

```text
GET /model-versions/{id} (existing GetVersion)
  -> verify returned model relationship as today
  -> --prompts without explicit view: existing prompt projector [legacy]
  -> --view full: ParseVersion -> WriteIndent           [legacy]
  -> default compact: ParseCompactVersion -> WriteIndent [new]
```

All paths preserve the existing model/version ownership check and HTTP/auth/retry behavior.

## 7. Size and record-preservation verification

Size tests compare semantically rendered full and compact outputs, not raw minified upstream bytes against pretty compact JSON.

For each checked-in representative fixture:

- assert the same top-level model count/order and complete version/file record counts required by the registry;
- assert excluded sentinels (`description`, `images`, `comfy`, `downloadUrl`, prompt/workflow content) are absent;
- assert compact output is less than 50% of the corresponding full indented output;
- verify the reduction comes from field removal, not whitespace-only minification or record truncation.

The 50% threshold is a regression ceiling for these fixtures, not a promise for every possible upstream response.

## 8. Compatibility, migration, and documentation

- README replaces the claim that default search JSON is raw passthrough and documents both new envelopes and `--view full`.
- Help and completion expose `compact|full` and default compact consistently on search/show.
- The Agent Skill uses default compact search/show for discovery/inspection, recipe for normalized generation context, and full only when a field was deliberately excluded.
- CHANGELOG marks the default JSON change explicitly for the next pre-1.0 release and gives one migration example per affected command form.
- Integration tests assert default compact and full legacy behavior; existing tests that expected raw/default full are changed only for these three forms, with every unrelated expectation retained.
- Existing `show --prompts`, vet, sync, tags, and human output remain compatibility checks.

## 9. Tradeoffs

- **Versioned envelopes versus preserving search's array:** envelopes add one object level but make the breaking normalized contract explicit and evolvable; full retains the legacy array.
- **One model summary versus separate search/show projections:** shared structure reduces drift and Agent learning cost, but fields must serve both use cases rather than being search-only.
- **All minimal versions versus latest-only:** all version refs preserve selection and avoid another request, at a slightly larger but still bounded-by-source-count payload.
- **All file hashes versus SHA256-only:** all hashes cost little relative to images and preserve interoperability; no arbitrary algorithm prioritization is needed.
- **Dedicated compact decoders versus changing legacy DTOs:** limited declaration duplication protects the full view and unrelated commands from presence/type drift.
- **No arbitrary field selector:** fixed fields are easier to version, test, document, and teach to Agents than a generic query language.

## 10. Rollback

Before release, revert the compact parser/projection, view flag, help/completion/docs/integration changes as one unit to restore previous default output. Full view is not a substitute for a clean rollback. No persistent data or migration cleanup exists.

After release, correctness fixes should preserve `civ.search/v1`/`civ.show/v1`; removing or changing contracted fields requires a schema/version and release-note review. If the entire compact feature must be withdrawn, restore the full legacy paths as defaults and remove the view surface together rather than leaving an undocumented compatibility mode.
