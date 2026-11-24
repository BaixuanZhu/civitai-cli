# Agent recipe and selection workflows

> Status: draft for user review. This task tree records product direction; it does not authorize implementation.

## Goal

Add a small set of Agent-oriented workflows that answer three concrete questions without exposing Agents to unnecessary REST orchestration:

- `recipe`: how is a selected model version actually used?
- `resolve`: can an explicitly supplied set of resources work in the user's environment?
- `compare`: which model or version facts differ between candidates?

An optional bounded community source may supplement official recipes when creator-provided examples are sparse. The parent task owns the shared contracts, command-surface coherence, and final integration review; child tasks own independently verifiable deliverables.

## Confirmed product decisions

1. Keep the existing commands. Do not remove or rename `search`, `show`, `vet`, `sync`, `tags`, authentication, proxy, skill, upgrade, completion, version, or help commands as part of this work.
2. Add `recipe` and `resolve`; the user explicitly accepted these capabilities on 2026-11-24.
3. Plan model/version `compare` and bounded community recipe sampling as child deliverables, subject to the open review topics below.
4. Do not add a standalone recipe/workflow `export` feature in this scope. It presupposes an artifact workflow the user has not requested.
5. Do not build a persistent local model library, `library` command, or local asset-management database. The user has not observed that need.
6. Existing low-level commands remain available as primitives and debugging/raw-data escape hatches. New high-level commands must not become a mandatory pipeline.
7. Reduce unnecessary `search`/`show` machine-output data as an additional child deliverable. The user explicitly requested this on 2026-11-24, approved default compact JSON with `--view compact|full` (default `compact`) plus a full legacy migration path, and subsequently approved the concrete D2 field registry. Human tables remain curated and unchanged.
8. Convert the final top-level help's flat command list into task-oriented groups as the last child task, after all accepted command deliverables determine the final inventory. Per-command help and command behavior do not change.
9. Apply one list/detail invariant across every child: list and summary outputs contain only small selection fields and never large prompt/description/raw-metadata/workflow payloads. Large fields require an explicitly selected detail view. Explicit legacy full/prompts views are compatibility detail paths, not default list behavior.
10. Use `civ recipe <versionId> --example <sourceIndex>` for singular official-example detail and `civ recipe image <imageId>` for singular community-image detail. The latter makes a separate bounded `GET /images` request with `imageId`, `withMeta=true`, `type=image`, and `limit=1`; same-invocation detail reuse is no longer an alternative.
11. Share one scalar normalization contract across official and community recipes: valid numeric strings in known numeric fields become JSON numbers (including LoRA weight), seed remains an exact decimal string, missing/null stays null, and invalid/non-scalar shapes become null plus a structured issue. Do not preserve number-versus-numeric-string as a semantic distinction, normalize arbitrary unknown fields, infer units, or invent defaults.
12. Set official recipe `--examples` to default 3 with accepted range 0–10. Zero returns version context and selection counts with `examples: []`; count 1 remains a prompt-free summary, and only `--example <sourceIndex>` returns singular detail.

## Evidence and current gaps

- The bundled Agent Skill currently teaches a multi-call search/show/vet workflow (`internal/skill/assets/civ-prompt/SKILL.md:30-49`). A normalized recipe view can reduce command chaining and private per-Agent JSON projections.
- Repository fixtures are substantially larger than the decision data an Agent usually needs: `testdata/models_search_anima.json` is 70,216 bytes and `testdata/version_3263843.json` is 101,395 bytes. Size alone is not a correctness defect, but it motivates compact summary views and bounded community output.
- Current `ImageMeta` deliberately ignores unknown generation fields (`internal/api/types.go:152-167`). New consumers must add only fields they actually use and preserve tolerant decoding rather than creating parallel ad hoc decoders.
- `vet` resolves identity for one file/hash (`internal/cmd/vet.go:105-155`), while `sync` emits per-file audit rows (`internal/cmd/sync.go:101-118`). Neither evaluates cross-resource readiness or compatibility.
- Existing public contracts include stdout-only results, stable JSON shapes, and exit codes (`README.md:494-512`; `.trellis/spec/backend/design-principles.md:247-263`). Additive commands must not silently change those contracts.

## Child task map

| Child | Owned deliverable | Ordering / dependency |
|---|---|---|
| `11-24-recipe-context` | Official recipe normalization, compact detail output, provenance, missing-data behavior | Contract foundation for community recipes; may proceed independently of compare and resolve after shared reference rules are reviewed |
| `11-24-resource-resolver` | Explicit resource-set identity, local exact-match, compatibility, and readiness evaluation | Independent MVP; any shared resource-reference helpers must follow the parent contract rather than creating a second parser |
| `11-24-model-version-compare` | Model comparison, version comparison, and derived differences | Independent; comparison output should expose exact version references reusable by recipe/resolve without inventing a second identifier form |
| `11-24-community-recipes` | Bounded community image-metadata summaries as an optional recipe source | Review and implement after the recipe core contract is stable; independently testable with fixtures |
| `11-24-search-show-compact` | Purpose-driven compact JSON for existing search/model-show/version-show commands, plus `--view full` legacy compatibility | Independent of recipe/resolve/compare; default compact and the D2 field registry are approved, pending final implementation approval |
| `11-24-grouped-help` | Final task-oriented top-level help groups with no command behavior changes | Explicitly last: run only after every accepted command child determines the final public command inventory |

Tree position is not an implicit scheduler. Each child's `prd.md` and `implement.md` records its own dependency and acceptance behavior.

## Parent requirements

### PR-1 — Coherent command surface

- Add only the reviewed top-level capabilities. Do not add separate top-level `context`, `pack`, `workflow`, `export`, `library`, `index`, `doctor`, or `outdated` commands.
- Exact nested command grammar remains a review topic; the final tree must avoid overlapping ways to perform the same operation.
- Every new command has curated dual-entry help, documented flags/defaults/examples, human output, and deliberate `--json` behavior.

### PR-2 — Agent-safe normalized data

- New machine output uses explicit, versioned schemas rather than raw upstream payloads where normalization is the feature's value.
- Distinguish observed facts, derived comparisons, missing data, unresolved data, and warnings. Never convert absence into invented triggers, parameters, compatibility, or quality claims.
- Keep bounded summary output separate from expensive full prompt/detail output. A caller must be able to select a precise resource or recipe without post-processing a large unrelated payload.
- No list or summary view may include large free-form fields or blobs, including prompt/negativePrompt, description HTML, raw generation metadata, or ComfyUI/workflow data. This applies to top-level lists and nested arrays such as recipe examples. Large fields are allowed only in an explicitly selected detail result or an explicitly requested legacy full/prompts compatibility view.
- Official and community recipe projections share the approved scalar normalizer: known numeric strings become numbers, seed is an exact decimal string, invalid/non-scalar values produce null plus issue, and no arbitrary unknown-field coercion or unit inference occurs.
- Include stable source identifiers and provenance sufficient to trace a result back to Civitai model/version/image URLs or a local content hash.

### PR-3 — Shared resource-reference semantics

- Define one reviewed way to represent exact model/version references across recipe, resolve, and compare.
- Local path, SHA256, AutoV2, AIR, URL, and numeric identifier support must be explicit per command; ambiguous forms are usage errors rather than guesses.
- Exact version/file identity and content hashes must remain distinguishable from filename-only or same-model/different-version matches.

### PR-4 — Read-only and bounded behavior

- No model/image downloads, automatic dependency installation, workflow mutation, uploads, generation jobs, or NSFW-gating bypass.
- Community sampling is bounded by an explicit limit and does not exhaust cursors, crawl a model, or persist a dataset.
- Resolve evaluates only the requested resource set in its MVP. It does not implicitly scan and index the user's whole machine.
- New functionality introduces no persistent library or database.

### PR-5 — Compatibility with existing behavior

- Commands outside the explicitly scoped `search`/`show` JSON compaction retain their flags, JSON shapes, exit meanings, retry/auth/proxy behavior, and help behavior except for additive help/dispatch entries.
- Search/show compaction follows the approved default-compact/`--view full` migration policy and approved D2 field registry/view-validation rules. No other existing output may be changed incidentally to share implementation with the compact views.
- Existing human search/show tables and the separate `show -v --prompts --json` view are not assumed to change; any such change needs its own product decision.
- All outbound requests continue through `internal/api.Client`; all logging/warnings stay on stderr.
- Preserve the standard-library-only, Windows-first, single-static-binary, offline-unit-test design.
- Every new or changed CLI surface updates `scripts/integration-test.sh` in the same child/integration change, including any intentionally changed default and retained legacy/full view.

### PR-6 — Documentation and Agent Skill integration

- README and curated help describe task-oriented use, not merely endpoint mappings.
- The bundled `civ-prompt` Skill uses normalized recipe output where it reduces prompt-building mistakes, while retaining low-level commands as fallbacks.
- Documentation states that community reactions are popularity evidence, comparison is not an opaque recommendation, and resolve is not a safety guarantee or an automatic downloader.

### PR-7 — Final grouped help

- After all accepted command children are integrated, replace only the top-level flat command-list block with `Discover`, `Recipes`, `Local models`, and `Utilities` groups covering the final inventory.
- Preserve command names, dispatch, per-command help, completion, output destinations, and existing exits. Deferred/unimplemented commands and internal `unbootstrap` must not appear.
- The grouping child is explicitly last; the parent final integration review follows it.

## Cross-child acceptance criteria

- [ ] **PAC-1 Recipe handoff:** a precise version reference emitted by `compare` can be passed to the reviewed `recipe` input form without manual JSON extraction or identifier conversion.
- [ ] **PAC-2 Resolve handoff:** a selected resource/version reference can be passed to `resolve` using the same documented reference grammar; no child defines a conflicting `model@version` or AIR interpretation.
- [ ] **PAC-3 Community handoff:** community listing returns bounded summaries with image/source identifiers. `civ recipe image <imageId>` retrieves one selected image's details through exactly one bounded `imageId+withMeta=true+type=image+limit=1` request, without automatically fetching every full prompt, reusing cross-invocation memory, following cursors, or persisting detail data.
- [ ] **PAC-4 Missing-data honesty:** official and community recipes with null/partial metadata are counted and visibly marked; no output invents default trigger words, sampler settings, weights, or compatibility.
- [ ] **PAC-5 Resolve independence:** resolve works for its reviewed explicit-input MVP without requiring the recipe, compare, community, export, or persistent-library features.
- [ ] **PAC-6 Compare independence:** compare can be used directly after search and never becomes a required precondition for recipe or resolve.
- [ ] **PAC-7 Legacy regression:** unaffected command unit/integration assertions remain green without changes that weaken previous JSON or exit-code expectations. Search/show assertions change only where the approved compaction contract requires it, and any retained full/legacy view proves the corresponding pre-change behavior.
- [ ] **PAC-8 Surface completeness:** every added command/subcommand/flag/input/output/exit behavior has curated help, README coverage, offline tests, and matching cases in `scripts/integration-test.sh`.
- [ ] **PAC-9 Output discipline:** `--json` stdout is parseable with no human decoration; logs and warnings are stderr-only; bounded summary views do not unexpectedly include all full prompts or raw image/workflow blobs.
- [ ] **PAC-10 Final integration:** `go vet ./...`, `go test ./...`, and `go test -race ./...` pass, `go list -m all` still lists only `civitai-cli`, and the parent reviews all child diffs together before release notes or archival.
- [ ] **PAC-11 Compact basic views:** the reviewed search/model-show/version-show compact forms preserve candidate and exact-version selection identifiers, make no additional API requests, materially reduce representative fixture payloads without silently dropping records, and have explicit default/migration documentation and integration coverage.
- [ ] **PAC-12 Final grouped help:** the last child presents every delivered public command exactly once under the reviewed task groups, preserves all non-command-list overview sections and per-command help, keeps `unbootstrap` hidden, and passes the stdout/stderr/exit and integration help matrix without advertising deferred functionality.
- [ ] **PAC-13 List/detail invariant:** contract tests scan every new or changed list/summary view, including nested recipe/community example arrays, and prove prompt/negativePrompt/description/raw-meta/workflow payloads are absent. Each large field is reachable only through a documented explicit detail selection or legacy full/prompts compatibility view, with no hidden bulk-detail fan-out.
- [ ] **PAC-14 Shared scalar normalization:** official/community fixture tests prove numeric `20` and string `"20"` normalize to the same JSON number in known fields, numeric-string LoRA weights normalize likewise, seed remains an exact decimal string, missing/null stays null, and invalid/non-scalar shapes produce null plus structured issue without defaults, unit inference, or coercion of unknown fields.
- [ ] **PAC-15 Resolve contract:** `civ resolve` accepts only path/full-SHA256/versioned-AIR/`version:<id>`; readiness requires explicit local presence and exactly one checkpoint; unknown compatibility blocks ready; `0`/`5`/`1`/`2`/`4` mapping is documented and tested; no name-only, download, scanner, cache, or workflow behavior exists.
- [ ] **PAC-16 Compare contract:** `civ compare models|versions` requires 2–5 unique exact IDs, uses first-subject baseline, strict all-or-nothing fetches, file-pairing cascade, full human/diff/JSON behavior, and excludes prompts/descriptions/raw images/download URLs/scores.

## Out of scope

- Standalone recipe/workflow export, lockfile generation, or recipe-directory formats.
- Persistent local model library, whole-machine discovery, update tracking, and asset-management commands.
- Automatic downloads, dependency remediation, model renaming, workflow editing, or ComfyUI execution.
- Image generation, uploads, paid/Buzz operations, or creator write operations.
- Opaque model/recipe quality scores, embedding search, semantic clustering, and claims that popular recipes are optimal.
- Exhaustive community crawling, offline community archives, or image downloading.
- A new MCP transport or a generic CLI framework migration.
- Replacing existing hash-cache behavior merely to support these features.

## Approved cross-child decisions

1. **Command grammar:** recipe uses exact `civ recipe <versionId>`, `--source official|community`, singular open `--example <sourceIndex>`; community singular detail is `civ recipe image <imageId>`. Compare uses `civ compare models <id>...` / `civ compare versions <id>...`. Resolve uses `civ resolve <ref>...`.
2. **Reference forms:** recipe is exact-version; compare uses explicit model/version IDs; resolve accepts only local path, full SHA256, versioned AIR, or `version:<id>`. No command accepts ambiguous bare IDs, names, or implicit latest selection across kinds.
3. **Resolve process:** ready exits 0, completed not-ready exits 5 with report on stdout; usage/local-I/O/decode 1, network 2, token 4; vet keeps exit 3.
4. **Resolve compatibility:** exactly one checkpoint anchor; exact-base equality for LORA/TextualInversion/Controlnet/VAE; missing/mismatched/unknown data yields unknown and blocks readiness; no aliases or name-only mode in MVP.
5. **Recipe schema:** approved prompt-free summary/detail DTOs, text-availability statuses, default `--examples 3` range 0–10, exact-version one-request-per-invocation, and shared scalar normalizer.
6. **Compare:** 2–5 unique subjects, full human output by default, `--diff` hides equal rows, `--diff --json` rejected, first subject baseline, strict all-or-nothing fetches, file-pairing cascade and sequential lookups.
7. **Community:** default source official, community list `--source community`, limit 20 (1–100), Most Reactions/AllTime default with Newest supported, per-image observations only (no aggregation/ranking), separate exact `imageId` detail.
8. **Sequencing:** recipe core first; compare/resolve independently after shared contract; community after recipe; search/show compaction under approved D2; grouped help last; parent integration review last.

## Planning status

All product and contract decisions across this task tree are resolved. Planning is converged and awaiting explicit final implementation approval. No implementation or `task.py start` has occurred.
