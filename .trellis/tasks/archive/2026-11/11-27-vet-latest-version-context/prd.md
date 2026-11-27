# Vet latest published version context

> Status: requirements are converged for final planning review. The concrete output names and rendering details below are the selected minor details for approval. This document does not authorize implementation or `task.py start`.

## Goal

Add informational latest-published visible-version context to the existing normal `civ vet` human and JSON reports. The context must identify the latest eligible embedded version, describe the vetted version's relationship to it, and report exact base-model change only when the evidence supports those statements. It must never recommend an upgrade or label the vetted file outdated.

## User value

- A user or Agent can tell whether the registry response contains a newer published visible version without manually reconciling `modelVersions` from another command.
- A user can see whether the latest version uses exactly the same base model, which is compatibility context rather than a preference or upgrade verdict.
- Automation receives explicit known, tied, and unknown states instead of inferring freshness from array order, version IDs, names, downloads, or missing values.

## Confirmed repository context

- `runVet` already performs the by-hash lookup and then one best-effort owning-model fetch before selecting the output view (`internal/cmd/vet.go:132-155`). The by-hash response is the vetted-version source of truth; model-fetch failure warns and degrades without changing a successful hit (`internal/cmd/vet.go:181-205`, `.trellis/spec/backend/error-handling.md:106-116`).
- The existing `api.Model` already decodes `modelVersions`, and each `api.ModelVersion` already exposes `ID`, `Name`, `BaseModel`, and `PublishedAt` (`internal/api/types.go:24-41`, `internal/api/types.go:59-84`). No endpoint or DTO expansion is required for the approved facts.
- The pinned owning-model fixture contains eight visible versions; version `3263843` / `turbo-v1.1` is the unique maximum at `2026-08-24T23:42:31.929Z` and matches the by-hash fixture (`testdata/models_2458426.json:1`, `testdata/byhash_anima_turbo.json:1`).
- The command-local `latestPublishedAt` used by search compares timestamp strings lexicographically and returns no version identity (`internal/cmd/search.go:304-314`). The compare package instead parses RFC3339 instants and treats missing/invalid timestamps as missing derived evidence (`internal/compare/compare.go:760-785`, `internal/compare/compare_test.go:264-314`). Vet needs the compare package's instant semantics plus identity, ties, current-version relationship, and base-model state; neither existing helper is sufficient as-is.
- Normal vet JSON currently emits `{hash,version,model,examples}` (`internal/cmd/vet.go:318-377`). The focused `vet --prompts --json` branch returns only the shared recipe view before normal report assembly (`internal/cmd/vet.go:143-148`, `internal/cmd/prompts.go:28-43`).
- Existing tests use offline `httptest` routes, inline synthetic bodies for edge cases, and command-local request recorders where exact request sequences matter (`internal/cmd/vet_test.go`, `internal/cmd/compare_test.go:16-60`). Shared integration, README, CHANGELOG, and spec updates belong to the parent orchestrator.

## Definitions

- **Vetted/current version:** the standalone version returned by the by-hash lookup. Its ID and base model remain authoritative for the file being vetted.
- **Visible version:** an entry actually present in the successful owning-model response's `modelVersions` array. The CLI does not synthesize additional visibility filtering from status, availability, array position, or download counts.
- **Latest:** the unique visible version whose valid RFC3339 `publishedAt` instant is the maximum. It is not the default, recommended, best, most-downloaded, first, highest-ID, or latest-downloaded version.
- **Complete timestamp evidence:** every visible entry has a non-empty parseable RFC3339 timestamp. Missing, null, empty, and malformed timestamps are not skipped: an entry with unknown publication time could be the actual maximum, so the latest claim becomes unknown instead.
- **Current latest:** the vetted version ID is the unique latest ID. **Tied latest:** the vetted version is one of multiple IDs sharing the maximum instant. **Newer published:** a unique different latest ID has a later timestamp than the matching visible current entry.
- **Exact base-model comparison:** case-sensitive string equality with no trimming, aliasing, normalization, or compatibility inference. An empty/missing value on either side makes change unknown.

## Requirements

### R1 — Preserve vet source, requests, and outcomes

- Keep the existing input classification, local-file hashing, by-hash lookup, decoded vetted version, model summary, license, examples, prompts, warnings, and exit-code behavior unchanged except for the additive report context.
- Reuse only the `api.Model` already returned by `fetchVetModel`. Add no endpoint call, retry, pagination, version-detail fetch, image request, download, upload, cache, or library scan.
- Hash and local-file inputs that reach the same by-hash/model responses must produce the same latest-published context.
- Do not add the feature to `sync`, create a top-level stats command, or introduce implicit version selection.

### R2 — Select latest only from defensible evidence

- Evaluate visible embedded versions by parsed RFC3339 instant, including fractional seconds and offsets; never compare timestamps lexicographically.
- Require positive, unique version IDs and complete timestamp evidence across the visible list before making a definitive latest claim. Invalid/duplicate/missing IDs, absent version data, an empty list, or any missing/malformed timestamp yields an explicit unknown state.
- If multiple distinct IDs share the maximum instant, report a tied state and the sorted tied IDs. Do not break the tie by response order, ID, name, base model, or current-version membership.
- Do not filter by the local clock or infer a historical trend, rollback, scheduled release, or preferred version. The result describes only the supplied visible response.

### R3 — Keep current-version relationship separate from latest identity

- A unique latest may still be reported when the current ID is absent from `modelVersions`; in that case the relationship is explicitly `currentVersionNotVisible`/unknown rather than newer, tied, or current latest.
- Report `currentLatest` only for a unique latest with the current ID, `newerPublished` only for a unique different latest whose instant is later than the visible current entry, and `tiedLatest` only when the current ID participates in the maximum tie.
- If a maximum tie excludes the current version, retain the tie facts but leave the current-to-latest relationship unknown; do not choose one tied candidate merely to claim a transition.
- Relationship unknowns do not erase independently known latest identity or base-model facts.

### R4 — Report exact base-model state independently

- For a unique known latest, report its base model and compare it with the by-hash current base model as exact strings.
- Emit `changed` or `unchanged` only when both values are non-empty. Emit `unknown` when either base is missing/empty or when no unique latest exists.
- Base comparison remains available when the current version is not visible, because it uses the authoritative by-hash current base and the known latest base; it does not imply a newer-version relationship.
- Do not infer base-model compatibility, upgrade safety, or migration paths from a string change.

### R5 — Add concise human context

- Insert one `LATEST` header line after the existing `VERSION` line in normal human output. Keep all existing report sections and append `--prompts` recipes as before.
- For a unique latest, include name, full numeric ID, publication date, latest base model, relationship, and exact base-change state. JSON retains the full source timestamp; human output follows the existing `YYYY-MM-DD` date convention.
- Use explicit `unknown` and tie wording rather than `-`, blank fields, or a guessed candidate. Unknown lines include a short reason such as model details unavailable, no visible versions, timestamps unavailable/invalid, or current version not visible.
- Do not print `outdated`, `upgrade`, `recommended`, `best`, rollback, or trend advice. Add no data-quality warnings for expected unknown states; the report itself carries the state.

### R6 — Add only normal-JSON structure

- Add one required top-level `latestPublished` object to normal vet JSON. It contains latest status/reason, nullable unique-version facts, a non-null sorted `tiedVersionIds` array, and a comparison object containing relationship status/reason plus `baseModelChange`.
- A unique version object contains `id`, nullable `name`, original `publishedAt`, and nullable `baseModel`. Tied/unknown states use `version: null`; no partially selected candidate is presented as latest.
- Use stable English state/reason tokens and deterministic arrays. Do not add a schema envelope, generation timestamp, wall-clock field, recommendation, or raw duplicate `modelVersions` payload.
- Preserve `vet --prompts --json` as exactly the focused `{prompts,skipped}` recipe view: no `latestPublished`, `hash`, `version`, `model`, or `examples` keys and no change to recipe entries.

### R7 — Degrade without operational behavior changes

- If the model is absent because of missing `modelId`, request failure, 404, or decode failure, the vet hit still succeeds with the existing warning and model behavior; latest and relationship/base-change states are unknown with `modelUnavailable` evidence.
- Missing versus empty version arrays both produce unknown latest, with distinct reasons where the decoded representation supports it (`versionsUnavailable` versus `noVisibleVersions`).
- Data-level unknowns and ties are report states, not operational errors, and produce no additional requests, retries, warnings, or nonzero exits.
- A by-hash failure remains authoritative and emits no partial latest context.

### R8 — Prove semantics and compatibility offline

- Reuse the pinned Anima fixtures for the current-latest case and inline synthetic bodies for older current, changed base, absent current, invalid/missing/tied timestamps, and degraded model cases. Do not alter verbatim fixtures to manufacture edge cases.
- Add pure table-driven tests for selection and state precedence, human/JSON command tests for the same facts, exact focused-prompt compatibility assertions, and route/request-count assertions for every output mode and degradation path.
- Provide the parent with the exact integration and documentation scenarios below. The child does not edit shared integration scripts, README, CHANGELOG, or specs.

## Observable acceptance criteria

- [ ] **AC1 (R1):** A successful normal vet hit performs exactly the existing two GETs in order: `/model-versions/by-hash/{hash}` then `/models/{modelId}`. Normal human, normal JSON, and focused prompt JSON add zero requests; a model 404 still performs only those two requests and returns a successful vet result. By-hash 404 behavior and exit `3` remain unchanged.
- [ ] **AC2 (R2, R5, R6):** With the pinned Anima fixtures, human output identifies `turbo-v1.1`, ID `3263843`, date `2026-08-24`, base `Anima`, `current latest`, and `base unchanged`. Normal JSON identifies the same full timestamp and `currentLatest`/`unchanged` states.
- [ ] **AC3 (R2, R3):** Given visible current version `10` at an earlier instant and unique version `20` at a later instant, both views select version `20` and report `newerPublished`; timestamps with different RFC3339 offsets are compared by instant, not text.
- [ ] **AC4 (R3):** When the current ID is the unique maximum, the result is `currentLatest`. When current and another ID share the maximum, latest is tied, sorted IDs are emitted, and relationship is `tiedLatest`. When a tie excludes current, relationship is unknown rather than an arbitrary candidate transition.
- [ ] **AC5 (R4):** Exact equal bases report `unchanged`; different bases, including case-only differences, report `changed`; an empty/missing current or latest base reports `unknown`. A unique latest with missing base still retains its other known identity/timestamp facts and has nullable base output.
- [ ] **AC6 (R2, R7):** A nil/absent version list and a supplied empty list produce explicit distinct unknown reasons. Any missing, null, empty, or malformed visible timestamp makes latest unknown rather than selecting the maximum of the remaining entries.
- [ ] **AC7 (R3):** When current is absent from an otherwise valid visible list, a unique latest and its base comparison may remain known, but relationship is unknown with `currentVersionNotVisible`; the output never calls it newer or current latest.
- [ ] **AC8 (R2):** Missing/nonpositive/duplicate visible version IDs produce explicit unknown identity evidence. No tie or malformed input is resolved by list order, highest ID, name, or base value, and tied ID arrays are sorted deterministically.
- [ ] **AC9 (R5, R6):** Human and normal JSON agree for current latest, older current, base changed/unchanged/unknown, empty versions, current absent, missing/invalid/tied timestamps, and model degradation. Human output remains one concise `LATEST` line and contains no recommendation/outdated language.
- [ ] **AC10 (R7):** Model 404, transport failure, undecodable model body, and missing `modelId` preserve the successful by-hash hit and existing warning behavior while emitting unknown latest/comparison/base states. No supplementary model/version request, new retry policy, or new warning is introduced; any transient-failure attempts come solely from the pre-existing client policy.
- [ ] **AC11 (R1, R6):** `vet --prompts --json` remains byte-contract compatible with the existing focused recipe renderer and has exactly the `prompts` and `skipped` top-level keys. Human `--prompts` retains the new normal report line followed by the existing recipe blocks.
- [ ] **AC12 (R1):** A local file and its corresponding full SHA256 input, served the same by-hash/model bodies, emit identical latest context and request sequences. Existing usage errors, not-found handling, license/creator/model-null behavior, and exit-code tests remain green.
- [ ] **AC13 (R8):** Offline route allowlists observe no version-detail, image, download, upload, sync, or other extra request. The full project quality gate passes, and the parent receives all shared-surface handoff items below.

## Parent-owned integration and documentation scenarios

The parent must apply these scenarios to `scripts/integration-test.sh` and align README/CHANGELOG/relevant specs with the delivered contract:

1. **Existing `vet_hit` normal JSON:** require the additive `latestPublished` object and validate its internal invariants without pinning live Anima to remain current latest forever. For `status: known`, require a version object with positive ID, name/publishedAt keys, original RFC3339 timestamp, and nullable base; `currentLatest` must match `report.version.id`, while `newerPublished` must differ. For `status: tied`, require `version: null`, at least two sorted unique tied IDs, and unknown base change; `tiedLatest` additionally requires the current ID in that array. For `status: unknown`, require `version: null`, a non-empty latest reason, unknown comparison, and unknown base change. Every unknown comparison requires a non-empty comparison reason. Assert relationship/base tokens belong to the approved vocabularies.
2. **Existing `vet_autov2` human hit:** require one `LATEST` line and known/tied/unknown state wording; reject `outdated`, upgrade recommendation, and best/recommended wording in that line. Keep the existing exit `0` assertion.
3. **Existing `vet_prompts` focused JSON:** assert exactly `prompts` and `skipped` at the top level, non-empty recipes as today, and absence of `latestPublished` and the normal vet report keys.
4. **Existing negative cases:** keep `vet_miss` at exit `3` and `vet_badinput` at exit `1`, with no partial normal report or latest block on stdout.
5. **Documentation:** describe latest as maximum valid RFC3339 publication instant in the visible owning-model list, the strict incomplete/tied behavior, exact case-sensitive base comparison, current-version-absent behavior, no-extra-request degradation, and the informational/non-recommendation scope. Update the normal JSON shape to `{hash,version,model,latestPublished,examples}` while explicitly retaining the focused prompt shape.
6. **Offline-only evidence:** current/older/base/timestamp/tie/current-absent/model-degradation matrices, exact existing request counts, and local-file equivalence remain unit-test scenarios. Ordinary success/404/decode paths make two requests; missing `modelId` or by-hash failure makes one; injected transient failures make only the attempts allowed by the pre-existing configured retry policy. Do not add downloads or manufacture live outage dependencies in integration.

## Out of scope

- Any `sync` output or request change, per-file owning-model fan-out, `sync --latest`, or a new top-level statistics/latest command.
- A new vet flag, default/recommended/best version selection, automatic download, upgrade/migration advice, outdated verdict, compatibility verdict, or acquisition workflow.
- Historical chronology beyond the current response, rollback detection, trend analysis, scheduled-release inference, or wall-clock freshness.
- Additional API requests, retries, endpoints, API response fields, version-detail lookups, image/community requests, downloads, uploads, local-library scanning, persistence, or caching.
- Changes to by-hash source of truth, current report facts, exit codes, authentication, NSFW behavior, normal recipe content, or the focused prompt JSON schema.
- Child edits to shared integration scripts, README, CHANGELOG, bundled skill references, or `.trellis/spec`; those are parent-coordinated.

## Planning status

No blocking open questions remain. The selected `LATEST` line, `latestPublished` field, state/reason vocabulary, strict incomplete-timestamp behavior, and conservative tie handling are ready for final output-contract approval. Implementation remains unstarted pending that approval.
