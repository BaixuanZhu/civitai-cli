# Design: shell completion (child 11-23-shell-completion)

## Summary

New subcommand `civ completion <shell>` printing hand-written, static completion scripts for bash, zsh, fish, and PowerShell. A Go-side spec table (commands → flags → enum value sets) is the single source; a drift-guard test proves the scripts' coverage stays equal to the real flag registrations.

## Current state (verified in code)

- Commands self-register flags through per-command `register(fs *flag.FlagSet)` closures on unexported options structs (package `cmd`) — a same-package test can enumerate every registered flag by building each command's FlagSet.
- main.go's dispatch switch is the authoritative command list (`unbootstrap` is an internal maintenance hook to exclude).
- Enum tables already exist for the API mapping in `internal/api/enums.go` (sort/period/type) — the completion enum lists mirror the CLI-facing names documented in each help page (`--sort` lists its orders, etc.). Free-form flags take no value completion.
- No completion exists today; nothing in `report`/`console` is needed (raw script consts).

## Design decisions

- **D1 — File layout.** `internal/cmd/completion.go`: the command (`completionHelp` const, `Completion(args []string)` entry via the standard `execute` runner, pure `runCompletion`), the spec table, and the argument validation. `internal/cmd/completion_scripts.go`: the four script consts (`bashCompletionScript`, `zshCompletionScript`, `fishCompletionScript`, `powershellCompletionScript`) — one per shell, kept as raw string consts with GoDoc explaining they are hand-maintained and drift-guarded. Splitting scripts out keeps each file skimmable and matches the one-concern-per-file convention.
- **D2 — Spec table.** `completionSpec`: ordered command entries `{name, flags: []flagSpec{name, values []string}}` plus global flags once. `values` empty ⇒ flag takes no value completion. The table drives (a) the drift-guard test and (b) nothing at runtime — scripts are static. This is deliberate: shipping static text keeps shell startup cost zero and needs no `civ` subprocess at completion time; the table exists so the test can prove the static scripts still cover reality.
- **D3 — Script mechanics per shell (all static):**
  - bash: a `_civ()` function completing command words on `$COMP_CWORD = 1`; per-command case blocks completing flags (`compgen -W`), with enum value completion after `--flag=`/space for the enumerable ones. `complete -F _civ civ`.
  - zsh: `#compdef civ` + `_arguments` with `-command-` dispatch: `1: :->cmds` then per-command nested `_arguments` specs; enum values as `(a b c)` lists.
  - fish: `complete -c civ -n '__fish_use_subcommand' -a 'search show ...'` plus per-command `complete -c civ -n '__fish_seen_subcommand_from search' -l sort -a 'hot highest-rated ...'`.
  - powershell: `Register-ArgumentCompleter -Native -CommandName 'civ' -ScriptBlock { ... }` switching on `$wordToComplete` position/command, completing `[System.Management.Automation.CompletionResult]` items; enum sets via switch on the flag name.
- **D4 — Arg behavior.** Bare `civ completion` → print `completionHelp`, exit 0. One positional in {bash,zsh,fish,powershell} → print script, exit 0. Anything else (unknown shell, extra args) → stderr message naming the valid set, exit 1 (usage error through the runner's normal mapping). Help via the runner: `civ completion help` / `-h`.
- **D5 — main + docs wiring.** Dispatch case in `main.go`; one usage line `completion <shell>  Print a shell completion script (bash, zsh, fish, powershell)`; help page carries per-shell install one-liners (`source <(civ completion bash)`, `civ completion zsh > .../completions/_civ`, fish `civ completion fish | source`, pwsh `civ completion powershell | Out-String | Invoke-Expression` note about $PROFILE). README gains a short Completion section; CHANGELOG entry under Added.
- **D6 — Drift guard.** `TestCompletionSpec_CoversRegisteredFlags`: for each command with a `register` func, build a FlagSet, `VisitAll`, assert every flag name (plus the five globals, which live in the runner's `registerGlobalFlags` — enumerate from a built globals FlagSet the same way) appears in the spec. `TestCompletionSpec_CoversDispatch`: spec command names == dispatch list minus {unbootstrap, version handled as command word too}. Script-content smoke: each const contains its shell's anchor (`complete -F _civ civ`, `#compdef civ`, `complete -c civ`, `Register-ArgumentCompleter`) and every command word.
- **D7 — Exclusions.** `unbootstrap` never completes. `help`/`version` complete as command words. Positional value completion (model IDs, tags, paths) out of scope except enum flags (R2).

## Contracts touched

- New exported `cmd.Completion` (called from main). New help page (runner contract: stdout, exit 0). usage() text. No exit-code contract changes beyond the new usage error for a bad shell name (exit 1, the standard bucket).

## Test plan

- Unit: script consts non-empty with anchors; spec table drift guards; `runCompletion` pure-function table (shell → script; unknown → error; bare → help) through the runner where feasible.
- Integration: `completion_bash`, `completion_zsh`, `completion_fish`, `completion_pwsh` (exit 0, script anchors), `completion_bad` (exit 1), `completion_kw` (`completion help`, exit 0).
- Local syntax sanity: `bash -n` on the emitted bash script in a unit test is not possible (needs the shell); do it in the integration script run if bash exists, else manual smoke per parent AC5.

## Rollback

Revert commit; no data/config impact. The spec table stays useful even if scripts are later replaced with generated ones.
