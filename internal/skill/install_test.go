package skill

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFakeSkill plants a minimal installed SKILL.md with a given version so
// conflict/update tests have a realistic prior install to read.
func writeFakeSkill(t *testing.T, dir, version string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	content := "---\nname: civ-prompt\ndescription: fake\nmetadata:\n  version: \"" + version + "\"\n---\nold body\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write fake SKILL.md: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestInstall_Fresh(t *testing.T) {
	home := isolateHome(t)
	setTestVersion(t, "1.2.3-test")
	res, err := Install(Options{Platforms: []Platform{PlatformClaude}, Scope: ScopeUser})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(res) != 1 || res[0].Action != ActionInstalled {
		t.Fatalf("want one installed result, got %+v", res)
	}
	dir := filepath.Join(home, ".claude", "skills", SkillName)
	if res[0].Path != dir {
		t.Errorf("result path = %q, want %q", res[0].Path, dir)
	}
	skillMD := readFile(t, filepath.Join(dir, "SKILL.md"))
	if strings.Contains(skillMD, "__CIV_VERSION__") {
		t.Errorf("version placeholder must be rendered")
	}
	if got := frontmatterVersion(skillMD); got != "1.2.3-test" {
		t.Errorf("installed version = %q, want 1.2.3-test", got)
	}
	for _, ref := range []string{"references/civ-commands.md", "references/prompt-patterns.md"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(ref))); err != nil {
			t.Errorf("%s not copied: %v", ref, err)
		}
	}
}

func TestInstall_ConflictSkipByDefault(t *testing.T) {
	home := isolateHome(t)
	setTestVersion(t, "2.0.0-test")
	dir := filepath.Join(home, ".claude", "skills", SkillName)
	writeFakeSkill(t, dir, "1.0.0-old")
	res, err := Install(Options{Platforms: []Platform{PlatformClaude}, Scope: ScopeUser})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if res[0].Action != ActionSkipped {
		t.Errorf("nil conflict must default to skip, got %q", res[0].Action)
	}
	if got := frontmatterVersion(readFile(t, filepath.Join(dir, "SKILL.md"))); got != "1.0.0-old" {
		t.Errorf("skip must preserve the installed version, got %q", got)
	}
}

func TestInstall_ConflictOverwrite(t *testing.T) {
	home := isolateHome(t)
	setTestVersion(t, "2.0.0-test")
	dir := filepath.Join(home, ".claude", "skills", SkillName)
	writeFakeSkill(t, dir, "1.0.0-old")
	called := false
	conflict := func(path, installed string) string {
		called = true
		if path != dir {
			t.Errorf("conflict path = %q, want %q", path, dir)
		}
		if installed != "1.0.0-old" {
			t.Errorf("conflict installed version = %q, want 1.0.0-old", installed)
		}
		return "overwrite"
	}
	res, err := Install(Options{Platforms: []Platform{PlatformClaude}, Scope: ScopeUser, Conflict: conflict})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !called {
		t.Errorf("conflict callback must be consulted for an existing target")
	}
	if res[0].Action != ActionInstalled {
		t.Errorf("overwrite must report installed, got %q", res[0].Action)
	}
	if got := frontmatterVersion(readFile(t, filepath.Join(dir, "SKILL.md"))); got != "2.0.0-test" {
		t.Errorf("overwrite must render the new version, got %q", got)
	}
}

func TestInstall_ConflictBackup(t *testing.T) {
	home := isolateHome(t)
	setTestVersion(t, "2.0.0-test")
	dir := filepath.Join(home, ".claude", "skills", SkillName)
	writeFakeSkill(t, dir, "1.0.0-old")
	var out bytes.Buffer
	conflict := func(path, installed string) string { return "backup" }
	res, err := Install(Options{Platforms: []Platform{PlatformClaude}, Scope: ScopeUser, Conflict: conflict, Out: &out})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if res[0].Action != ActionBackedUp {
		t.Errorf("want backed-up, got %q", res[0].Action)
	}
	if got := frontmatterVersion(readFile(t, filepath.Join(dir+".bak", "SKILL.md"))); got != "1.0.0-old" {
		t.Errorf("backup must preserve the old version, got %q", got)
	}
	if got := frontmatterVersion(readFile(t, filepath.Join(dir, "SKILL.md"))); got != "2.0.0-test" {
		t.Errorf("new install must render the new version, got %q", got)
	}
	if !strings.Contains(out.String(), "backed up") {
		t.Errorf("backup progress must be reported to Out, got %q", out.String())
	}
}

func TestInstall_ForceOverwrites(t *testing.T) {
	home := isolateHome(t)
	setTestVersion(t, "2.0.0-test")
	dir := filepath.Join(home, ".zcode", "skills", SkillName)
	writeFakeSkill(t, dir, "1.0.0-old")
	res, err := Install(Options{Platforms: []Platform{PlatformZcode}, Scope: ScopeUser, Force: true})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if res[0].Action != ActionInstalled {
		t.Errorf("force overwrite must report installed, got %q", res[0].Action)
	}
	if got := frontmatterVersion(readFile(t, filepath.Join(dir, "SKILL.md"))); got != "2.0.0-test" {
		t.Errorf("force must render the new version, got %q", got)
	}
}

func TestInstall_CodexUsesAgentsRoot(t *testing.T) {
	home := isolateHome(t)
	setTestVersion(t, "1.2.3-test")
	res, err := Install(Options{Platforms: []Platform{PlatformCodex}, Scope: ScopeUser})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	want := filepath.Join(home, ".agents", "skills", SkillName)
	if res[0].Path != want {
		t.Errorf("codex path = %q, want %q", res[0].Path, want)
	}
	if _, err := os.Stat(filepath.Join(want, "SKILL.md")); err != nil {
		t.Errorf("codex SKILL.md missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "skills", SkillName)); !os.IsNotExist(err) {
		t.Errorf("codex must never write .codex/skills (stat err = %v)", err)
	}
}

func TestInstall_ProjectScope(t *testing.T) {
	isolateHome(t)
	setTestVersion(t, "1.2.3-test")
	proj := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(proj); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	res, err := Install(Options{Platforms: []Platform{PlatformCodex}, Scope: ScopeProject})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	want := filepath.Join(proj, ".agents", "skills", SkillName)
	if res[0].Path != want {
		t.Errorf("project codex path = %q, want %q", res[0].Path, want)
	}
	if _, err := os.Stat(filepath.Join(want, "SKILL.md")); err != nil {
		t.Errorf("project SKILL.md missing: %v", err)
	}
}

func TestRemove(t *testing.T) {
	home := isolateHome(t)
	setTestVersion(t, "1.2.3-test")
	if _, err := Install(Options{Platforms: []Platform{PlatformClaude}, Scope: ScopeUser}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	dir := filepath.Join(home, ".claude", "skills", SkillName)
	res, err := Remove(Options{Platforms: []Platform{PlatformClaude}, Scope: ScopeUser})
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if res[0].Action != ActionRemoved {
		t.Errorf("want removed, got %q", res[0].Action)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("skill dir must be gone, stat err = %v", err)
	}
	res, err = Remove(Options{Platforms: []Platform{PlatformClaude}, Scope: ScopeUser})
	if err != nil {
		t.Fatalf("Remove again: %v", err)
	}
	if res[0].Action != ActionSkipped {
		t.Errorf("removing a missing skill must be skipped, got %q", res[0].Action)
	}
}

func TestRemove_PreservesZcodeLock(t *testing.T) {
	home := isolateHome(t)
	setTestVersion(t, "1.2.3-test")
	root := filepath.Join(home, ".zcode", "skills")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	lock := filepath.Join(root, ".skills_store_lock.json")
	if err := os.WriteFile(lock, []byte(`{"name":"other"}`), 0o644); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	if _, err := Install(Options{Platforms: []Platform{PlatformZcode}, Scope: ScopeUser}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if _, err := Remove(Options{Platforms: []Platform{PlatformZcode}, Scope: ScopeUser}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(lock); err != nil {
		t.Errorf("zcode .skills_store_lock.json must survive remove: %v", err)
	}
}

func TestUpdate_UpToDate(t *testing.T) {
	isolateHome(t)
	setTestVersion(t, "1.2.3-test")
	if _, err := Install(Options{Platforms: []Platform{PlatformClaude}, Scope: ScopeUser}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	res, err := Update(Options{Platforms: []Platform{PlatformClaude}, Scope: ScopeUser})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if res[0].Action != ActionUpToDate {
		t.Errorf("same version must report up-to-date, got %q", res[0].Action)
	}
}

func TestUpdate_DifferentVersionOverwrites(t *testing.T) {
	home := isolateHome(t)
	setTestVersion(t, "2.0.0-test")
	dir := filepath.Join(home, ".claude", "skills", SkillName)
	writeFakeSkill(t, dir, "1.0.0-old")
	res, err := Update(Options{Platforms: []Platform{PlatformClaude}, Scope: ScopeUser})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if res[0].Action != ActionUpdated {
		t.Errorf("different version must report updated, got %q", res[0].Action)
	}
	skillMD := readFile(t, filepath.Join(dir, "SKILL.md"))
	if got := frontmatterVersion(skillMD); got != "2.0.0-test" {
		t.Errorf("update must render the new version, got %q", got)
	}
	if !strings.Contains(skillMD, "name: civ-prompt") {
		t.Errorf("update must preserve the skill name")
	}
}

func TestUpdate_NotFound(t *testing.T) {
	isolateHome(t)
	setTestVersion(t, "1.2.3-test")
	res, err := Update(Options{Platforms: []Platform{PlatformClaude}, Scope: ScopeUser})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if res[0].Action != ActionNotFound {
		t.Errorf("missing install must report not-found, got %q", res[0].Action)
	}
}

func TestUpdate_ForceRewritesSameVersion(t *testing.T) {
	isolateHome(t)
	setTestVersion(t, "1.2.3-test")
	if _, err := Install(Options{Platforms: []Platform{PlatformClaude}, Scope: ScopeUser}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	res, err := Update(Options{Platforms: []Platform{PlatformClaude}, Scope: ScopeUser, Force: true})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if res[0].Action != ActionUpdated {
		t.Errorf("force must rewrite even at the same version, got %q", res[0].Action)
	}
}
