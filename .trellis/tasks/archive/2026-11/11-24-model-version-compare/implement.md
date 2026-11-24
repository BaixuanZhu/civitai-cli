# Implementation and validation plan: model and version comparison

> Status: approved plan, unstarted. D1–D8 are decided. Every checkbox is intentionally unchecked. Planning has not run `task.py start`, changed product code, or executed the implementation/validation steps below.

## Wave scope override (2026-11-24 parallel wave)

This child runs in a parallel wave on the shared working tree. These overrides win over any conflicting step below.

- **Owned files (only these may be created/edited):** `internal/api/compare.go`, `internal/api/compare_test.go` (use the `compare` prefix so they cannot collide with siblings' `recipe.go` / `compact*.go`), `internal/compare/**`, `internal/cmd/compare.go`, `internal/cmd/compare_test.go`. Prefix package-level test helpers with `compare` (e.g. `compareNewTestServer`).
- **Forbidden (parent integration pass owns them):** `main.go`, `internal/cmd/help.go`, `internal/cmd/help_test.go`, `internal/cmd/help_local_test.go`, `internal/cmd/completion*.go`, `internal/cmd/completion_test.go`, `scripts/integration-test.sh`, `README.md`, `CHANGELOG.md`, `internal/skill/assets/**`, `go.mod`, `go.sum`, `internal/api/types.go`, every other `internal/cmd`/`internal/api` file, and every `.trellis/tasks/` directory except this one.
- **Parent integration pass takes over:** Section 7 dispatch/usage/help-map/completion wiring, README, integration-script cases. Do not treat their absence as a failure; still define `compareHelp` in `internal/cmd/compare.go` as a package-level constant.
- **No git commands:** never run `git add`, `git commit`, or `git push`.
- **Sibling coexistence:** other agents are editing `internal/api` and `internal/cmd` concurrently. Ignore build/test failures that trace to files you do not own, and say so in the report; do not "fix" them.
- **Verification substitute:** run focused tests for your own files. If `go test ./...` fails only because a sibling's in-progress file is broken, report that instead of editing it.

## 0. Review and readiness gate

- [x] D1–D8 are approved: syntax, 2–5 unique subjects, `--diff`/JSON behavior, field registry, first-subject baseline, strict parent failures, file-pairing cascade, and sequential fetch.
- [x] `prd.md`, `design.md`, and this plan use the approved decisions without open alternatives.
- [ ] Review at least one concrete two-model JSON example, one old/new version JSON example, and full/`--diff` human examples against the approved field registry and missing-data vocabulary.
- [ ] Confirm the implementation boundary remains limited to additive compare/API/domain/command/help/completion/test/integration/documentation work. Keep recipe, resolve, local-library, export, and community work in their own tasks.
- [ ] Have the authorized planning owner curate this task’s `implement.jsonl` and `check.jsonl` with the relevant backend specs and approved research/context. The planning subtask that produced these artifacts deliberately did not edit those manifests.
- [ ] Obtain explicit user approval of the final planning summary. Only then may the main/authorized session run `python ./.trellis/scripts/task.py start 11-24-model-version-compare`; do not start from this planning draft automatically.

**Gate A:** implementation may begin only after parent convergence and explicit task approval; the functional contract itself is fixed.

## 1. Establish the compatibility baseline

- [ ] Inspect and snapshot the dirty tree with `git status --short`; do not include unrelated task directories or user changes in implementation commits.
- [ ] Run the existing offline suite before edits:

  ```bash
  go test ./...
  ```

- [ ] Note current `search`, `show`, `vet`, help, completion, and JSON-contract tests that must pass without changed expectations.
- [ ] Verify `go list -m all` currently prints only `civitai-cli`; preserve that zero-dependency baseline.
- [ ] If new live fixtures are necessary, plan deliberate captures of canonical model/version responses and record source URL/fetch date in `testdata/FIXTURES.md`. Do not silently edit existing verbatim fixtures to manufacture differences; use inline synthetic bodies for edge cases.

**Rollback point R1:** if the baseline is not green or unrelated changes cannot be isolated, stop before editing compare code and resolve/schedule the unrelated issue separately.

## 2. Add presence-aware API comparison snapshots

- [ ] Add comparison-specific model/version/file snapshot decoding in `internal/api`, reusing the current tolerant-decoding vocabulary and existing `Client.GetModel`/`GetVersion` methods.
- [ ] Preserve supplied value versus explicit null versus missing for every approved compared field, including booleans, numbers, strings, lists, files, images, and license grants.
- [ ] Decode the approved standalone fields needed for comparison (including status/availability where supplied) and retain matching embedded-version availability without substituting model availability.
- [ ] Keep existing `api.Model`, `api.ModelVersion`, `ParseModel`, `ParseVersion`, and their marshaled JSON shapes unchanged for current commands.
- [ ] Keep raw presence maps and unknown fields out of the public compare Result; the API layer owns schema extraction, while the compare layer owns field order and diff semantics.
- [ ] Add GoDoc to every new exported API type/function explaining source form and missing-data semantics.
- [ ] Add table-driven API tests for:
  - [ ] absent versus `[]` versus null `trainedWords`;
  - [ ] absent versus `false` license booleans;
  - [ ] absent versus `""` availability/status and explicit null;
  - [ ] standalone version detail with/without availability and owning model ID;
  - [ ] matching embedded version by ID, with no fallback to model availability;
  - [ ] explicit true/false/null/missing `hasMeta`, null image metadata despite `hasMeta: true`, supplied empty images, and missing images;
  - [ ] unknown enum/hash algorithm preservation and numeric string/number drift;
  - [ ] malformed JSON returning a wrapped decode error.

**Stage validation:**

```bash
go test ./internal/api
go test ./internal/cmd
```

The second command is a compatibility check: no current command output/test expectation should need modification in this stage.

**Rollback point R2:** remove only the additive comparison snapshot/decoding files and their tests, then rerun both commands. Do not “fix” a decode ambiguity by changing existing show/vet JSON types; return to D4/D6 review instead.

## 3. Implement the pure comparison Result and field registries

- [ ] Create `internal/compare` with no I/O, HTTP, printing, flags, filesystem, clock, random source, or process exit.
- [ ] Define the shared Result, subject identity, tri-state values, per-value sources, field entries, comparison states, and typed derived-operation structures proposed in `design.md`.
- [ ] Implement ordered model/version field registries from the approved D4 field set. Ensure `values[i]` always aligns with `subjects[i]` and all registry entries have a stable shape.
- [ ] Implement equality/completeness rules exactly, especially different-known-values-plus-missing versus equal-known-values-plus-missing.
- [ ] Project model summaries without selecting a representative latest version for detailed facts.
- [ ] Project version recipe coverage as independent counts and fixed per-field non-empty counts, discarding raw prompts/image URLs after projection.
- [ ] Ensure all public Go types/functions have GoDoc and all public JSON fields have documented semantics.

### Pure semantic test matrix

- [ ] Identical decoded inputs yield equal fields and no derived changes; missing/missing and known-plus-missing yield the approved indeterminate states.
- [ ] Null/null, null/empty, false/missing, zero/missing, and empty/missing cases match the design table.
- [ ] Model scalar, set, license, adoption, version-count, and latest-publication facts align in input order; RFC3339 offsets compare by instant, and missing/invalid timestamps make the summary incomplete/indeterminate.
- [ ] Version base model/type, publication, availability, status, model-source license, and adoption facts retain their correct sources.
- [ ] Numeric deltas are exact integers and no recommendation/rank/score field exists.

**Stage validation:**

```bash
go test ./internal/compare
```

**Rollback point R3:** delete the new pure package/tests. No CLI surface exists yet, so no user-facing rollback is required.

## 4. Implement transparent derived differences

- [ ] Add baseline-to-each-later-subject comparisons without sorting subjects or inferring chronology.
- [ ] Implement exact-string set add/remove operations with deterministic lexical output and missing/null guards.
- [ ] Implement the approved file-pairing cascade: unique file ID, unique SHA256, then unique exact name/type; retain match basis and ambiguity/unmatched reasons.
- [ ] Implement matched-file metadata, pickle/virus result, scan timestamp, and per-algorithm hash changes. Keep unmatched registry additions/removals visible.
- [ ] Implement recipe-coverage from/to/delta records, including declared-but-missing metadata and supplied-empty versus missing-images behavior.
- [ ] Ensure unavailable derived operands produce an indeterminate derived state, not empty add/remove arrays.

### Derived-difference test matrix

- [ ] Base model A to B and base-model-type change produce explicit from/to records.
- [ ] Trigger words added, removed, unchanged, reordered, case-distinct, empty, null, and missing are distinguished.
- [ ] Same file ID with changed hash reports a matched file/hash change.
- [ ] Same unique SHA256 with a new file ID/name reports a SHA256-based match plus registry/metadata changes.
- [ ] Same unique name/type with different SHA256 reports a name/type-based match and content/hash change.
- [ ] Duplicate/ambiguous SHA256 or name/type candidates remain unmatched with explicit reasons; no array-position or size-only pairing occurs.
- [ ] File/hash algorithm added, removed, changed, reordered, and case-only known-hex variants follow the approved normalization rules.
- [ ] Pickle/virus result changes are independent; timestamp-only rescans do not masquerade as result transitions; missing scans never become `Success`.
- [ ] Recipe examples with explicit true/false/null/missing `hasMeta`, no metadata, null metadata, sparse metadata, and non-empty supported fields produce correct independent counts/deltas.
- [ ] Three subjects produce baseline-to-subject-1 and baseline-to-subject-2 only, while N-way field equality still considers all three.
- [ ] Shuffling Go map insertion/iteration inputs cannot change canonical JSON ordering.

**Stage validation and review:**

```bash
go test ./internal/compare
go test -race ./internal/compare
```

- [ ] Review the pure test cases against PRD AC4–AC7 before adding output code. A test that would still pass after removing the comparison feature is not sufficient evidence.

**Gate B:** stop and return to design if file identity, missing-data, or baseline semantics require an output-specific exception. The same Result must serve both human and JSON views.

**Rollback point R4:** revert the derived-difference files/tests while retaining or removing the snapshot stage as a coherent additive unit. Do not relax matching rules merely to make fixtures pass.

## 5. Add command validation and canonical fetch orchestration

- [ ] Implement the approved `Compare(args []string)` entry and pure `runCompare` using the existing `execute` runner, `newAPIClient(g)` seam, global flags, and exit mapping.
- [ ] Parse exactly the approved model/version forms; validate kind, IDs, count, duplicates, and flag interactions before HTTP.
- [ ] Fetch model detail for model comparison and standalone version plus deduplicated parent detail for version comparison, sequentially per approved D8.
- [ ] Attribute errors to the selected subject/parent with `%w`; abort before stdout on selected or required-parent failure under the approved policy.
- [ ] Retain only snapshots/Result, not all raw responses; do not add persistent caches or package-level mutable state.
- [ ] Build the complete Result before invoking either renderer.

### Offline command tests

- [ ] Missing kind, unknown kind, missing/too many subjects, zero/negative/non-numeric IDs, duplicates, extra positionals, and rejected `--diff --json` (if retained) all fail before any request.
- [ ] Flags before/after positionals follow existing `permuteArgs` behavior; `-v` is not introduced and `--verbose` remains the debug flag.
- [ ] Model routes hit exactly `/models/{id}` in input order; no `/models` search request occurs.
- [ ] Version routes hit exactly the selected `/model-versions/{id}` endpoints and one `/models/{modelId}` per distinct parent; shared-parent request counts prove deduplication.
- [ ] Cross-parent versions retain both owning identities and request both parents.
- [ ] A selected 404, required-parent 404, decode error, injected network failure, and token rejection preserve typed exit mapping and leave stdout empty.
- [ ] Successful runs emit subjects in input order even if response order/timing differs in future concurrent implementations.
- [ ] Tests observe only official model/version GET routes and no image browse, community, local file, cache, or export side effect.

**Stage validation:**

```bash
go test ./internal/cmd -run Compare
go test ./internal/cmd
```

**Rollback point R5:** remove the unregistered compare command orchestration/tests and pure package if necessary. Existing `main.go` still exposes no compare route at this checkpoint.

## 6. Add human and stable JSON rendering

- [ ] Render both views from the same Result; do not recalculate differences in the human renderer.
- [ ] Implement full human subject headers, aligned facts, source/missing/null/empty indicators, and baseline-derived sections using ASCII text and existing report primitives.
- [ ] Implement `--diff` filtering of confirmed-equal fields while retaining changes and indeterminate states, with distinct no-differences versus not-fully-determinable messages.
- [ ] Emit machine output with `report.WriteIndent`, the approved `schemaVersion`, fixed registry order, stable derived ordering, and no timestamps/raw payload concatenation.
- [ ] Document any human-only truncation; never truncate machine hashes, IDs, triggers, or derived values.

### Rendering tests

- [ ] Full human output contains all approved aligned sections and source/missing indicators.
- [ ] `--diff` hides equal fields, shows changed/indeterminate fields, and prints the correct empty-diff message.
- [ ] Two runs over identical snapshots produce byte-identical JSON and stable human output.
- [ ] JSON assertions verify top-level shape, kind, schema version, baseline, subject/value alignment, fixed field order, state/source values, derived operations, and non-null empty arrays where required.
- [ ] JSON contains no raw `description`, example-image array/prompt payload, request token, local path, fetch timestamp, winner, rank, or score.
- [ ] stdout/stderr separation remains intact for success and failure.
- [ ] Terminal-width tests cover the approved maximum subject count without structural glyphs or ambiguous missing markers.

- [ ] Pause for Gate C review of representative golden outputs against D3/D4 before registering the public command. If the output contract changes, update all three artifacts and relevant tests together.

**Stage validation:**

```bash
go test ./internal/cmd -run 'Compare|Help'
go test ./internal/compare
```

**Rollback point R6:** remove rendering only and return to Gate C with the pure Result intact. Do not patch human-only arithmetic to hide a Result defect.

## 7. Wire the complete CLI surface and documentation

- [ ] Add `compare` to `main.go` dispatch and top-level usage with no other command changes.
- [ ] Add curated compare help to `helpPages`; ensure `civ compare help`, `civ compare -h`, and `civ help compare` are byte-identical, stdout-only, exit `0`, and perform no run work.
- [ ] Add compare to static completion metadata/scripts and drift-guard tests; include only the approved flags/kind vocabulary.
- [ ] Document approved syntax, subject limit, old-then-new baseline usage, full/diff/JSON behavior, field coverage, missing semantics, request cost, exit codes, and examples in `README.md`.
- [ ] Update `scripts/integration-test.sh` in the same change, including compare in help loops and explicit response/exit assertions.
- [ ] Record provenance for any new verbatim fixtures; keep all synthetic difference cases offline.
- [ ] Review the final diff for accidental changes to existing command output, sibling tasks, `go.mod`, release/reproducibility configuration, or unrelated user files.

### Intended integration cases (approved D1 syntax)

- [ ] `compare models 2458426 260267 --json`: two canonical subjects, model kind/schema, aligned core model/license/base/adoption fields, no representative-version detail or score.
- [ ] `compare versions 3108589 3263843 --json`: version kind/schema, ordered identities, base/triggers/files/hashes/scans/license/availability/recipe fields and baseline-derived data.
- [ ] The same two versions with human `--diff`: exit `0`, subject headers retained, output contains changed/indeterminate information without raw JSON decoration.
- [ ] Too few subjects and duplicate IDs: usage exit `1`; help forms: exit `0` with matching stdout.
- [ ] A selected unknown ID: exit `1`, empty stdout, not-found hint on stderr.
- [ ] A `1ns` timeout on an otherwise valid compare invocation: network-class exit `2` without result output.
- [ ] Existing search/show/vet/help/completion assertions remain unchanged and green.

**Stage validation:**

```bash
go test ./...
go vet ./...
git diff --check
```

**Rollback point R7:** if only wiring/documentation fails review, revert `main.go`, help/completion registrations, integration additions, and docs while leaving unexposed internal code for another review iteration. For a complete pre-release withdrawal, revert all compare stages; no state cleanup is needed.

## 8. Full verification and release readiness

- [ ] Format all changed Go files and inspect the resulting diff; do not run broad formatting over unrelated files.
- [ ] Run the project’s required full quality trio from the repository root:

  ```bash
  go vet ./... && go test ./... && go test -race ./...
  ```

- [ ] Verify the zero-dependency and build contracts:

  ```bash
  go list -m all
  make build
  git diff --check
  ```

  `go list -m all` must still print only `civitai-cli`.

- [ ] Run targeted compare tests once more after formatting/wiring:

  ```bash
  go test ./internal/api ./internal/compare ./internal/cmd -run 'Compare|Completion|Help'
  ```

- [ ] Build and run the updated live/manual integration canary when network policy permits:

  ```bash
  bash scripts/integration-test.sh dist/amd64/civ.exe
  ```

  Do not substitute live calls for offline tests or make daily unit tests depend on Civitai.

- [ ] Perform a full-scope cross-layer review: CLI args -> endpoint routes -> snapshot states -> field registry -> derived operations -> human/JSON -> docs/integration. Verify no field is re-decoded or reinterpreted independently by a renderer.
- [ ] Compare old and new behavior for all existing commands and confirm no existing golden output was edited merely to accommodate compare.
- [ ] Inspect `git status --short` and the changed-file list; confirm only approved task/product/docs/test files are included and no sibling task artifacts or local-library/export/community files changed.
- [ ] Load `trellis-check` for the full-scope quality review and resolve findings against actual code/evidence.
- [ ] During finish work, use `trellis-update-spec` to record the new package/dependency direction and stable comparison-output conventions if they become enduring project architecture. Do not modify specs during this planning-only subtask.
- [ ] Treat the first released machine contract as schema `1`; document any later additive/incompatible change deliberately rather than letting a new field leak in accidentally.

**Gate D:** all offline checks are green, integration coverage matches the new surface, representative outputs have been reviewed, and rollback remains a clean additive revert before release.

## 9. Rollback summary

| Situation | Rollback action | Verification after rollback |
|---|---|---|
| Snapshot decoder is ambiguous or breaks current types | Remove additive snapshot files; never alter current JSON scalar types as a shortcut | `go test ./internal/api ./internal/cmd` |
| Pure diff semantics fail review | Remove/revert `internal/compare`; return to D5/D7 design review | `go test ./...` after coherent revert |
| Command fetching/error policy is wrong | Remove unregistered `internal/cmd/compare*` changes; retain no partial route | Existing `internal/cmd` tests and no `compare` in main/help |
| Human/JSON output is rejected | Revert renderer/goldens and revisit D3/D4; keep one shared Result | Pure compare and existing command tests |
| Pre-release wiring fails | Revert dispatch, usage, help, completion, integration, and docs together; for full withdrawal revert all compare files | Full quality trio and current help/completion tests |
| Post-release correctness issue | Prefer forward fix with schema compatibility review; if withdrawal is unavoidable, remove the complete surface and document the public removal | Full quality trio, integration surface parity, release-note review |

No rollback step requires deleting a cache, export, database, downloaded model, or user configuration because this feature creates none.
