package skill

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"civitai-cli/internal/app"
)

// isolateHome points os.UserHomeDir at a fresh temp dir (HOME on Unix,
// USERPROFILE on Windows) so tests never touch a real user profile.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

// setTestVersion pins app.Version for deterministic render/compare assertions
// and restores it on cleanup.
func setTestVersion(t *testing.T, v string) {
	t.Helper()
	old := app.Version
	app.Version = v
	t.Cleanup(func() { app.Version = old })
}

func TestRootFor(t *testing.T) {
	home := isolateHome(t)
	proj := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(proj); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	cases := []struct {
		p    Platform
		s    Scope
		want string
	}{
		{PlatformClaude, ScopeUser, filepath.Join(home, ".claude", "skills")},
		{PlatformClaude, ScopeProject, filepath.Join(proj, ".claude", "skills")},
		{PlatformZcode, ScopeUser, filepath.Join(home, ".zcode", "skills")},
		{PlatformZcode, ScopeProject, filepath.Join(proj, ".zcode", "skills")},
		{PlatformCodex, ScopeUser, filepath.Join(home, ".agents", "skills")},
		{PlatformCodex, ScopeProject, filepath.Join(proj, ".agents", "skills")},
	}
	for _, c := range cases {
		got, err := RootFor(c.p, c.s)
		if err != nil {
			t.Errorf("RootFor(%q,%q): %v", c.p, c.s, err)
			continue
		}
		if got != c.want {
			t.Errorf("RootFor(%q,%q) = %q, want %q", c.p, c.s, got, c.want)
		}
		if c.p == PlatformCodex && strings.Contains(got, ".codex") {
			t.Errorf("codex must never map to .codex/skills, got %q", got)
		}
	}
}

func TestRootFor_Invalid(t *testing.T) {
	isolateHome(t)
	if _, err := RootFor(Platform("bogus"), ScopeUser); err == nil {
		t.Errorf("unknown platform must error")
	}
	if _, err := RootFor(PlatformClaude, Scope("bogus")); err == nil {
		t.Errorf("unknown scope must error")
	}
}

func TestDetectPlatforms(t *testing.T) {
	home := isolateHome(t)
	t.Setenv("PATH", t.TempDir()) // no platform binaries discoverable on PATH

	if got := DetectPlatforms(); len(got) != 0 {
		t.Fatalf("empty home + empty PATH must detect nothing, got %v", got)
	}

	mkdir := func(name string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(home, name), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
	}
	mkdir(".claude")
	mkdir(".zcode")
	if got, want := DetectPlatforms(), []Platform{PlatformClaude, PlatformZcode}; !reflect.DeepEqual(got, want) {
		t.Errorf("after .claude/.zcode: got %v, want %v", got, want)
	}

	// codex is detected via the shared .agents root even without .codex.
	mkdir(".agents")
	if got, want := DetectPlatforms(), []Platform{PlatformClaude, PlatformZcode, PlatformCodex}; !reflect.DeepEqual(got, want) {
		t.Errorf("after .agents: got %v, want %v", got, want)
	}
}

func TestRenderVersion(t *testing.T) {
	setTestVersion(t, "9.9.9-test")
	got := string(renderVersion([]byte("metadata:\n  version: \"__CIV_VERSION__\"\n")))
	if strings.Contains(got, "__CIV_VERSION__") {
		t.Errorf("placeholder must be replaced, got %q", got)
	}
	if !strings.Contains(got, "9.9.9-test") {
		t.Errorf("rendered output must carry app.Version, got %q", got)
	}
}

func TestFrontmatterVersion(t *testing.T) {
	cases := []struct{ name, content, want string }{
		{"block quoted", "---\nname: civ-prompt\nmetadata:\n  version: \"1.2.3\"\n---\nbody", "1.2.3"},
		{"block unquoted", "---\nmetadata:\n  version: 1.2.3\n---\n", "1.2.3"},
		{"single quoted", "---\nmetadata:\n  version: '1.2.3'\n---\n", "1.2.3"},
		{"no metadata", "---\nname: civ-prompt\n---\n", ""},
		{"no frontmatter", "name: civ-prompt\nversion: 1.2.3\n", ""},
		{"version outside metadata", "---\nname: x\nversion: 9.9.9\nmetadata:\n  other: y\n---\n", ""},
		{"placeholder", "---\nmetadata:\n  version: \"__CIV_VERSION__\"\n---\n", "__CIV_VERSION__"},
	}
	for _, c := range cases {
		if got := frontmatterVersion(c.content); got != c.want {
			t.Errorf("%s: frontmatterVersion = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestEmbedFS_ContainsSkillTree(t *testing.T) {
	for _, name := range []string{
		"assets/civ-prompt/SKILL.md",
		"assets/civ-prompt/references/civ-commands.md",
		"assets/civ-prompt/references/prompt-patterns.md",
	} {
		b, err := FS.ReadFile(name)
		if err != nil {
			t.Errorf("embedded %s missing: %v", name, err)
			continue
		}
		if len(b) == 0 {
			t.Errorf("embedded %s is empty", name)
		}
	}
}

// TestSkillMD_Valid enforces the agentskills.io portable contract on the bundled
// SKILL.md: only the standard six frontmatter fields, name equal to the directory
// name, a single-line description within 1024 chars and free of angle brackets,
// a body under 500 lines, and no TODO residue (prd R1/R2/R4/R5/R19).
func TestSkillMD_Valid(t *testing.T) {
	b, err := FS.ReadFile("assets/civ-prompt/SKILL.md")
	if err != nil {
		t.Fatalf("read embedded SKILL.md: %v", err)
	}
	content := strings.ReplaceAll(string(b), "\r\n", "\n")
	lines := strings.Split(content, "\n")
	if strings.TrimSpace(lines[0]) != "---" {
		t.Fatalf("SKILL.md must open with a --- frontmatter delimiter")
	}
	fmEnd := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			fmEnd = i
			break
		}
	}
	if fmEnd < 0 {
		t.Fatalf("SKILL.md frontmatter must close with ---")
	}
	fmLines := lines[1:fmEnd]
	bodyLines := lines[fmEnd+1:]

	allowed := map[string]bool{
		"name": true, "description": true, "license": true,
		"compatibility": true, "metadata": true, "allowed-tools": true,
	}
	var name, description string
	for _, line := range fmLines {
		// Top-level keys have no leading whitespace; indented lines (e.g.
		// metadata.version) are nested and not validated as fields.
		if line != strings.TrimSpace(line) || !strings.Contains(line, ":") {
			continue
		}
		key := strings.TrimSpace(strings.SplitN(line, ":", 2)[0])
		val := strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
		if !allowed[key] {
			t.Errorf("non-standard frontmatter field %q; only the agentskills.io six are portable", key)
		}
		switch key {
		case "name":
			name = val
		case "description":
			description = val
		}
	}

	if name != SkillName {
		t.Errorf("name = %q, want %q (must equal the directory name)", name, SkillName)
	}
	if description == "" {
		t.Fatalf("description must be present")
	}
	if n := utf8.RuneCountInString(description); n > 1024 {
		t.Errorf("description is %d chars, must be <= 1024", n)
	}
	if strings.ContainsAny(description, "<>") {
		t.Errorf("description must not contain angle brackets: %q", description)
	}
	if len(bodyLines) >= 500 {
		t.Errorf("body is %d lines, must be < 500", len(bodyLines))
	}
	if strings.Contains(content, "TODO") {
		t.Errorf("SKILL.md must not contain TODO residue")
	}
	t.Logf("SKILL.md valid: name=%q, description=%d chars (single line, no angle brackets), body=%d lines, frontmatter fields within the standard six",
		name, utf8.RuneCountInString(description), len(bodyLines))
}
