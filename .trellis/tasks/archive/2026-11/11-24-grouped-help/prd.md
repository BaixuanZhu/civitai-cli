# Grouped top-level help

> Status: lightweight final-child PRD. This task is deliberately ordered after every other child in `11-24-agent-workflows`; it does not authorize implementation or `task.py start`. PRD-only is sufficient unless review uncovers a larger help-rendering redesign.

## Goal and user value

Replace the final top-level `civ` command list's flat presentation with task-oriented groups so users can identify discovery, recipe, local-model, and utility operations without reading every row.

This is an information-architecture change only. Command names, dispatch, flags, per-command help, JSON, exit behavior, and shell completion remain those delivered by the preceding children.

## Confirmed repository facts

- The current overview is one flat `Commands:` block in `main.go:141-154`, followed by global flags and other reference sections in `main.go:156-188`.
- Bare `civ` prints the overview to stderr with exit 1; `civ help`, `civ -h`, and `civ --help` print it to stdout with exit 0; unknown commands print the overview to stderr with exit 1 (`main.go:41-43`, `main.go:87-114`). Grouping must not alter these process contracts.
- Named/per-command help is independently owned by `internal/cmd/help.go:20-68` and the command-specific help constants. This task does not need to rewrite those pages.
- `unbootstrap` is an internal uninstall hook intentionally absent from the public overview (`main.go:78-84`). It must remain hidden.
- The parent task plans `recipe`, `resolve`, and `compare` in addition to the current commands. Their rows can be grouped only after the corresponding child behavior is actually delivered; help must never advertise placeholders or unimplemented commands.

## Final-child ordering

1. Complete and integrate all accepted sibling deliverables: recipe context, resource resolver, model/version compare, bounded community recipes, and search/show compaction.
2. If any sibling is deferred, descoped, or withdrawn, update the expected final command inventory rather than reserving an empty group or placeholder row.
3. Perform this grouping task against that final inventory.
4. Parent performs the final cross-child review after this child.

This is an explicit dependency, not an inference from tree position.

## Requirements

### H1 — Task-oriented command groups

Use the previously discussed four-group model, adapted to the final delivered inventory:

| Group | Command rows when the corresponding siblings are delivered |
|---|---|
| `Discover:` | `search`, `show`, `compare`, `tags` |
| `Recipes:` | `recipe` |
| `Local models:` | `vet`, `sync`, `resolve` |
| `Utilities:` | `login`, `logout`, `proxy`, `skill`, `upgrade`, `completion`, `version`, `help` |

- Keep one concise synopsis per public command, based on the final command behavior.
- Preserve the group and row order above unless a final-command inventory review makes a documented adjustment.
- Do not add `export`, `library`, or any other excluded/unimplemented command.
- Keep `unbootstrap` hidden.

### H2 — Only the overview command-list layout changes

- Keep the existing title, `Usage:` line, global flags, login/proxy/environment/exit reference sections, and their order.
- Compare against the final pre-grouping overview: outside the command-list block, output remains byte-identical except for an independently reviewed correction required by a preceding child.
- Do not rewrite per-command help pages, named help routing, completion scripts, README command semantics, or command behavior as part of this grouping task.

### H3 — Preserve all output destinations and exits

- `civ help`, `civ -h`, and `civ --help` emit the same grouped overview to stdout with exit 0.
- Bare `civ` continues to emit the grouped overview to stderr with exit 1.
- Unknown commands and unknown named-help targets continue to emit the grouped overview to stderr with exit 1.
- Group headings and rows are plain English ASCII, with no color, pagination, interactive input, or terminal-control side effects.

### H4 — Final inventory coverage and drift prevention

- Every public command delivered by the parent tree appears exactly once in exactly one group.
- No current command disappears during regrouping.
- Add a deterministic test/golden or structural assertion for group order, row order, unique membership, final command coverage, and hidden `unbootstrap` exclusion.
- Keep the grouping representation maintainable so a future command cannot be added to dispatch without an overview review; do not create a second conflicting command-description registry merely for presentation.

### H5 — Documentation and verification

- Update any README/help snapshot that reproduces the flat command list, without changing unrelated documentation.
- Update `scripts/integration-test.sh` for the grouped top-level output and existing stdout/stderr/exit matrix, while retaining named/per-command help assertions.
- Run offline unit/help tests and the project quality gates. No live Civitai request is needed to verify grouping; existing live help checks must continue to pass.

## Observable acceptance criteria

- [ ] **AC1 — Grouped stdout help:** after all accepted siblings are integrated, `civ help`, `civ -h`, and `civ --help` produce byte-identical stdout containing `Discover:`, `Recipes:`, `Local models:`, and `Utilities:` in that order, followed by the existing non-command-list sections, with exit 0.
- [ ] **AC2 — Correct membership:** the grouped list contains every final public command exactly once: current commands plus each actually delivered `recipe`, `resolve`, and `compare`; their placement matches H1. `unbootstrap`, excluded commands, and deferred/unimplemented siblings are absent.
- [ ] **AC3 — Non-command sections preserved:** against the final pre-grouping snapshot, only the flat command-list block is replaced. Title, usage, global flags, reference sections, and trailing guidance remain otherwise unchanged.
- [ ] **AC4 — Error/help process behavior:** bare `civ`, unknown commands, and unknown named-help targets preserve their existing stderr/exit-1 behavior while showing the grouped overview. No grouped output contaminates stdout on those failures.
- [ ] **AC5 — Per-command help unchanged:** every `civ <command> help`, `civ <command> -h`, and `civ help <command>` result is byte-identical before and after this task, including upgrade's special route.
- [ ] **AC6 — Completion and dispatch unchanged:** shell completion output and actual command dispatch are unaffected; no group label becomes a callable command or completion entry.
- [ ] **AC7 — Integration parity:** `scripts/integration-test.sh` asserts the four headings, final command membership/uniqueness or equivalent structural coverage, output destinations, and exits. Offline tests cover the complete grouping without network access.
- [ ] **AC8 — Project quality:** `go vet ./...`, `go test ./...`, and `go test -race ./...` pass; all output remains English and no dependency is added.

## Out of scope

- Adding, removing, renaming, or implementing any command.
- Changing command flags, JSON, API calls, exit-code allocation, or completion behavior.
- Rewriting curated per-command help or global-flag ownership.
- Help pagination, interactive menus, search within help, color styling, or a new CLI/help framework.
- Persistent preferences, localization, export, local library, or any sibling feature implementation.
- Grouping before the final command inventory is known.

## Planning status

The four-group layout and final-child ordering are recorded from the ongoing user discussion. There is no separate blocking product question in this lightweight task; its exact final row inventory is intentionally determined by the accepted preceding children. Implementation still requires the parent planning review and fresh explicit approval after all prerequisite children are complete.
