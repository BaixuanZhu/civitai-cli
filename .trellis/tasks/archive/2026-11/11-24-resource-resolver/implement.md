# Resource set resolver — implementation plan

> Status: converged plan, unstarted. Do not run `task.py start` until the parent planning review and explicit implementation approval.

## Wave scope override (2026-11-24 parallel wave)

This child runs in a parallel wave on the shared working tree. These overrides win over any conflicting step below.

- **Owned files (only these may be created/edited):** `internal/resolver/**`, `internal/cmd/resolve.go`, `internal/cmd/resolve_test.go`, and only the non-serialized `AIR` addition plus its test in `internal/api/types.go` / `internal/api/types_test.go`.
- **Forbidden (parent integration pass owns them):** `main.go`, `internal/cmd/help.go`, `internal/cmd/help_test.go`, `internal/cmd/help_local_test.go`, `internal/cmd/completion*.go`, `internal/cmd/completion_test.go`, `scripts/integration-test.sh`, `README.md`, `CHANGELOG.md`, `internal/skill/assets/**`, `go.mod`, `go.sum`, every other `internal/cmd`/`internal/api` file, and every `.trellis/tasks/` directory except this one.
- **Parent integration pass takes over:** Stage 6 dispatch/help-map/completion wiring, docs, CHANGELOG, integration-script cases. Do not treat their absence as a failure; still define `resolveHelp` in `internal/cmd/resolve.go` as a package-level constant.
- **No git commands:** never run `git add`, `git commit`, or `git push`.
- **Sibling coexistence:** other agents are editing `internal/api` and `internal/cmd` concurrently. Ignore build/test failures that trace to files you do not own, and say so in the report; do not "fix" them.
- **Verification substitute:** run focused tests for your own files. If `go test ./...` fails only because a sibling's in-progress file is broken, report that instead of editing it.

## 1. Pre-implementation gates

- [x] D1 approved: `civ resolve <ref>...`, accepting only path, full SHA256, versioned AIR, or `version:<id>`; no name-only or bare numeric forms.
- [x] D2 approved: ready → 0, completed not-ready → 5, usage/local-I/O/decode → 1, network → 2, rejected token → 4.
- [x] D3 approved: one checkpoint anchor; exact-base compatibility for LoRA/TextualInversion/Controlnet/VAE; unknown blocks readiness; no aliases.
- [x] Context manifests curated and validated.
- [ ] Parent convergence review and fresh explicit implementation approval.

## 2. Change map

| Path | Responsibility |
|---|---|
| `internal/resolver/types.go` | Request, metadata, environment function values, policy, report, findings with GoDoc |
| `internal/resolver/policy.go` | `resolver-v1` roles, anchor/dependent rules, compatibility |
| `internal/resolver/resolve.go` | Validation, local/registry evidence, correlation, findings, readiness |
| `internal/resolver/*_test.go` | Pure offline table tests |
| `internal/cmd/resolve.go` | Approved flags/help, production environment adapter, rendering, exit mapping |
| `internal/cmd/resolve_test.go` | Temp-file + httptest command tests |
| `internal/api/types.go` | Add non-serialized AIR metadata; no legacy JSON change |
| `main.go`, `cmd/help.go`, completion files/tests | Additive command/help/completion surface |
| `README.md`, `CHANGELOG.md`, `scripts/integration-test.sh` | Approved docs and surface coverage |

No changes to `vet`, `sync`, existing endpoint methods, shared `exitCodeFor`, hash-cache persistence, or sibling tasks.

## 3. Ordered checklist

### Stage 1 — AIR metadata without output drift

- [ ] Add exported `AIR` to `api.ModelVersion` with `json:"-"`; populate from a wire-only auxiliary field in the existing unmarshaler.
- [ ] Decode fixture AIR and tolerate absence.
- [ ] Marshal a decoded fixture and assert no `air` key appears, protecting existing show/vet JSON.
- [ ] Run `go test ./internal/api`.

Rollback: remove the field and tests only.

### Stage 2 — Pure resolver contracts and policy

- [ ] Implement resolver-owned request/metadata/result/finding/compatibility/report types and `Environment` function values.
- [ ] Validate the four approved reference forms and reject all other forms before environment calls.
- [ ] Implement distinct checkpoint anchor and dependent exact-base policy; cover missing/conflicting anchor, exact match, missing base, mismatched base, unknown type, and idempotent duplicate.
- [ ] Implement deterministic ordering and no score/rank field.
- [ ] Add table tests; no HTTP/filesystem/printing in the package.

Rollback: delete `internal/resolver`.

### Stage 3 — Local and registry evidence

- [ ] Hash explicit paths through the environment seam; do not load/save the sync cache.
- [ ] Map missing paths to findings; non-regular targets to usage; read/hash failures to operational errors.
- [ ] Lookup paths by computed hash, hashes by full SHA, versions by ID; dedupe identical lookups in invocation memory.
- [ ] Convert registry 404 to unresolved resource; preserve network/auth/decode errors.
- [ ] Match SHA256 against all visible files, bind one exact `fileId`, and report ambiguous/missing identity without fallback.
- [ ] Implement `wrong_version` for content/expected-version disagreement.
- [ ] Test non-primary file, same-name wrong bytes, multi-file version, missing path, unresolved hash/version, and network/token failures.

Rollback: resolver still unwired; delete/revert pipeline.

### Stage 4 — Readiness and deterministic reports

- [ ] Compute readiness only when every path is present exactly, one anchor exists, no blocking finding remains, and every compatibility result is compatible.
- [ ] Preserve all independent findings and sort deterministically.
- [ ] Implement versioned JSON report and human READY/NOT READY output with full IDs/hashes and reasons.
- [ ] Test ready, missing, unresolved, wrong-version, unknown compatibility, missing/conflicting anchor, mixed failures, and repeated-run determinism.

Rollback: internal code is not publicly exposed.

### Stage 5 — Command adapter and exits

- [ ] Add `--json`, curated help with the four accepted refs and `version:`/AIR examples, and pure `runResolve`.
- [ ] Build the environment from `newAPIClient(g)`, existing GETs, `os.Stat`, and `computeHash`.
- [ ] Use `executeWithExitCode`: ready 0, completed not-ready 5 after rendering, all other errors delegated unchanged.
- [ ] Test Windows paths/spaces, usage errors with zero I/O, same request counts for human/JSON, report on stdout, and empty stdout on operational failures.
- [ ] Regression-test vet exit 3 and sync not_found behavior.

Rollback: remove `internal/cmd/resolve.go` and registration.

### Stage 6 — Surface, docs, integration

- [ ] Add dispatch/help/completion for resolve; run drift tests.
- [ ] Document READY's narrow meaning, accepted refs, exit 5, unknown compatibility, conservative policy, no downloads/library writes, and output examples.
- [ ] Add integration cases: help/named help, malformed refs, unknown full SHA (not-ready/unresolved), JSON/verdict/exit, timeout/network, no model-byte download.
- [ ] Add CHANGELOG entry for the new command and exit 5.
- [ ] Run `gofmt`, `go vet ./...`, `go test ./...`, `go test -race ./...`, `go list -m all`, `make build`, `git diff --check`.
- [ ] Run updated integration script under normal manual/CI preconditions.
- [ ] Request full-scope `trellis-check` and parent review; update specs only if a durable convention emerges.

## 4. Required test matrix

| Area | Cases |
|---|---|
| CLI validation | Empty set; bare integer/name/model ID; AutoV2/short hash; AIR without version; `version:0`; path+extra identity grammar rejected; windows path with spaces |
| Identity | Version ID, versioned AIR, full SHA, path; AIR absent/present; AIR not serialized in legacy output |
| Local presence | Exact hash, non-primary file, same-name different bytes, multi-file, missing, directory, read/hash error |
| Identity-only | Version/SHA/AIR resolved but `unverified`, cannot ready |
| Wrong version | Path hash resolves to other version; no fallback |
| File identity | Match SHA to visible file; omitted match; duplicate/ambiguous file hash |
| Policy | Exact compatible, missing base, unequal base, baseModelType mismatch/absence, unknown type, missing/conflicting/idempotent anchor |
| Readiness | All-pass ready; one bad member; unknown blocks; remote-only blocks; independent findings retained |
| Reports | Human/JSON parity, full evidence, deterministic order, empty arrays, no score |
| Errors/exits | Ready 0, not-ready 5, usage/local I/O/decode 1, network 2, token 4; vet/sync regressions |
| Side effects | Only approved GETs and explicit file reads; no POST/download/scan/cache/config writes |
| Surface | Help/completion/README/Skill/integration parity |

## 5. Risk and rollback triggers

| Risk | Response |
|---|---|
| AIR field changes legacy JSON | Revert shared field; keep resolve metadata local or add a dedicated non-serialized decode |
| Policy marks aliases compatible without evidence | Keep unknown; require a separate reviewed policy change |
| Name-fallback creeps in | Reject ref before I/O; never search/guess |
| Missing local path treated ready | Block; readiness requires explicit paths |
| Not-ready exit conflicts with existing semantics | Keep 5 distinct, preserve report stdout, regression-test 1/2/3/4 |
| Operational error emits partial verdict | Abort before report/ready; stdout empty |
| Command becomes a directory scanner/downloader | Reject scope; no cache/model library/download |

## 6. Rollback summary

- Before wiring: delete `internal/resolver` and its tests.
- After wiring: remove resolve dispatch/help/completion/docs/integration and the non-serialized AIR field as one additive revert.
- No persisted data, cache migration, or user configuration cleanup is required.
