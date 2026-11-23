package cmd

import (
	"bytes"
	"context"
	"flag"
	"strings"
	"testing"
)

// completionCommandWords is the user-facing dispatch list: every command in
// main's switch minus the internal unbootstrap maintenance hook (design D7).
var completionCommandWords = []string{
	"search", "show", "vet", "sync", "tags", "login", "logout", "proxy",
	"skill", "upgrade", "completion", "version", "help",
}

// completionRegisterFuncs maps each command that owns flags today to a
// builder of its real FlagSet through its real register closure — the
// drift-guard input (prd R5). Commands absent here (logout, proxy,
// upgrade, completion, version, help) must carry no flags in the spec.
var completionRegisterFuncs = map[string]func(*flag.FlagSet){
	"search": func(fs *flag.FlagSet) { var o searchOptions; o.register(fs) },
	"show":   func(fs *flag.FlagSet) { var o showOptions; o.register(fs) },
	"vet":    func(fs *flag.FlagSet) { var o vetOptions; o.register(fs) },
	"sync":   func(fs *flag.FlagSet) { var o syncOptions; o.register(fs) },
	"tags":   func(fs *flag.FlagSet) { var o tagsOptions; o.register(fs) },
	"login":  func(fs *flag.FlagSet) { var o loginOptions; o.register(fs) },
	"skill":  func(fs *flag.FlagSet) { var o skillOptions; o.register(fs) },
}

// completionSpecFlags returns the spec's flag set for name, failing the
// test when the command is missing.
func completionSpecFlags(t *testing.T, name string) map[string]completionFlagSpec {
	t.Helper()
	for _, c := range completionSpec {
		if c.name == name {
			m := make(map[string]completionFlagSpec, len(c.flags))
			for _, f := range c.flags {
				m[f.name] = f
			}
			return m
		}
	}
	t.Fatalf("completionSpec has no entry for command %q", name)
	return nil
}

// TestCompletionScripts_Anchors pins the script consts: each is non-empty,
// carries its shell's anchor, contains every user-facing command word, and
// never mentions the internal unbootstrap hook (prd AC1, design D6/D7).
// It also pins the one positional enum the spec table cannot express: the
// proxy set schemes (prd R2).
func TestCompletionScripts_Anchors(t *testing.T) {
	scripts := []struct {
		shell  string
		script string
		anchor string
	}{
		{"bash", bashCompletionScript, "complete -F _civ civ"},
		{"zsh", zshCompletionScript, "#compdef civ"},
		{"fish", fishCompletionScript, "complete -c civ"},
		{"powershell", powershellCompletionScript, "Register-ArgumentCompleter"},
	}
	for _, s := range scripts {
		if s.script == "" {
			t.Errorf("%s: script const is empty", s.shell)
			continue
		}
		if !strings.Contains(s.script, s.anchor) {
			t.Errorf("%s: script missing anchor %q", s.shell, s.anchor)
		}
		for _, word := range completionCommandWords {
			if !strings.Contains(s.script, word) {
				t.Errorf("%s: script missing command word %q", s.shell, word)
			}
		}
		if strings.Contains(s.script, "unbootstrap") {
			t.Errorf("%s: internal command unbootstrap must never complete", s.shell)
		}
		for _, scheme := range []string{"http://", "https://", "socks5://", "socks5h://"} {
			if !strings.Contains(s.script, scheme) {
				t.Errorf("%s: script missing proxy scheme %q", s.shell, scheme)
			}
		}
	}
}

// TestCompletionSpec_CoversRegisteredFlags is the drift guard (prd AC3,
// design D6): for every command with a register func, build its real
// FlagSet and assert the spec covers every registered flag name, and the
// spec invents none. The five global flags are checked against the runner's
// registerGlobalFlags on a globals FlagSet the same way.
func TestCompletionSpec_CoversRegisteredFlags(t *testing.T) {
	for name, register := range completionRegisterFuncs {
		fs := flag.NewFlagSet(name, flag.ContinueOnError)
		register(fs)
		registered := map[string]bool{}
		fs.VisitAll(func(f *flag.Flag) { registered[f.Name] = true })

		spec := completionSpecFlags(t, name)
		for flagName := range registered {
			if _, ok := spec[flagName]; !ok {
				t.Errorf("drift: command %q registers --%s but completionSpec does not list it (update the spec and all four scripts)", name, flagName)
			}
		}
		for flagName := range spec {
			if !registered[flagName] {
				t.Errorf("drift: completionSpec lists --%s for command %q but its register func does not define it", flagName, name)
			}
		}
	}

	// Commands without a register func own no flags: the spec must not
	// invent any for them.
	for _, c := range completionSpec {
		if _, hasRegister := completionRegisterFuncs[c.name]; hasRegister {
			continue
		}
		if len(c.flags) != 0 {
			t.Errorf("drift: command %q has no register func but completionSpec lists flags %v", c.name, c.flags)
		}
	}

	// The five global flags, enumerated from the runner's own registration.
	fs := flag.NewFlagSet("globals", flag.ContinueOnError)
	var g globals
	registerGlobalFlags(fs, &g)
	globalNames := map[string]bool{}
	for _, f := range completionGlobalFlags {
		globalNames[f.name] = true
	}
	fs.VisitAll(func(f *flag.Flag) {
		if !globalNames[f.Name] {
			t.Errorf("drift: global flag --%s registered by registerGlobalFlags is missing from completionGlobalFlags", f.Name)
		}
	})
	for name := range globalNames {
		if fs.Lookup(name) == nil {
			t.Errorf("drift: completionGlobalFlags lists --%s but registerGlobalFlags does not define it", name)
		}
	}
}

// TestCompletionSpec_CoversDispatch pins the spec's command list to main's
// dispatch list minus unbootstrap (prd R5): same words, same order. A new
// command added to main without a spec entry (and script coverage) fails
// here.
func TestCompletionSpec_CoversDispatch(t *testing.T) {
	var got []string
	for _, c := range completionSpec {
		got = append(got, c.name)
	}
	if strings.Join(got, " ") != strings.Join(completionCommandWords, " ") {
		t.Errorf("spec commands = [%s], want dispatch list minus unbootstrap = [%s]", strings.Join(got, " "), strings.Join(completionCommandWords, " "))
	}
}

// TestCompletionHelp_AuditPins pins the conventions the cross-page audits
// rely on: the curated section order, and a --json example line. The
// example `civ completion bash --json` is literal and truthful: the runner's
// permuteArgs moves the trailing global flag before the positional, accepts
// it, and runCompletion still prints the identical script text (--json is
// accepted-but-inert here, which the Options section documents). No audit
// exemption is needed for this page.
func TestCompletionHelp_AuditPins(t *testing.T) {
	u := strings.Index(completionHelp, "Usage:")
	o := strings.Index(completionHelp, "Options:")
	e := strings.Index(completionHelp, "Examples:")
	if !(u >= 0 && u < o && o < e) {
		t.Errorf("completionHelp section order Usage(%d) < Options(%d) < Examples(%d) violated", u, o, e)
	}
	if !hasJSONExample(completionHelp) {
		t.Errorf("completionHelp: no example line carries --json")
	}
	if strings.Contains(completionHelp, "Global flags") {
		t.Errorf("completionHelp body must not embed the global block (the runner appends it)")
	}
	for _, row := range globalHelpRows() {
		if strings.Contains(completionHelp, row) {
			t.Errorf("completionHelp body duplicates the global row %q (belongs to the appended block only)", row)
		}
	}
	var buf bytes.Buffer
	writeCommandHelp(&buf, "completion", completionHelp)
	if !strings.HasSuffix(buf.String(), globalHelpBlock) {
		t.Errorf("rendered completion page must end with the shared global block")
	}
}

// TestRunCompletion_Table drives the pure runCompletion: a known shell
// prints exactly its script const, bare invocation prints the help page
// (exit 0 through the runner), and unknown shells / extra arguments are
// usage errors naming all four valid shells (exit 1 through the runner's
// normal mapping).
func TestRunCompletion_Table(t *testing.T) {
	cases := []struct {
		name        string
		pos         []string
		wantScript  string // when set, output must equal this const exactly
		wantPage    bool   // when true, output must be the rendered help page
		wantErrPart []string
	}{
		{"bash", []string{"bash"}, bashCompletionScript, false, nil},
		{"zsh", []string{"zsh"}, zshCompletionScript, false, nil},
		{"fish", []string{"fish"}, fishCompletionScript, false, nil},
		{"powershell", []string{"powershell"}, powershellCompletionScript, false, nil},
		{"bare prints help", nil, "", true, nil},
		{"unknown shell", []string{"tcsh"}, "", false, completionShells},
		{"extra args", []string{"bash", "zsh"}, "", false, completionShells},
		{"empty string shell", []string{""}, "", false, completionShells},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			g := &globals{out: &buf}
			err := runCompletion(context.Background(), g, tc.pos)
			if tc.wantErrPart == nil {
				if err != nil {
					t.Fatalf("runCompletion(%v) error = %v, want nil", tc.pos, err)
				}
				if tc.wantScript != "" && buf.String() != tc.wantScript {
					t.Errorf("output is not exactly the %s script const (%d bytes, want %d)", tc.pos[0], buf.Len(), len(tc.wantScript))
				}
				if tc.wantPage {
					got := buf.String()
					if !strings.HasPrefix(got, "civ completion - ") || !strings.Contains(got, "Usage:") || !strings.HasSuffix(got, globalHelpBlock) {
						t.Errorf("bare runCompletion must print the rendered help page, got:\n%s", got)
					}
				}
				return
			}
			if err == nil {
				t.Fatalf("runCompletion(%v) error = nil, want usage error naming the valid shells", tc.pos)
			}
			if buf.Len() != 0 {
				t.Errorf("usage error must not write to stdout, got %q", buf.String())
			}
			for _, want := range tc.wantErrPart {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name valid shell %q", err.Error(), want)
				}
			}
		})
	}
}
