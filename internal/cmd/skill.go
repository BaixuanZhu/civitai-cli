package cmd

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"civitai-cli/internal/app"
	"civitai-cli/internal/console"
	"civitai-cli/internal/report"
	"civitai-cli/internal/skill"
)

// skillHelp is the curated `civ skill help` / `-h` page (without the
// shared global block, which writeCommandHelp appends): the actions and
// the platform/scope/force flags.
const skillHelp = `install, remove, or update the bundled civ-prompt skill

Usage:
  civ skill <install|remove|update> [flags]

Options:
  --platform <list>  Target platforms: claude, zcode, codex (comma-separated)
                     or all; omit for interactive selection
  --scope <name>     Install scope: user (home directory) or project
                     (current directory) (default user)
  --force            Overwrite an existing skill without prompting

Examples:
  civ skill install
  civ skill install --platform zcode --scope project
  civ skill install --platform zcode --json
  civ skill install --platform all --force
  civ skill update --platform claude,zcode
`

// Skill runs the skill command: civ skill <install|remove|update> [flags]. It
// installs, removes, or updates the bundled civ-prompt Agent Skill across the
// claude / zcode / codex platforms. With --platform it runs non-interactively;
// without it, it asks interactively — arrow-key menus on a terminal (detected
// platforms pre-toggled), text prompts on stderr otherwise — and runs the
// action once the choices are made (no confirmation step). Output is a human
// summary or --json []Result. This is a purely local file operation (no
// network). Exit codes: 0 success, 1 usage error (unknown action, invalid
// platform/scope, or interactive cancel).
func Skill(args []string) {
	var o skillOptions
	execute("skill", skillHelp, args, o.register, func(ctx context.Context, g *globals, pos []string) error {
		return runSkill(ctx, g, pos, &o, os.Stdin)
	})
}

// skillOptions carries the skill-specific flags.
type skillOptions struct {
	platform string
	scope    string
	force    bool
}

// register binds the skill flags to fs.
func (o *skillOptions) register(fs *flag.FlagSet) {
	fs.StringVar(&o.platform, "platform", "", "target platforms: claude,zcode,codex or all (omit for interactive selection)")
	fs.StringVar(&o.scope, "scope", "user", "install scope: user (global home) or project (current directory)")
	fs.BoolVar(&o.force, "force", false, "overwrite an existing skill without prompting")
}

// skillPlatformMenu is the fixed display order for interactive selection.
var skillPlatformMenu = []skill.Platform{skill.PlatformClaude, skill.PlatformZcode, skill.PlatformCodex}

// Interactive-menu seams (function values, the newAPIClient pattern): the
// console-backed menu operations and the console gate are package vars so
// tests can drive the arrow-key path with fakes instead of a terminal.
var (
	skillMenuSelectMulti = console.SelectMulti
	skillMenuSelectOne   = console.SelectOne
	skillStdinIsConsole  = defaultSkillStdinIsConsole
)

// errSkillMenuCanceled marks an explicit arrow-key menu cancel (q, Esc, or
// Ctrl+C); skillChooseInteractive maps it to the shared "skill: canceled by
// user" error so the exit code matches the text flow (prd R3).
var errSkillMenuCanceled = errors.New("menu canceled")

// runSkill is the pure skill implementation with an injectable stdin so tests
// can drive the interactive flow with a strings.Reader (no terminal). It
// validates the action and scope, resolves platforms (flag or interactive),
// dispatches to internal/skill, and renders the results.
func runSkill(ctx context.Context, g *globals, pos []string, o *skillOptions, stdin io.Reader) error {
	if len(pos) != 1 {
		return errors.New("skill requires one action: install, remove, or update")
	}
	action := pos[0]
	switch action {
	case "install", "remove", "update":
	default:
		return fmt.Errorf("unknown skill action %q; want install, remove, or update", action)
	}

	scope := skill.Scope(strings.ToLower(strings.TrimSpace(o.scope)))
	switch scope {
	case skill.ScopeUser, skill.ScopeProject:
	default:
		return fmt.Errorf("unknown --scope %q; want user or project", o.scope)
	}

	var platforms []skill.Platform
	var conflict skill.ConflictFunc
	if strings.TrimSpace(o.platform) == "" {
		var err error
		platforms, scope, conflict, err = skillChooseInteractive(action, scope, stdin)
		if err != nil {
			return err
		}
	} else {
		var err error
		platforms, err = parsePlatforms(o.platform)
		if err != nil {
			return err
		}
	}

	// The codex advisory concerns writing to the shared .agents/skills root,
	// so it only applies to install/update, not remove.
	if action == "install" || action == "update" {
		warnUndetectedCodex(platforms)
	}

	opts := skill.Options{
		Platforms: platforms,
		Scope:     scope,
		Force:     o.force,
		Out:       os.Stderr,
		Conflict:  conflict,
	}
	var results []skill.Result
	var err error
	switch action {
	case "install":
		results, err = skill.Install(opts)
	case "remove":
		results, err = skill.Remove(opts)
	case "update":
		results, err = skill.Update(opts)
	}
	if err != nil {
		return err
	}
	return renderSkillResults(g, action, results)
}

// skillChooseInteractive resolves platforms, scope, and the install conflict
// callback interactively. On a real console it drives arrow-key menus; every
// other stdin — and a console that fails raw-mode setup or errors mid-menu —
// runs the text prompt flow on the same stdin instead (prd R4: the fallback
// is the safety net for pipes, CI, injected readers, and exotic terminals).
// A menu cancel returns the existing "skill: canceled by user" error (prd R3).
func skillChooseInteractive(action string, defaultScope skill.Scope, stdin io.Reader) ([]skill.Platform, skill.Scope, skill.ConflictFunc, error) {
	if skillStdinIsConsole(stdin) {
		platforms, scope, conflict, err := skillMenuSelect(action, defaultScope)
		switch {
		case err == nil:
			return platforms, scope, conflict, nil
		case errors.Is(err, errSkillMenuCanceled):
			return nil, "", nil, errors.New("skill: canceled by user")
		}
		// Any other console error falls through to the text prompts below.
	}
	p := &skillPrompter{sc: bufio.NewScanner(stdin), out: os.Stderr}
	platforms, scope, err := p.selectPlatformsAndScope(action, defaultScope)
	if err != nil {
		return nil, "", nil, err
	}
	var conflict skill.ConflictFunc
	if action == "install" {
		conflict = p.conflictFunc()
	}
	return platforms, scope, conflict, nil
}

// defaultSkillStdinIsConsole reports whether the interactive stdin is the
// process's real console. Menus need raw mode on the actual handle, so only
// an *os.File that passes console.IsTerminal qualifies; pipes, redirected
// files, and test-injected readers take the text prompt path.
func defaultSkillStdinIsConsole(stdin io.Reader) bool {
	f, ok := stdin.(*os.File)
	return ok && console.IsTerminal(f.Fd())
}

// skillMenuSelect drives the arrow-key menus for the interactive flow: the
// platform multi-select (detected platforms pre-toggled, cursor on the
// first), the scope single-select (default highlighted), then a one-time
// record of the resolved target paths (prd R2: the choices themselves are
// the confirmation — there is no extra Proceed step). Install conflicts
// resolve through a menu-backed ConflictFunc whose cancel maps to skip.
func skillMenuSelect(action string, defaultScope skill.Scope) ([]skill.Platform, skill.Scope, skill.ConflictFunc, error) {
	detected := skill.DetectPlatforms()
	detectedSet := make(map[skill.Platform]bool)
	for _, d := range detected {
		detectedSet[d] = true
	}
	rows := make([]string, len(skillPlatformMenu))
	preselect := make([]int, 0, len(detected))
	for i, pl := range skillPlatformMenu {
		root, err := skill.RootFor(pl, defaultScope)
		if err != nil {
			return nil, "", nil, err
		}
		rows[i] = fmt.Sprintf("%-7s %s", pl, root)
		if detectedSet[pl] {
			rows[i] += "  [detected]"
			preselect = append(preselect, i)
		}
	}
	sel, err := skillMenuSelectMulti(fmt.Sprintf("Select platforms for civ skill %s:", action), rows, preselect)
	if err != nil {
		return nil, "", nil, err
	}
	if sel.Canceled {
		return nil, "", nil, errSkillMenuCanceled
	}
	platforms := make([]skill.Platform, 0, len(sel.Indexes))
	for _, i := range sel.Indexes {
		platforms = append(platforms, skillPlatformMenu[i])
	}

	defaultIdx := 0
	if defaultScope == skill.ScopeProject {
		defaultIdx = 1
	}
	scopeSel, err := skillMenuSelectOne(fmt.Sprintf("Scope (default %s):", defaultScope),
		[]string{"user (home directory)", "project (current directory)"}, defaultIdx)
	if err != nil {
		return nil, "", nil, err
	}
	if scopeSel.Canceled {
		return nil, "", nil, errSkillMenuCanceled
	}
	scope := skill.ScopeUser
	if len(scopeSel.Indexes) > 0 && scopeSel.Indexes[0] == 1 {
		scope = skill.ScopeProject
	}

	fmt.Fprintln(os.Stderr, "Target paths:")
	for _, pl := range platforms {
		root, err := skill.RootFor(pl, scope)
		if err != nil {
			return nil, "", nil, err
		}
		fmt.Fprintf(os.Stderr, "  %s: %s\n", pl, filepath.Join(root, skill.SkillName))
	}

	var conflict skill.ConflictFunc
	if action == "install" {
		conflict = skillMenuConflictFunc()
	}
	return platforms, scope, conflict, nil
}

// skillMenuConflictFunc returns the menu-backed install conflict decision: a
// single-select overwrite/skip/backup menu with the cursor on the safe
// default skip (mirroring the text prompt). Cancel and any menu error
// resolve to skip — a broken menu must not abort the install (prd R3).
func skillMenuConflictFunc() skill.ConflictFunc {
	return func(path, installed string) string {
		v := installed
		if v == "" {
			v = "unknown"
		}
		sel, err := skillMenuSelectOne(
			fmt.Sprintf("civ-prompt already exists at %s (version %s):", path, v),
			[]string{"overwrite", "skip (keep the installed copy)", "backup (rename to .bak, then install)"},
			1)
		if err != nil || sel.Canceled || len(sel.Indexes) == 0 {
			return "skip"
		}
		switch sel.Indexes[0] {
		case 0:
			return "overwrite"
		case 2:
			return "backup"
		default:
			return "skip"
		}
	}
}

// parsePlatforms canonicalizes the comma-separated --platform value ("all"
// expands to every platform). Unknown names are usage errors; duplicates are
// collapsed preserving first-seen order.
func parsePlatforms(s string) ([]skill.Platform, error) {
	s = strings.TrimSpace(s)
	if strings.EqualFold(s, "all") {
		return append([]skill.Platform(nil), skillPlatformMenu...), nil
	}
	var out []skill.Platform
	seen := make(map[skill.Platform]bool)
	for _, part := range strings.Split(s, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		p := skill.Platform(part)
		switch p {
		case skill.PlatformClaude, skill.PlatformZcode, skill.PlatformCodex:
		default:
			return nil, fmt.Errorf("unknown --platform %q; want claude, zcode, codex, or all", part)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("--platform must list at least one of claude, zcode, codex (or all)")
	}
	return out, nil
}

// warnUndetectedCodex notes when codex was requested but not detected: the skill
// is still written to the shared ~/.agents/skills root and becomes effective once
// codex is installed (prd R12). The warning never blocks the write.
func warnUndetectedCodex(platforms []skill.Platform) {
	selected := false
	for _, p := range platforms {
		if p == skill.PlatformCodex {
			selected = true
		}
	}
	if !selected {
		return
	}
	for _, p := range skill.DetectPlatforms() {
		if p == skill.PlatformCodex {
			return // detected, no warning
		}
	}
	app.Warnf("codex not detected; civ-prompt is written to the shared .agents/skills root and takes effect once codex is installed")
}

// renderSkillResults writes the per-platform outcomes: a --json array, or a
// human summary line (action, platform, path) per result. An update that found
// no installed skill adds a stderr hint to install first.
func renderSkillResults(g *globals, action string, results []skill.Result) error {
	if g.JSON {
		return report.WriteIndent(g.stdout(), results)
	}
	for _, r := range results {
		fmt.Fprintf(g.stdout(), "%-10s %-7s %s\n", r.Action, r.Platform, r.Path)
		if action == "update" && r.Action == skill.ActionNotFound {
			fmt.Fprintf(os.Stderr, "hint: no civ-prompt installed for %s; run civ skill install first\n", r.Platform)
		}
	}
	return nil
}

// skillPrompter drives the interactive flow over a single stdin scanner, writing
// prompts to out (stderr). It mirrors login's stdin-parameterized pattern so the
// whole flow is testable with a strings.Reader.
type skillPrompter struct {
	sc  *bufio.Scanner
	out io.Writer
}

// ask prints a prompt and reads one trimmed line. EOF or a read error cancels
// the interaction (mapped to exit 1 by the runner).
func (p *skillPrompter) ask(prompt string) (string, error) {
	fmt.Fprint(p.out, prompt)
	if !p.sc.Scan() {
		if err := p.sc.Err(); err != nil {
			return "", fmt.Errorf("skill: read stdin: %w", err)
		}
		return "", errors.New("skill: interactive selection canceled (no input)")
	}
	return strings.TrimSpace(p.sc.Text()), nil
}

// selectPlatformsAndScope runs the text-fallback interactive flow (non-TTY
// stdin): the platform menu (detected platforms preselected), the scope
// choice (default user), and a one-time record of the target paths. There is
// no confirmation step (prd R2) — the choices themselves commit the run. EOF
// or a read error cancels the interaction (mapped to exit 1 by the runner).
func (p *skillPrompter) selectPlatformsAndScope(action string, defaultScope skill.Scope) ([]skill.Platform, skill.Scope, error) {
	detected := skill.DetectPlatforms()
	detectedSet := make(map[skill.Platform]bool)
	for _, d := range detected {
		detectedSet[d] = true
	}

	fmt.Fprintf(p.out, "Select platforms for civ skill %s:\n", action)
	for i, pl := range skillPlatformMenu {
		mark := ""
		if detectedSet[pl] {
			mark = "  [detected]"
		}
		root, _ := skill.RootFor(pl, defaultScope)
		fmt.Fprintf(p.out, "  %d) %-7s %s%s\n", i+1, pl, root, mark)
	}
	ans, err := p.ask("Platforms (numbers/names, 'all', or Enter for detected): ")
	if err != nil {
		return nil, "", err
	}
	platforms, err := parseInteractivePlatforms(ans, detected)
	if err != nil {
		return nil, "", err
	}

	scopeAns, err := p.ask(fmt.Sprintf("Scope [user/project] (default %s): ", defaultScope))
	if err != nil {
		return nil, "", err
	}
	scope := defaultScope
	if scopeAns != "" {
		switch skill.Scope(strings.ToLower(scopeAns)) {
		case skill.ScopeUser:
			scope = skill.ScopeUser
		case skill.ScopeProject:
			scope = skill.ScopeProject
		default:
			return nil, "", fmt.Errorf("unknown scope %q; want user or project", scopeAns)
		}
	}

	fmt.Fprintln(p.out, "Target paths:")
	for _, pl := range platforms {
		root, err := skill.RootFor(pl, scope)
		if err != nil {
			return nil, "", err
		}
		fmt.Fprintf(p.out, "  %s: %s\n", pl, filepath.Join(root, skill.SkillName))
	}
	return platforms, scope, nil
}

// conflictFunc returns the install conflict decision callback: it prompts
// overwrite/skip/backup for an existing skill (default skip). A read failure
// mid-conflict defaults to the safe skip rather than aborting the whole run.
func (p *skillPrompter) conflictFunc() skill.ConflictFunc {
	return func(path, installed string) string {
		v := installed
		if v == "" {
			v = "unknown"
		}
		fmt.Fprintf(p.out, "civ-prompt already exists at %s (version %s).\n", path, v)
		ans, err := p.ask("Overwrite, skip, or backup? [overwrite/skip/backup] (default skip): ")
		if err != nil {
			return "skip"
		}
		switch strings.ToLower(ans) {
		case "overwrite", "o":
			return "overwrite"
		case "backup", "b":
			return "backup"
		default:
			return "skip"
		}
	}
}

// parseInteractivePlatforms interprets the platform menu answer: empty accepts
// the detected preselection, "all" selects every platform, and a comma/space
// list mixes 1-based menu numbers with platform names. An empty answer with
// nothing detected is an error (the user must choose explicitly).
func parseInteractivePlatforms(ans string, detected []skill.Platform) ([]skill.Platform, error) {
	ans = strings.TrimSpace(ans)
	if ans == "" {
		if len(detected) == 0 {
			return nil, errors.New("no platforms detected; enter numbers/names or 'all' (or use --platform)")
		}
		return detected, nil
	}
	if strings.EqualFold(ans, "all") {
		return append([]skill.Platform(nil), skillPlatformMenu...), nil
	}
	var out []skill.Platform
	seen := make(map[skill.Platform]bool)
	tokens := strings.FieldsFunc(ans, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
	for _, tok := range tokens {
		tok = strings.ToLower(strings.TrimSpace(tok))
		if tok == "" {
			continue
		}
		var pl skill.Platform
		if n, err := strconv.Atoi(tok); err == nil {
			if n < 1 || n > len(skillPlatformMenu) {
				return nil, fmt.Errorf("invalid platform number %q; want 1-%d", tok, len(skillPlatformMenu))
			}
			pl = skillPlatformMenu[n-1]
		} else {
			pl = skill.Platform(tok)
			switch pl {
			case skill.PlatformClaude, skill.PlatformZcode, skill.PlatformCodex:
			default:
				return nil, fmt.Errorf("unknown platform %q; want claude, zcode, codex, a number 1-%d, or all", tok, len(skillPlatformMenu))
			}
		}
		if !seen[pl] {
			seen[pl] = true
			out = append(out, pl)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no platforms selected")
	}
	return out, nil
}
