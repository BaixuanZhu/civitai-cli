package cmd

import (
	"context"
	"fmt"
	"strings"
)

// completionHelp is the curated `civ completion help` / `-h` page (without
// the shared global block, which writeCommandHelp appends): the four shell
// names and the per-shell install one-liners (prd R6).
const completionHelp = `print a shell completion script (bash, zsh, fish, powershell)

Usage:
  civ completion <shell> [flags]

Options:
  <shell>          One of: bash, zsh, fish, powershell. The matching static
                   completion script prints to stdout.
  --json           Accepted after the command name as usual; the script is
                   plain text, so --json never changes this command's output

Examples:
  civ completion bash
  source <(civ completion bash)
  civ completion zsh > "${fpath[1]}/_civ"
  civ completion fish | source
  civ completion powershell | Out-String | Invoke-Expression
  civ completion bash --json

Notes:
  The scripts are static: they complete command names, per-command flags
  (including the global flags after the command name), and the enum values
  of enumerable flags. To persist the PowerShell completion, add the
  Invoke-Expression line above to $PROFILE. To persist fish, save to
  ~/.config/fish/completions/civ.fish instead of piping to source.
`

// completionShells is the display order of the supported shells.
// runCompletion accepts exactly these names, the usage error lists them in
// this order, and the completion scripts offer them as values.
var completionShells = []string{"bash", "zsh", "fish", "powershell"}

// completionScripts maps each supported shell to its static script. The
// scripts are hand-written consts (completion_scripts.go): shipping static
// text keeps shell startup cost zero and needs no `civ` subprocess at
// completion time (prd R3).
var completionScripts = map[string]string{
	"bash":       bashCompletionScript,
	"zsh":        zshCompletionScript,
	"fish":       fishCompletionScript,
	"powershell": powershellCompletionScript,
}

// completionTypeValues is the enum value list for search's repeatable
// --type flag: every CLI-facing name accepted by api.MapType, with the
// short aliases (ti, hypernet) listed next to their long forms. --base is
// deliberately absent: unregistered values only warn, so no fixed value
// list exists (api.CanonicalBaseModel is a whitelist, not user docs).
var completionTypeValues = []string{
	"checkpoint", "lora", "locon", "ti", "textualinversion",
	"hypernet", "hypernetwork", "dora", "controlnet", "upscaler",
	"motionmodule", "vae", "textencoder", "unet", "clipvision",
	"poses", "wildcards", "workflows", "comfyworkflows", "detection",
	"visionlanguage", "clip", "llm", "aestheticgradient", "other",
}

// completionFlagSpec is one completable flag: the registered flag name
// (without dashes) plus, for enumerable flags, the value candidates.
// values empty means the flag takes a free-form value (--base, --tag,
// --user, --token, URLs, ...) and gets no value completion (prd R2).
type completionFlagSpec struct {
	name   string
	values []string
}

// completionCommandSpec is one user-facing command word with its flags.
type completionCommandSpec struct {
	name  string
	flags []completionFlagSpec
}

// completionSpec is the single source of truth the four static scripts are
// written against. It drives nothing at runtime — the scripts are static —
// but the drift-guard tests compare it against each command's real
// register function and main's dispatch list, so a flag or command added
// without updating the spec (and the scripts) fails the tests (prd R5).
// Command order mirrors main's dispatch switch; unbootstrap is internal
// and never appears (design D7).
var completionSpec = []completionCommandSpec{
	{"search", []completionFlagSpec{
		{"type", completionTypeValues},
		{"base", nil},
		{"sort", []string{"hot", "highest-rated", "most-downloaded", "most-liked", "most-discussed", "most-collected", "most-images", "newest", "oldest", "recently-added"}},
		{"period", []string{"day", "week", "month", "year", "all"}},
		{"tag", nil},
		{"user", nil},
		{"limit", nil},
	}},
	{"show", []completionFlagSpec{
		{"v", nil},
		{"ver", nil},
		{"prompts", nil},
	}},
	{"vet", []completionFlagSpec{
		{"prompts", nil},
	}},
	{"sync", []completionFlagSpec{
		{"ext", nil},
		{"format", []string{"table", "csv", "json"}},
		{"concurrency", nil},
		{"rehash", nil},
	}},
	{"tags", []completionFlagSpec{
		{"limit", nil},
	}},
	{"login", []completionFlagSpec{
		{"key", nil},
		{"t", nil},
	}},
	{"logout", nil},
	{"proxy", []completionFlagSpec{
		{"github", nil},
	}},
	{"skill", []completionFlagSpec{
		{"platform", []string{"claude", "zcode", "codex", "all"}},
		{"scope", []string{"user", "project"}},
		{"force", nil},
	}},
	{"upgrade", nil},
	{"completion", nil},
	{"version", nil},
	{"help", nil},
}

// completionGlobalFlags lists the five global flags once. Every script
// offers them after each command word — the only position the tool accepts
// them in (main rejects flags before the command name, design D1).
var completionGlobalFlags = []completionFlagSpec{
	{"json", nil},
	{"token", nil},
	{"proxy", nil},
	{"timeout", nil},
	{"verbose", nil},
}

// Completion runs the completion command: civ completion <shell>. It prints
// the static completion script of one of bash, zsh, fish, or powershell to
// stdout (exit 0); bare `civ completion` prints this command's help page
// (exit 0, the friendly default matching civ proxy); an unknown shell name
// or extra arguments is a usage error naming the four valid shells (exit 1
// through the runner's normal mapping). The scripts are static consts — no
// network, no subprocess.
func Completion(args []string) {
	execute("completion", completionHelp, args, nil, runCompletion)
}

// runCompletion is the pure completion implementation (testable without
// os.Exit). Bare invocation prints the curated help page through the same
// renderer the runner uses; a known shell prints its script; anything else
// returns a usage error listing the valid shells.
func runCompletion(ctx context.Context, g *globals, pos []string) error {
	if len(pos) == 0 {
		writeCommandHelp(g.stdout(), "completion", completionHelp)
		return nil
	}
	if len(pos) != 1 {
		return fmt.Errorf("completion takes exactly one shell (%s); got %d arguments", strings.Join(completionShells, ", "), len(pos))
	}
	script, ok := completionScripts[pos[0]]
	if !ok {
		return fmt.Errorf("unknown shell %q; valid shells: %s", pos[0], strings.Join(completionShells, ", "))
	}
	_, err := fmt.Fprint(g.stdout(), script)
	return err
}
