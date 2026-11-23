# Implement: shell completion (child 11-23-shell-completion)

> **Wave scope override (parent prd constraint):** touch ONLY new files `internal/cmd/completion.go`, `internal/cmd/completion_scripts.go`, `internal/cmd/completion_test.go`. Do NOT touch `main.go`, `scripts/`, `CHANGELOG.md`, `README.md`, or any existing `internal/cmd/*.go`/`internal/cmd/*_test.go` — the parent integration pass owns shared files. Do NOT git commit. The drift-guard test reads sibling commands' real `register` funcs — reading them is expected; modifying them is not.

Execution order and validation. Largest of the five children; land it after the two tiny ones if you want early wins, but it is independent of the upgrade children.

## Checklist

1. **internal/cmd/completion.go**
   - `completionHelp` const following the shared page layout (Usage/Options/Examples; Options documents the four shell names; Examples show the install one-liners per design D5; keep the `--json`-example convention — a `civ completion bash --json`? No: scripts are text; instead satisfy the audit by keeping an example block; if the audit test demands a `--json` example line, include `civ completion bash --json` is wrong — check `hasJSONExample` expectation during implementation and reconcile: the convention is one example line carrying `--json`; decide then whether `completion` documents that `--json` is accepted-but-inert via an example `civ completion --json bash` or relax via test exemption with a comment. RECORD the final decision here: __________).
   - Spec table `completionSpec` (design D2): ordered entries for search, show, vet, sync, tags, login, logout, proxy, skill, upgrade, completion, version, help; per-command flags with enum `values`; the five global flags once.
   - `Completion(args []string)` via `execute` runner; pure `runCompletion`: bare → print help const through the runner path (return nil after writing), one known shell → print script const, else usage error naming valid shells.
2. **internal/cmd/completion_scripts.go** — the four script consts (design D3 mechanics). Keep each script: (a) completing command words in position 1; (b) per-command flags incl. globals after the command; (c) enum values for sort/period/type/format/scope/platform + proxy schemes after `proxy set`; (d) no completion for `unbootstrap`.
3. **main.go** — dispatch `case "completion": cmd.Completion(os.Args[2:])`; usage line (design D5).
4. **internal/cmd/completion_test.go**
   - `TestCompletionScripts_Anchors`: each const non-empty and carries its shell anchor + every command word.
   - `TestCompletionSpec_CoversRegisteredFlags`: build each command's FlagSet through its real `register`, `VisitAll`, assert spec coverage for command flags AND the five globals (from `registerGlobalFlags` on a globals FlagSet).
   - `TestCompletionSpec_CoversDispatch`: spec names == dispatch list minus `unbootstrap`.
   - `TestRunCompletion_Table`: known shells → script, unknown → error listing valid names.
5. **Docs** — README Completion section (install one-liners per shell); CHANGELOG Unreleased → Added; root usage already updated in step 3.
6. **scripts/integration-test.sh** — `run completion_bash completion bash`, `run completion_zsh completion zsh`, `run completion_fish completion fish`, `run completion_pwsh completion powershell`, `run completion_bad completion tcsh`, `run completion_kw completion help`, `run completion_bare completion`.

## Validation

- [ ] `go build ./...`
- [ ] `go vet ./...`
- [ ] `go test ./...`
- [ ] `bash -n <(go run . completion bash)` (Git Bash local sanity)
- [ ] Manual smoke (parent AC5): in Git Bash, `source <(go run . completion bash)` then `civ <Tab><Tab>` lists commands; `civ search --<Tab>` lists flags; `civ search --sort <Tab>` lists orders. PowerShell: `civ completion powershell | Out-String | iex` then complete.

## Review gates

- Gate A (after step 1): enum value lists diffed against the corresponding help-page flag docs (single source of truth for legal values = help pages; enums.go is API-mapping, not user docs).
- Gate B (after step 2): scripts reviewed in four real shells' syntax checkers where available (bash -n locally; zsh/fish/pwsh best-effort).
- Gate C (before commit): drift-guard tests fail when a flag is deliberately removed from the spec — mutate-then-fail check (run once, revert).

## Rollback point

Single commit; revert. Scripts are additive consts; no runtime behavior changes for other commands.

## Notes for the implementer

- Keep scripts dependency-free and POSIX-where-possible; bash script must work on Git Bash (Windows) — avoid GNU-only `compgen` flags beyond `-W`.
- PowerShell script targets Windows PowerShell 5.1+ syntax (no ternary/?? operators).
- Do not auto-edit shell profiles (out of scope per prd); docs only.
