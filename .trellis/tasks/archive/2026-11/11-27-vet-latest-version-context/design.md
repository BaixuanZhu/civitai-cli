# Technical design: vet latest published version context

> Status: converged design for final planning review. Concrete names and rendering are selected minor output details, not yet implementation approval. No task has been started and no product code has changed.

## 1. Behavior gap and change boundary

Vet already has all required upstream data but uses the owning model only for creator, license, model name, and comments. The missing behavior is a truthful local projection from the embedded version list to:

1. a unique latest-published visible version or an explicit tie/unknown state;
2. the vetted version's relationship to that latest result; and
3. an exact current/latest base-model comparison.

That projection belongs beside vet's command-specific report contract. It does not belong in the by-hash endpoint, sync, search's display-only date helper, or compare's model-summary value helper.

### Child implementation boundary after approval

| Path | Responsibility |
|---|---|
| `internal/cmd/vet_latest.go` (new) | Pure latest/context evaluator, private report DTOs, state/reason constants, and latest-line formatting helpers |
| `internal/cmd/vet.go` | Evaluate once for normal output, pass the result to human/JSON assembly, add the additive JSON field and human line, update vet help/GoDoc |
| `internal/cmd/vet_latest_test.go` (new) | Pure table-driven semantics and deterministic edge-case tests |
| `internal/cmd/vet_test.go` | Fixture/command rendering, degradation, hash/local equivalence, route allowlist, and request-count tests |
| `internal/cmd/prompts_test.go` | Pin the focused recipe-only JSON contract and human prompt compatibility |

The existing `api.Model`, `api.ModelVersion`, fixtures, API client, compare package, search helper, sync code, and report package require no changes. If implementation finds that the approved facts require an API type or endpoint change, stop and return to planning rather than widening scope.

### Parent-coordinated boundary

The parent serializes `scripts/integration-test.sh`, README, CHANGELOG, bundled skill references if needed, and durable backend spec updates. The child supplies exact scenarios and does not edit those shared files or any sibling task directory.

## 2. Architecture and data flow

```text
path/hash input
  └─ existing vet classification + optional streaming SHA256
       └─ GET /model-versions/by-hash/{hash}       (authoritative current version)
            └─ existing best-effort GET /models/{modelId}
                 └─ optional *api.Model             (creator/license + visible versions)
                      └─ pure deriveVetLatestPublished(current, model)
                           ├─ latest status/reason/version/tied IDs
                           ├─ current-to-latest comparison status/reason
                           └─ exact baseModelChange
                                ├─ normal human: one LATEST line
                                └─ normal JSON: latestPublished block

focused --prompts --json
  └─ existing renderPrompts return before normal report derivation/rendering
```

`runVet` derives the context once after the focused prompt-JSON early return and before the normal human/JSON branch. Human and JSON therefore consume the same result and cannot make different latest or base decisions. The focused path keeps its existing by-hash plus model fetch behavior but does not acquire the new output field.

The helper has no HTTP, filesystem, clock, logging, random source, process exit, or rendering writer. It accepts the decoded by-hash version and optional model and returns a complete deterministic value. No new package-level mutable state or public API is introduced.

## 3. Why existing latest helpers are not reused directly

- `internal/cmd/search.go:304-314` returns the lexicographically largest timestamp string. That is suitable only for its existing homogeneous display assumption; offsets can order differently as text than as instants, and the helper has no identity/tie/missing-data contract.
- `internal/compare/compare.go:760-785` correctly uses `time.Parse(time.RFC3339)` and rejects incomplete timestamp evidence, but it accepts `*api.CompareModel`, returns a generic comparison `Value`, and deliberately derives only a normalized timestamp. Exporting or reshaping it to serve vet would couple two different public output contracts without supplying identity, ties, relationship, or base state.

The vet helper therefore reuses only the standard-library parsing pattern and the compare package's conservative completeness principle. It does not modify either helper or add a dependency on `internal/compare`.

## 4. Pure evaluation contract

### 4.1 Input interpretation

- `current` is the standalone by-hash `*api.ModelVersion`; only its positive ID and exact `BaseModel` are needed for this feature. Its own `PublishedAt` is not substituted for the matching embedded timestamp when determining relationship.
- `model == nil` represents every existing model-fetch degradation path. The helper does not distinguish operational causes because the existing warning already does so.
- A nil `model.ModelVersions` means the field was absent or null. A non-nil zero-length slice means the API supplied `[]`. This preserves the project's missing-versus-empty distinction using the existing DTO.
- Every entry present in the array is visible evidence. No additional status/availability filter or wall-clock check is invented.
- Empty names/bases are unavailable display/comparison values, not fabricated strings. Timestamp strings are never trimmed into validity; parse the supplied value as-is.

### 4.2 Fixed evaluation precedence

The evaluator returns at the first latest-level failure in this order, making reason selection deterministic:

1. **Model unavailable:** `model == nil` -> latest `unknown/modelUnavailable`.
2. **Version list unavailable:** `model.ModelVersions == nil` -> `unknown/versionsUnavailable`.
3. **No visible versions:** non-nil length zero -> `unknown/noVisibleVersions`.
4. **Identity evidence invalid:** any entry has `ID <= 0`, or the same positive ID occurs more than once -> `unknown/versionIdentityUnavailable`. Requiring one unambiguous identity per visible entry prevents conflicting duplicate entries from masquerading as a unique latest.
5. **Timestamp evidence incomplete:** any entry has an empty timestamp or `time.Parse(time.RFC3339, value)` fails -> `unknown/timestampUnavailable`. Do not skip that entry and select from the remainder.
6. **Maximum tie:** find the maximum parsed instant and all entries equal to it. More than one entry (necessarily distinct IDs after step 4) -> latest `tied/latestTied`, `version: null`, and ascending numeric `tiedVersionIds`.
7. **Unique latest:** exactly one maximum entry -> latest `known/none` with its source facts and an empty, non-null `tiedVersionIds` array.

Timestamp comparisons use `Time.Before`, `After`, and `Equal`; equivalent offsets representing the same instant tie. No ID or response-order comparison participates in maximum selection. The selected JSON `publishedAt` preserves the winning entry's original source string rather than normalizing it; parsing is for comparison only.

### 4.3 Current relationship algorithm

After latest evaluation, derive `comparison.status` and `comparison.reason` independently:

| Latest result | Current visibility/evidence | Comparison status | Comparison reason |
|---|---|---|---|
| Any `unknown` latest | Not evaluated further | `unknown` | `latestUnavailable` |
| `known` or `tied` | Current ID has no visible entry | `unknown` | `currentVersionNotVisible` |
| `tied` | Current ID is in `tiedVersionIds` | `tiedLatest` | `latestTied` |
| `tied` | Current is visible but not in the maximum tie | `unknown` | `latestTied` |
| `known` | Current ID equals unique latest ID | `currentLatest` | `none` |
| `known` | Different current entry exists and latest instant is after current instant | `newerPublished` | `none` |
| `known` | Any inconsistent residual ordering (defensive unreachable case after valid maximum evaluation) | `unknown` | `timestampOrderIndeterminate` |

A tie excluding current deliberately does not emit `newerPublished`, even though at least one tied entry is later. The feature's contract concerns relationship to *the latest version*, and no unique latest exists; keeping that relationship unknown avoids implying a selected transition target.

When current is absent, latest identity remains visible if unique, but relationship stays unknown. The matching embedded current timestamp, not standalone current `publishedAt`, is used for `newerPublished`, so both operands come from one visible-list snapshot.

### 4.4 Exact base-model algorithm

Base comparison is independent of relationship:

- If latest is not `known`, `baseModelChange` is `unknown`; no tied candidate is chosen for comparison.
- If latest is known and both `current.BaseModel` and `latest.BaseModel` are non-empty, exact Go string equality yields `unchanged`, otherwise `changed`.
- If either value is empty, `baseModelChange` is `unknown`.
- This calculation still runs when relationship is `currentVersionNotVisible`; known base equality/difference is not a chronology claim.

No case folding, whitespace trimming, alias table, semantic base-family mapping, or compatibility decision is applied. Thus `Anima` and `anima` are changed, while two byte-identical strings are unchanged.

## 5. Normal JSON contract

Add the following required field to the existing unversioned legacy vet report, between `model` and `examples` in Go struct/JSON order:

```json
"latestPublished": {
  "status": "known",
  "reason": "none",
  "version": {
    "id": 3263843,
    "name": "turbo-v1.1",
    "publishedAt": "2026-08-24T23:42:31.929Z",
    "baseModel": "Anima"
  },
  "tiedVersionIds": [],
  "comparison": {
    "status": "currentLatest",
    "reason": "none",
    "baseModelChange": "unchanged"
  }
}
```

### 5.1 Field invariants

| Field | Contract |
|---|---|
| `status` | `known`, `tied`, or `unknown` |
| `reason` | `none` only for known; `latestTied` for tied; otherwise `modelUnavailable`, `versionsUnavailable`, `noVisibleVersions`, `versionIdentityUnavailable`, or `timestampUnavailable` |
| `version` | Object only for unique known latest; otherwise JSON null |
| `version.id` | Positive unique visible version ID |
| `version.name` | Source name, or null when empty |
| `version.publishedAt` | Original non-empty valid RFC3339 source string; no date truncation or timezone rewriting |
| `version.baseModel` | Source base, or null when empty |
| `tiedVersionIds` | Always a JSON array; empty for known/unknown, ascending unique positive IDs for tied |
| `comparison.status` | `currentLatest`, `newerPublished`, `tiedLatest`, or `unknown` |
| `comparison.reason` | `none`, `latestUnavailable`, `latestTied`, `currentVersionNotVisible`, or `timestampOrderIndeterminate`, following section 4.3 |
| `comparison.baseModelChange` | `changed`, `unchanged`, or `unknown`, following section 4.4 |

All keys are always emitted; there are no `omitempty` fields in this additive block. This makes unknown structure as stable as known structure. No raw embedded version array, fetch time, current-time value, recommendation, or derived duration is added.

Example current-absent result with independently known base equality:

```json
"latestPublished": {
  "status": "known",
  "reason": "none",
  "version": {"id": 20, "name": "v2", "publishedAt": "2026-02-02T00:00:00Z", "baseModel": "Anima"},
  "tiedVersionIds": [],
  "comparison": {"status": "unknown", "reason": "currentVersionNotVisible", "baseModelChange": "unchanged"}
}
```

Example tie including current:

```json
"latestPublished": {
  "status": "tied",
  "reason": "latestTied",
  "version": null,
  "tiedVersionIds": [10, 20],
  "comparison": {"status": "tiedLatest", "reason": "latestTied", "baseModelChange": "unknown"}
}
```

Model degradation uses `status: unknown`, `reason: modelUnavailable`, `version: null`, an empty tied array, and `comparison: {status: unknown, reason: latestUnavailable, baseModelChange: unknown}`. Existing top-level `model` remains null exactly as before.

## 6. Human contract

Insert one line after `VERSION` and before `CREATOR`. Existing lines and later sections retain their content and order.

Unique current-latest example:

```text
LATEST    turbo-v1.1 (id 3263843, 2026-08-24)   base: Anima   relation: current latest; base unchanged
```

Unique newer/changed example:

```text
LATEST    v2 (id 20, 2026-02-02)   base: SDXL   relation: newer published; base changed
```

Current-absent example:

```text
LATEST    v2 (id 20, 2026-02-02)   base: Anima   relation: unknown (current version not visible); base unchanged
```

Tie example:

```text
LATEST    unknown (latest tied; version ids 10,20)   relation: tied latest; base change unknown
```

Model-degraded example:

```text
LATEST    unknown (model details unavailable)   relation: unknown; base change unknown
```

Rendering rules:

- Map `currentLatest`, `newerPublished`, and `tiedLatest` to `current latest`, `newer published`, and `tied latest`; map `baseModelChange` to `base changed`, `base unchanged`, or `base change unknown`.
- For known latest, print the source name or `unknown`, full ID, `report.DateText(publishedAt)`, and source base or `unknown`. Only the display name is eligible for existing console-width truncation; IDs, statuses, reasons, timestamps, and base values are not abbreviated by this feature.
- For tied/unknown latest, do not print a candidate name/timestamp/base. Print sorted tie IDs for a tie or a concise reason mapped from the stable reason token.
- When comparison is unknown and latest is known/tied, append its specific reason (`current version not visible`, `latest tied`, or the defensive `timestamp order indeterminate`). For latest-level unknown, one concise latest reason is sufficient rather than duplicating `latest unavailable`.
- Human `--prompts` continues to append the existing prompt blocks after the full report. Focused prompt JSON emits none of this human decoration.

The word `latest` is a defined timestamp observation, not advice. The renderer has no strings for outdated/upgrade/recommended/best/rollback/trend claims.

## 7. Compatibility, degradation, and observability

- The by-hash version remains the current identity/base source. No latest version replaces or mutates it, and no file hash is recomputed against another version.
- `fetchVetModel` remains the sole model request and keeps its existing warning text/control flow. A nil model becomes explicit unknown output; there is no second warning for latest context and no change to creator/license degradation.
- Expected data-quality states (empty list, bad timestamp, duplicate identity, tie, current absent, missing base) are silent report states, not log events. Existing debug/info logging is unchanged.
- Normal JSON is intentionally additive rather than a new versioned envelope or `--view` mode. Focused recipe JSON bypasses `newVetReport` and remains exactly `{prompts,skipped}`.
- Vet help gains a short note defining latest-published visible context and its non-recommendation nature; no option or example syntax changes. Public `Vet` GoDoc and relevant private comments are updated with the behavior.
- No migration, persisted state, configuration, or rollout flag is needed.

## 8. Test design

### 8.1 Pure evaluator table

Use synthetic `api.ModelVersion` values with IDs `10`, `20`, and `30` and explicit timestamps. Each case asserts the complete latest block, not just one field:

1. unique current latest, same base;
2. older current, unique later version;
3. base changed, unchanged, case-only changed, current empty, latest empty;
4. nil model, nil versions, supplied empty versions;
5. current ID absent while unique latest/base comparison remain known;
6. each of missing/empty and malformed timestamp on both a non-maximum and a potential-maximum entry, proving invalid entries are never skipped;
7. maximum tie including current and excluding current, with shuffled input order and sorted tied IDs;
8. RFC3339 offset cases where lexicographic order differs from chronological order, plus different offset strings at the same instant producing a tie;
9. zero/negative and duplicate IDs;
10. empty latest name/base producing nullable identity fields while other known facts survive;
11. defensive residual timestamp-order state if directly constructible, otherwise document why the valid-input algorithm cannot reach it.

Repeat tied cases with reversed input and assert byte-identical JSON to prove response order does not select a winner.

### 8.2 Command and output tests

- Extend the Anima human/JSON fixture tests with the exact current-latest facts and original timestamp.
- Add an inline older-current model body to prove human and JSON agree on latest identity, `newerPublished`, and base state.
- Extend model-degraded human and JSON tests to require unknown context while preserving `CREATOR -`, `LICENSE unknown`, `model: null`, existing warning text, and nil error. Cover model 404, undecodable model JSON, missing `modelId`, and an injected transport failure with the existing `WithBackoffs()` test option used to avoid retry waits.
- Add a shared synthetic command matrix that runs both human and normal JSON for current latest, older current, base changed/unchanged/unknown, empty versions, current absent, missing/invalid/tied timestamps, and degraded model evidence. Assert complete corresponding states/facts in each view rather than checking only that the line/key exists.
- Use a vet-prefixed route recorder, separate from compare helpers, to assert exact request paths and counts for normal human, normal JSON, focused prompt JSON, model 404/decode/missing-ID/transport degradation, and by-hash 404. Unmapped/unexpected routes fail the test rather than silently succeeding; transient tests distinguish configured pre-existing attempts from supplementary endpoint calls.
- Run direct full-hash and corresponding local-file inputs against identical response bodies and compare the complete `latestPublished` value (and human latest line), not the unrelated input-hash handling.
- Decode focused prompt JSON into a map and assert exactly `prompts`/`skipped`; additionally compare with the existing `renderPrompts` result to catch accidental additive fields. Existing recipe content/count tests remain unchanged.
- Assert human unknown/tie/newer output never contains outdated/upgrade/recommended/best wording in the latest line.

No live request, new verbatim fixture, API test change, or sync test change is needed.

## 9. Tradeoffs and rollback

| Decision | Benefit | Cost / rejected alternative |
|---|---|---|
| Require complete timestamps across all visible entries | Never labels a maximum-of-known-subset as definitive latest | One malformed old entry makes the whole latest claim unknown; skipping it would be more available but less truthful |
| Reject duplicate/invalid IDs | Prevents conflicting entries from becoming false unique evidence | Slightly stricter degradation for malformed payloads |
| Preserve ties instead of deterministic winner selection | Honest ambiguity with stable sorted evidence | Consumers must handle a tied state; ID/order tie-breaks would invent meaning |
| Separate latest, relationship, and base states | Retains independently supported facts | More structured fields than one flattened freshness label |
| Vet-local pure helper | Minimal boundary and no cross-command coupling | Reimplements a small RFC3339 loop because existing helpers return the wrong abstraction |
| Original timestamp in JSON | Preserves upstream source fact while comparisons use instants | Equivalent offsets remain textually different; consumers must not reintroduce lexicographic ordering |

Rollback is a clean additive revert of `vet_latest*`, the vet call/render/report additions, and their tests. Hash lookup, model fetching, prompts, license/creator reporting, and exit behavior remain intact. The parent removes corresponding shared docs/integration assertions in the same pre-release rollback. After release, prefer a forward correctness fix because normal JSON consumers may already rely on the additive field; any removal must be explicitly documented rather than silently reverting state semantics.
