# Recipe context command — provisional implementation plan

> **No-execution status:** requirements and design are converged. The CLI/selector syntax, exact-version identity, sibling-owned community detail syntax, redaction invariant, shared scalar normalization, summary budget, schema fields, and human/JSON presentation are approved. No step below has been executed and none authorizes product edits or `task.py start` until explicit final approval.

## Wave scope override (2026-11-24 parallel wave)

This child runs in a parallel wave on the shared working tree. These overrides win over any conflicting step below.

- **Owned files (only these may be created/edited):** `internal/api/recipe.go`, `internal/api/recipe_test.go`, `internal/cmd/recipe.go`, `internal/cmd/recipe_test.go`. Prefer existing `testdata/` fixtures; add no verbatim fixtures unless unavoidable.
- **Forbidden (parent integration pass owns them):** `main.go`, `internal/cmd/help.go`, `internal/cmd/help_test.go`, `internal/cmd/help_local_test.go`, `internal/cmd/completion*.go`, `internal/cmd/completion_test.go`, `scripts/integration-test.sh`, `README.md`, `CHANGELOG.md`, `internal/skill/assets/**`, `go.mod`, `go.sum`, and every `.trellis/tasks/` directory except this one.
- **Parent integration pass takes over:** Step 4 dispatch/overview/help-map/completion wiring, Step 5 docs/Skill, Step 6 integration-script cases. Do not treat their absence as a failure; still define `recipeHelp` in `internal/cmd/recipe.go` as a package-level constant.
- **No git commands:** never run `git add`, `git commit`, or `git push`.
- **Sibling coexistence:** other agents are editing `internal/api` and `internal/cmd` concurrently. Ignore build/test failures that trace to files you do not own, and say so in the report; do not "fix" them.
- **Verification substitute:** run focused tests for your own files. If `go test ./...` fails only because a sibling's in-progress file is broken, report that instead of editing it.

## 1. Pre-start review gates

- [x] **G0.1a — Record approved CLI decisions.** Exact-version invocation, `--examples` summary-only behavior, `--example <sourceIndex>` singular detail, and mutual exclusion are approved; placeholders have been removed from acceptance criteria.
- [x] **G0.1b — Record RT1 summary budget.** Default 3 and range 0–10 are approved, including zero-context behavior and count-1-stays-summary semantics.
- [x] **G0.1c — Record RT3 presentation.** Human summary is prompt-free, explicit human detail prints the selected text, and JSON remains opt-in for the stable schema.
- [x] **G0.2 — Record approved v1 schema fields.** Distinct summary/detail kinds and DTOs, prompt-free summary keys, text availability statuses, small-parameter whitelist, singular detail text fields, nullable types, issue vocabulary, source-index behavior, and unknown-images counts are approved. Shared numeric normalization and decimal-string seed remain fixed.

- [x] **G0.3 — Confirm the approved redaction invariant.** Every human/JSON summary at every count including one omits prompt values, negativePrompt values, description HTML, raw meta, and Comfy/workflow content; only explicit singular detail may return large text; raw meta/workflow export remains out of scope for recipe detail.
- [ ] **G0.4 — Confirm child boundaries.** Confirm that resource resolution and comparison remain in their sibling tasks and that all community retrieval/sampling remains in `11-24-community-recipes`.
- [ ] **G0.5 — Parent contract review.** Ask the parent task to review whether per-example provenance, summary statuses, selection metadata, and singular detail are sufficient for the future community sibling without committing this child to community behavior. Confirm that a community sample of one remains prompt-free.
- [x] **G0.6 — Prepare Trellis execution context.** The main/session owner curated and validated the required `implement.jsonl` and `check.jsonl` entries with applicable backend specs and parent research. Revalidate once more after any final integration edits.
- [ ] **G0.7 — Fresh implementation approval.** Present the revised planning summary and obtain explicit approval in a subsequent user message before running `task.py start`.

**Stop condition:** any unresolved RT item, schema disagreement, redaction exception, or cross-child dependency keeps the task in planning. The approved CLI/selector/detail-flow decisions are not open for renegotiation.

## 2. Ordered implementation checklist

### Step 1 — Pin both approved contracts and redaction in tests before public wiring

- [ ] Add focused API summary tests using `testdata/version_3263843.json` for model/version context, official provenance, first-N order, counts, text statuses, exact small-parameter values, seed precision, and object-shaped dimensions.
- [ ] Add focused API detail tests for explicit source-index selection and the selected example's exact prompt/negativePrompt, including fixture sentinels `rx-78-2 gundam` and `chromatic aberration`, plus the same small parameters and seed precision as summary.
- [ ] For every summary fixture case and every count including `1`, recursively assert that no `prompt`, `negativePrompt`, `description`, raw `meta`, `comfy`, or workflow member exists and that neither prompt sentinel nor any source prompt/negativePrompt value appears. Assert that summary DTOs have no detail-text member capable of leaking later.
- [ ] Assert that selecting detail index 1 emits only example 1's text, selecting another index omits example 1's unique positive-prompt sentinel `rx-78-2 gundam`, and neither view emits nonselected text, raw meta, description HTML, or Comfy/workflow content. Do not treat the negative text shared by all fixture examples as a unique selector sentinel.
- [ ] Add synthetic table cases for absent versus null versus explicit empty values; null `meta` with `hasMeta: true`; absent/null/empty `images`; absent/null/empty `trainedWords`; provided/empty/not-provided/unsupported prompt statuses; numeric strings; invalid numbers; fractional integers; object/list scalar shapes; unknown enums; and ignored large/unknown fields.
- [ ] Assert the approved shared normalizer with paired numeric `20`/string `"20"` cases producing equivalent JSON numbers, exact decimal-string seed preservation, null-plus-issue behavior for invalid/non-scalar values, and no coercion of unknown fields or units.

- [ ] Add schema assertions for `schemaVersion`, summary/detail `kind`, required nullable keys, `issues: []`, summary `examples: []`, singular detail `example`, selection counts, and deterministic repeat output. Summary must not contain singular `example`; detail must not contain `examples`.
- [ ] Add independence tests proving that nonselected prompt, negativePrompt, unknown/raw metadata content, or Comfy changes do not affect detail output when full-source metadata-presence counts remain unchanged.
- [ ] Add request-independent tests proving that values are never sourced from `comfy`, outer image dimensions, model-description text, other examples, or defaults.
- [ ] Review the concrete expected summary and detail JSON with the user/parent before proceeding if either differs materially from the reviewed design.

**Gate G1 — Contract and redaction review:** tests must express approved behavior rather than merely mirroring an implementation already written. Any schema change requires explicit change review before implementation. Any prompt-bearing summary, including count `1`, violates the approved invariant and blocks implementation.

### Step 2 — Implement isolated summary/detail decoders and projectors

- [ ] Add `internal/api/recipe.go` with private narrow source structs and exported, GoDoc-documented summary/detail entry points and DTO types required by `cmd`.
- [ ] Keep all raw JSON field access, presence detection, text-status derivation, scalar normalization, issue construction, provenance, and selection counting inside `internal/api`.
- [ ] Use distinct summary and detail DTOs: summary example types must have no prompt, negativePrompt, textParameters, raw-meta, or workflow fields; detail embeds/reuses summary fields and adds only the approved singular `textParameters` object.
- [ ] Reuse existing helpers only where their behavior is identical; implement small private recipe scalar helpers for absent/null/empty/invalid/unsupported distinctions. Use only the Go standard library.
- [ ] Implement deterministic first-N summary selection, exact source-index detail selection, and full-source metadata counts without sorting, deduplicating, trimming, or reindexing source records.
- [ ] Implement `provided`/`empty`/`notProvided`/`unsupportedJsonShape` text statuses without copying prompt content into summary. Preserve exact prompt/negativePrompt only while projecting the selected detail.
- [ ] Exclude raw workflows, unknown metadata bags, descriptions, files/hashes/stats, download URLs, and nonselected example text from both DTO families.
- [ ] Leave `internal/api/types.go`, `ParseVersion`, `ImageMeta`, and legacy prompt code unchanged unless a review-approved shared extraction is demonstrably behavior-preserving.
- [ ] Run focused API tests and the full existing API test suite after this isolated step.

**Gate G2 — Type-level redaction and legacy decode safety:** inspect the summary DTO and marshaled output, not only renderer code. If a prompt can be added to summary without changing the DTO contract, strengthen the tests/design. If implementation needs to alter shared API types or legacy expected output, stop and retain the dedicated decoder unless the user approves a compatibility review.

**Rollback point R1:** remove the new API recipe files/tests; no public surface exists yet and there is no persisted state.

### Step 3 — Add pure command orchestration without changing legacy commands

- [ ] Add `internal/cmd/recipe.go` with curated `recipeHelp`, command-specific options/registration, and a pure `runRecipe` that returns errors rather than exiting.
- [ ] Validate exactly one positive version ID, a positive detail index, and the approved summary count range before any request. Track explicit `--examples` presence so the default summary count does not conflict with `--example`, while explicitly supplying both selectors remains an error.
- [ ] Reject extra positionals, malformed selectors, and explicit summary/detail selector combinations with zero HTTP requests. After the one fetch, reject an out-of-range detail index with no partial stdout and no summary fallback.
- [ ] Use `newAPIClient(g)` and the existing `GetVersion`; do not add `net/http`, model-detail fallback, image fetch, by-hash fallback, or storage calls.
- [ ] Pass raw version bytes through the appropriate summary/detail projector, reject a conflicting nonzero returned version ID, and render with `report.WriteIndent` for JSON.
- [ ] Implement only the approved thin human rendering. Human summaries at every count must be prompt-free; only explicit singular human detail may print prompt/negativePrompt. Keep source strings truthful, label missing metadata/parameters, and avoid generate/download/export instructions.
- [ ] Add command tests with a counting `httptest` handler for exactly one request/path per invocation, no secondary requests, no filesystem writes, both stdout shapes, deterministic summary selection, singular detail, local validation with zero requests, out-of-range detail, malformed responses, ID mismatch, and shared API error classes.
- [ ] For human and JSON summary output at default, count `0`, count `1`, a middle count, maximum, empty source, and missing metadata, assert prompt keys and both fixture prompt sentinels are absent. For detail, assert the selected unique positive-prompt sentinel and expected negative text are present while nonselected unique positive-prompt sentinels are absent.
- [ ] Keep existing `show`/`vet` handlers and tests unchanged; run their explicit legacy full/prompts detail and error-mapping tests as compatibility checks.

**Gate G3 — Behavior, token, and redaction review:** inspect actual fixture summary and detail output for context completeness, compactness, provenance, missing-data clarity, and absence of capability overclaims. If any summary leaks large text, stop rollout; do not fix it through truncation. If summary is not materially smaller/more useful than the current multi-call flow, return to design instead of adding more flags.

**Rollback point R2:** remove the new command files/tests; no dispatch or documentation has yet exposed them.

### Step 4 — Wire the additive public CLI surface

- [ ] Add one `recipe` dispatch case and one overview row in `main.go`; keep main free of command logic.
- [ ] Register `recipeHelp` in `internal/cmd/help.go` and add curated/named-help assertions, including stdout-only exit 0, standard section order, summary-only `--examples`, and detail-only `--example` semantics.
- [ ] Add recipe and both reviewed selectors to `completionSpec`, `completionRegisterFuncs`, and all four static completion scripts. Preserve command order and existing completions.
- [ ] Add offline integration-script cases for dual help forms, named help, invalid ID/count/index, explicit selector combination, and unknown flags; local usage cases must assert exit 1 before network where applicable.
- [ ] Run help and completion drift tests before editing user documentation.

**Gate G4 — Surface consistency:** dispatch, overview, help map, completion spec, all shell scripts, tests, and approved syntax must agree exactly. No help example may imply that count `1` unlocks detail or that summary output contains prompts. A syntax change here returns to G0.1 and updates all acceptance examples.

**Rollback point R3:** revert dispatch, overview, help, completion, and integration wiring together with Step 3. Do not leave partial discoverability or dead completion entries.

### Step 5 — Update user and Agent guidance

- [ ] Document the approved summary and detail commands, bounded-selection behavior, source-index detail flow, stable schemas/provenance, issue semantics, redaction invariant, and read-only limitations in `README.md`.
- [ ] Update `internal/skill/assets/civ-prompt/SKILL.md` so exact-version prompt grounding uses prompt-free summary first, chooses a source index, and requests singular detail only when full prompt text is needed; retain search/show for discovery and vet for local-file/hash identification.
- [ ] Update `internal/skill/assets/civ-prompt/references/civ-commands.md` with both recipe contracts and retain accurate legacy explicit full/`--prompts` compatibility-detail documentation. Do not describe legacy prompt arrays as default list behavior.
- [ ] Update recipe-adoption guidance only as needed to cover summary statuses, seed/size/denoise, null parameters, source indexes, selected detail text, and the rule that empty/missing data must not be replaced with guesses.
- [ ] State explicitly that official provenance is not author endorsement or guaranteed reproducibility, image URLs are not fetched, recipe JSON is not an executable workflow export, and raw meta/Comfy/workflow are not returned by recipe detail.
- [ ] Do not document community selection as available. Add only a neutral forward-compatibility statement if the parent review requests one, and state that future community lists/samples obey the same redaction invariant.
- [ ] Add/update release notes only after both public contracts are stable and according to the repository's existing changelog conventions.

**Gate G5 — Documentation truthfulness:** compare every documented flag, JSON path, default, error, and limitation with implementation and tests. No “generate,” “download,” “export,” “recommended by Civitai,” or “ready to run” capability claim may creep into recipe wording. No summary example may contain prompt text or imply that a one-item list is detail.

### Step 6 — Add live drift coverage and perform final review

- [ ] Add known-version summary and selected-detail `recipe ... --json` runs to `scripts/integration-test.sh` using the existing Anima version ID and a pinned source index.
- [ ] For summary, assert exit 0, schema/kind/provenance, correct model/version identity, bounded source-order examples, representative small-parameter values, selection counts, and absence of prompt keys, both prompt sentinels, raw workflow, description HTML, and download fields.
- [ ] For detail, assert the selected source index/provenance, exact selected prompt text, small-parameter fidelity, singular `example`, absence of `examples`, and absence of nonselected unique positive-prompt sentinels/raw workflow/download fields.
- [ ] Retain all existing show/vet/prompts assertions unchanged as explicit legacy compatibility-detail coverage.
- [ ] Add offline help/usage/completion assertions required by the CLI-surface convention and ensure live cases use the script's existing anonymous/isolated environment.
- [ ] Run the complete validation matrix below, inspect the final diff for scope, and request full-scope `trellis-check` review.
- [ ] Ask the parent reviewer to verify the community sibling can reuse both contracts without making this child depend on community implementation or permitting prompt-bearing samples.

**Gate G6 — Final acceptance:** every applicable PRD AC must map to a green offline assertion or an explicitly run live/manual check. Any unresolved acceptance failure or summary redaction leak blocks archive; do not describe planning proposals as shipped behavior.

## 3. Validation plan (execution phase only)

### Focused offline checks

Run from the repository root after implementation:

```bash
gofmt -w <all-changed-go-files>
go test ./internal/api -run 'Recipe|ParseVersion|Image|ModelsRawItems'
go test ./internal/cmd -run 'Recipe|Prompts|Show|Vet|Help|Completion|ExitCode'
git diff --check
```

Review requirements:

- Verify the test server counts all requests, not only the last path.
- Verify at least one test would fail if null-meta examples were silently dropped/reindexed.
- Verify at least one test would fail if seed were emitted through a lossy float64.
- Verify at least one test would fail if outer image dimensions replaced unsupported generation dimensions.
- Verify summary tests fail if prompt/negativePrompt keys, source prompt values, negative prompt values, raw meta, description HTML, or Comfy/workflow content appear at any count, especially count `1`.
- Verify detail tests fail if a nonselected prompt appears or if explicit empty text is converted to a missing/default value.
- Verify selector tests distinguish an explicitly supplied summary count from its default and reject explicit summary+detail combinations before network access.
- Verify legacy test expected outputs were not edited merely to accommodate recipe changes.

### Required project quality gates

```bash
go vet ./...
go test ./...
go test -race ./...
go build ./...
go list -m all
```

Expected `go list -m all` output remains only `civitai-cli`; no third-party module is approved. Do not set `CIV_TEST_REGISTRY` or run registry-writing tests on a developer machine.

### CLI and integration checks

Build the real binary using the repository's normal target, then run:

```bash
bash scripts/integration-test.sh dist/amd64/civ.exe
```

or the equivalent reviewed `make integration` flow. Live integration is a manual/scheduled upstream-drift check and must not replace offline tests. Confirm the script's isolated homes, `CIV_NO_BOOTSTRAP=1`, and `CIV_NO_CACHE=1` remain intact.

### Final diff and scope audit

- [ ] Confirm no product files were changed before approval and no unrelated refactors were added during implementation.
- [ ] Confirm no edits to sibling task directories, parent/shared task artifacts, `task.json`, or planning-only JSONL restrictions were bypassed by this subtask.
- [ ] Confirm every new default/list/summary surface is prompt-free and every large-text path requires explicit singular detail.
- [ ] Confirm no new persistence path, output-file flag, export command, generation/download call, images endpoint, community flag, or third-party dependency exists.
- [ ] Confirm all public exported Go symbols have meaningful GoDoc and all user-facing text is English.
- [ ] Confirm `show`/`vet` compatibility, stdout/stderr separation, exit codes, help, completion, README, bundled Skill, and integration coverage all describe the same summary/detail surface.
- [ ] Perform the Phase 3.3 spec-learning review; update specs only if the implementation establishes a durable convention, through the normal reviewed workflow.

## 4. Risk register and rollback triggers

| Risk | Early signal | Required response / rollback |
|---|---|---|
| Large text leaks into a summary | Prompt sentinel/key appears at any count, in either format, or in a community projection | Stop at G1/G3/G6; remove public wiring if already exposed, enforce DTO-level redaction, and never substitute truncation |
| Count `1` implicitly becomes detail | Single-item summary gains text fields or different shape | Reject the behavior in contract tests; only the explicit singular selector may choose detail |
| Legacy JSON drift from shared type edits | Existing show/vet snapshots or prompt tests change | Stop at G2; revert shared edits and keep recipe decoding isolated |
| Schema reflects implementation rather than user intent | Tests are written after output or BT placeholders remain | Return to G0; do not start or continue public wiring |
| Token reduction is negated by unbounded examples/raw fields | Summary grows with omitted examples or contains large source blocks | Stop at G3; restore summary whitelist and approved limit rather than documenting away the problem |
| Missing values are silently synthesized | Empty/absent matrix lacks distinctions or dimensions come from outer image/workflow | Block review; remove fallback and add explicit statuses/issues |
| Large seed loses precision | JSON round-trip through a JavaScript-like float changes value | Keep decimal-string seed and add a precision regression |
| Detail leaks nonselected content | Another example's sentinel or raw workflow appears in singular detail | Restrict projection to the selected source index and add nonselected-data independence tests |
| One-call command grows hidden fallbacks | Handler observes model/images/community requests | Remove fallback; represent missing summary fields or defer to the owning sibling task |
| Public surface drift | Help/completion/dispatch/docs disagree on selector meaning | Revert or repair the whole Step 4 wiring before release |
| Community scope or redaction exception leaks into this child | Community flags/endpoints/sampling or prompt-bearing sample appears in diff | Remove it and route requirements to `11-24-community-recipes`; preserve one global invariant |
| Rollback requires data cleanup | Any recipe state is found on disk | Stop and redesign; this task is not approved to persist recipe/model state |

## 5. Planning-only completion statement

The planning artifacts are converged for implementation review. All product/UX/schema decisions for this child are fixed; only the parent planning review and fresh explicit implementation approval remain. No implementation, task activation, product validation, or release action is part of this planning pass.
