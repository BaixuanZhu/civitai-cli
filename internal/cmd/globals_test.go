package cmd

import (
	"errors"
	"flag"
	"testing"
	"time"

	"civitai-cli/internal/api"
	"civitai-cli/internal/config"
)

func TestExitCodeFor(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		hasToken bool
		want     int
	}{
		{"success", nil, false, 0},
		{"network", &api.NetworkError{Attempts: 4, Err: errors.New("x")}, false, 2},
		{"unauthorized with token", &api.UnauthorizedError{}, true, 4},
		{"unauthorized anonymous", &api.UnauthorizedError{}, false, 1},
		{"not found (M1 usage-class)", &api.NotFoundError{}, false, 1},
		{"api rejection", &api.ApiError{Status: 400}, false, 1},
		{"generic", errors.New("x"), false, 1},
	}
	for _, tc := range cases {
		if got := exitCodeFor(tc.err, tc.hasToken); got != tc.want {
			t.Errorf("%s: exitCodeFor = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestResolveTokenPriority(t *testing.T) {
	isolateConfigDir(t) // hermetic: no real user config token
	t.Setenv("CIVITAI_API_KEY", "env-key")
	g := &globals{}
	if got := g.resolveToken(); got != "env-key" {
		t.Errorf("env fallback = %q", got)
	}
	g.Token = "flag-key"
	if got := g.resolveToken(); got != "flag-key" {
		t.Errorf("flag must win over env, got %q", got)
	}
	// Flag and env empty with no config file → anonymous.
	t.Setenv("CIVITAI_API_KEY", "")
	g.Token = ""
	if got := g.resolveToken(); got != "" {
		t.Errorf("anonymous default (no config) = %q", got)
	}
	// Third layer: the saved config token (M3).
	if err := config.Save(config.Config{Token: "config-key"}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if got := g.resolveToken(); got != "config-key" {
		t.Errorf("config layer = %q, want config-key", got)
	}
	// env still wins over config; flag still wins over both.
	t.Setenv("CIVITAI_API_KEY", "env-key")
	if got := g.resolveToken(); got != "env-key" {
		t.Errorf("env must win over config, got %q", got)
	}
	g.Token = "flag-key"
	if got := g.resolveToken(); got != "flag-key" {
		t.Errorf("flag must win over env and config, got %q", got)
	}
}

func TestRegisterGlobalFlags_Defaults(t *testing.T) {
	var g globals
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	registerGlobalFlags(fs, &g)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if g.Timeout != 30*time.Second {
		t.Errorf("default timeout = %v", g.Timeout)
	}
	if g.JSON || g.Verbose || g.Token != "" {
		t.Errorf("unexpected defaults: %+v", g)
	}
	// --verbose must have no -v short alias (show owns -v, design D6).
	if fs.Lookup("v") != nil {
		t.Errorf("global -v alias must not exist")
	}
}

func TestPermuteArgs(t *testing.T) {
	var g globals
	var o searchOptions
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	registerGlobalFlags(fs, &g)
	o.register(fs)

	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"query first", []string{"stocking", "--type", "lora"}, []string{"--type", "lora", "stocking"}},
		{"bool flag after query", []string{"anima", "--json"}, []string{"--json", "anima"}},
		{"equals form", []string{"anima", "--limit=5"}, []string{"--limit=5", "anima"}},
		{"multiword query", []string{"foo", "bar", "--json"}, []string{"--json", "foo", "bar"}},
		{"double dash terminator", []string{"--json", "--", "--weird"}, []string{"--json", "--weird"}},
		{"flags first untouched", []string{"--json", "anima"}, []string{"--json", "anima"}},
	}
	for _, tc := range cases {
		got := permuteArgs(fs, tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
				break
			}
		}
	}
}

func TestPermuteArgs_ShowVersionFlag(t *testing.T) {
	var g globals
	var o showOptions
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	registerGlobalFlags(fs, &g)
	o.register(fs)
	// The plan's UX: civ show 182296 -v 482391 — -v consumes its value
	// even after the positional id.
	got := permuteArgs(fs, []string{"182296", "-v", "482391"})
	want := []string{"-v", "482391", "182296"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if err := fs.Parse(got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if o.ver != 482391 || len(fs.Args()) != 1 || fs.Args()[0] != "182296" {
		t.Errorf("ver = %d, args = %v", o.ver, fs.Args())
	}
}
