# Resource set resolver — approved technical design

> Status: requirements and design converged for implementation review. The reference grammar, exit mapping, and initial compatibility policy are approved. Implementation awaits parent convergence and fresh explicit task approval.

## 1. Purpose and boundary

`civ resolve` answers a set-level question that `vet` and `sync` do not:

> Given this explicitly declared set of model resources, does each one have canonical identity and exact local presence, and do the resources satisfy reviewed compatibility rules?

It is a read-only evaluator and environment checker, not a package manager, downloader, scanner, workflow parser, or compatibility database.

The implementation boundary is:

- a pure `internal/resolver` domain package for validation, correlation, policy, findings, and readiness;
- a thin `internal/cmd/resolve.go` adapter for flags, filesystem/API evidence, rendering, and exit mapping;
- existing `Client.GetVersion`/`GetVersionByHash`/`GetModel` and `hashcache.ComputeSHA256` for evidence;
- a non-serialized `AIR` field on `api.ModelVersion` for resolve reporting without changing existing show/vet JSON.

No new endpoint, database, cache, persistent index, download, workflow writer, or generation call is introduced.

## 2. Approved CLI and reference grammar

```text
civ resolve <ref> [<ref>...] [--json]
```

Each positional `<ref>` is exactly one of:

1. an existing local regular-file path;
2. a full SHA256 (64 hex characters);
3. an AIR URN with an exact `@<versionId>`;
4. `version:<positiveId>`.

Bare integers, AutoV2/short hashes, model IDs, names, fuzzy/name-scoped references, and AIR without a version are usage errors before filesystem or network activity. There is no name-only diagnostic mode.

All refs are independent. A path supplies both content identity and local presence. A SHA/AIR/version ref supplies canonical registry identity only and is locally `unverified`.

At least one ref is required. Exact duplicate references are idempotent; the output retains one result per distinct input identity in first-appearance order. No dedupe may hide a conflicting canonical identity.

## 3. Architecture and seams

```text
main.go
  -> cmd.Resolve
       -> validate approved reference forms
       -> build resolver.Environment
       -> resolver.Resolve(ctx, requests, policy, env)
       -> render Report (human or JSON)
       -> map result to approved exit code

internal/resolver
  standard library only
  no direct filesystem, HTTP, config, cache, or printing
```

`internal/resolver` may import API DTOs only if that preserves one-way dependencies and does not make the domain HTTP-aware; preferred metadata types are resolver-owned projections. `cmd` maps API responses into resolver metadata.

The primary seam is one call:

```go
func Resolve(ctx context.Context, requests []Request, policy Policy, env Environment) (Report, error)
```

- nil error means evaluation completed, including `Ready == false`;
- non-nil error means usage/operational failure and never yields a ready verdict;
- policy is a value with a stable version;
- environment uses function values for file hashing and registry lookups, allowing offline tests without a new interface hierarchy or mutable global.

## 4. Core data contracts

### 4.1 Request

```text
Request
  InputIndex  // stable zero-based input order
  Kind        // path | sha256 | airVersion | versionID
  Path        // path kind only
  SHA256      // full hash kind only, normalized uppercase
  VersionID   // air/version kind only
  AIR         // optional source AIR
```

A ref cannot combine multiple identity claims; the approved grammar does not support path+expected-version or path+expected-hash associations. A path is identified by its own content hash. This deliberately keeps the MVP unambiguous and avoids a parallel-list grammar.

### 4.2 Registry metadata

```text
ModelMetadata
  ModelID, Name, Type

VersionMetadata
  ModelID, VersionID
  AIR, Name, BaseModel, BaseModelType
  ModelName, ModelType
  Files []FileMetadata

FileMetadata
  FileID, Name, Type, Primary
  SHA256 // projected case-insensitively from hashes["SHA256"]
```

Labels and names are display data, not identity. The exact SHA256 and numeric model/version/file IDs are authoritative.

### 4.3 Per-resource result

```text
ResourceResult
  InputIndex
  RequestedRef
  Resolution        // resolved | unresolved | mismatch | ambiguous_file
  Identity          // model/version IDs, optional AIR, optional matched file ID
  Names             // model/version/file display names
  VisibleFiles      // ordered IDs/names/types/SHA256; no arbitrary selection
  Local
    State           // present | missing | unverified | unresolved | mismatch
    Path
    ComputedSHA256
    MatchedFileID
  Issues []Issue
```

A path that exists but is not registered is `unresolved`, not silently present. A bare version/hash is `unverified` locally. A same-name file with different bytes is not present.

### 4.4 Findings and compatibility

```text
Finding
  Code             // stable machine string
  Blocking         // always true in the initial policy's failure set
  ResourceIndexes
  Message
  Evidence         // typed expected/actual IDs/hashes/names/types/bases
  RuleID           // policy rule when applicable

CompatibilityResult
  LeftIndex, RightIndex
  Status           // compatible | unknown | not_evaluated
  RuleID
  Left/Right model type, baseModel, baseModelType
  Reason
```

Initial finding codes include:

- `missing_resource`;
- `unresolved_resource`;
- `wrong_version`;
- `local_presence_unverified`;
- `local_hash_mismatch`;
- `missing_anchor`;
- `conflicting_anchor`;
- `unknown_compatibility`;
- `ambiguous_file_identity`;
- `role_type_mismatch`.

No score, confidence, rank, or percentage is produced.

### 4.5 Report

```json
{
  "schemaVersion": 1,
  "policyVersion": "resolver-v1",
  "ready": false,
  "resources": [],
  "compatibility": [],
  "findings": [],
  "summary": {
    "resourceCount": 0,
    "blockingFindingCount": 0
  }
}
```

Resources follow input order; findings and compatibility results use stable input/rule ordering. The report is rendered only after evaluation completes. It contains no raw API response, prompt, description, workflow, token, or local directory contents.

## 5. Evaluation flow

### 5.1 Validate

Validate all refs before I/O: allowed forms, positive IDs, full-hash syntax, AIR version presence, non-empty set, duplicate handling, and path-shape rules. Usage failures exit 1 with no report.

### 5.2 Gather local evidence

For each path in input order:

- missing file → completed `missing_resource` result;
- directory/non-regular target → usage error;
- open/read/hash failure → operational error, no ready report;
- compute fresh SHA256 through the environment seam; do not read/save the sync hash cache;
- continue evaluating other resources where a missing/unregistered file is a normal not-ready result.

### 5.3 Gather registry evidence

- path → hash file, then by-hash;
- SHA256 → by-hash;
- exact version → version-by-ID;
- AIR with exact version → version-by-ID (numeric ID authoritative; AIR is output metadata).

Deduplicate identical GETs only in memory for this invocation. Convert a well-formed registry 404 into `unresolved_resource`; preserve network/auth/decode errors as operational failures. Never retry through search, another version, filename, or primary-file guessing.

### 5.4 Correlate identity

1. Project visible files and match the computed/requested full SHA256 against every `files[].sha256`.
2. Exactly one matching visible file binds `FileID`; duplicate/ambiguous matches produce `ambiguous_file_identity`; no match leaves file identity unresolved.
3. If a path's hash resolves to a different version than an explicitly requested version, report `wrong_version` with both identities.
4. Never substitute a primary file, first file, filename, or another version when identity is incomplete.

### 5.5 Evaluate policy

- infer role from registry `model.type` only;
- require exactly one canonical checkpoint anchor;
- evaluate known dependents against the anchor using the policy in §6;
- keep independent findings for all resources rather than stopping at the first;
- unknown/missing metadata is never compatible.

### 5.6 Readiness reduction

```text
ready =
  at least one resource
  AND every resource has canonical identity where required
  AND every explicitly declared path has exact local presence
  AND exactly one checkpoint anchor exists
  AND no blocking finding exists
  AND every applicable compatibility result is compatible
```

Remote-only refs are diagnostic and cannot make a set ready. Unknown compatibility makes the set not ready.

## 6. Initial compatibility policy (`resolver-v1`)

- Anchor: exactly one distinct canonical `Checkpoint`.
- Known dependents: `LORA`, `TextualInversion`, `Controlnet`, `VAE`.
- Compatible: dependent and checkpoint each have non-empty `baseModel` and they are exactly equal. If both have non-empty `baseModelType`, those must also be exactly equal.
- Unknown: missing base metadata, unequal base values, unknown roles/types, uncovered tuples, or otherwise unproven compatibility. Unknown blocks readiness.
- No aliases, case folding, substring matching, family inference, or `baseModelWhitelist` compatibility use in MVP.
- Multiple distinct checkpoints → `conflicting_anchor`; zero checkpoints → `missing_anchor`.
- Repeated identical canonical resource is idempotent.
- Policy is compiled and versioned; no user policy file or remote fetch.

## 7. Output and exit contract

Human output starts with `READY` or `NOT READY`, then per-resource identity/local state and blocking findings with exact expected/actual evidence. Full hashes and IDs are never truncated. JSON uses the versioned report above.

| Outcome | Exit |
|---|---|
| Ready, report on stdout | 0 |
| Completed not-ready, report on stdout | 5 |
| Usage/malformed input or unexpected local I/O/decode failure | 1 |
| Network failure/cancellation | 2 |
| Rejected token | 4 |

`executeWithExitCode` may add a resolve-specific mapper for `ready`/`not_ready`, delegating all operational classes to unchanged `exitCodeFor`. Vet exit 3 and sync semantics are untouched. `--json` output is stdout-only; diagnostics remain stderr.

## 8. Compatibility with existing behavior

- Add `AIR` to `api.ModelVersion` as exported metadata with `json:"-"` populated by its custom unmarshaler; existing show/vet JSON must not gain an `air` key.
- Do not modify `exitCodeFor`, vet, sync, hash-cache persistence, or existing endpoint methods.
- All HTTP continues through `api.Client.Get`; all output through `report`/cmd.
- No persistent state means no migration; rollback is deletion of the additive command/domain/AIR field registrations.

## 9. Tradeoffs

- **Explicit local paths for READY:** conservative but makes readiness meaningful; remote-only refs still diagnose identity.
- **No name-only mode:** removes ambiguity and extra API surface at the cost of requiring exact refs.
- **Exact-base compatibility only:** avoids false readiness and alias drift; more results remain unknown until policy evidence grows.
- **Exit 5 for completed not-ready:** convenient for automation while keeping stdout report intact; adds one documented code.
- **Sequential lookups:** simple deterministic error handling and low peak memory; large sets may be slower, which is acceptable for an explicitly bounded MVP.

## 10. Rollout and rollback

1. Add/tests API AIR metadata without serializing it in legacy output.
2. Implement/test pure resolver types, policy, correlation, and readiness offline.
3. Add thin command adapter, output, and exit mapping using temp files/httptest.
4. Register dispatch/help/completion and update README/integration/docs.
5. Run full offline/race quality gates and parent review.

Rollback removes the additive command/domain files, registrations, tests/docs, and the non-serialized AIR field if necessary. No persisted state or data migration exists.
