package cmd

import (
	"strings"
	"testing"
)

// TestLocalHelpPages_Content pins the curated help pages owned in this
// file set (login, logout, proxy, skill) to their signature facts: every
// page carries Usage: and Examples:, login spells out the key precedence
// and the stdin forms, proxy the accepted schemes, skill its flags and
// actions. Pure constant checks — no process spawning, no network.
func TestLocalHelpPages_Content(t *testing.T) {
	cases := []struct {
		name string
		page string
		want []string
	}{
		{"login", loginHelp, []string{
			"Usage:", "Options:", "Examples:",
			"--key", "-t", "--token", "stdin",
			`echo "$KEY" | civ login --key -`,
			"Exit codes:",
		}},
		{"logout", logoutHelp, []string{
			"Usage:", "Examples:",
			"civ logout", "proxy",
		}},
		{"proxy", proxyHelp, []string{
			"Usage:", "Examples:",
			"set", "get", "unset", "socks5", "socks5h", "HTTP_PROXY",
		}},
		{"skill", skillHelp, []string{
			"Usage:", "Options:", "Examples:",
			"--platform", "--scope", "--force",
			"install", "remove", "update",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, w := range tc.want {
				if !strings.Contains(tc.page, w) {
					t.Errorf("%sHelp must mention %q", tc.name, w)
				}
			}
			// The global flags block is appended by writeCommandHelp from
			// the single shared constant; a copy inside a page would drift.
			if strings.Contains(tc.page, "Global flags") {
				t.Errorf("%sHelp must not embed the global flags block", tc.name)
			}
		})
	}
}

// TestLocalHelpPages_SectionOrder keeps the layout contract (design D3 /
// prd AC8) on the pages owned in this file set: Usage before Options (only
// for commands with own flags) before Examples. logout and proxy take no
// flags of their own, so their pages must omit the Options section.
func TestLocalHelpPages_SectionOrder(t *testing.T) {
	pages := []struct {
		name    string
		page    string
		options bool
	}{
		{"login", loginHelp, true},
		{"logout", logoutHelp, false},
		{"proxy", proxyHelp, false},
		{"skill", skillHelp, true},
	}
	for _, p := range pages {
		u := strings.Index(p.page, "Usage:")
		e := strings.Index(p.page, "Examples:")
		if u < 0 || e < 0 || u >= e {
			t.Errorf("%sHelp: Usage must precede Examples", p.name)
			continue
		}
		if p.options {
			if o := strings.Index(p.page, "Options:"); o < u || o >= e {
				t.Errorf("%sHelp: Options must sit between Usage and Examples", p.name)
			}
		} else if strings.Contains(p.page, "Options:") {
			t.Errorf("%sHelp: no own flags, so the Options section must be omitted", p.name)
		}
	}
}
