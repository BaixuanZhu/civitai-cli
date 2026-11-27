# Agent-oriented CLI output insights

## Goal

Reduce repetitive agent-side scanning and reconciliation by adding four bounded, read-only output improvements to `civ`: community recipe stack aggregation, search result/base-model summaries, complete model names in non-TTY search output, and informational latest-published version context in `vet`.

The parent owns the source requirement set, child-task map, shared integration/documentation surface, and final cross-child review. It is not the default implementation target for child-owned features.

## User Value

- Answer “which resources and parameters occur together in this bounded community sample?” without manually scanning every image observation.
- Answer “which base models are represented in this returned search page?” without a second JSON-processing pass.
- Preserve model identity in piped/redirected human output instead of forcing another call to recover truncated names.
- Show whether a vetted file differs from the latest published visible version without implying that the newer version is preferable.

## Confirmed Facts

- Community mode already performs one bounded `/images` request and exposes normalized parameters and resource stacks (`internal/cmd/recipe.go:251-287`, `internal/api/community.go:88-143`). Aggregation can be local and prompt-free.
- Search already has model-level `baseModels` and embedded version references in the same `/models` response (`internal/api/types.go:24-41`, `internal/cmd/search.go:171-205`). Its continuation metadata does not establish a total match count.
- Non-console output currently falls back to 80 columns (`internal/report/table.go:31-39`); the search layout then reduces `NAME` to 12 runes (`internal/cmd/search.go:242-259`).
- `vet` already fetches the owning model after its by-hash lookup (`internal/cmd/vet.go:132-147`, `internal/cmd/vet.go:181-205`), so latest-visible-version context needs no third request when that fetch succeeds.
- Every CLI-surface change requires offline expected-failure coverage and corresponding `scripts/integration-test.sh` updates (`.trellis/spec/backend/quality-guidelines.md:89-119`).

## Task Map and Scope

| Priority | Child task | Deliverable | Ordering |
|---|---|---|---|
| P1 | `11-27-community-stack-summary` | Aggregate-only human and JSON community stack summary from the existing bounded response | Independent code track |
| P1 | `11-27-search-base-model-summary` | Returned-count and explicitly defined base-model coverage in human and compact JSON search output | Before the non-TTY child |
| P1 | `11-27-non-tty-search-names` | Full model `NAME` values in non-TTY human search output; TTY behavior unchanged | After the search-summary child because both touch search rendering/tests |
| P2 | `11-27-vet-latest-version-context` | Informational latest-published visible-version and base-model comparison using the existing model fetch | Independent code track; optional to schedule after P1 work |

Each child PRD owns its detailed output contract and observable acceptance criteria. This parent does not duplicate those field-level contracts.

## Cross-Child Requirements

### P-R1 — Read-only bounded behavior

- Add no model/image download, upload, local-library scan, automatic workflow export, or NSFW-gating change.
- Add no API request beyond each command's existing successful-path request chain: one bounded `/images` request for community recipe, one `/models` search request, and the existing by-hash plus best-effort owning-model fetches for `vet`.
- Do not paginate community samples or follow continuations automatically.

### P-R2 — Evidence, not invented conclusions

- Search counts describe returned items, never total registry matches.
- Community aggregates describe only the selected bounded sample and expose their denominators; they are not random-sample estimates, quality scores, recommendations, or stability guarantees.
- `latest` means latest published visible version, not default, recommended, best, or an upgrade instruction.
- Do not infer prompt/version selection, historical trends, rollback, or user preferences.

### P-R3 — Agent-consumable output

- New aggregated information has both a concise human representation and a structured machine representation, except that focused legacy views explicitly documented by a child remain unchanged.
- Existing raw/full compatibility views remain raw where their contract requires it.
- Output remains bounded and deterministic; missing, malformed, empty, tied, and degraded states stay explicit rather than becoming fabricated zeroes or defaults.
- stdout contains results only; diagnostics remain on stderr.

### P-R4 — Independent delivery with shared-file coordination

- Child-owned implementation and unit tests can be reviewed independently.
- The parent orchestrator serializes edits to shared documentation, skill references, specs, and `scripts/integration-test.sh`.
- Search-summary implementation lands before non-TTY search rendering; the latter must preserve the former's footer and JSON summary.
- No child may revert another child's merged output contract during rebase or integration.

### P-R5 — Project quality contracts

- Preserve the standard-library-only, all-English, versioned-output, tolerant-decoding, and existing exit-code conventions.
- Add table-driven success and expected-failure tests, including zero-request/empty-stdout assertions for pre-network validation failures where applicable.
- Update command help, README, relevant bundled skill references, integration coverage, and specs for every changed CLI contract.

## Parent Acceptance Criteria

- [ ] **P-AC1:** Every child satisfies its own PRD and its implementation/check reports identify the exact contracts verified.
- [ ] **P-AC2:** Request-count tests prove the no-extra-request invariants, and no implementation introduces downloads, pagination, implicit version selection, trend/rollback claims, or recommendation scores.
- [ ] **P-AC3:** Human and JSON outputs agree on their facts; empty/partial/degraded inputs have tested, non-fabricated representations and deterministic ordering.
- [ ] **P-AC4:** `scripts/integration-test.sh` covers every new flag and changed output shape, including expected failures and legacy-view compatibility; README/help/skill/spec references match the delivered surface.
- [ ] **P-AC5:** The final tree passes `gofmt` verification, `go vet ./...`, `go test ./...`, `go test -race ./...`, `go list -m all` with only `civitai-cli`, and `git diff --check`.
- [ ] **P-AC6:** The parent runs the online integration suite when network access permits, records any skipped live verification explicitly, and performs a final cross-child review for request count, output density, compatibility, and scope drift.
- [ ] **P-AC7:** All four child tasks are completed or explicitly removed from scope through a new user-approved planning review before the parent is archived.

## Out of Scope

- `resolve --plan`, a new `civ stats` command, and `show --description`.
- `show --prompts` without an explicit version, implicit “primary” version selection, and flat all-version prompt aggregation.
- `sync --latest` or per-file owning-model fan-out.
- Model/image downloads, automatic acquisition manifests, library scanning, upload, generation, and workflow execution.
- Time-series popularity, trend detection, rollback detection, model ranking, “winner” selection, and automated upgrade advice.
- Additional features not represented by a child task in the approved task map.

## Planning Status

No blocking open questions remain. Minor field names and rendering details are recommendations in the child design documents and become approved only through the final planning review.
