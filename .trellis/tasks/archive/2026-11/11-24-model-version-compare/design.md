# Technical design: model and version comparison

> Status: approved design for implementation review. D1–D8 are decided; this document does not authorize implementation.

## 1. Behavior gap and change boundary

Today a user can search for models and inspect one model or version at a time, but there is no operation that:

1. fetches several explicitly selected canonical resources,
2. projects the same curated facts for each resource,
3. preserves unavailable versus empty values, and
4. derives transparent candidate or version changes.

That behavior belongs in a new comparison orchestration path and a pure comparison engine. It does not belong in `search`, whose raw JSON passthrough and result table are existing contracts, or in `show`, whose single-subject output should remain unchanged.

### Proposed implementation boundary

- Add one `compare` dispatch route and command; do not add comparison flags to existing commands.
- Reuse `api.Client.GetModel` and `api.Client.GetVersion`; add no HTTP endpoint or outbound bypass.
- Add presence-aware comparison snapshots at the API decode boundary without changing the field types or JSON shape of existing `api.Model` and `api.ModelVersion`.
- Add a pure `internal/compare` package for projection, normalization, and derived differences.
- Keep command-specific human rendering in `internal/cmd`, following the existing `show`/`vet` pattern, and reuse `internal/report` for tables and JSON encoding.
- Change help, completion, integration, and user documentation only as required by the new surface.

### Explicit non-boundaries

No local-library/resolver code, hash cache, export writer, community client, recipe workflow, model downloader, persistent store, new third-party module, scoring package, or changes to sibling task directories are part of this design.

## 2. Architecture and dependency direction

```text
main.go
  └─ cmd.Compare
       ├─ parse/validate kind, IDs, --diff, global flags
       ├─ api.Client
       │    ├─ GET /models/{id}
       │    └─ GET /model-versions/{id}
       ├─ api presence-aware snapshot decoders
       ├─ internal/compare pure builder/differ
       │    └─ versioned comparison Result (no I/O)
       └─ output
            ├─ cmd human renderer + report table/format primitives
            └─ report.WriteIndent(Result) for --json
```

Dependency direction remains one-way:

```text
main -> cmd -> {api, compare, report}
              compare -> api
              api -> app/proxy
              report -> stdlib only
```

`internal/compare` owns domain and machine-contract types but performs no HTTP, printing, flag parsing, file access, or process exit. `internal/cmd` owns fetch order, error attribution, and rendering. `internal/report` remains unaware of Civitai and comparison types; `WriteIndent` already accepts `any`.

A new package is preferable to placing all comparison arithmetic in `cmd`: set operations, missing-data semantics, file matching, and JSON stability need a substantial pure test matrix. Recipe and resolve remain product-level siblings; neither is required to import this package.

### Expected code/document touch points after approval

| Path | Responsibility |
|---|---|
| `main.go` | Add dispatch and one usage row only |
| `internal/cmd/compare.go` | Candidate syntax, options, validation, canonical fetch orchestration, error wrapping |
| `internal/cmd/compare_render.go` | Full and `--diff` human views over the shared Result |
| `internal/cmd/compare_test.go`, `compare_render_test.go` | Offline command, output, request, and failure tests |
| `internal/api/compare_types.go` (or focused equivalent) | Presence-aware model/version/file snapshots and tolerant decoders |
| `internal/api/compare_types_test.go` | Missing/null/empty, standalone/embedded, and numeric-drift tests |
| `internal/compare/compare.go`, `diff.go` | Field registries, Result contract, normalization, derived differences |
| `internal/compare/*_test.go` | Pure table-driven semantic and determinism tests |
| `internal/cmd/help.go`, `completion.go` and associated tests | Discoverability and drift guards |
| `scripts/integration-test.sh` | Required CLI-surface smoke coverage |
| `README.md` | Reviewed syntax, limits, field semantics, examples, and exit behavior |
| `testdata/FIXTURES.md` and any new `testdata/*.json` | Provenance for any newly captured verbatim live bodies; synthetic edge cases stay inline in tests |

If implementation reveals that an existing file can own a responsibility without expanding its cohesion, the exact file names may change, but the layer boundaries may not.

## 3. Proposed CLI contract

### 3.1 Approved syntax (D1)

```text
civ compare models <modelId> <modelId> [<modelId>...] [--diff] [--json]
civ compare versions <versionId> <versionId> [<versionId>...] [--diff] [--json]
```

Rationale:

- The kind is explicit; a numeric model ID can never be accidentally treated as a version ID.
- Positional order is natural for copying IDs from search/show and for old-then-new version review.
- A single command fits current `main.go` dispatch and curated-help conventions.
- Existing `show -v` remains single-version drill-down; compare does not overload `-v` or the global `--verbose` contract.

Alternatives such as singular kinds, `-v` flag syntax, or separate top-level commands are rejected. The implementation has exactly one canonical model form and one canonical version form; aliases are not added.

### 3.2 Validation (D2, D3)

Perform all checks before constructing/fetching with the client:

1. Exactly one supported kind is present.
2. The kind consumes all remaining positional arguments as positive decimal IDs.
3. Subject count is at least two and at most five.
4. Duplicate IDs within the invocation are usage errors; do not silently deduplicate.
5. `--diff --json` is a usage error because JSON already carries both facts and derived changes.
6. Bare `help` and `-h`/`-help/--help` follow the shared runner and do no validation, network, or config work beyond existing runner behavior.

Usage errors identify the offending kind, ID, count, or flag combination. They map to exit `1` and emit no stdout result.

### 3.3 Input order and baseline (D5)

- `subjects[i]` always corresponds to input ID `i`.
- `baseline` is index `0`, not a separate user-selected ID and not a recommendation.
- For every later subject `i`, derived operations compare subject `0` to subject `i`.
- No sorting by `publishedAt`, downloads, rating, name, or ID occurs before comparison.
- Human help explicitly tells version users to supply the older/reference version first when they want added/removed language.

This yields N-1 directional derived comparisons. Confirmed N-way equality/difference is still calculated across all values independently of the baseline.

## 4. Canonical source and fetch contract

### 4.1 Model comparison

For each input model ID:

1. Call `GetModel(ctx, id)`.
2. Decode the body into a comparison model snapshot.
3. Project model-level fields and summary facts derived from the embedded version list.
4. Discard the raw body and unneeded large description/image/version detail as soon as projection is complete.

Do not call `SearchModels`, paginate, select versions automatically, or reuse search items as canonical model detail.

### 4.2 Version comparison

For each input version ID:

1. Call `GetVersion(ctx, id)` and decode the standalone version snapshot.
2. Require a positive owning `modelId`; its absence prevents reliable license/identity enrichment and is an error under the approved strict policy.
3. Fetch each distinct owning model once with `GetModel(ctx, modelId)` and cache it only in memory for this invocation.
4. Locate the selected version ID in the owning model’s embedded `modelVersions` when available.
5. Build the version snapshot from standalone detail plus explicitly sourced parent/embedded fields.

Versions from different parents are valid. With N versions from distinct parents the MVP performs 2N GETs; with one shared parent it performs N+1. Parent deduplication is request optimization only and never changes subject order or values.

### 4.3 Field-source precedence

| Fact | Primary source | Permitted fallback/enrichment | Forbidden inference |
|---|---|---|---|
| Version identity/base/triggers/files/images/stats | Standalone version detail | None required | Embedded summary substituted wholesale for detail |
| Version availability | Standalone `availability` when supplied | Matching embedded version’s `availability` | Model availability, `status`, NSFW, or download URL |
| Version status | Standalone `status` when supplied | Matching embedded `status` only if explicitly decoded and reviewed | Treating status as availability |
| Creator/type/model name | Owning model detail | Standalone model summary for display only if strict parent policy is relaxed | Guessing from version name |
| Four license grants | Owning model detail | None | Version file metadata or description text |
| Recipe coverage | Standalone version’s embedded `images[]` | None | `/images` browse, image downloads, community sampling |

Each projected value retains a source label such as `version`, `embeddedVersion`, `model`, or `derived`. The source is per value, not merely per field, because availability can come from different response forms across subjects.

### 4.4 Failure atomicity (D6)

The approved initial behavior is strict:

- Any selected-resource or required-parent request/decode error aborts the command.
- Wrap errors with comparison and subject context while preserving `%w` classification, for example `compare: version 3263843: %w` and `compare: parent model 2458426 for version 3263843: %w`.
- Build the complete Result in memory before writing anything. stdout remains empty on failure.
- Existing `execute` maps network to `2`, rejected token to `4`, and not-found/usage/decode to `1`. Compare does not use vet’s exit `3`.
- Upstream field omission in an otherwise valid successful response is not a fetch failure; it becomes explicit missing/indeterminate data.

This deliberately differs from vet’s best-effort parent enrichment because comparison promises aligned license/availability facts. Best-effort parents are not an approved alternative; adding them would require a new public incompleteness/error contract.

### 4.5 Execution and resource bounds (D2, D8)

The MVP uses sequential fetches. This keeps error attribution simple and bounds peak raw-body memory despite the client’s 5 MiB per-response limit. Snapshots retain only curated fields and recipe counts, not raw prompt/image payloads.

The existing client still applies token/proxy/timeout/retry/User-Agent/body-limit behavior. Its default concurrency gate remains available, but compare does not add a concurrency flag or spawn fetch goroutines initially. If profiling shows unacceptable latency at the approved subject limit, bounded concurrency can later reuse the existing client without changing output order; that change needs cancellation/error-aggregation and race tests.

## 5. Presence-aware decode and canonical snapshots

### 5.1 Do not change existing JSON contracts

`show --json` marshals existing `api.Model`/`api.ModelVersion` values. Replacing their booleans/strings with pointers or generic optional fields to support compare would change that public output and complicate current callers.

Instead, add comparison-specific snapshot decoding in `internal/api`:

- Decode known fields with the same tolerant rules as current types.
- Retain presence/null metadata for compared scalar/container fields.
- Preserve unknown enum values verbatim.
- Use `json.Number` for drift-prone numeric values and integers for known count/stat fields where safe.
- Ignore unknown upstream fields rather than placing them in machine output.
- Reuse existing fixture provenance and add focused synthetic inline bodies for absent/null/empty drift.

A snapshot may internally combine a typed value with a field-presence index decoded from `map[string]json.RawMessage`. That map is an API-boundary implementation detail, not part of the public comparison JSON and not iterated to define field order.

### 5.2 Tri-state value contract

Every aligned value has:

- `state: "value"`: a supplied non-null value, including `false`, `0`, `""`, `[]`, and `{}`;
- `state: "null"`: an explicitly supplied JSON null;
- `state: "missing"`: no supplied field;
- `source`: the source response/projection that supplied the state;
- `value`: the curated typed value when state is `value`, otherwise JSON null.

A field also has `complete` and `comparison`:

- `complete` is false if any subject value is missing. Explicit null is supplied and therefore complete.
- `comparison: "equal"` requires all subjects to have equal supplied states/values and no missing values.
- `comparison: "different"` means at least two supplied values/states differ, even if another subject is missing.
- `comparison: "indeterminate"` covers all-missing and equal-known-values-plus-missing cases where equality cannot be confirmed.

Examples:

| Values | complete | comparison |
|---|---:|---|
| `false`, `false` | true | equal |
| missing, missing | false | indeterminate |
| `[]`, missing | false | indeterminate |
| `1`, missing, `2` | false | different |
| null, null | true | equal |
| null, `[]` | true | different |

Derived operations have their own state. They run only when their required baseline and candidate operands are supplied and non-null; otherwise they return `indeterminate`, never empty added/removed arrays that look like “no change.”

## 6. Curated field projection

The field registry is an ordered slice per comparison kind, not a map. It is the single owner of public field keys, source projection, value type, normalization, and derived-operation kind. Human and JSON views consume the same Result.

### 6.1 Model fields

Proposed registry groups:

1. **Identity:** ID, name, URL, creator username, type.
2. **Selection/compatibility:** exact-string `baseModels` set, tags set, NSFW flag/level, availability.
3. **License:** four independent grant fields; `allowCommercialUse` is a set, not a comma-joined display string.
4. **Adoption:** model download/thumbs-up/thumbs-down/comment/tipped counts. Human ratio formatting may reuse `report.RatioText`; JSON retains source counts and transparent integer deltas.
5. **Version-list summaries:** version count and the maximum successfully parsed RFC3339 `publishedAt` as `latestPublishedAt`. Missing/invalid timestamps make the summary incomplete or indeterminate rather than becoming a zero date or a confirmed latest value. No file, trigger, recipe, or scan field is projected from the latest version.

The summary `latestPublishedAt` is a named derived fact only; it never selects a representative version for other fields.

### 6.2 Version fields

Proposed registry groups:

1. **Identity:** version ID/name, parent model ID/name/URL/type, creator.
2. **Compatibility/publication:** base model, base-model type, publication timestamp, status, availability, NSFW level.
3. **License:** the four parent-model grants, labeled with `model` source for each subject.
4. **Adoption:** version downloads and thumbs up/down only.
5. **Triggers:** exact-string set with missing/null/empty preserved.
6. **Files:** curated file entries containing registry ID, name, type, primary flag, sizeKB, file format/FP/size metadata, scan results/timestamp, and a sorted list of `{algorithm, value}` hashes. Download URLs are excluded.
7. **Recipe coverage:** total embedded examples, explicit `hasMeta` true/false/null/missing counts, non-null `meta` count, declared-but-unavailable count where derivable, and fixed per-field non-empty counts for prompt, negative prompt, steps, cfgScale, sampler, scheduler, seed, width, and height.

“Non-empty” is a coverage observation, not a quality judgment. In particular, an intentionally empty negative prompt is not called invalid. Width/height count as non-empty when supplied with a non-null JSON value, including ComfyUI object forms; the objects themselves are not compared or emitted.

## 7. Derived-difference algorithms

### 7.1 Scalars, sets, and numbers

- **Scalar transition:** for baseline `b` and candidate `c`, emit `{from: b, to: c}` when both are supplied and differ. This covers base model/type, availability, license booleans, status, and similar fields.
- **Exact-string set difference:** deduplicate exact strings, compare case-sensitively, and sort output lexicographically. Do not trim, lowercase, or semantically normalize trigger words/tags/base models/commercial grants without a separate reviewed rule. Input order and duplicate repetitions alone do not create differences.
- **Integer delta:** emit `from`, `to`, and `to - from` for supplied integer statistics and coverage counts. Do not convert counts into a weighted score. Floating file sizes remain source facts; no approximate size verdict is needed.
- **Timestamp:** compare supplied timestamp strings as source facts. Only the explicitly named model summary selects a latest timestamp: parse RFC3339 values and compare instants rather than relying on offsets to sort lexicographically. Missing/invalid values make that derived summary incomplete/indeterminate under the value-state rules; they never become a zero date or a confirmed latest value.

### 7.2 File pairing (D7)

For each baseline/candidate version pair, pair unpaired files using this deterministic evidence cascade:

1. **Registry ID:** the same positive file ID, unique on both sides. This detects changes to the same registry file entity.
2. **Binary SHA256:** the same non-empty SHA256, unique among remaining files on both sides. This detects a carried binary with a new file ID and can expose rename/metadata/scan changes.
3. **Name and type:** the same exact `(name, type)` pair, unique among remaining files on both sides. This detects likely content replacement while recording that the match is name/type-based.

Record `matchBasis: "fileId" | "sha256" | "nameType"` on every pair. Never pair by size alone, fuzzy name, array position, or an ambiguous SHA/name. Ambiguous or residual files remain explicit added/removed entries; include an ambiguity reason where known instead of guessing.

Within each pair:

- Compare file name/type/primary/size/format/FP/size metadata and scan timestamp as aligned facts.
- Compare pickle and virus scan results independently with from/to values. A timestamp-only rescan is not presented as a scan-result safety transition.
- Compare hashes by exact algorithm name. Emit algorithm added/removed and value changed with old/new values. Do not treat different algorithms as interchangeable merely because one is a prefix/derivative of another.
- For known hexadecimal hash values, case-only normalization may be used for equality while original values remain in aligned facts. Unknown algorithms remain exact strings.

Unmatched registry file additions/removals stay visible even when another pair has the same file name. This preserves both registry-level and content-level meaning.

### 7.3 Trigger, license, availability, and scan differences

These are separate transparent facts, not inputs to a verdict:

- Trigger additions/removals use the exact-string set algorithm and require both trigger fields to be present non-null.
- License compares each boolean independently; commercial grants additionally have set add/remove details.
- Availability/status changes are scalar transitions only when supplied. Unknown/missing availability never becomes “Private,” “Unavailable,” or “Public.”
- Scan changes remain attached to the matched file and match basis. Missing scans are missing, not `Success`; an added file has no implied scan equivalence to a removed file.

### 7.4 Recipe-coverage differences

Calculate counts independently before comparing:

- `imagesTotal = len(images)`;
- `hasMetaTrue` and `hasMetaFalse` count only explicitly supplied boolean values, with separate `hasMetaNull` and `hasMetaMissing` counts so an absent flag is not treated as false;
- `metaPresent = count(meta is non-null)`;
- `declaredMetaMissing = count(explicit hasMeta=true and meta is null/absent)`;
- each recipe field has a non-empty count over non-null metadata objects.

Derived count changes use integer from/to/delta records. If the images field itself is missing rather than supplied empty, coverage is indeterminate. If it is supplied as `[]`, all coverage counts are valid zero. No prompt similarity, recipe ranking, or metadata completeness score is calculated.

## 8. Public output contracts

### 8.1 Shared Result

Human and machine rendering consume one Result so they cannot disagree about source values or differences. The following is the approved structural contract; final field constants are implementation-owned within D4:

```json
{
  "schemaVersion": 1,
  "kind": "version",
  "baseline": 0,
  "subjects": [
    {"index": 0, "modelId": 2458426, "versionId": 3108589, "name": "turbo-v1.0"},
    {"index": 1, "modelId": 2458426, "versionId": 3263843, "name": "turbo-v1.1"}
  ],
  "fields": [
    {
      "key": "baseModel",
      "valueType": "string",
      "values": [
        {"state": "value", "source": "version", "value": "Anima"},
        {"state": "value", "source": "version", "value": "Anima"}
      ],
      "complete": true,
      "comparison": "equal",
      "derived": null
    }
  ]
}
```

Contract invariants:

- Top level always has `schemaVersion`, `kind`, `baseline`, `subjects`, and `fields`.
- `kind` is `model` or `version`; `schemaVersion` starts at integer `1` when first released.
- Subject identity includes model IDs for model comparisons and both model/version IDs for version comparisons. It never contains API tokens or request headers.
- `values.length == subjects.length` for every field, and `values[i]` belongs to `subjects[i]`.
- Field entries come from the fixed registry and always use the same shape, including `derived: null` where no derivation applies.
- Set-like source values and hash entries are sorted; files have a documented deterministic canonical order; derived comparisons are ordered by candidate subject index. Core output does not depend on Go map iteration.
- No `generatedAt`, current time, random ID, local path, raw response envelope, or full prompt/image payload is included.
- `schemaVersion` covers value-state semantics, field-key meaning, and derived-operation shape. Pre-release implementation may refine concrete names only through a documented contract update; after release, incompatible changes require a version bump and release notes.

### 8.2 Human full view

The default view should contain:

1. A numbered subject header with explicit model/version IDs and names.
2. Aligned fact rows/sections using the same registry order as JSON.
3. Derived baseline-to-subject sections with labels such as `changed`, `added`, `removed`, `match: sha256`, and numeric `delta`.
4. Explicit `missing`, `null`, and empty representations; `missing` must not share the same display as a successful empty comparison.

Use ASCII structural text, no color requirement, no Unicode diff arrows, and no terminal-control animation. Reuse table/width/truncation helpers where appropriate, but never truncate hash values, IDs, or the actual added/removed trigger strings in machine output. Human truncation must not change the underlying Result.

### 8.3 Human `--diff` view (D3)

- Suppress fields with `comparison: "equal"` and no separately meaningful derived detail.
- Retain `different` fields and their derived operations.
- Present fields with missing operands in a distinct `indeterminate`/missing state rather than claiming either equality or a concrete add/remove.
- Preserve subject and baseline headers so a diff remains interpretable.
- If every field is confirmed equal, print `No observed differences.`; if nothing is different but some fields are indeterminate, say that differences could not be fully determined instead.
- Under the approved contract, `--diff --json` is rejected. JSON consumers select `fields[key]` and inspect `comparison`/`derived` without a second filtered schema.

## 9. Compatibility, errors, and observability

- Existing source structs and their JSON field types remain untouched; comparison snapshots are additive.
- Existing endpoint methods and `Client.Get` remain the only outbound path. No new API token scope or authentication behavior is introduced.
- Existing exit codes retain their meanings. In particular, compare 404 is exit `1`, not vet’s exit `3`.
- The shared runner supplies global flags after the command name, curated help, verbose setup, and error hints. Command errors wrap with `%w` and subject context.
- Stdout is written only after every required fetch, decode, projection, and diff succeeds. Warnings, if any, use existing stderr logging. Strict parent failures are errors rather than silent warnings.
- Help pages, named-help routing, top-level usage, static completion metadata, and integration assertions are updated atomically with command registration.
- Unit tests use `httptest` and fixtures only. Live integration is the existing manual/weekly drift canary, not a unit-test dependency or release-blocking addition.

## 10. Tradeoffs

| Choice | Benefit | Cost / rejected alternative |
|---|---|---|
| Canonical detail per subject | Comparable fields and accurate triggers/files/license | More requests than reusing search items; search items are cheaper but incomplete |
| Parent fetch for each distinct version parent | Real model identity/license and possible embedded availability | Up to 2N requests; version-only output would omit required selection facts |
| Strict all-or-nothing | No partial comparison is mistaken for complete evidence | Less tolerant of parent outage than vet; best-effort would need a new incompleteness contract |
| Curated versioned JSON | Stable Agent consumption and explicit derived semantics | Requires field ownership and schema review; raw passthrough is easier but not a useful comparison contract |
| Baseline-to-each-later diffs | N-1 clear directional changes and simple old/new usage | Not a complete all-pairs matrix; all-pairs creates quadratic noise, adjacent-only loses baseline comparison |
| Evidence-cascade file matching | Detects registry edits, carried binaries, and replacements | More logic than ID/name-only matching; ambiguity must be retained rather than hidden |
| Separate comparison snapshots | Existing show/vet JSON remains stable | Some decode projection is duplicated; changing existing scalar types would break consumers |
| New pure compare package | Deep, offline-testable semantics and one human/JSON source | Adds a package; command-local logic would reduce files but mix orchestration with complex diff rules |
| Sequential initial fetches | Simple errors, deterministic tests, bounded peak memory | Higher latency; concurrency can be revisited after limit/profiling review |
| No representative latest version in model compare | Avoids false model-level file/trigger claims | Users must make a second explicit version selection for detailed diffs |

## 11. Rollout and rollback shape

### Rollout

1. **Contract review:** the field registry, input bound, JSON state semantics, and file-pairing rules are approved in D1–D8; implementation review verifies they are represented consistently in tests and docs.
2. **Internal foundations:** add presence-aware snapshots and the pure comparison engine with offline tests; no user-facing route yet.
3. **Additive CLI exposure:** wire command, human/JSON output, help, completion, and docs together. Do not partially register a command without its help/completion/integration surface.
4. **Verification:** run the full offline quality trio, build, and the updated integration script in its normal manual/weekly context.
5. **Release:** treat the first published JSON schema as version `1`; document the baseline, missing-data, limit, and request-cost behavior.

No migration, feature flag, persistent data, or user configuration is required. The additive command is the rollout boundary.

### Rollback

- **Before CLI registration:** revert the additive API snapshots and `internal/compare` package; existing behavior is untouched.
- **After registration but before release:** revert the complete compare change set, including dispatch, help, completion, integration, and documentation. There is no persisted state to clean.
- **After release:** prefer a forward fix for comparison correctness because downstream scripts may already consume schema `1`. If withdrawal is necessary, revert the whole additive surface and explicitly document command/schema removal; do not leave a partially functioning command or silently change value semantics.
- **Any failed validation:** stop rollout at the ordered checkpoint in `implement.md`, revert only that stage’s uncommitted additive changes, rerun existing command tests, and return to planning if the failure exposes a contract ambiguity.
