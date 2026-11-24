# Resource set resolver

## Status

Requirements are converged for implementation review. The approved command/reference grammar, exit mapping, and initial compatibility policy are recorded below. This is a complex-task plan, not approval to implement; `task.py start` remains blocked until parent review and fresh explicit approval.

## Goal

Add a new `civ resolve` command that answers whether an explicitly supplied set
of model resources can work together. The answer must combine canonical registry
identity, exact local-file presence by hash, reviewed base-model compatibility
rules, and an explainable aggregate `ready` / `not ready` verdict.

The command is a set resolver, not another batch vetting command: a set containing
individually valid resources can still be not ready because resources are missing,
unresolved, wrong-version, conflicting, or of unknown compatibility.

## User value

A user or agent can check a proposed checkpoint/adapter/resource combination
before trying to use it. The result identifies what was resolved, which exact
local bytes were found, which cross-resource checks passed or failed, and what
must be corrected. The command never downloads, installs, rewrites, or guesses a
replacement resource, and it produces no opaque recommendation score.

## Confirmed facts and constraints

- The repository is a standard-library-only Go CLI. Command dispatch and curated
  help are in `main.go:46-55` and `internal/cmd/help.go:20-36`; the shared runner
  and current exit mapping are in `internal/cmd/globals.go:93-164`.
- `internal/api/versions.go:9-21` already provides numeric version-ID and by-hash
  GET lookups. `internal/api/types.go:59-78` already decodes version/model IDs,
  `baseModel`, `baseModelType`, model type, and files;
  `internal/api/types.go:104-119` decodes file IDs, names, types, primary flags,
  and all returned hashes.
- `internal/hashcache/cache.go:20-37` computes a local file's SHA256 with bounded
  memory. `civ vet` uses it directly without the persistent sync hash cache
  (`internal/cmd/vet.go:110-155`); resolve can follow that read-only pattern.
- The pinned Anima fixtures demonstrate distinct canonical identities: model
  `2458426`, version `3263843`, file `3147349`, and a full SHA256. They also carry
  AIR `urn:air:anima:checkpoint:civitai:2458426@3263843`. The model fixture has a
  different version containing Model, Text Encoder, and VAE files, so an
  implementation must not assume every version has one file or that the primary
  file is the queried file (`testdata/models_2458426.json`).
- The official Civitai reference, checked on 2026-11-24 at
  `https://developer.civitai.com/site/reference/model-versions.md`, documents
  `GET /api/v1/model-versions/{id}` and
  `GET /api/v1/model-versions/by-hash/{hash}`. It describes `air` as the canonical
  AIR identifier for a specific model version. It does not document AIR as an
  input to these lookup endpoints.
- The same official reference documents no cross-resource compatibility matrix,
  dependency graph, or local-generator readiness verdict. `supportsGeneration`
  and Orchestration-specific `canGenerate` are not evidence that a local resource
  set will run in a user's generator. Registry responses can also omit
  unpublished, private, or archived files.
- The existing `baseModelWhitelist` in `internal/api/enums.go:73-144` is a search
  spelling/filter aid. It is not a compatibility matrix and must not be reused as
  one.

## Definitions

- **Resource request**: one explicitly declared member of the set. Conceptually it
  may contain an expected version reference, a content reference (an explicit
  local path or full hash), an optional role/slot, and optional name qualifiers.
  This is a semantic definition, not approved CLI syntax.
- **Canonical registry identity**: numeric `modelId` + `versionId` for a version,
  plus `fileId` + an exact content hash for a file when available. AIR is
  additional canonical version metadata, not a file hash or a substitute for
  local presence.
- **Exact local presence**: an explicitly supplied regular file was read, its
  computed full SHA256 matches a specific file in the intended registry version,
  and the matching `fileId` is reported. A matching filename, a primary-file
  flag, a short hash, or a bare hash argument alone is not proof of local bytes.
- **Name-only match**: a candidate supported only by model/version/file display
  names, without an exact version ID or content-hash binding. It remains
  non-canonical even when it is the only candidate.
- **Unknown compatibility**: the reviewed rules do not establish compatibility or
  incompatibility for the observed role/base-model tuple. Unknown is distinct
  from a known conflict.
- **Ready**: every declared resource and every reviewed set constraint passed the
  checks in this document. Ready describes only this declared set and these
  checks; it is not a workflow-completeness or local-runtime guarantee.

## Requirements

### R1 — Add only the new command surface

Add `resolve` as a new command with human and global `--json` output, curated
help, named-help routing, shell completion, README coverage, and integration
coverage. Do not change the behavior, output schemas, flags, or exit-code
semantics of `search`, `show`, `vet`, `sync`, or other existing commands.

### R2 — Require explicit, typed resource requests

Accept one or more explicitly supplied resource references in these forms only:

- an existing local regular-file path;
- a full 64-character SHA256 content reference;
- an AIR URN that includes an exact `@<versionId>` (the numeric version remains authoritative);
- `version:<positiveId>` for an exact numeric version without AIR.

Bare positive integers, AutoV2/short hashes, model IDs, display names, fuzzy name matches, and implicit latest-version selection are rejected as usage errors. There is no name-only diagnostic mode in the MVP. Refs remain ordered and independent; no parallel-list association or heuristic reinterpretation is allowed.

Full SHA256 is the only hash evidence that can satisfy exact local presence. Remote-only refs are useful for canonical identity and compatibility diagnosis but remain `unverified` locally and cannot produce `ready=true`.

### R3 — Resolve and display canonical identity

For each resolved resource, report available `modelId`, `versionId`, AIR, model
type, model/version names, `baseModel`, and `baseModelType`. When a specific file
is selected by hash, also report its `fileId` and exact SHA256. A version-only
result retains ordered summaries of all visible files instead of arbitrarily
selecting one. Preserve the distinction between version identity and file
identity.

Never promote a model name, version name, filename, or primary-file marker into
canonical identity. If a hash lookup returns a version but the corresponding
hash cannot be found in its visible `files[]`, report incomplete/unresolved file
identity rather than selecting the primary or first file.

### R4 — Verify local presence by content, not names

For an explicit local path, compute its full SHA256 and match it exactly against
the intended version's visible files. Report the actual matching `fileId`, even
when it is not the primary file. A same-named or similar-named file with a
different hash is not present.

A missing path produces a missing-resource finding. A directory, unreadable
file, or hashing failure must not be mislabeled as an exact hash mismatch. A
bare version ID or bare hash can support registry/compatibility diagnosis, but
its local presence is `unverified` unless associated with an explicit path; it
must never be displayed as locally present.

### R5 — Diagnose wrong versions explicitly

When a request binds an expected version to local content and the content
resolves to a different canonical version, emit `wrong_version` with both
expected and actual model/version IDs and hashes. Do not collapse this case into
generic “not found,” “missing,” or “base-model conflict.”

### R6 — No name-based identity or fallback

- Name-only resolution, fuzzy matching, filename-similarity matching, model-scoped version-name lookup, and automatic latest-version selection are forbidden.
- A filename or display name never satisfies canonical identity. Requested refs are resolved only by exact path content hash, full SHA256, AIR with exact version, or `version:<id>`.
- If a hash lookup returns a version but the visible files cannot bind the actual content identity, report incomplete/unresolved file identity instead of selecting by name, primary flag, or list position.


### R7 — Apply reviewed, role-aware cross-resource rules

Evaluate compatibility between the appropriate resources, not as an arbitrary
all-pairs string comparison. The reviewed rule contract must define:

- which model type is the compatibility anchor and which types depend on it;
- the roles/slots for which a missing or duplicate resource is a finding;
- how `baseModel` and `baseModelType` participate;
- which tuples are compatible, which are known conflicts, and which are unknown;
- whether any aliases/equivalences exist; and
- how missing or unrecognized metadata is handled.

Every decision must carry a stable rule ID and the observed left/right values.
No alias, family equivalence, or disjoint-family assumption is approved merely
because it appears in search filters or seems commonly used. The default for an
uncovered tuple is `unknown`, never silent compatibility.

### R8 — Preserve distinct findings

The result must distinguish at least:

- `missing_resource` for an explicitly required but absent local resource or
  reviewed required role;
- `unresolved_resource` for a well-formed registry reference that cannot be
  resolved, including a registry 404;
- `wrong_version`;
- `local_presence_unverified` and `local_hash_mismatch`;
- `unknown_compatibility` and any explicitly reviewed known conflict; and
- reviewed role/slot conflicts such as a missing or incompatible duplicate anchor.

Findings must be explainable with resource indexes or labels and structured
expected/actual evidence. One bad member must not erase successful identity or
compatibility facts about the other members.

### R9 — Compute a fail-closed, explainable set verdict

`ready=true` requires all of the following:

1. every declared resource has canonical version identity and, where file
   identity is required, canonical file identity;
2. every declared resource has exact local presence through an associated path;
3. no ref is unresolved, missing, wrong-version, or conflicting;
4. every applicable compatibility rule returns compatible, with no unknown
   result; and
5. every reviewed required role/slot is satisfied exactly as specified.

Otherwise `ready=false`. The report must enumerate the blocking findings rather
than reduce them to a percentage, confidence value, or recommendation score.
There is no remote-only or “probably ready” MVP verdict.

### R10 — Produce deterministic human and machine reports

Both output forms must include the aggregate verdict, per-resource identity and
local-presence states, compatibility outcomes, and stable finding codes with
expected/actual evidence. Resource order follows input order; compatibility and
finding order is deterministic. Repeated evaluation of identical inputs and
fixtures must produce identical stdout.

Human output must state `READY` or `NOT READY` and list actionable reasons.
JSON must be one versioned object, not a log stream, and must retain distinctions
that the text rendering abbreviates. Stdout is results only; warnings, progress,
and operational diagnostics remain on stderr.

### R11 — Separate a completed not-ready evaluation from operational failure

A well-formed missing/unknown/conflicting resource set is a completed evaluation
with a not-ready report. Invalid syntax is a usage error. Network exhaustion,
token rejection, malformed upstream data, and unexpected local I/O failures are
operational failures and must never produce `ready=true`.

The existing usage/network/token classes remain distinguishable. The process
mapping for a completed not-ready evaluation is intentionally open in D2; it
must not reuse or alter vet's command-specific exit `3` without explicit user
approval.

### R12 — Add no resource-management side effects

Resolve may read explicitly named files and perform read-only registry metadata
GETs. It must not:

- create a persistent local model library, location index, or resource database;
- write a model, workflow, config, or persistent resolve/hash-cache file;
- scan a directory to discover resources;
- download or install a missing/wrong-version resource;
- rewrite a workflow or substitute another version; or
- invoke generation/orchestration services.

In-memory deduplication of identical lookups within one invocation is allowed.

### R13 — Keep the project's quality and documentation contracts

Implementation must remain standard-library-only, offline-unit-testable,
English-only, and covered by `go vet`, unit tests, and race tests. Because this
adds CLI surface, help/completion drift guards and
`scripts/integration-test.sh` must be updated in the same implementation change.
Live integration tests must not require downloading registered model bytes.

## Acceptance criteria

These criteria are observable behavior targets. References to a “reviewed rule”
or “approved mapping” must be replaced with the concrete reviewed values before
implementation starts.

| ID | Requirements | Observable acceptance criterion |
|---|---|---|
| AC1 | R1 | The top-level help lists `resolve`; `civ resolve help`, `civ resolve -h`, and `civ help resolve` render the same curated page on stdout with exit 0. All four completion scripts offer the new command and its approved flags. Existing command tests and documented exit meanings remain unchanged; in particular, adding AIR metadata for resolve must not add an `air` field to existing `show`/`vet` JSON. |
| AC2 | R2, R3, R4, R9 | Given explicit paths for a checkpoint and a dependent resource whose reviewed base-model rule is compatible, fake registry responses bind both paths to the expected versions/files and the output says `READY`. JSON reports each model/version/file ID, exact SHA256, available AIR, local state `present`, and the compatible rule ID. |
| AC3 | R3, R4 | A non-primary file whose SHA256 appears in the intended version is reported present with that file's ID. A file with the same filename but different bytes is not reported present, and no primary/first-file fallback occurs. |
| AC4 | R2, R4, R9 | A bare exact version ID or bare full SHA256 can show canonical registry metadata, but its local state is `unverified`; without an associated explicit path it cannot contribute to a `READY` verdict. |
| AC5 | R5 | A request expecting version N while its path hashes to version M emits `wrong_version`, expected N, actual M, both hashes/IDs where available, and `ready=false`; it is not reported merely as missing or incompatible. |
| AC6 | R2, R6 | Bare numeric IDs, AutoV2/short hashes, model IDs, display names, fuzzy/name-scoped refs, and AIR without an exact version are rejected before any filesystem or network access. No search or name resolution request is made. |
| AC7 | R4, R8 | A nonexistent explicit path yields `missing_resource` with that path. A well-formed hash/version 404 yields `unresolved_resource` with the original reference. Both produce a completed not-ready report and are visibly different from usage/network/token failures. |
| AC8 | R7, R8, R9 | Rule-table tests cover one compatible tuple, one reviewed known conflict, and one uncovered/unknown tuple. Their outputs are respectively `compatible`, `base_model_conflict`, and `unknown_compatibility`, with left/right base metadata and stable rule IDs. The latter two cannot be collapsed into one status and neither permits ready. Missing base metadata is unknown. |
| AC9 | R7, R8, R9 | A set with two valid members and one unresolved, unknown, or conflicting member is `NOT READY`; valid member details and all independent blocking findings remain present. No numeric score is emitted. Reviewed missing-anchor and duplicate-anchor cases produce their specific findings. |
| AC10 | R8, R10 | JSON parses as one versioned object containing `ready`, ordered `resources`, compatibility results, and findings with stable codes and structured evidence. Text output contains the same verdict and blocking codes/reasons. Repeated fixture runs are byte-identical, and stdout contains no warnings/progress. |
| AC11 | R11, D2 | Offline tests pin the approved exit mapping: `0` ready, `5` completed not-ready (report still emitted on stdout), `1` usage/unexpected local I/O or decode failure, `2` network exhaustion, `4` rejected token. Network/token/usage failures never emit a ready verdict. Vet's unknown-hash exit `3`, sync's `not_found` row behavior, and all other existing mappings are regression-tested unchanged. |
| AC12 | R12 | Tests use only explicitly named temporary files and an `httptest` server. Request logs contain only approved read-only metadata GETs: no file download, generation, orchestration, or bulk-write call. No directory walk, persistent model index, resolve cache, hash-cache save, config write, or workflow/model mutation occurs. |
| AC13 | R13 | `go vet ./...`, `go test ./...`, and `go test -race ./...` pass; `go list -m all` still lists only `civitai-cli`. Unit tests use no external network. README/help describe the narrow readiness guarantee and all approved finding classes. |
| AC14 | R1, R13 | Integration coverage includes resolve help/named help, malformed input (usage, no lookup), a known exact-version/hash diagnostic, an unknown full SHA256 (unresolved/not-ready), JSON shape and verdict, the approved not-ready exit mapping, and a timeout/network case. Offline tests, not live CI, cover the fully ready path and wrong-version path because registered model bytes are unavailable. |

## Out of scope

- A persistent local model library, automatic local discovery, directory
  scanning, filename indexing, or a replacement for `civ sync`.
- Automatic downloads, installation, upgrades, model renaming, or any repair of
  a not-ready set.
- Image ingestion, generation-metadata resource extraction, ComfyUI/A1111/other
  workflow ingestion, workflow rewriting, or custom-node dependency analysis.
- Inferring resources that were not explicitly declared, including an unlisted
  VAE, text encoder, LoRA, custom node, or other workflow dependency.
- Filename-similarity matching, fuzzy name search, automatic selection of a
  latest/best version, or silent use of an alias/equivalence not in the reviewed
  rules.
- A universal Civitai/base-model compatibility database, arbitrary generator
  support claims, model execution, or a guarantee that a ready set will generate
  an image successfully.
- Treating Civitai `supportsGeneration`, `canGenerate`, AIR, or the Orchestration
  API as a substitute for local compatibility and presence checks.
- Bulk hash POST optimization, persistent API response caching, concurrent batch
  scanning, and performance work not needed by small explicit sets.
- Changes to existing command behavior or unrelated refactors.

## Approved decisions

### D1 — CLI and reference grammar

```text
civ resolve <ref> [<ref>...] [--json]
```

Accepted refs are exactly:

- an existing local regular-file path;
- a full 64-character SHA256;
- an AIR URN with an explicit `@<versionId>`;
- `version:<positiveId>`.

Bare integers, AutoV2/short hashes, model IDs, display names, name-only refs, fuzzy matching, and implicit latest selection are usage errors. There is no name-scoped candidate mode. Every ref is independently resolved; remote-only refs are diagnostic and `unverified` locally. A `ready` verdict requires an explicit local path for every resource.

### D2 — Exit mapping

| Condition | Exit |
|---|---|
| Completed ready, report on stdout | `0` |
| Completed not-ready, report on stdout | `5` |
| Usage error, malformed/unexpected local I/O, decode/conflict-independent command failure | `1` |
| Network exhaustion/cancellation | `2` |
| Rejected token | `4` |

Vet keeps exclusive use of `3`, and sync's `not_found` remains a data row. A not-ready report is machine output, not an error banner.

### D3 — Initial compatibility and role policy

- The compatibility anchor is exactly one `Checkpoint` resource. Zero checkpoints is `missing_anchor`; two or more distinct canonical checkpoints is `conflicting_anchor`. Repeating the identical canonical checkpoint/resource is idempotent and does not conflict.
- Roles derive from registry `model.type`, never filenames. Known dependent types are `LORA`, `TextualInversion`, `Controlnet`, and `VAE`.
- A dependent is compatible only when its non-empty `baseModel` exactly equals the checkpoint's `baseModel`. If both sides provide a non-empty `baseModelType`, it must also match exactly. Missing required base metadata, mismatched base values, unknown model types, or uncovered tuples produce `unknown`, never silent compatibility.
- No aliases or family-equivalence rules are active in the MVP. `baseModelWhitelist` is a search filter, not a compatibility matrix.
- `ready=true` requires every declared path present with exact SHA256, exactly one checkpoint, no blocking identity/presence finding, and every applicable compatibility result `compatible`. Unknown is not ready.
- Policy version is compiled into the binary; no user-editable policy file or remote policy fetch.

## Planning status

All previously listed product decisions for resolve are resolved. Implementation remains unapproved and unstarted pending parent convergence and explicit task approval.
