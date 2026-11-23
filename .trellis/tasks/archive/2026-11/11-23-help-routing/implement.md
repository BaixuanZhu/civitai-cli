# Implement: help routing (child 11-23-help-routing)

> **Wave scope override (parent prd constraint):** touch ONLY `internal/cmd/help.go` and `internal/cmd/help_test.go`. Do NOT touch `main.go` (routing + usage text), `scripts/`, `CHANGELOG.md`, `README.md`, or any other `internal/cmd/*.go` — the parent integration pass owns shared files. Do NOT git commit. Other children's changes may be dirty in the tree — ignore errors in files you don't own.

Execution order and validation. Independent of the upgrade children; can land in any order relative to them.

## Checklist

1. **internal/cmd/help.go**
   - Add the unexported `helpPages` map (nine entries: search/show/vet/sync/tags/login/logout/proxy/skill → the existing consts).
   - Add exported `WriteHelpPage(w io.Writer, name string) bool` with GoDoc: renders the named command's curated page (title + body + shared global block) on hit; writes nothing and returns false on miss (design D1).
2. **internal/cmd/help_test.go**
   - `TestWriteHelpPage_CoversNineCommands`: every map key renders, output starts `civ <name> - ` and contains the global block; unknown name returns false with empty buffer.
   - `TestHelpPages_MapMatchesExpectedCommands`: map keys == exactly the nine names (guards the D1 single-source property).
3. **main.go**
   - `case "help", "-h", "--help":` branch (design D2): exactly one arg after `help` → lookup; hit → stdout, return (exit 0); `upgrade` → `fmt.Print(upgrade.Help())`, return; miss → `fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", arg)`, `usage(os.Stderr)`, `os.Exit(1)`. Zero/≥2 args after `help` → existing overview behavior (for `help`; for `-h`/`--help` forms keep overview too, D3).
   - usage() text: `Run "civ <command> help" (or civ <command> -h) for flags and examples.` → mention `civ help <command>` as an equivalent form (one line, D4).
4. **scripts/integration-test.sh**
   - Add: `run help_search_named help search`, `run help_upgrade_named help upgrade`, `run help_unknown help nope` (exit 1), `run help_extra help search extra` (exit 1 per D2 strictness).

## Validation

- [ ] `go build ./...`
- [ ] `go vet ./...`
- [ ] `go test ./...`
- [ ] Manual smoke: `go run . help search` ≡ `go run . search help` (diff the two outputs); `go run . help nope` exit 1; `go run . help` unchanged.

## Review gates

- Gate A (after step 2): the two new tests are the contract — review expected-keys list against main's dispatch (nine + upgrade handled by main).
- Gate B (before commit): `civ search help` output byte-equality with `civ help search` (manual diff in smoke); grep confirms no page const moved or duplicated.

## Rollback point

Single commit; revert restores the old `case "help"` body.

## Notes for the implementer

- Do not add `upgrade` to the cmd map — cmd cannot import upgrade (design D4); main owns that branch.
- Keep `-h`/`--help` behavior untouched (overview): only the literal `help` keyword takes a target word (D3), and say so in usage text.
