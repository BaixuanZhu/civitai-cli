# Compact search and show JSON — provisional implementation plan

> Status: unstarted. D1 and D2 are approved on 2026-11-24, including the concrete registry and parent-wide list/detail invariant. Do not run `task.py start` until the final revised planning summary is explicitly approved.

## Wave scope override (2026-11-24 parallel wave)

This child runs in a parallel wave on the shared working tree. These overrides win over any conflicting step below.

- **Owned files (only these may be created/edited):** `internal/api/compact*.go` (`compact.go`, `compact_models.go`, `compact_versions.go`, `compact_test.go`, …), `internal/cmd/search.go`, `internal/cmd/search_test.go`, `internal/cmd/show.go`, `internal/cmd/show_test.go`.
- **Forbidden (parent integration pass owns them):** `main.go`, `internal/cmd/help.go` (curated `--view` help rows move to the parent pass; you may leave `searchHelp`/`showHelp` wording unchanged), `internal/cmd/help_test.go`, `internal/cmd/help_local_test.go`, `internal/cmd/completion*.go`, `internal/cmd/completion_test.go`, `scripts/integration-test.sh`, `README.md`, `CHANGELOG.md`, `internal/skill/assets/**`, `go.mod`, `go.sum`, `internal/api/types.go`, `internal/api/models.go`, `internal/api/versions.go`, `internal/cmd/prompts.go`, every other `internal/cmd`/`internal/api` file, and every `.trellis/tasks/` directory except this one.
- **Parent integration pass takes over:** Step 5 help/completion wiring and integration assertions, Step 6 docs/Skill/live integration. Do not treat their absence as a failure.
- **No git commands:** never run `git add`, `git commit`, or `git push`.
- **Sibling coexistence:** other agents are editing `internal/api` and `internal/cmd` concurrently. Ignore build/test failures that trace to files you do not own, and say so in the report; do not "fix" them. Prefix package-level test helpers with `compact`.
- **Verification substitute:** run focused tests for your own files. If `go test ./...` fails only because a sibling's in-progress file is broken, report that instead of editing it.

## 1. Pre-implementation gates

- [x] User approved D2 on 2026-11-24: shared `modelSummary`, search/model/version envelopes, version file/hash/scan retention, presence/null rules, `--view` validation, and the parent-wide prompt-free list/detail invariant.
- [ ] Parent review confirms this child alone may change existing default JSON; recipe/compare/resolve and unrelated legacy outputs remain isolated.
- [ ] Replace all proposal language that affects observable behavior with the approved registry and update PRD AC examples/thresholds if needed.
- [ ] Confirm `implement.jsonl`/`check.jsonl` still point to the applicable specs and parent research.
- [ ] Obtain fresh explicit approval of the final planning summary before `task.py start`; the D1 choice is not implementation approval.

## 2. Ordered implementation checklist

### Step 1 — Pin compact schemas with offline fixture tests

- [ ] Add search tests using `testdata/models_search_anima.json`: versioned envelope, item order/count, shared model summary, all minimal version refs, exact representative values, excluded description/files/images/download fields.
- [ ] Add model-show tests using `testdata/models_2458426.json`: identical model-summary shape to search items, every version retained, no implicit latest selection.
- [ ] Add version-show tests using `testdata/version_3263843.json`: identity/base/triggers, all visible files/hashes/scans, actual metadata coverage, no prompt/images/comfy/download payload.
- [ ] Add synthetic table cases for absent/null/empty arrays, absent stats, false/zero values, unknown enums/hash algorithms, numeric drift, invalid numbers, malformed root containers, and unknown ignored fields.
- [ ] Add deterministic repeat-output and record-count tests. Include a test that fails if a version or file is silently dropped.
- [ ] Add semantic size tests against full indented outputs using the reviewed fixture threshold; do not pass by whitespace minification.

**Gate G1 — Contract proof:** expected JSON must express the approved D2 registry before public wiring. Any field change returns to D2 and updates PRD/design/tests together.

### Step 2 — Implement dedicated compact decoders/projections

- [ ] Add focused `internal/api/compact*.go` source/DTO/projection code with meaningful GoDoc on every exported symbol.
- [ ] Implement `ParseCompactModels`, `ParseCompactModel`, and `ParseCompactVersion` with one shared model-summary projector and one version-ref projector.
- [ ] Track required absent/null/empty states with narrow source structs/raw messages; do not change existing `Model`, `ModelVersion`, `ImageMeta`, or their JSON tags/types.
- [ ] Preserve source item/version/file order and all supplied hash algorithms. Rely on deterministic standard-library JSON encoding rather than iterating maps into ad hoc output.
- [ ] Keep excluded raw/unknown fields out of public DTOs and never fetch image/CDN URLs.
- [ ] Run `go test ./internal/api` and existing command tests as a no-shared-drift baseline.

**Rollback R1:** delete the compact API files/tests; no command behavior has changed yet.

### Step 3 — Add explicit view parsing and validation

- [ ] Add command-local `--view` options with default `compact`, lowercase `compact|full` validation, and explicit-set detection through the parsed `FlagSet`.
- [ ] Reject explicit view without `--json` before network.
- [ ] Reject explicit view with `show --prompts`; retain the legacy prompts path when view is omitted.
- [ ] Keep human rendering untouched for all invocations without an explicit view.
- [ ] Add table tests for omitted/explicit/invalid view, JSON/human combinations, prompts conflict, extra positionals, and zero-request usage errors.

**Rollback R2:** remove view options and validation; compact internals remain unexposed.

### Step 4 — Branch after the existing response

- [ ] Search: preserve `SearchModels`; full calls `ModelsRawItems` + `WriteRaw`, compact calls the new parser + `WriteIndent`.
- [ ] Model show: preserve `GetModel`; full uses existing `ParseModel`, compact uses `ParseCompactModel`.
- [ ] Version show: preserve `GetVersion` and ownership validation; full uses existing `ParseVersion`, compact uses `ParseCompactVersion`; omitted-view prompts use the existing prompt projector.
- [ ] Do not add model/images/by-hash/mini fallback requests, output files, caches, or logging of source payloads.
- [ ] Add fake-server tests proving identical endpoint paths/query/request counts for full and compact, plus unchanged 404/network/token behavior and stdout-empty failures.

**Gate G2 — Behavior and size review:** inspect real fixture outputs for field usefulness, record preservation, payload reduction, and absence of capability/schema drift. Return to D2 if a missing field forces another request in the normal selection flow.

**Rollback R3:** remove compact branches/view wiring and restore the pre-change default path without touching endpoint methods.

### Step 5 — Complete CLI/help/completion surface

- [ ] Add `--view compact|full` and default-compact wording to curated search/show help; do not duplicate global flag rows.
- [ ] Update search/show completion metadata and all four static scripts; run help/completion drift tests.
- [ ] Ensure `civ search help`, `civ show help`, `civ help search`, and `civ help show` remain equivalent stdout/exit-0 pages.
- [ ] Add offline integration assertions for default compact, explicit compact, full, invalid view, human-with-view usage error, and prompts/view conflict.
- [ ] Preserve all unrelated top-level and command-help expectations.

**Rollback R4:** revert help/completion/integration wiring together with the view/default branches; do not leave documentation advertising an unimplemented migration path.

### Step 6 — Documentation, Agent guidance, and live integration

- [ ] Update README search/show JSON sections, schema examples, view/default table, API notes, and migration note for the intentional pre-1.0 default break.
- [ ] Update `internal/skill/assets/civ-prompt/references/civ-commands.md` and Skill workflow so Agents use compact by default, full only for deliberately excluded fields, and recipe for normalized generation context.
- [ ] Add a CHANGELOG entry explicitly stating the default `--json` breaking change and `--view full` migration path; do not modify released heading format.
- [ ] Update `scripts/integration-test.sh` for search/model/version compact schema/fields/counts/exclusions and full legacy shape. Keep live assertions tolerant of upstream value changes while checking structure and known fixture IDs.
- [ ] Retain `show --prompts`, vet, sync, tags, and human-output assertions unchanged.
- [ ] Record no new fixture if existing checked-in bodies suffice; any additional verbatim fixture requires source/date provenance in `testdata/FIXTURES.md`.

### Step 7 — Full verification and parent review

- [ ] Run focused API/cmd tests, then `go vet ./...`, `go test ./...`, and `go test -race ./...`.
- [ ] Run `go list -m all` and confirm only `civitai-cli`; build the development binary with bootstrap disabled.
- [ ] Run the updated integration script under its normal manual/CI network preconditions; do not make unit tests depend on live Civitai.
- [ ] Request full-scope `trellis-check`, including source -> compact DTO -> JSON -> help/README/Skill/integration tracing.
- [ ] Parent verifies PAC-7/PAC-11 and that no other command output changed to accommodate shared code.
- [ ] Evaluate durable conventions for `.trellis/spec` during finish work; archive only after acceptance and normal commit workflow.

## 3. Validation commands

Focused checks after implementation:

```bash
gofmt -w <owned-go-files>
go test ./internal/api -run 'Compact|ParseModel|ParseVersion|ModelsRawItems'
go test ./internal/cmd -run 'Search|Show|Prompts|Vet|Help|Completion|ExitCode'
git diff --check
```

Full quality gates:

```bash
go vet ./...
go test ./...
go test -race ./...
go list -m all
make build
bash scripts/integration-test.sh dist/amd64/civ.exe
```

Live integration follows its existing isolation/network rules and is not a replacement for offline tests. Do not run registry-writing tests locally.

## 4. Required acceptance-to-test matrix

| Acceptance | Required proof |
|---|---|
| AC1 search selection | Same model order/count, all version refs, representative identity/stats/base fields, no heavy source objects |
| AC2 model summary | Same summary schema as search item, no latest-only truncation or embedded detail |
| AC3 version inspection | All visible files/hashes/scans and trigger identity retained; examples reduced to counts |
| AC4 size | Reviewed <50% fixture ceiling via fields, not whitespace or record loss |
| AC5 determinism/types | Required keys/null/empty/number behavior and repeat-output tests |
| AC6 requests/errors | Same route/count/query; usage before HTTP; existing 1/2/4 classes |
| AC7 full compatibility | Search raw byte equality; show existing decoded JSON equality |
| AC8 unrelated surfaces | Human/prompts/vet/sync/tags tests unchanged and green |
| AC9 surface completeness | Help/completion/README/Skill/CHANGELOG/integration parity |
| AC10 scope | No new requests, persistence, export, library, dynamic field language, or sibling behavior |

## 5. Risk register and rollback triggers

| Risk | Required response |
|---|---|
| Compact omits IDs needed for the next command | Stop at G2; restore required version refs rather than forcing another request |
| File/hash trimming removes vetting value | Keep all visible file/hash/scan entries; exclude large generation payloads instead |
| Shared DTO edits alter full or other commands | Revert shared edits; keep dedicated compact source/DTO types |
| Output shrinks only through minification | Use semantic full-vs-compact comparison and excluded-field/record-count assertions |
| `--view` silently does nothing | Reject explicit invalid combinations before HTTP; test prompts/human interactions |
| Existing tests are broadly rewritten | Allow only the three approved default-contract changes; restore every unrelated expectation |
| Full is not actually legacy | Search must use raw items; show must use the existing decoded DTO/writer, not compact-with-extra-fields |
| Rollback leaves two conflicting defaults | Revert parser, view, help/completion/docs/integration as one coherent unit |

## 6. Planning status

D1 and D2 are resolved and recorded. Only the normal final implementation approval remains open; no product code, live tests, task activation, commit, or release action is part of this planning pass.
