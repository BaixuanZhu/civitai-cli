# Agent workflow execution plan

> Status: planning converged and unstarted. Do not run `task.py start` until explicit implementation approval of the final planning summary.

## 1. Review gates before implementation

- [x] All parent and child “Review topic” sections have been replaced by approved decisions or explicitly deferred items.
- [x] Final planning convergence pass completed with requirement IDs, evidence anchors, and acceptance mappings preserved.
- [x] Command tree and reference grammar approved: recipe exact-version/community image, compare models/versions, resolve exact four-form refs.
- [x] Resolve readiness/exit semantics and conservative compatibility policy approved (ready 0, not-ready 5, exact-base policy, no name-only).
- [x] Summary/detail schema, community defaults (20/Most Reactions/no aggregation), and all child output contracts approved.
- [x] Default compact with `--view full`, the search/show compact D2 field registry, presence rules, and view validation are approved. Preserve the registry in implementation docs and migration wording.
- [x] Official detail uses `civ recipe <versionId> --example <sourceIndex>`; community detail uses separate `civ recipe image <imageId>` with one exact bounded `imageId+withMeta=true+type=image+limit=1` request and no cross-invocation state.
- [x] Shared scalar normalization is approved: known numeric strings become numbers (including weights), seed stays decimal string, missing/null stays null, invalid/non-scalar values produce null plus issue, and unknown fields/units are not inferred.
- [x] Official `--examples` is approved as default 3 with range 0–10; zero returns context/counts with empty examples, and count 1 remains summary-only.
- [x] Curate real `implement.jsonl` and `check.jsonl` entries for every task that will be started.
- [ ] Obtain explicit user approval of the converged final planning summary. Task creation and this plan are not implementation approval.

## 2. Ordered delivery checklist

### Stage A — Contract and fixture preparation

- [ ] Convert approved command grammar and reference rules into table-driven cases before implementation.
- [ ] Identify the minimum additive DTO fields and endpoint calls for each child; search for existing decoders/projections before adding new ones.
- [ ] Add only the fixtures needed by the active child, with source/date entries in `testdata/FIXTURES.md` and no secrets or private content.
- [ ] Write expected normalized JSON examples for selected, missing, partial, and unknown data.

### Stage B — Recipe core (`11-24-recipe-context`)

- [ ] Start only the recipe child after parent approval and its own explicit implementation authorization.
- [ ] Implement exact-version prompt-free summaries and `civ recipe <versionId> --example <sourceIndex>` singular detail, including API orchestration, normalized output, help, and errors.
- [ ] Add offline unit/fixture tests and the child's integration-test cases.
- [ ] Run child quality review, update relevant specs, and archive only after acceptance is demonstrated.
- [ ] Parent reviews the resulting reference/projection contract before community work begins.

### Stage C — Independent children

`11-24-resource-resolver` and `11-24-model-version-compare` may be delivered independently once their reference dependencies are satisfied.

- [ ] Start one child per independently verifiable deliverable; do not start the parent merely because children exist.
- [ ] If parallelized, assign disjoint files and serialize shared API/reference ownership. Parent/orchestrator owns `main.go`, README/CHANGELOG, integration script, and final Skill assets.
- [ ] Resolve: implement explicit resource-set parsing, exact local identity, compatibility/readiness evaluation, reason codes, help, JSON, and reviewed exit behavior.
- [ ] Compare: implement model/version modes, bounded fetching, aligned output, derived differences, diff-only view, help, and JSON.
- [ ] Search/show compaction: using the approved D2 registry and default-compact policy, implement compact projections over the existing responses, preserve selection identifiers and record counts, wire `--view compact|full`, and update all affected existing-command integration assertions without weakening unrelated contracts.
- [ ] Each child updates its own integration coverage and passes offline/race checks before archival.

### Stage D — Community recipe source (`11-24-community-recipes`)

- [ ] Reconfirm the recipe core handoff and bounded list/detail UX after Stage B.
- [ ] Add the `/images` endpoint and tolerant free-form metadata fixtures needed by the reviewed source.
- [ ] Implement bounded prompt-free summaries, coverage/provenance, separate `civ recipe image <imageId>` detail with one exact `imageId+withMeta=true+type=image+limit=1` request, and any approved observed-value aggregation.
- [ ] Test null/partial metadata, empty pages with cursors, visibility-safe errors, unknown resource forms, and the prohibition on automatic exhaustive pagination.
- [ ] Add integration coverage and complete the child quality/spec/archive cycle.

### Stage E — Final grouped help (`11-24-grouped-help`)

- [ ] Confirm every accepted preceding child is integrated and every deferred/withdrawn command is removed from the expected inventory; do not start this child earlier.
- [ ] Snapshot the final pre-grouping top-level help and per-command help/completion behavior.
- [ ] Replace only the flat overview command-list block with the reviewed `Discover`, `Recipes`, `Local models`, and `Utilities` groups, preserving command rows and `unbootstrap` hiding.
- [ ] Add structural inventory/order/uniqueness and stdout/stderr/exit tests, update any flat README snapshot, and adjust `scripts/integration-test.sh` without changing named/per-command help or completion.
- [ ] Run offline/full quality checks and complete the final child review before parent integration.

### Stage F — Parent integration review

- [ ] Verify PAC-1 through PAC-14 in the parent PRD across the assembled command surface.
- [ ] Audit every new/changed list and nested summary array for large fields; prove prompt/negativePrompt/description/raw-meta/workflow data appears only in explicitly selected detail or legacy full/prompts compatibility views.
- [ ] Update top-level help grouping only as approved; do not hide or remove existing commands.
- [ ] Update README task flows, command references, JSON/exit documentation, API notes, and boundaries.
- [ ] Update the embedded `civ-prompt` Skill to use recipe normalized output and optional compare/resolve only where they reduce Agent errors; retain low-level fallbacks.
- [ ] Reconcile all `scripts/integration-test.sh` additions with the final surface, including offline usage/help cases and live success/error cases.
- [ ] Update CHANGELOG only for actually delivered behavior, not for unimplemented children or deferred ideas.

## 3. Validation commands

Planning artifact validation:

```bash
python ./.trellis/scripts/task.py validate 11-24-agent-workflows
python ./.trellis/scripts/task.py validate 11-24-recipe-context
python ./.trellis/scripts/task.py validate 11-24-resource-resolver
python ./.trellis/scripts/task.py validate 11-24-model-version-compare
python ./.trellis/scripts/task.py validate 11-24-community-recipes
python ./.trellis/scripts/task.py validate 11-24-search-show-compact
python ./.trellis/scripts/task.py validate 11-24-grouped-help
```

Per-child and final code validation after implementation is authorized:

```bash
gofmt -w <owned-go-files>
go vet ./...
go test ./...
go test -race ./...
go list -m all
bash scripts/integration-test.sh
```

The integration script is an online/drift check and is not a replacement for offline unit tests. Do not run registry-mutating tests locally. Build a development binary with bootstrap disabled for manual smoke tests.

## 4. Risky files and review focus

- `main.go`: additive dispatch/help only; no business logic. The final grouped-help child changes only the overview command-list block after all other edits are complete.
- `internal/api/types.go` and new endpoint files: tolerant drift handling and no accidental legacy JSON shape changes.
- `internal/cmd/search.go`, `internal/cmd/show.go`, and focused projection files: compact views must not change request behavior, drop records, or accidentally alter human/`--prompts`/unrelated legacy output.
- `internal/cmd/globals.go`: shared flags/error behavior must not change unintentionally; resolve outcome mapping needs explicit review.
- `internal/skill/assets/civ-prompt/**`: instructions must not claim missing data, compatibility, or generation capabilities.
- `scripts/integration-test.sh`: surface parity is mandatory; avoid fixtures/caches that hide upstream drift.
- `README.md` and `CHANGELOG.md`: final behavior only, with no promised export/library functionality.

## 5. Rollback points

- Before starting each child, confirm its diff can be removed without migrating state or changing legacy outputs.
- Roll back a failing child's dispatch and consumer code before changing established commands to accommodate it.
- If shared reference behavior is wrong, fix it in the single owner and rerun all consuming child tests; never patch command-local interpretations.
- If compact search/show projection causes drift, restore the previous output paths and remove the compact view wiring together; do not keep a partial default change or alter unrelated command tests to hide the regression.
- If live community metadata differs from fixtures, preserve tolerant decoding, narrow the normalized claim, and do not compensate by exhaustive requests or guessing.
- No commit, release, task archival, or implementation is part of the current planning-only request.
