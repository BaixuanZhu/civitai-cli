# Implementation and validation plan: vet latest published version context

> Status: converged execution plan for final planning review. Every implementation checkbox is intentionally unchecked. Planning has not run `task.py start`, changed product code, or executed the implementation steps below.

## 0. Review and readiness gate

- [ ] Review the PRD's selected `LATEST` line, additive `latestPublished` JSON block, state/reason vocabulary, strict timestamp-completeness rule, sorted tie behavior, and separate relationship/base states.
- [ ] Confirm the child owns only `internal/cmd/vet*` implementation/test work and the focused assertion in `internal/cmd/prompts_test.go`; shared integration/docs/spec work remains with the parent.
- [ ] Confirm both JSONL manifests contain real spec-only context entries and no code paths.
- [ ] Obtain explicit approval of the latest planning summary before the authorized session runs `python ./.trellis/scripts/task.py start 11-27-vet-latest-version-context`. Do not start the parent for this child implementation.
- [ ] Inspect `git status --short` before implementation and record unrelated dirty files/task directories; never revert or include them in this child's change set.

**Gate A:** no implementation until final output-contract approval and explicit child activation. Any need for another request, implicit version choice, or less conservative evidence rule returns to planning.

## 1. Establish the compatibility baseline

- [ ] Read the curated backend specs, this PRD/design, and the current parent artifacts again immediately before editing; do not modify parent/sibling artifacts.
- [ ] Run the focused existing tests before edits:

  ```bash
  go test ./internal/cmd -run 'TestVet|TestRenderPrompts|TestShow_Prompts'
  ```

- [ ] Confirm the current Anima fixture still supplies the expected unique latest version `3263843` and the focused prompt path returns before `newVetReport`.
- [ ] Confirm no API type/fixture change is necessary. Use existing verbatim fixtures unchanged and synthetic inline model/version bodies for edge cases.
- [ ] Verify `go list -m all` prints only `civitai-cli`; preserve standard-library-only implementation.

**Rollback point R1:** if the baseline fails in files outside this child, stop and report the unrelated failure rather than repairing or formatting it. If fixture evidence contradicts the planned contract, return to planning without changing fixtures.

## 2. Implement the pure latest/context evaluator

- [ ] Add `internal/cmd/vet_latest.go` with private DTOs matching the design: latest status/reason, nullable version facts, non-null tied IDs, and nested comparison status/reason/base change.
- [ ] Implement fixed latest evaluation precedence: nil model, nil versions, empty versions, invalid/duplicate IDs, incomplete/invalid timestamps, maximum tie, unique latest.
- [ ] Parse every supplied timestamp with `time.Parse(time.RFC3339)` and compare instants. Preserve the selected source timestamp string for JSON.
- [ ] Implement current relationship from the matching embedded current entry and unique/tied latest evidence; do not use by-hash `publishedAt`, list order, ID order, or local time.
- [ ] Implement independent exact base comparison from by-hash current base to unique latest base. Keep missing bases and ties unknown.
- [ ] Ensure tied IDs are ascending and unique, tied/unknown versions serialize as null, empty tied IDs serialize as `[]`, and empty latest name/base serialize as null.
- [ ] Add concise comments for non-obvious conservative choices (complete timestamp evidence, tie behavior, independent base comparison). Add/update GoDoc wherever an exported symbol is introduced; the preferred design introduces no exported production symbol.

### Pure test matrix in `internal/cmd/vet_latest_test.go`

- [ ] Current latest and older current, including full latest ID/name/publishedAt/base and comparison assertions.
- [ ] Base changed, unchanged, case-only changed, missing current base, and missing latest base.
- [ ] Nil model, nil version list, and supplied empty version list with their distinct reasons.
- [ ] Current absent: unique latest remains known, relationship is `unknown/currentVersionNotVisible`, and base comparison follows the two available base strings.
- [ ] Missing, empty, and malformed timestamps on both lower and maximum candidate entries; each makes latest unknown instead of skipping the bad entry.
- [ ] Tied maximum including current (`tiedLatest`) and excluding current (`unknown/latestTied`), with sorted IDs, null version, and unknown base change.
- [ ] RFC3339 fractional/offset cases, including lexicographic-versus-instant ordering and equivalent offsets tying at the same instant.
- [ ] Zero, negative, and duplicate IDs yielding `versionIdentityUnavailable`.
- [ ] Empty latest name/base yielding nullable fields without discarding known ID/timestamp or unrelated base state.
- [ ] Reverse/shuffle tied input and assert the complete result/JSON is deterministic and does not select the current version as a tie-breaker.

**Stage validation:**

```bash
gofmt -w internal/cmd/vet_latest.go internal/cmd/vet_latest_test.go
go test ./internal/cmd -run 'TestVetLatest'
```

**Rollback point R2:** delete only the additive evaluator/test file if the state model is wrong. Do not simplify ties, skip invalid timestamps, or reuse search's lexicographic helper to make tests pass; return to design review instead.

## 3. Wire normal vet output without changing request flow

- [ ] In `runVet`, retain the focused `g.JSON && o.prompts` early return. Derive latest context once only for normal output, after the existing by-hash and best-effort model fetch.
- [ ] Pass the same derived value to normal human and JSON assembly; do not recalculate selection in either renderer.
- [ ] Add `latestPublished` to `vetReport` and populate it in `newVetReport`. Preserve existing `hash`, `version`, `model`, and `examples` fields and model-null behavior.
- [ ] Insert one `LATEST` line after `VERSION` in `renderVetReport`, using the design's known/tied/unknown formats and concise reason mapping. Preserve all existing sections and human prompt appending.
- [ ] Update `Vet` GoDoc, `runVet`/report comments, and the curated vet help note to describe informational latest-published visible context and no upgrade recommendation. Add no flags or invocation changes.
- [ ] Leave `fetchVetModel`, by-hash error handling, `vetExitCode`, warning text, API client options, and prompt renderer untouched.

### Command/output tests

- [ ] Extend `TestVet_ReportFixture` for the Anima `LATEST` line and `TestVet_JSON` for the complete known `latestPublished` block, including original full timestamp and empty tied array.
- [ ] Add an inline older-current case that checks human/JSON agreement for unique latest, `newerPublished`, and changed/unchanged base as applicable.
- [ ] Extend human and JSON model-degradation tests for `unknown/modelUnavailable`, `unknown/latestUnavailable`, and `unknown` base change while retaining existing creator/license/model/warning assertions and nil error. Cover model 404, undecodable JSON, missing `modelId`, and an injected transport failure using the existing `WithBackoffs()` option to avoid retry waits without changing production retry policy.
- [ ] Add a shared synthetic command matrix that runs both human and normal JSON for current latest, older current, base changed/unchanged/unknown, empty versions, current absent, missing/invalid/tied timestamps, and model degradation; assert corresponding facts/states in both views in addition to the complete pure matrix.
- [ ] Check the latest line never uses outdated/upgrade/recommended/best/rollback/trend language and that no new warning is emitted for data-level unknown/tied states.
- [ ] Run existing vet exit, input-classification, license/creator, examples, AutoV2, and usage tests without weakening their expectations.

**Stage validation:**

```bash
gofmt -w internal/cmd/vet.go internal/cmd/vet_test.go
go test ./internal/cmd -run 'TestVet'
```

**Rollback point R3:** revert only the run/render/report wiring and output assertions while retaining or removing the coherent pure evaluator. Never change by-hash/model error behavior to accommodate the new output.

## 4. Prove focused-view, input-equivalence, and request-count compatibility

- [ ] Add a vet-prefixed invocation-local route recorder (or equivalent vet-local handler) that records ordered paths and rejects unexpected routes. Do not modify shared `testEnv` or reuse compare-prefixed helpers.
- [ ] For normal human, normal JSON, and focused `vet --prompts --json`, assert exactly two successful-path requests in order: by-hash, then owning model. Assert no `/model-versions/{id}`, `/images`, download, upload, or other route occurs.
- [ ] For model 404 and undecodable model JSON, assert the same two requests and a successful vet hit. For missing `modelId`, assert only the by-hash request; for by-hash 404, assert only one request and no partial stdout report with exit `3` unchanged. For injected model transport failure, use the existing no-backoff test option and assert one by-hash plus one model attempt, proving no supplementary endpoint call while leaving production retry configuration untouched.
- [ ] Serve identical by-hash/model bodies to a corresponding full-SHA256 input and local-file input; compare their complete latest JSON block and human `LATEST` line, plus ordered request paths. Keep unrelated hash computation/report assertions intact.
- [ ] Strengthen `TestVet_PromptsJSON` to assert exactly the `prompts` and `skipped` top-level keys, no `latestPublished`/normal report keys, and equality with the existing focused `renderPrompts` result for the same images.
- [ ] Keep `TestVet_PromptsAppendsRecipes` coverage proving the normal human report (including `LATEST`) is followed by the unchanged recipe blocks.

**Stage validation:**

```bash
gofmt -w internal/cmd/vet_test.go internal/cmd/prompts_test.go
go test ./internal/cmd -run 'TestVet|TestRenderPrompts'
go test -race ./internal/cmd -run 'TestVet|TestRenderPrompts'
```

**Gate B:** review representative known, newer, tied, current-absent, missing-base, and degraded outputs against the PRD. If focused JSON gained any key or any path gained a request, stop before parent handoff.

**Rollback point R4:** remove only the new test recorder/assertions if they are defective; product behavior must not be changed to satisfy a weak request counter. If genuine scope expansion is revealed, revert the additive output and return to planning.

## 5. Child quality gate and diff review

- [ ] Run focused and full offline validation from the repository root:

  ```bash
  go test ./internal/cmd -run 'TestVetLatest|TestVet|TestRenderPrompts'
  go vet ./...
  go test ./...
  go test -race ./...
  go list -m all
  git diff --check
  ```

- [ ] Verify `go list -m all` still prints only `civitai-cli` and no new dependency, generated file, fixture mutation, or network-dependent unit test was introduced.
- [ ] Inspect the complete child diff for English-only output/comments, GoDoc synchronization, stable required JSON keys, non-null tied arrays, deterministic ordering, and stdout/stderr separation.
- [ ] Trace the full data flow once more: by-hash current -> existing optional model -> pure result -> one human line or one normal JSON block. Confirm focused prompt JSON bypasses it and sync remains untouched.
- [ ] Review test quality: each required scenario fails if latest selection/base/relationship behavior is removed or replaced with first-entry/highest-ID/lexicographic selection; no tautological status-only assertions substitute for exact facts.
- [ ] Run `trellis-check` for the child scope and resolve findings against actual code/spec evidence. Do not edit parent/sibling/shared files to resolve child-local findings.

**Gate C:** all child-owned checks are green and the exact parent handoff below is ready. The child is not fully checked/archived until the parent applies its shared integration/docs ownership.

## 6. Parent-coordinated integration and documentation handoff

The parent/main orchestrator, not the child implementation agent, performs these edits serially:

- [ ] Update `scripts/integration-test.sh` using exactly PRD scenarios 1–4: conditional normal-JSON `latestPublished` invariants that tolerate live version drift, one human `LATEST` line for AutoV2, exact focused-prompt top-level keys, and unchanged miss/bad-input exits/empty partial output.
- [ ] Keep the complete edge-case/request-count/local-file matrix offline as PRD scenario 6 specifies; do not add a model/image download or live outage dependency to integration.
- [ ] Update README's vet section: normal JSON shape `{hash,version,model,latestPublished,examples}`, visible-maximum RFC3339 semantics, strict unknown/tie/current-absent states, exact base comparison, no-extra-request model degradation, focused prompt compatibility, and informational/non-recommendation wording.
- [ ] Review the child-updated vet help against README and integration signature assertions; no new flag row is required.
- [ ] Update CHANGELOG through the repository release process without inventing a version number in child code.
- [ ] Review durable spec updates under parent ownership: vet's pure additive derived context in backend design/directory guidance and model-degraded unknown latest behavior in error handling. Do not change global output/exit contracts or sibling feature specs.
- [ ] Update bundled `internal/skill/assets/civ-prompt` guidance only if the parent determines Agents should consume the new field; preserve the focused recipe-only contract either way.
- [ ] Build the tested binary and run the normal online integration canary when network policy permits:

  ```bash
  bash scripts/integration-test.sh <path-to-tested-civ.exe>
  ```

  Record skipped/failing live verification honestly; never replace offline edge cases with live calls.

- [ ] Re-run the full offline quality gate after shared edits and perform the parent's cross-child review for output density, request fan-out, compatibility, and scope drift.

**Rollback point R5:** for pre-release withdrawal, revert the child additive evaluator/wiring/tests and the corresponding parent integration/docs assertions together. No cache, config, downloaded content, migration, or persisted state needs cleanup. After release, prefer a forward fix and explicit compatibility review over silently removing the JSON field.

## 7. Final completion evidence

- [ ] Report the exact changed product/test files and parent-owned shared files separately.
- [ ] Map test names to PRD AC1–AC13, especially every required current/older/base/empty/absent/timestamp/tie/degraded/focused/request-count scenario.
- [ ] State the observed successful-path and degraded-path request counts and confirm no new API endpoint/retry was added.
- [ ] State whether online integration ran, passed, failed, or was skipped, with any upstream drift distinguished from offline contract failures.
- [ ] During finish work, use `trellis-update-spec` only through the authorized parent/spec owner, and follow the repository's separate commit-confirmation workflow. Planning and implementation do not themselves authorize commits or pushes.
