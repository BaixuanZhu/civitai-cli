package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"civitai-cli/internal/app"
	"civitai-cli/internal/skill"
)

// newSkillCmdEnv isolates HOME/USERPROFILE and PATH (so DetectPlatforms sees no
// real platform binaries) and wires a stdout buffer into globals.
func newSkillCmdEnv(t *testing.T) (*globals, *bytes.Buffer, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("CIVITAI_API_KEY", "")
	var g globals
	var buf bytes.Buffer
	g.out = &buf
	return &g, &buf, home
}

func setSkillTestVersion(t *testing.T, v string) {
	t.Helper()
	old := app.Version
	app.Version = v
	t.Cleanup(func() { app.Version = old })
}

// runSkillCmd mirrors execute's flag handling (global + skill flags) without
// os.Exit, then drives the pure runSkill with an injectable stdin.
func runSkillCmd(t *testing.T, g *globals, stdin io.Reader, args ...string) error {
	t.Helper()
	var o skillOptions
	pos := parseForTest(t, g, o.register, args)
	return runSkill(context.Background(), g, pos, &o, stdin)
}

func TestSkill_FlagInstall(t *testing.T) {
	g, buf, home := newSkillCmdEnv(t)
	setSkillTestVersion(t, "1.2.3-test")
	if err := runSkillCmd(t, g, strings.NewReader(""), "install", "--platform", "claude", "--scope", "user"); err != nil {
		t.Fatalf("runSkill: %v", err)
	}
	dir := filepath.Join(home, ".claude", "skills", "civ-prompt")
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md missing: %v", err)
	}
	if !strings.Contains(buf.String(), "installed") || !strings.Contains(buf.String(), dir) {
		t.Errorf("summary must report installed + path, got %q", buf.String())
	}
}

func TestSkill_FlagInstallAllPlatforms(t *testing.T) {
	g, _, home := newSkillCmdEnv(t)
	setSkillTestVersion(t, "1.2.3-test")
	if err := runSkillCmd(t, g, strings.NewReader(""), "install", "--platform", "all"); err != nil {
		t.Fatalf("runSkill: %v", err)
	}
	for _, sub := range []string{".claude", ".zcode", ".agents"} {
		p := filepath.Join(home, sub, "skills", "civ-prompt", "SKILL.md")
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s missing: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "skills", "civ-prompt")); !os.IsNotExist(err) {
		t.Errorf("codex must never write .codex/skills (stat err = %v)", err)
	}
}

func TestSkill_InvalidPlatform(t *testing.T) {
	g, _, _ := newSkillCmdEnv(t)
	err := runSkillCmd(t, g, strings.NewReader(""), "install", "--platform", "bogus")
	if err == nil || exitCodeFor(err, false) != 1 {
		t.Errorf("invalid platform must map to exit 1, got %v", err)
	}
}

func TestSkill_InvalidScope(t *testing.T) {
	g, _, _ := newSkillCmdEnv(t)
	err := runSkillCmd(t, g, strings.NewReader(""), "install", "--platform", "claude", "--scope", "bogus")
	if err == nil || exitCodeFor(err, false) != 1 {
		t.Errorf("invalid scope must map to exit 1, got %v", err)
	}
}

func TestSkill_UnknownAction(t *testing.T) {
	g, _, _ := newSkillCmdEnv(t)
	err := runSkillCmd(t, g, strings.NewReader(""), "bogus")
	if err == nil || exitCodeFor(err, false) != 1 {
		t.Errorf("unknown action must map to exit 1, got %v", err)
	}
}

func TestSkill_MissingAction(t *testing.T) {
	g, _, _ := newSkillCmdEnv(t)
	err := runSkillCmd(t, g, strings.NewReader(""))
	if err == nil || exitCodeFor(err, false) != 1 {
		t.Errorf("missing action must map to exit 1, got %v", err)
	}
}

func TestSkill_JSONOutput(t *testing.T) {
	g, buf, _ := newSkillCmdEnv(t)
	setSkillTestVersion(t, "1.2.3-test")
	if err := runSkillCmd(t, g, strings.NewReader(""), "install", "--platform", "claude,zcode", "--json"); err != nil {
		t.Fatalf("runSkill: %v", err)
	}
	var results []skill.Result
	if err := json.Unmarshal(buf.Bytes(), &results); err != nil {
		t.Fatalf("--json must parse into []Result: %v (output %q)", err, buf.String())
	}
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Action != skill.ActionInstalled {
			t.Errorf("action = %q, want installed", r.Action)
		}
		if r.Platform == "" || r.Path == "" {
			t.Errorf("result must carry platform + path: %+v", r)
		}
	}
}

func TestSkill_Remove(t *testing.T) {
	g, _, home := newSkillCmdEnv(t)
	setSkillTestVersion(t, "1.2.3-test")
	if err := runSkillCmd(t, g, strings.NewReader(""), "install", "--platform", "claude"); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := runSkillCmd(t, g, strings.NewReader(""), "remove", "--platform", "claude"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	dir := filepath.Join(home, ".claude", "skills", "civ-prompt")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("skill dir must be gone after remove (stat err = %v)", err)
	}
}

func TestSkill_UpdateSkipAndOverwrite(t *testing.T) {
	g, buf, home := newSkillCmdEnv(t)
	setSkillTestVersion(t, "1.2.3-test")
	if err := runSkillCmd(t, g, strings.NewReader(""), "install", "--platform", "claude"); err != nil {
		t.Fatalf("install: %v", err)
	}
	buf.Reset()
	if err := runSkillCmd(t, g, strings.NewReader(""), "update", "--platform", "claude"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if !strings.Contains(buf.String(), "up-to-date") {
		t.Errorf("same-version update must report up-to-date, got %q", buf.String())
	}

	dir := filepath.Join(home, ".claude", "skills", "civ-prompt")
	old := "---\nname: civ-prompt\ndescription: x\nmetadata:\n  version: \"0.0.1-old\"\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(old), 0o644); err != nil {
		t.Fatalf("plant old version: %v", err)
	}
	buf.Reset()
	if err := runSkillCmd(t, g, strings.NewReader(""), "update", "--platform", "claude"); err != nil {
		t.Fatalf("update over old version: %v", err)
	}
	if !strings.Contains(buf.String(), "updated") {
		t.Errorf("different-version update must report updated, got %q", buf.String())
	}
}

func TestSkill_InteractiveInstall(t *testing.T) {
	g, buf, home := newSkillCmdEnv(t)
	setSkillTestVersion(t, "1.2.3-test")
	stdin := strings.NewReader("1\nuser\ny\n") // 1=claude, scope user, confirm yes
	var err error
	stderr := captureStderr(t, func() { err = runSkillCmd(t, g, stdin, "install") })
	if err != nil {
		t.Fatalf("interactive install: %v", err)
	}
	dir := filepath.Join(home, ".claude", "skills", "civ-prompt")
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Errorf("interactive install missing SKILL.md: %v", err)
	}
	if !strings.Contains(stderr, "Select platforms") {
		t.Errorf("interactive flow must prompt on stderr, got %q", stderr)
	}
	if !strings.Contains(buf.String(), "installed") {
		t.Errorf("summary must report installed, got %q", buf.String())
	}
}

func TestSkill_InteractiveAcceptsDetected(t *testing.T) {
	g, _, home := newSkillCmdEnv(t)
	setSkillTestVersion(t, "1.2.3-test")
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	stdin := strings.NewReader("\n\ny\n") // empty platforms → detected, empty scope → user, confirm
	var err error
	captureStderr(t, func() { err = runSkillCmd(t, g, stdin, "install") })
	if err != nil {
		t.Fatalf("interactive detected default: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "civ-prompt", "SKILL.md")); err != nil {
		t.Errorf("detected claude must be installed: %v", err)
	}
}

func TestSkill_InteractiveConflictOverwrite(t *testing.T) {
	g, _, home := newSkillCmdEnv(t)
	setSkillTestVersion(t, "2.0.0-test")
	dir := filepath.Join(home, ".claude", "skills", "civ-prompt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	old := "---\nname: civ-prompt\ndescription: x\nmetadata:\n  version: \"1.0.0-old\"\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(old), 0o644); err != nil {
		t.Fatalf("plant old: %v", err)
	}
	stdin := strings.NewReader("1\nuser\ny\noverwrite\n")
	var err error
	captureStderr(t, func() { err = runSkillCmd(t, g, stdin, "install") })
	if err != nil {
		t.Fatalf("interactive conflict install: %v", err)
	}
	b, readErr := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if readErr != nil {
		t.Fatalf("read SKILL.md: %v", readErr)
	}
	if !strings.Contains(string(b), "2.0.0-test") {
		t.Errorf("conflict overwrite must render the new version, got %q", string(b))
	}
}

func TestSkill_InteractiveCancel(t *testing.T) {
	g, _, _ := newSkillCmdEnv(t)
	stdin := strings.NewReader("1\nuser\nn\n")
	var err error
	captureStderr(t, func() { err = runSkillCmd(t, g, stdin, "install") })
	if err == nil || exitCodeFor(err, false) != 1 {
		t.Errorf("confirmation cancel must map to exit 1, got %v", err)
	}
}

func TestSkill_InteractiveEOF(t *testing.T) {
	g, _, _ := newSkillCmdEnv(t)
	var err error
	captureStderr(t, func() { err = runSkillCmd(t, g, strings.NewReader(""), "install") })
	if err == nil || exitCodeFor(err, false) != 1 {
		t.Errorf("EOF during interaction must map to exit 1, got %v", err)
	}
}

func TestParsePlatforms(t *testing.T) {
	all := []skill.Platform{skill.PlatformClaude, skill.PlatformZcode, skill.PlatformCodex}
	cases := []struct {
		in      string
		want    []skill.Platform
		wantErr bool
	}{
		{"claude", []skill.Platform{skill.PlatformClaude}, false},
		{"claude,zcode,codex", all, false},
		{"all", all, false},
		{"ALL", all, false},
		{"claude,claude", []skill.Platform{skill.PlatformClaude}, false},
		{" Claude , Zcode ", []skill.Platform{skill.PlatformClaude, skill.PlatformZcode}, false},
		{"bogus", nil, true},
		{"", nil, true},
		{"claude,bogus", nil, true},
	}
	for _, c := range cases {
		got, err := parsePlatforms(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parsePlatforms(%q) must error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parsePlatforms(%q): %v", c.in, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("parsePlatforms(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseInteractivePlatforms(t *testing.T) {
	detected := []skill.Platform{skill.PlatformClaude}
	all := []skill.Platform{skill.PlatformClaude, skill.PlatformZcode, skill.PlatformCodex}
	cases := []struct {
		ans     string
		want    []skill.Platform
		wantErr bool
	}{
		{"", detected, false},
		{"all", all, false},
		{"1", []skill.Platform{skill.PlatformClaude}, false},
		{"1,3", []skill.Platform{skill.PlatformClaude, skill.PlatformCodex}, false},
		{"2 3", []skill.Platform{skill.PlatformZcode, skill.PlatformCodex}, false},
		{"claude,codex", []skill.Platform{skill.PlatformClaude, skill.PlatformCodex}, false},
		{"4", nil, true},
		{"bogus", nil, true},
	}
	for _, c := range cases {
		got, err := parseInteractivePlatforms(c.ans, detected)
		if c.wantErr {
			if err == nil {
				t.Errorf("answer %q must error", c.ans)
			}
			continue
		}
		if err != nil {
			t.Errorf("answer %q: %v", c.ans, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("answer %q = %v, want %v", c.ans, got, c.want)
		}
	}
	if _, err := parseInteractivePlatforms("", nil); err == nil {
		t.Errorf("empty answer with nothing detected must error")
	}
}
