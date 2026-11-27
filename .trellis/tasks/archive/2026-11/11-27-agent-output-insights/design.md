# Agent-oriented CLI output insights — Technical Design

## Architecture and Ownership

The work remains inside the existing `main -> cmd -> api/report` dependency direction. No new command framework, third-party module, persistent cache, downloader, or background service is introduced.

| Track | Child-owned implementation surface | Parent-coordinated shared surface |
|---|---|---|
| Community stack | `internal/cmd/recipe*`, `internal/api/community*`, focused fixtures/tests | Recipe help examples, README, bundled `civ-prompt` references, integration script, specs |
| Search summary | `internal/cmd/search*`, `internal/api/compact*` and focused tests | Search docs, integration script, skill command reference, specs |
| Non-TTY names | `internal/report/width_*`, `internal/report/table*`, search layout/tests after search summary lands | Output-behavior docs and integration assertions |
| Vet latest context | `internal/cmd/vet*` and focused tests/fixtures | Vet docs and integration assertions |

Shared files are edited by the main orchestrator, never concurrently by child implementation agents. A child is not checked/archived until its corresponding shared integration and documentation updates are present.

## Data Flow

### Community stack summary

1. The command validates the existing community selectors plus `--stack-summary` before network activity.
2. The existing client performs the same single bounded `/images` request.
3. The API/community projection reads normalized scalar/resource observations and prompt-status evidence from that response.
4. A pure aggregation layer computes identity-frequency, numeric-weight, parameter-distribution, and coverage facts with explicit denominators and deterministic ordering.
5. A new aggregate-only DTO/rendering path emits human or versioned JSON output. It has no fields capable of carrying prompt text, raw metadata, workflows, observations, or media locations.

The existing per-image community sample remains available when `--stack-summary` is absent. Exact-image detail remains a separate explicit command.

### Search summary

1. The existing `/models` response is decoded through the current human or compact projection.
2. A pure local projector counts returned models and observed base-model coverage under the child-defined fallback/deduplication rules.
3. Human rendering flushes the table before writing the footer. Compact JSON adds a structured summary; raw `--view full` continues to emit only the upstream items array.

### Non-TTY search names

1. A report-layer output-mode seam distinguishes a successful stdout console query from non-console/query-failure fallback.
2. Pure search-layout logic selects existing width budgets for a console and no NAME truncation for non-console output.
3. JSON rendering does not consult this human-layout decision. The search-summary footer remains downstream of table flushing.

### Vet latest context

1. The existing by-hash response remains authoritative for the vetted version.
2. The existing best-effort owning-model response supplies visible embedded versions.
3. A pure helper derives latest-published context only when timestamps, visibility, identity, and uniqueness rules in the child contract are satisfied.
4. Existing model-fetch degradation continues to warn and render the vet hit, with unknown latest context. The focused `--prompts --json` path remains a recipe-only response.

## Contract and Compatibility Rules

- New normalized machine views use the existing versioned-envelope conventions; additive fields must not silently alter legacy raw/focused views.
- Summary DTOs enforce redaction by construction where applicable, not only by renderer discipline.
- Aggregate logic is pure and separated from HTTP/rendering so duplicates, malformed values, ties, empty sets, and numeric boundaries can be tested offline.
- Existing exit codes remain unchanged. Validation failures occur before requests and produce no stdout.
- Every derived count carries enough scope information to prevent it being read as a global total or representative estimate.

## Ordering and Parallelism

- Community stack, search summary, and optionally vet may be implemented on disjoint code files in parallel.
- Non-TTY search work starts only after search summary is integrated; both modify `internal/cmd/search.go` and its tests.
- Shared README/CHANGELOG/skill/spec/integration files are serialized by the orchestrator regardless of code parallelism.
- Each child receives its own scoped review and change set. The parent performs only final integration and cross-contract review after all children finish.

## Rollout and Rollback

- Land each child independently with its unit and integration coverage; do not create one monolithic four-feature commit.
- Roll back a faulty child by reverting its scoped implementation and corresponding shared assertions/docs while preserving later independent children.
- If non-TTY rendering regresses, restore the previous layout seam without removing the search summary.
- If community aggregation proves upstream-fragile, disable the new flag path and retain the existing per-image community sample unchanged.
- Vet latest context is informational and can be removed without changing hash lookup, exit codes, or model-fetch degradation.
