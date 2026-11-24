# Model and version comparison

> Status: requirements are converged for implementation review. D1–D8 are approved. This document does not authorize implementation or `task.py start`.

## Goal

Add an optional, read-only way to compare explicitly selected Civitai models or model versions using aligned selection facts and meaningful derived differences. The result must help a user choose candidates or understand what changed without concatenating raw API responses or reducing the comparison to an opaque “best model” score.

## User value

- A user can copy model IDs from `civ search` and compare candidates on the same rows instead of manually reconciling several `civ show` outputs.
- A user can pass an older version ID followed by a newer version ID and see base-model, trigger, file/hash, license/availability, scan, and recipe-coverage changes directly.
- An Agent can consume a stable, curated JSON contract that preserves source facts, missing-data states, and derived changes separately.
- Comparison remains an optional sibling of recipe context and resource resolution, not a required stage in an Agent workflow.

## Confirmed repository context

- `civ search` already returns model candidates and IDs; its human table and raw `--json` items array are public contracts (`internal/cmd/search.go:92-147`).
- `civ show` fetches either one full model or one full version, while `civ vet` supplements a version with its owning model for creator/license context (`internal/cmd/show.go:64-119`, `internal/cmd/vet.go:181-205`).
- Existing `GET /models/{id}` and `GET /model-versions/{id}` endpoints are sufficient for canonical comparison input; no search, image-browse, or community endpoint is needed (`internal/api/models.go:70-74`, `internal/api/versions.go:9-13`).
- Embedded versions may omit `trainedWords`; `nil` currently means “not provided,” not an empty trigger set (`internal/api/types.go:59-78`). The checked-in model fixture demonstrates that omission, while standalone version detail provides the field.
- The standalone version fixture has no `availability` field, whereas the matching embedded version does. It also lacks model-level creator/license fields. Version comparison therefore needs explicit source and missing-data rules rather than assuming one response contains everything.
- The CLI is standard-library-only, offline-testable, and reserves stdout for results. Existing commands, flags, JSON shapes, and exit-code meanings are compatibility contracts.

## Definitions

- **Subject:** one explicitly requested model or version. Subjects remain in input order.
- **Aligned fact:** a curated field with one value position per subject, so the same fact can be inspected across all subjects.
- **Derived difference:** a transparent operation over aligned facts, such as a scalar transition, numeric delta, set add/remove, file match, or hash change.
- **Missing:** the source did not provide a field. Missing is not false, zero, empty, or a confirmed equality.
- **Null:** the source explicitly supplied JSON null. Null is distinct from a missing field and from an empty string/list/object.
- **Empty:** a supplied empty string, list, object, or zero-valued statistic. Empty and zero are facts, not missing data.
- **Baseline:** the first input subject, used only to directionally label derived changes. It does not mean “best,” “latest,” or “recommended.”

## Requirements

### R1 — Additive, optional command

Add a comparison command without changing the invocation, flags, output, or behavior of any existing command, including `search`, `show`, `vet`, `sync`, and `tags`, or the planned sibling `recipe`/`resolve` workflows. Comparison must work independently after a user has selected IDs; no other command may invoke it implicitly or require it as a pipeline stage.

### R2 — Model candidate comparison

- Accept two or more explicit positive model IDs.
- Fetch canonical model detail for every selected ID rather than comparing abbreviated search rows or assuming a name identifies a model.
- Align model-level selection facts and derived version-list summaries such as version count and latest publication timestamp.
- Do not silently choose a latest/primary version and present its detailed facts as model-level facts. Users who need file, trigger, scan, or recipe differences use version comparison with explicit version IDs.

### R3 — Version comparison

- Accept two or more explicit positive version IDs, in oldest-to-newest order when directional change language is desired.
- Fetch canonical detail for every selected version and the owning-model context required for model-level facts.
- Allow versions from the same or different owning models; always show owning model identity so cross-model comparisons cannot be mistaken for same-model history.
- Do not reorder versions by date, infer an upgrade path, or require the API’s version-list order to match input order.

### R4 — Curated aligned selection facts

The proposed core field set is below. Exact names and any additions/deletions are review topic D4, but the shipped set must retain the required coverage.

| Area | Model comparison | Version comparison |
|---|---|---|
| Identity | Model ID, name, URL, creator, type | Version ID/name, owning model ID/name/URL, creator, model type |
| Compatibility | Base-model set; version count and latest publication summary | `baseModel`, `baseModelType`, publication timestamp |
| Content/visibility | Tags, NSFW flag/level, model availability | Version NSFW level, version availability and status when supplied |
| License | `allowNoCredit`, `allowCommercialUse`, `allowDerivatives`, `allowDifferentLicense` | The same four grants from the owning model, explicitly identified as model-level |
| Adoption | Downloads, thumbs up/down, comments, tips | Version downloads and thumbs up/down; do not invent version comment/tip counts |
| Triggers | Not inferred from an arbitrary version | `trainedWords`, preserving missing versus empty |
| Files | Not aggregated into misleading model-wide file facts | File ID/name/type/primary/size, format/precision/size metadata, all supplied hash algorithms/values |
| Scans | Not aggregated into a safety score | Per-file pickle/virus results and scan timestamp |
| Recipe coverage | Not inferred from an arbitrary version | Example count, `hasMeta` count, non-null metadata count, and non-empty coverage for the supported recipe fields |

Large descriptions, raw example-image arrays, prompt text, image URLs, download URLs, and unreviewed training/paid-access objects are not part of the proposed core comparison. Existing `show --json` remains available for full source detail.

### R5 — Meaningful derived differences

- Report scalar changes with explicit from/to values against the baseline.
- Report exact-string set additions/removals for triggers, tags, base models, and commercial-use grants; API ordering alone must not create a difference.
- Report numeric deltas where meaningful, using integer counts rather than an opaque weighted score.
- For versions, identify file additions/removals and matched-file changes. Within matched files, report hash algorithms added/removed/changed, file metadata changes, and pickle/virus scan transitions.
- Detect the same binary carried to a new registry file ID when a unique SHA256 match proves it, and detect content replacement for a uniquely matched file name/type. Do not hide registry additions/removals merely because file names happen to match.
- Report recipe-coverage count changes, including images whose `hasMeta` is true but whose metadata is null or absent.
- Never emit a winner, recommendation, rank, safety verdict, compatibility verdict, or composite score.

### R6 — Truthful missing-data behavior

- Preserve `missing`, `null`, and supplied empty/zero values through projection and comparison.
- A field missing from any required source must be visibly indeterminate; two missing values must not be presented as confirmed equal.
- Do not infer version availability from model availability or from a status such as `Published`. If standalone detail omits version availability, the matching embedded version may supply it; otherwise it remains missing.
- Do not treat missing triggers as an empty set, missing scans as successful scans, or absent recipe metadata as an empty recipe.
- Any fallback or enrichment must retain its source so users and scripts can distinguish version data, owning-model data, and derived data.

### R7 — Human diff and stable machine output

- Default human output should present aligned facts and derived sections in a readable, deterministic form.
- A proposed human `--diff` view hides confirmed-equal rows while retaining changed and indeterminate information; it must say when there are no observed differences.
- `--json` should emit one curated, versioned object containing subject identities, aligned facts, value states, and derived differences. It must not be an array of raw model/version responses.
- Machine output must be deterministic for identical decoded input: input subject order is retained, field order is fixed, set-like values are sorted, and no generation timestamp or random ordering is introduced.
- stdout remains result-only; warnings and errors remain on stderr.

### R8 — Bounded, validated input

- Require at least two subjects and enforce a reviewed maximum before any network request.
- Reject malformed/non-positive IDs, unknown comparison kinds, unsupported flag combinations, and duplicate IDs before any network request.
- The approved maximum is five subjects per invocation.
- Preserve exactly one output value position per input subject; never silently deduplicate or drop a subject.

### R9 — Failure and compatibility behavior

- Validate the complete invocation before fetching.
- A selected model/version fetch or decode failure must produce no partial comparison on stdout and must use existing exit semantics: usage/not-found class `1`, network class `2`, rejected token `4`; exit `3` remains exclusive to `vet`.
- Required owning-model context follows approved D6: any failure aborts the comparison without a partial stdout result rather than returning incomplete license/availability facts.
- Reuse existing token, proxy, timeout, retry, body-limit, and verbose behavior.
- Add curated help, named-help routing, completion metadata, offline command tests, and integration coverage for every new CLI surface.

### R10 — No local or community expansion

Use only official model/version API responses and their embedded official example-image metadata. Do not read a local model library, hash local files, inspect local presence, persist comparison results, write an export file, crawl community images/recipes, or download model/image content.

## Observable acceptance criteria

The examples below use the approved syntax. Exact JSON field names/order are the design contract; any later change follows the versioning rules.

- **AC1 — Existing behavior is unchanged.** Existing command/unit/integration assertions for `search`, `show`, `vet`, and the other current commands pass without expected-output edits. Their JSON remains byte-contract compatible where currently asserted.
- **AC2 — Models align.** `civ compare models 2458426 260267 --json` fetches both canonical model bodies and returns two subjects in that order with aligned identity, base models, license, availability, adoption, tags, and version-summary fields. It contains no arbitrary “selected latest version” file/trigger payload and no recommendation/score field.
- **AC3 — Versions align.** `civ compare versions 3108589 3263843 --json` returns both explicit versions in input order, including owning-model identity, base model/type, triggers, files/hashes, scans, model-level license, availability state, and recipe coverage. The test proves required owning-model detail is requested only once for versions sharing a parent.
- **AC4 — Derived version changes are actionable.** Synthetic fixtures covering a base-model transition, trigger addition/removal, same-ID file replacement, same-SHA256 file rename/new ID, same-name content replacement, hash algorithm/value changes, and scan-result transitions produce the corresponding from/to, added/removed, matched-file, and hash-change records. Merely reordering triggers/files/hashes produces no false difference.
- **AC5 — License and availability are sourced correctly.** A version fixture whose standalone body omits availability uses the matching embedded version’s availability when present. Model license grants are labeled model-level. Model availability and `status: Published` are never substituted for missing version availability.
- **AC6 — Recipe coverage is transparent.** Fixtures with zero images, explicit true/false/null/missing `hasMeta`, `hasMeta: true` with null metadata, non-null but sparse metadata, and non-empty supported recipe fields produce independently correct counts and deltas. No image/community request occurs and no recipe-quality score is emitted.
- **AC7 — Missing is not empty or equal.** Fixtures distinguish an omitted trigger list from `[]`, an omitted license boolean from `false`, explicit null from empty, and an omitted scan from `Success`. Missing values appear as indeterminate in both human and machine views, and derived add/remove operations are not fabricated from incomplete inputs.
- **AC8 — Human diff is selective.** Given different IDs whose compared facts are all equal, `--diff` prints a clear no-observed-differences result; given changed and missing fields, it hides confirmed-equal rows but shows changes and a distinct indeterminate section/state. Full human output still exposes the aligned source facts.
- **AC9 — JSON is stable and curated.** Repeated runs against identical `httptest` responses produce byte-identical JSON. Assertions verify `schemaVersion`, comparison kind, subject order, fixed field order, value states, baseline-relative derived changes, and empty-array/object behavior. The payload is not raw `Model`/`ModelVersion` concatenation and includes no fetch timestamp.
- **AC10 — Usage and failure behavior are atomic.** Too few/many subjects, duplicate or invalid IDs, an unknown kind, and any rejected flag combination fail before HTTP. A 404 for any selected subject returns exit `1` with empty stdout; injected network/token failures map to `2`/`4`. No selected subject is silently omitted.
- **AC11 — Request and scope boundaries hold.** Offline route assertions observe only the expected official model/version GETs. Tests with local files present prove they are neither opened nor hashed, and no cache/export/community/image-browse path is called.
- **AC12 — CLI surface is discoverable and covered.** `civ compare help`, `civ compare -h`, and `civ help compare` agree on stdout with exit `0`; top-level help and completion include compare. `scripts/integration-test.sh` covers model JSON, version JSON, human diff, help/usage, and failure/timeout behavior in its normal offline/live split.

## Out of scope

- Changing existing command syntax, output, flags, search filters, or search ranking.
- Automatic candidate discovery, name-based selection, pagination, or passing an entire search result set into compare.
- A best-model recommendation, weighted/compatibility/safety score, or automatic selection for a workflow.
- Local library discovery, local file hashing/presence, resource resolution, or installation checks.
- Recipe generation, recipe-quality evaluation, raw prompt diffing, or making compare a prerequisite for recipe/resolve.
- Community recipe sampling, image browsing/downloading, community crawling, or persistent caching.
- CSV/Markdown/file export, redirection helpers, or a comparison history/database. Normal `--json` stdout is the only machine-output addition.
- Model/file downloads, uploads, writes to Civitai, or changes to authentication/NSFW gating.
- Exhaustive diffing of every upstream field, large HTML descriptions, arbitrary unknown JSON, or unreviewed training/paid-access schemas.

## Approved decisions

| ID | Approved decision |
|---|---|
| D1 | Canonical syntax is `civ compare models <id> <id> [...]` and `civ compare versions <id> <id> [...]`; no flag-based kind ambiguity or aliases. |
| D2 | Minimum 2 subjects, maximum 5, duplicate IDs rejected before any fetch. |
| D3 | Full aligned human output by default; `--diff` hides confirmed-equal rows and retains changed/indeterminate fields; `--diff --json` is a usage error because JSON always carries facts plus derived changes. |
| D4 | Use the curated R4 field registry; exclude descriptions, raw prompts/images, download URLs, training/paid-access objects, and arbitrary unknown fields. |
| D5 | Subject order is input order. The first subject is the directional baseline; derived changes are baseline→each later subject. No date/name/ID sorting. |
| D6 | Strict all-or-nothing fetches: any selected subject or required owning-model fetch/decode failure exits with no partial comparison on stdout. |
| D7 | File pairing uses unique file ID, then unique SHA256, then unique exact `(name,type)`; match basis is explicit and ambiguous/unmatched files remain added/removed, never guessed. |
| D8 | Fetch sequentially with invocation-local deduplication of distinct owning models; no new concurrency flag in the MVP. |

## Planning status

All product and output-contract decisions for this child are resolved. Implementation remains unapproved and unstarted pending parent convergence and explicit task approval.
