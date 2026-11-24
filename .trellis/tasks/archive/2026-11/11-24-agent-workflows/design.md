# Agent workflow integration design

> Status: proposed design for review. No implementation is authorized by this document.

## 1. Design objective

Add task-oriented projections and evaluators over the existing registry client without turning the codebase into a second REST framework or changing established commands. The key boundary is:

```text
upstream API/local files
  -> tolerant API DTOs and content identity
  -> small typed domain projections/evaluators
  -> command-specific versioned output contracts
  -> human rendering or Agent JSON
```

The domain layer owns meaning; `report` owns presentation only. Commands must not each invent their own untyped extraction of `meta`, resource identifiers, compatibility, or readiness.

## 2. Boundaries and ownership

### Existing packages

- `internal/api` remains the only outbound HTTP owner. Add endpoint methods and tolerant DTO fields only when a reviewed consumer needs them.
- `internal/cmd` owns flags, input validation, orchestration, error-to-exit mapping, and curated help.
- `internal/report` owns human table/text formatting and writer helpers; it does not define readiness, recipe semantics, or comparison rules.
- `internal/hashcache` remains the existing optional size/mtime hash cache. This scope does not replace it with a model library.
- `internal/skill` owns embedded Agent Skill assets and their installation behavior.

### Proposed pure domain ownership

Introduce small packages only when a reviewed child has a real need and at least one concrete consumer:

- **Resource reference/identity owner:** parses reviewed exact references and represents model/version/file/hash identity. It performs no HTTP and no printing. If a second child needs the same grammar, this logic must be shared rather than copied.
- **Recipe projection owner:** converts tolerant API DTOs into normalized recipe facts, coverage, provenance, and warnings. It does not choose a “best” prompt.
- **Resolve evaluator:** consumes canonical resource facts and local hash evidence, then returns a typed readiness result. It does not infer local files from filenames or download anything.
- **Compare projector:** aligns model/version facts and computes explicit differences. It does not assign a weighted score.

Avoid a generic `domain`, `core`, or `utils` package with unrelated responsibilities. Package names and dependency direction must be finalized in child design review before code is written.

## 3. Contract strategy

### 3.1 Additive versioned JSON

New commands should use explicit schema versions in their normalized envelopes. Existing raw/passthrough and decoded JSON outputs are the compatibility baseline; `11-24-search-show-compact` has an approved default-compact policy, approved D2 field registry/view validation, and `--view full` as the exact legacy path for the three scoped search/show forms. No other legacy output may drift as a side effect of shared implementation.

Approved common conventions:

- `schemaVersion`: command-family-specific version identifier.
- `source` / `provenance`: exact upstream identifiers and URLs used for the result.
- `warnings`/`issues`: machine-readable code plus concise English message; operational warnings still go to stderr.
- `missing`, `unresolved`, or coverage counts: explicit rather than omitted silently.
- Exact references include numeric model/version IDs and, where available, AIR and primary-file SHA256. They are not replaced by display names.

Do not force every command into one artificial envelope. Shared vocabulary is valuable; identical response shapes are not.

### 3.2 Summary versus detail

- Every list/summary view is prompt-free and large-field-free by contract, including nested example arrays. Selection records carry identifiers, small scalar facts, coverage, and provenance only.
- Recipe example detail returns prompt/negativePrompt and other approved large fields only for one explicitly selected official example or community image. A list must never include those fields merely because its parent version/detail response contained them.
- Official recipe summaries use first-N source order with approved default 3 and range 0–10; zero returns context/counts only, and count 1 remains prompt-free. Detail selection always uses the separate singular selectors.
- Community list returns bounded summaries and coverage; selected-image detail is the separate explicit `civ recipe image <imageId>` operation with one exact bounded request and no cross-invocation state.
- Compare returns aligned fields and differences, not concatenated raw model/version bodies, prompts, descriptions, or raw images.
- Resolve returns one result per requested resource plus issues/summary, not full version objects.
- Explicit legacy full/prompts views remain compatibility exceptions because the caller deliberately requested their established detail payloads; they are not the default for any list.

Human output and JSON are projections of the same typed result so their facts cannot diverge.

The first-draft scalar conflict is resolved by one shared normalizer: valid numeric strings in known fields become JSON numbers, LoRA weights follow the same rule, seed remains an exact decimal string, missing/null stays null, and invalid/non-scalar shapes produce null plus a structured issue. Number-versus-numeric-string is not retained as a semantic distinction; arbitrary unknown fields and units are not inferred.

The detail-flow conflict is also resolved: official detail uses `--example <sourceIndex>` on the exact-version recipe invocation, while community detail uses a separate `civ recipe image <imageId>` invocation with one exact bounded `imageId+withMeta=true+type=image+limit=1` request. No cross-invocation in-memory handoff or same-invocation alternative remains.

### 3.3 Reference grammar

The approved command-specific grammar is:

- recipe: exact `civ recipe <versionId>`, with `--example <sourceIndex>` for official detail and `civ recipe image <imageId>` for community detail;
- compare: explicit numeric model or version IDs under `civ compare models|versions`;
- resolve: local path, full SHA256, versioned AIR, or `version:<id>` only.

No command guesses a bare integer across kinds or accepts names/implicit latest selection. Unsupported or ambiguous forms are usage errors before network activity. AIR is output metadata and, for recipe/resolve, an exact-version input only when it carries `@versionId`.

## 4. Child data flows

### 4.1 Recipe core

```text
reviewed version/image reference
  -> resolve the appropriate existing endpoint(s)
  -> tolerant model/version/image DTOs
  -> normalized recipe projection
  -> prompt-free example summaries + provenance/coverage for list calls
  -> full approved generation fields only for one explicitly selected example detail
```

Each recipe invocation makes exactly one `/model-versions/{id}` request. There is no owning-model/license fallback request; absent model-summary fields are exposed as missing rather than fetched separately.

### 4.2 Resource resolver

```text
explicit refs (local path/hash/exact remote version)
  -> canonical requested resource set
  -> registry identity + local SHA256 evidence
  -> per-resource local status
  -> cross-resource compatibility/readiness rules
  -> typed ready/not-ready result with reason codes
```

The MVP does not require image/workflow parsing or whole-directory indexing. A later input adapter may construct the same requested resource set from an image or workflow, but that is not a hidden MVP dependency.

The readiness evaluator is pure and independent of process exit mapping. The approved mapping is ready `0`, completed not-ready `5`, usage/local-I/O/decode `1`, network `2`, token `4`; the engine therefore returns a typed result and the command maps it.

### 4.3 Model/version compare

```text
explicit model refs OR explicit version refs
  -> bounded endpoint fetches (deduplicate shared owning models)
  -> normalized comparison rows
  -> field equality/difference and selected set diffs
  -> full or diff-only view
```

Version comparison should derive trigger additions/removals and file/hash sameness from canonical values. Model comparison should not expand every version's full images/files merely to produce a summary.

### 4.4 Community recipe source

```text
exact modelVersionId + bounded options
  -> GET /images?modelVersionId=...&withMeta=true&limit=...
  -> tolerant free-form meta decoding
  -> per-image recipe summary + resource references + provenance
  -> returned/withMeta/usable coverage and per-image observations only (no aggregation)
```

No automatic cursor exhaustion is allowed. A next cursor may be returned as provenance/continuation data if useful, but following it requires another explicit bounded invocation.

Selected community detail is a separate flow:

```text
civ recipe image <imageId>
  -> GET /images?imageId=...&withMeta=true&type=image&limit=1
  -> require exactly one matching visible item
  -> selected single-image detail only; no prior-list cache or broadened retry
```

Official example detail instead uses `civ recipe <versionId> --example <sourceIndex>` and one ordinary version-detail request.

### 4.5 Existing search/show compaction

```text
existing /models, /models/{id}, or /model-versions/{id} response
  -> same tolerant source data used by the current command
  -> purpose-driven compact projection with no additional request
  -> reviewed compact default or opt-in view
  -> optional full/legacy path preserving the exact prior raw/decoded behavior
```

Compact projection must preserve model/version selection identifiers and must not silently drop records. Search's legacy path is raw-items passthrough, whereas show's legacy path is the existing decoded-object JSON; a generic `full` implementation must not incorrectly claim both are upstream raw responses. Default compact and the D2 field registry/view validation are approved.

### 4.6 Final grouped help

```text
final delivered command inventory
  -> one reviewed overview command-list projection
  -> Discover / Recipes / Local models / Utilities
  -> existing global/reference sections unchanged
```

This is presentation-only and explicitly runs after every other child. It must not create a second dispatch mechanism or command-metadata registry, advertise deferred commands, or alter named help/completion. Compare before/after output from the final pre-grouping snapshot so only the command-list block changes.

## 5. API and fixture strategy

- Reuse `/models/{id}`, `/model-versions/{id}`, and `/model-versions/by-hash/{hash}` where sufficient.
- Add `/images` only for the reviewed community/image-detail use case. Use `url.Values` and preserve the separate images-endpoint NSFW decoding rules.
- AIR, `civitaiResources`, reaction statistics, and free-form generation fields are decoded additively and tolerantly. Unknown metadata is preserved or ignored, never validated against a closed schema.
- Do not add bulk hash endpoints solely for this task tree; without a persistent library, the reviewed explicit-input MVP does not need 10,000-hash discovery.
- Store representative verbatim fixtures with provenance. Include null meta, partial meta, multiple resources, missing hashes, model/version drift, and unknown fields. Unit tests never require live network.

See `research/api-evidence.md` for source links and current evidence.

## 6. Compatibility and error behavior

- Existing exit codes 0/1/2/3/4 remain unchanged for old commands.
- Resolve's `not_ready` mapping is deliberately deferred to product review. Internally it is a typed evaluation outcome, not an API error.
- A failed primary fetch fails the relevant command using existing error classes. A failed secondary enrichment may degrade only when the output explicitly marks the affected facts missing and acceptance criteria permit it.
- Unknown base-model compatibility is `unknown`, never silently compatible.
- A filename match without content identity is `name_only`, never `matched`.
- Registry pickle/virus scan results are reported as registry evidence, not a local malware-safety guarantee.

## 7. Rollout and file ownership

Recommended sequence:

1. Parent and child planning reviews are converged; implementation waits for explicit approval.
2. Recipe core establishes normalized recipe/reference conventions.
3. Resolve and compare may be planned/checked independently; shared reference code is a real dependency and must not be duplicated for parallelism.
4. Community sampling follows the recipe core contract.
5. Search/show compaction proceeds independently using the approved D2 field registry and default-compact policy, with explicit legacy/default integration coverage.
6. Grouped help runs as the final child against the actual delivered command inventory, changing only the top-level command-list presentation.
7. Parent performs final command-surface, Skill, README, integration-test, changelog, and cross-child output review.

If children are implemented in parallel, ownership must be disjoint. The orchestrator retains `main.go`, README/CHANGELOG, `scripts/integration-test.sh`, and final embedded Skill integration unless explicitly assigned otherwise. Shared API/reference files are dependencies, not parallel scratch space.

## 8. Rollback shape

Each new-command child must be independently removable by deleting its additive dispatch/help/command/domain/API surface and its tests/docs. Search/show compaction instead rolls back by restoring the previous output paths and removing its compact projection/view surface; any retained full/legacy path provides a forward migration route but does not replace a clean revert. Grouped help rolls back by restoring the final pre-grouping command-list block without touching dispatch or named help. No child may require persistent state migration. Since there is no new database or export format, rollback has no data-migration phase.

If shared reference/projection code causes a regression, roll back the consuming child first and restore the previous additive API DTO behavior without changing legacy command output.

## 9. Tradeoffs

- **Normalized schemas versus raw passthrough:** normalization adds maintenance but removes private Agent projections and makes missing-data semantics testable. Existing raw surfaces remain for escape hatches.
- **Shared reference logic versus independent child delivery:** sharing adds an ordering constraint only where the same grammar is consumed; duplicating it would create incompatible CLI contracts.
- **Bounded community summaries versus exhaustive mining:** bounded queries cannot claim representative coverage of all community usage, but they avoid crawler behavior, large output, and unrequested persistence.
- **Explicit resolve inputs versus automatic workflow discovery:** explicit inputs require more caller information but make the first resolver deterministic and independently useful.
- **No recommendation score:** users receive fewer automatic decisions but more auditable, defensible facts.
