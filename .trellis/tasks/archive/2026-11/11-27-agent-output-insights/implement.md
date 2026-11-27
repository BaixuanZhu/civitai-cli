# Agent-oriented CLI output insights — Execution Plan

## Review Gate

- [ ] Review the parent and all child PRD/design/implement artifacts with the user.
- [ ] Obtain explicit approval of the latest planning summary before running `task.py start` for any child.
- [ ] Start only the child that owns the active deliverable; do not start the parent for child implementation.

## 1. Independent Child Implementation Tracks

The following tracks may proceed in parallel only with disjoint file ownership and separate work products:

### Track A — `11-27-community-stack-summary`

- [ ] Implement the validated flag, pure aggregation, aggregate-only DTOs, and human/JSON renderers according to the child artifacts.
- [ ] Add offline success, malformed/partial-data, deterministic-order, request-count, and expected-failure tests.
- [ ] Provide the parent with the exact integration and documentation edits listed in the child plan.

### Track B1 — `11-27-search-base-model-summary`

- [ ] Implement the shared local summary projector, compact JSON addition, and human footer.
- [ ] Prove returned-only counts, multi-base reconciliation, fallback behavior, deterministic order, empty results, raw-view compatibility, and validation request counts.
- [ ] Provide shared integration/documentation edits to the parent.

### Track C — `11-27-vet-latest-version-context` (P2)

- [ ] Implement the pure latest-visible-version projection using the existing owning-model response.
- [ ] Add human and normal-JSON output while leaving focused prompt JSON, by-hash behavior, model degradation, and exit codes unchanged.
- [ ] Add timestamp/visibility/tie/base-unknown tests and prove the existing request count.
- [ ] Provide shared integration/documentation edits to the parent.

## 2. Parent-Coordinated Integration for Each Child

Perform these edits serially in the main session before that child's final check:

- [ ] Update curated command help and README examples/flag tables/contracts.
- [ ] Update `scripts/integration-test.sh` for new surface, changed output, compatibility views, and expected failures.
- [ ] Update bundled `internal/skill/assets/civ-prompt` guidance when the consumer workflow changes, especially aggregate-first community usage.
- [ ] Update `.trellis/spec/backend` only where a durable contract changes; remove obsolete “no aggregation” language if the community summary is approved.
- [ ] Update CHANGELOG according to the repository release process; do not invent a release version during child implementation.
- [ ] Review the complete child diff, run its required checks, and create a scoped commit only with the user's commit authorization.

## 3. Ordered Search Follow-up — `11-27-non-tty-search-names`

- [ ] Start only after Track B1 is integrated and its search summary tests are green.
- [ ] Rebase/merge without deleting the summary projector, footer, JSON field, or integration assertions.
- [ ] Implement the report output-mode seam and pure NAME-truncation decision; leave TTY and JSON behavior unchanged.
- [ ] Add non-console/query-failure, narrow-TTY, long/Unicode-name, and unchanged-other-column tests.
- [ ] Apply the parent-owned integration/docs updates and run the child quality gate.

## 4. Final Parent Integration Review

- [ ] Activate the parent only after all child deliverables are complete and the parent itself is ready for direct integration work.
- [ ] Inspect the combined diff for scope drift, accidental output duplication, request fan-out, cross-child regressions, and docs/script parity.
- [ ] Verify that no withdrawn feature (`resolve --plan`, `stats`, `--description`, implicit prompt version selection, `sync --latest`) entered the implementation.
- [ ] Run formatting and the complete offline quality gate.
- [ ] Run the online integration suite when permitted; report live failures or skipped verification faithfully.
- [ ] Update final specs/task artifacts and obtain any required commit/archive authorization.

## Validation Commands

```bash
gofmt -w <changed-go-files>
go vet ./...
go test ./...
go test -race ./...
go list -m all
git diff --check
bash scripts/integration-test.sh <path-to-tested-civ.exe>
```

Use focused package tests during implementation, but do not substitute them for the final full-scope gate. Do not run upgrade smoke against the working binary unless using a disposable copy.

## Rollback Points

- Roll back each child at its own scoped change boundary; preserve independent child changes.
- For search regressions, separate the summary change from the later non-TTY layout change so either can be isolated.
- For shared integration/docs errors, correct only the relevant child's assertions and documentation rather than reverting code blindly.
- Stop and return to planning if upstream evidence requires extra requests, a different statistical meaning, implicit version selection, or a new download/acquisition capability.
