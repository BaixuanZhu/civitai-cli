package cmd

import (
	"bytes"
	"maps"
	"slices"
	"strings"
	"testing"
)

// TestWriteCommandHelp_Rendering pins the shared renderer contract: the
// "civ <name> - <purpose>" title line, the command body preserved between
// title and globals, and the page ending with the global block verbatim —
// HasSuffix on globalHelpBlock proves the block is appended from the one
// constant, not copy-pasted into pages (prd R4).
func TestWriteCommandHelp_Rendering(t *testing.T) {
	const body = `search Civitai models

Usage:
  civ search <query> [flags]

Examples:
  civ search anima
`
	var buf bytes.Buffer
	writeCommandHelp(&buf, "search", body)
	got := buf.String()

	first, _, _ := strings.Cut(got, "\n")
	if want := "civ search - search Civitai models"; first != want {
		t.Errorf("title line = %q, want %q", first, want)
	}
	if !strings.Contains(got, "Usage:\n  civ search <query> [flags]") {
		t.Errorf("body sections not preserved:\n%s", got)
	}
	if !strings.HasSuffix(got, globalHelpBlock) {
		t.Errorf("page must end with the shared global block:\n%s", got)
	}
	// The --proxy row is the AC1 marker for the appended global section.
	if !strings.Contains(got, "--proxy <url>") {
		t.Errorf("appended global block must carry the --proxy row:\n%s", got)
	}
}

// TestIsBareHelp covers the literal-word escape (prd R6): only a single
// "help" positional triggers the page. Multi-word queries stay queries,
// and the flag form (-h) is owned by flag parsing, not this predicate.
func TestIsBareHelp(t *testing.T) {
	cases := []struct {
		name string
		pos  []string
		want bool
	}{
		{"bare help", []string{"help"}, true},
		{"multi-word query", []string{"help", "me"}, false},
		{"other positional", []string{"stocking"}, false},
		{"no positionals", nil, false},
		{"case-sensitive keyword", []string{"Help"}, false},
		{"flag form is not a positional", []string{"-h"}, false},
	}
	for _, tc := range cases {
		if got := isBareHelp(tc.pos); got != tc.want {
			t.Errorf("%s: isBareHelp(%v) = %v, want %v", tc.name, tc.pos, got, tc.want)
		}
	}
}

// TestCuratedHelpPages checks the five Wave A1 pages against the layout
// contract (design D3): the three sections in order, signature flag rows
// with real enum/default values, and at least one --json example. The
// cross-all-pages section-order test is Wave C — login/logout/proxy/skill
// belong to Wave B and are deliberately not referenced here.
func TestCuratedHelpPages(t *testing.T) {
	pages := []struct {
		name       string
		body       string
		signatures []string
	}{
		{"search", searchHelp, []string{
			"--type <list>", "--sort <order>", "most-downloaded", "recently-added",
			"--period <p>", "--limit <n>", "(default 20)",
		}},
		{"show", showHelp, []string{"-v <id>", "--ver <id>", "--prompts"}},
		{"vet", vetHelp, []string{"--prompts", "3 hash not registered"}},
		{"sync", syncHelp, []string{
			"--ext <list>", "--format <f>", "--concurrency <n>", "--rehash", "CIV_NO_CACHE",
		}},
		{"tags", tagsHelp, []string{"--limit <n>"}},
	}
	for _, p := range pages {
		for _, section := range []string{"Usage:", "Options:", "Examples:"} {
			if !strings.Contains(p.body, section) {
				t.Errorf("%s help: missing %q section", p.name, section)
			}
		}
		for _, sig := range p.signatures {
			if !strings.Contains(p.body, sig) {
				t.Errorf("%s help: missing signature %q", p.name, sig)
			}
		}
		u := strings.Index(p.body, "Usage:")
		o := strings.Index(p.body, "Options:")
		e := strings.Index(p.body, "Examples:")
		if !(u < o && o < e) {
			t.Errorf("%s help: section order Usage(%d) < Options(%d) < Examples(%d) violated", p.name, u, o, e)
		}
		if !hasJSONExample(p.body) {
			t.Errorf("%s help: no example line shows --json", p.name)
		}
		if strings.Contains(p.body, "Global flags") {
			t.Errorf("%s help: page body must not embed the global block (runner appends it)", p.name)
		}
	}
}

// hasJSONExample reports whether any example line (an indented "civ ..."
// command) carries --json, the machine-output escape every page shows.
func hasJSONExample(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "  civ ") && strings.Contains(line, "--json") {
			return true
		}
	}
	return false
}

// TestAllHelpPages_SectionOrder is the Wave C cross-all-pages audit the
// parallel wave tests deliberately skipped: every curated page, rendered
// through writeCommandHelp exactly as the runner prints it, must keep the
// uniform section order Usage: -> Options: (only for commands with own
// flags) -> Examples: -> the appended shared Global flags block (prd AC8),
// must document json availability (--json reaches the rendered page via
// the appended global block), and must not duplicate any global-flag row
// outside that block (prd R4: one shared constant, never copy-pasted).
// The tenth page, upgrade.Help(), takes the same audit in
// internal/upgrade/upgrade_test.go — cmd cannot import upgrade (design D4
// keeps the runner unaware of it), so the table stops at the nine
// runner-routed constants.
func TestAllHelpPages_SectionOrder(t *testing.T) {
	pages := []struct {
		name    string
		body    string
		options bool // command has own flags -> page carries an Options section
	}{
		{"search", searchHelp, true},
		{"show", showHelp, true},
		{"vet", vetHelp, true},
		{"sync", syncHelp, true},
		{"tags", tagsHelp, true},
		{"login", loginHelp, true},
		{"logout", logoutHelp, false},
		{"proxy", proxyHelp, false},
		{"skill", skillHelp, true},
	}
	for _, p := range pages {
		var buf bytes.Buffer
		writeCommandHelp(&buf, p.name, p.body)
		got := buf.String()

		u := strings.Index(got, "Usage:")
		e := strings.Index(got, "Examples:")
		g := strings.Index(got, "Global flags")
		if u < 0 || e < 0 || g < 0 {
			t.Errorf("%s: rendered page missing a section marker (Usage %d, Examples %d, Global flags %d)", p.name, u, e, g)
			continue
		}
		if p.options {
			if o := strings.Index(got, "Options:"); !(u < o && o < e) {
				t.Errorf("%s: section order Usage(%d) < Options(%d) < Examples(%d) violated", p.name, u, o, e)
			}
		} else if strings.Contains(got, "Options:") {
			t.Errorf("%s: no own flags, so the rendered page must omit Options:", p.name)
		}
		if e >= g {
			t.Errorf("%s: Examples(%d) must precede Global flags(%d)", p.name, e, g)
		}
		if !strings.Contains(got, "--json") {
			t.Errorf("%s: rendered page must document json availability (--json)", p.name)
		}
		for _, row := range globalHelpRows() {
			if n := strings.Count(got, row); n != 1 {
				t.Errorf("%s: global flag row %q appears %d times, want exactly 1 (no copy outside the appended block)", p.name, row, n)
			}
		}
	}
}

// globalHelpRows returns the flag-row lines of the shared global block.
// Each row must appear exactly once in a rendered page: the appended block
// contributes the one legal copy, so any second occurrence means a command
// body embedded a drifting duplicate (prd R4).
func globalHelpRows() []string {
	var rows []string
	for _, line := range strings.Split(strings.TrimRight(globalHelpBlock, "\n"), "\n") {
		if line != "" && !strings.HasPrefix(line, "Global flags") {
			rows = append(rows, line)
		}
	}
	return rows
}

// TestWriteHelpPage_CoversNineCommands is the AC1 contract for the exported
// accessor: every page indexed by helpPages renders through WriteHelpPage
// with the "civ <name> -" title line and the shared global block appended,
// while an unknown name writes nothing and returns false so main can reuse
// the unknown-command error path. "upgrade" is asserted to miss on purpose
// — cmd cannot import internal/upgrade (design D4), so main owns that page
// and the lookup must not swallow it.
func TestWriteHelpPage_CoversNineCommands(t *testing.T) {
	for name := range helpPages {
		var buf bytes.Buffer
		if !WriteHelpPage(&buf, name) {
			t.Errorf("%s: WriteHelpPage returned false for an indexed page", name)
			continue
		}
		got := buf.String()
		if !strings.HasPrefix(got, "civ "+name+" - ") {
			t.Errorf("%s: rendered page must start with the title line, got:\n%s", name, got)
		}
		if !strings.HasSuffix(got, globalHelpBlock) {
			t.Errorf("%s: page must end with the appended shared global block:\n%s", name, got)
		}
	}

	for _, name := range []string{"nope", "", "Search", "upgrade"} {
		var buf bytes.Buffer
		if WriteHelpPage(&buf, name) {
			t.Errorf("%s: WriteHelpPage returned true, want false", name)
		}
		if buf.Len() != 0 {
			t.Errorf("%s: WriteHelpPage wrote %d bytes on a miss, want nothing:\n%s", name, buf.Len(), buf.String())
		}
	}
}

// TestHelpPages_MapMatchesExpectedCommands pins the D1 single-source
// property: the lookup index must cover exactly the nine cmd-owned pages —
// no less (a page added without joining the map would be unreachable via
// `civ help <name>`) and no more (upgrade stays out; main special-cases it).
func TestHelpPages_MapMatchesExpectedCommands(t *testing.T) {
	want := []string{"login", "logout", "proxy", "search", "show", "skill", "sync", "tags", "vet"}
	got := slices.Sorted(maps.Keys(helpPages))
	if !slices.Equal(got, want) {
		t.Errorf("helpPages keys = %v, want exactly %v", got, want)
	}
}
