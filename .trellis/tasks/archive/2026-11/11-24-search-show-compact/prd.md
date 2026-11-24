# Compact search and show JSON

> Status: D1 and D2 are approved on 2026-11-24. Default compact JSON, the full/legacy view, and the concrete field registry are decided. The user also approved the parent-wide prompt-free list/detail invariant. This artifact does not authorize implementation or `task.py start`.

## Goal and user value

Reduce the default Agent-facing payload produced by `civ search --json` and `civ show --json` so callers receive the fields needed for candidate/version selection and inspection rather than every upstream field that happens to exist.

The change should make the existing commands useful on their own instead of telling users to avoid them in favor of a new command. `recipe`, `resolve`, and `compare` remain task-specific siblings, not workarounds for unnecessarily large basic output.

## Confirmed repository facts

- `search --json` extracts and writes the upstream `items` array verbatim, including unknown fields and original formatting (`internal/cmd/search.go:135-140`; `internal/api/types.go:216-229`). Its human table is already a curated projection (`internal/cmd/search.go:203-242`).
- `show <modelId> --json` marshals the complete decoded `api.Model`, including description, license/stats/creator, and every embedded version/files/images representation retained by the DTO (`internal/cmd/show.go:107-117`; `internal/api/types.go:24-41`). Its human model/version tables are already curated (`internal/cmd/show.go:121-181`).
- `show <modelId> -v <versionId> --json` marshals the complete decoded `api.ModelVersion`; with `--prompts`, it switches to the separate narrow prompts view (`internal/cmd/show.go:77-104`). Version detail human output is independently curated (`internal/cmd/show.go:221-279`).
- The checked-in search fixture is 70,216 bytes and the standalone version fixture is 101,395 bytes. The latter contains large generation/workflow metadata not needed for basic identity/version inspection (`.trellis/tasks/11-24-agent-workflows/research/api-evidence.md`).
- Search results need to preserve model IDs and enough version identity to support the documented search -> show/recipe/compare flow (`internal/skill/assets/civ-prompt/references/civ-commands.md:18-21`). Removing all version IDs merely to make the payload smaller would create another mandatory API round trip.
- Existing machine shapes are public contracts today, even though the project is pre-1.0. Changing a default, adding a view flag, and retaining a full/legacy mode each have different migration and test requirements (`.trellis/spec/backend/design-principles.md:247-263`; `.trellis/spec/backend/quality-guidelines.md:95-111`).

## Scope interpretation

This child currently targets **JSON/machine output only**. Human search/show tables are already field-limited and are not assumed to need changes. Any human-column reduction requires separate evidence and a later product decision.

The initial scope covers:

1. `civ search <query> --json`.
2. `civ show <modelId> --json`.
3. `civ show <modelId> -v <versionId> --json`.

It does not change the established `show -v --prompts --json` recipe view, `vet`, `sync`, `tags`, or non-JSON behavior in this draft.

## Requirements

### C1 — Purpose-driven field whitelist

- Replace whole-response exposure in the reviewed compact view with explicitly owned fields needed for discovery, selection, and inspection.
- Search compact output must retain model identity and minimal version references sufficient for the next command; it must not silently discard older versions or select an implicit latest version without documenting that policy.
- Model-show compact output must retain model-level selection facts and useful version summaries without embedding every version's images/files/generation metadata.
- Version-show compact output must retain version identity, base/trigger context, and deliberate file/hash/scan information. Do not over-trim the security/identity fields that make version inspection useful.
- Exclude description HTML, raw example-image/generation/workflow payloads, download URLs, and unreviewed training/paid-access/extension objects from compact output unless a concrete selection requirement justifies them.

### C2 — Stable normalized contract

- Define exact JSON field names, top-level shape, required/nullable fields, missing-versus-empty behavior, deterministic ordering, and schema/version policy before implementation.
- Compact output must no longer promise unknown-upstream-field passthrough. Unknown fields remain tolerated by the decoder but are not emitted.
- Preserve large integer values such as seeds only if those fields are included; basic search/show compact views should normally omit generation records rather than invent a second recipe schema.
- Do not use field omission to turn absent data into false, zero, empty, or confirmed equality.

### C3 — No additional upstream requests

- Produce compact output from the same `/models`, `/models/{id}`, or `/model-versions/{id}` response used today.
- Do not call `/images`, by-hash, owning-model, mini, or other endpoints to fill omitted fields.
- Preserve existing token/proxy/timeout/retry/error behavior and stdout/stderr separation.

### C4 — Decided default-compact compatibility policy

- `--json` defaults to the compact view for all three scoped command forms. The user explicitly approved changing this default contract on 2026-11-24.
- Add `--view compact|full`, with `compact` as the default. `--view full` is the explicit migration/escape path for callers that need the previous result.
- Full view reproduces the appropriate pre-change behavior exactly: search's raw items passthrough and show's existing decoded-object JSON, not a new approximated “full” projection.
- The view option is meaningful only for the scoped JSON forms. Its interaction with human output and `--prompts` must be explicit in help/tests rather than silently ignored.
- Update help, completion, README, Agent guidance, and integration assertions atomically with the selected policy. Document the intentional pre-1.0 breaking default change rather than hiding it as an internal refactor.

### C5 — Bounded output by projection, not silent record loss

- Reduce payload by removing unneeded fields, not by silently dropping model items or versions.
- Any limit on versions or other nested lists requires a separate user decision, explicit counts/continuation semantics, and must not be introduced as an implementation shortcut.
- Compact output must be materially smaller than full output on the checked-in representative fixtures while preserving the reviewed selection fields.

### C6 — Independent delivery and sibling boundaries

- Keep recipe's normalized generation context, compare's aligned/difference contract, and resolve's readiness report separate from basic compact search/show views.
- Share a projection only when semantics genuinely match; do not create a generic dynamic field-selection language or a universal JSON framework for this task.
- No export, local library, persistence, download, generation, or workflow behavior.

## Observable acceptance criteria

The approved compact invocation is the existing `--json` form without a view override. The approved legacy invocation adds `--view full`. The compact fields and view-validation rules in `design.md` are approved.

- [ ] **AC1 — Search selection survives:** Against `models_search_anima.json`, `the default `--json` compact view` returns the same model items in the same order, each retaining the approved identity/base/creator/stats/version-reference fields. Every model/version ID needed to choose a subsequent show/recipe/compare input is preserved. No description HTML, files, images, generation metadata, or download URLs are emitted.
- [ ] **AC2 — Model show is a summary:** Against `models_2458426.json`, `the default `--json` compact view` retains the approved model header and every version as a minimal summary without silently choosing only the latest. Embedded files/images/workflows and large unreviewed objects are absent.
- [ ] **AC3 — Version show remains useful for inspection:** Against `version_3263843.json`, `the default `--json` compact view` retains model/version/base/trigger identity and the approved file identity, SHA256/scan/format/size facts while replacing raw example metadata with the approved counts or omission. No `comfy` payload or download URL appears.
- [ ] **AC4 — Material size reduction:** For all three representative fixtures, compact output is materially smaller than the current full JSON with no dropped top-level model items or silent nested-version truncation. The exact size threshold is set after the field registry is reviewed, not manipulated by minifying whitespace alone.
- [ ] **AC5 — Deterministic and typed:** Repeated projections of identical decoded responses produce equivalent deterministic JSON. Required keys/types, empty arrays, missing values, and schema/version behavior match the reviewed contract.
- [ ] **AC6 — Same request behavior:** Fake-server tests observe exactly the same endpoint and request count as the corresponding current command. Invalid IDs/flags fail before network; existing 404/network/token error classes and exit meanings remain unchanged.
- [ ] **AC7 — Full compatibility path:** `--json --view full` preserves search's raw items payload byte-for-byte and show's existing decoded model/version JSON shape and formatting. Tests and integration explicitly distinguish these two legacy meanings of full.
- [ ] **AC8 — Existing narrow surfaces remain distinct:** `show -v --prompts --json`, `vet --json`, `sync --json`, and human search/show outputs remain byte/contract compatible unless separately reviewed. Recipe/compare/resolve tasks are not used to hide a basic-output regression.
- [ ] **AC9 — Surface completeness:** Help, named help, completion, README, Agent references, and `scripts/integration-test.sh` describe and assert the same compact/default/full behavior. Offline tests cover all field/ordering/error cases; live integration covers representative search/show compact and any retained legacy mode.
- [ ] **AC10 — No scope leakage:** The diff adds no endpoint calls, persistence, export, local library, arbitrary field-selection engine, or changes to unrelated commands.

## Out of scope

- Human table/column redesign in the initial interpretation.
- `vet`, `sync`, `tags`, login/proxy/skill/upgrade output changes.
- Recipe prompt normalization, community sampling, comparison differences, and resource readiness.
- Server-side field selection, arbitrary `--fields` expressions, JSONPath/JQ-like query languages, and user-defined projection plugins.
- New API requests to reconstruct omitted information or an implicit latest-version policy.
- Truncating prompt/description text as a substitute for excluding unneeded source objects.
- Persistent response caches, local libraries, export files, or downloads.

## Decided output policy

### D1 — Compact is the default JSON contract

The user selected default compact on 2026-11-24 and explicitly accepted the breaking `--json` default change. The command-specific view spelling is `--view compact|full`, default `compact`; full is the mandatory legacy path described above. This decision no longer blocks design.

## Decided field registry and list/detail invariant

### D2 — Compact field registry approved

The user approved the concrete search/model/version fields, top-level envelopes, missing/empty behavior, and `--view` validation rules in `design.md` on 2026-11-24. Search items and model-show share one model-summary projection; all model/version records are retained; version-show keeps every visible file's identity, hashes, and scan facts.

The parent-wide list/detail invariant is also approved: no list or summary view, including nested arrays, may return large fields such as prompts, negative prompts, description HTML, raw metadata, or workflows. Such fields require an explicitly selected detail result. The explicit `--view full` and legacy `--prompts` paths remain intentional compatibility detail views.

No additional human-output change is approved. Release notes and README must call out the default JSON change and the `--view full` migration path. Implementation now waits only for the normal final planning review and fresh explicit implementation approval.
