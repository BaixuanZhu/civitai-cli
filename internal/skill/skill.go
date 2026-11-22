// Package skill installs, removes, and updates the bundled civ-prompt Agent
// Skill across the claude / zcode / codex platforms. It is a leaf package: it
// embeds the skill tree (go:embed), maps each platform to its native skill root,
// and performs pure local file operations. It never touches the network and
// never reads stdin — interactive conflict decisions are injected by the cmd
// layer through a ConflictFunc callback, keeping this package testable offline.
//
// Platform roots follow the agentskills.io conventions verified in
// research/platform-skill-conventions.md: claude -> .claude/skills,
// zcode -> .zcode/skills, codex -> .agents/skills (never .codex/skills).
package skill

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"civitai-cli/internal/app"
)

// Platform identifies a target agent platform whose native skill root civ writes.
type Platform string

// Supported platforms. codex reads the shared .agents/skills root, not
// .codex/skills (research/codex-skill-uncertainty.md).
const (
	// PlatformClaude targets Claude Code (.claude/skills under home or cwd).
	PlatformClaude Platform = "claude"
	// PlatformZcode targets zcode (.zcode/skills under home or cwd).
	PlatformZcode Platform = "zcode"
	// PlatformCodex targets OpenAI Codex CLI via the shared .agents/skills root.
	PlatformCodex Platform = "codex"
)

// Scope selects between the user-level (global) and project-level skill root.
type Scope string

// Supported scopes. user resolves under os.UserHomeDir(); project under the
// current working directory (design §2).
const (
	// ScopeUser installs under the user home (global, cross-project).
	ScopeUser Scope = "user"
	// ScopeProject installs under the current working directory.
	ScopeProject Scope = "project"
)

// versionPlaceholder is the SKILL.md template token replaced with app.Version
// when the skill is written to disk.
const versionPlaceholder = "__CIV_VERSION__"

// skillsSubdir maps a platform to its native skills directory relative to the
// scope base. codex deliberately uses the shared .agents/skills root.
func skillsSubdir(p Platform) (string, bool) {
	switch p {
	case PlatformClaude:
		return filepath.Join(".claude", "skills"), true
	case PlatformZcode:
		return filepath.Join(".zcode", "skills"), true
	case PlatformCodex:
		return filepath.Join(".agents", "skills"), true
	default:
		return "", false
	}
}

// RootFor returns the skills root directory for a platform and scope: the
// user home (os.UserHomeDir) for ScopeUser, or the current working directory
// (os.Getwd) for ScopeProject, joined with the platform's native skills subdir.
// The skill itself lives at RootFor + "/" + SkillName. Unknown platform/scope
// values and an unresolvable base directory return an error.
func RootFor(p Platform, s Scope) (string, error) {
	sub, ok := skillsSubdir(p)
	if !ok {
		return "", fmt.Errorf("skill: unknown platform %q (want claude, zcode, or codex)", p)
	}
	var base string
	var err error
	switch s {
	case ScopeUser:
		base, err = os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("skill: resolve home directory: %w", err)
		}
	case ScopeProject:
		base, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("skill: resolve working directory: %w", err)
		}
	default:
		return "", fmt.Errorf("skill: unknown scope %q (want user or project)", s)
	}
	return filepath.Join(base, sub), nil
}

// DetectPlatforms reports which platforms appear installed, in the fixed order
// claude, zcode, codex. A platform is detected when its config directory exists
// under the user home (~/.claude, ~/.zcode, ~/.codex, or the shared ~/.agents)
// or its binary is on PATH. codex is also detected via ~/.agents because that is
// its native skill root even when ~/.codex is absent. The result preselects the
// interactive platform menu; an empty result means nothing was detected.
func DetectPlatforms() []Platform {
	home, homeErr := os.UserHomeDir()
	dirExists := func(name string) bool {
		if homeErr != nil || home == "" {
			return false
		}
		fi, err := os.Stat(filepath.Join(home, name))
		return err == nil && fi.IsDir()
	}
	onPath := func(bin string) bool {
		_, err := exec.LookPath(bin)
		return err == nil
	}

	var found []Platform
	if dirExists(".claude") || onPath("claude") {
		found = append(found, PlatformClaude)
	}
	if dirExists(".zcode") || onPath("zcode") {
		found = append(found, PlatformZcode)
	}
	if dirExists(".codex") || dirExists(".agents") || onPath("codex") {
		found = append(found, PlatformCodex)
	}
	return found
}

// renderVersion substitutes the __CIV_VERSION__ placeholder in the embedded
// SKILL.md template with the running binary's app.Version, so the installed
// skill records the civ version that produced it (used by update to compare).
func renderVersion(content []byte) []byte {
	return []byte(strings.ReplaceAll(string(content), versionPlaceholder, app.Version))
}

// installedVersion reads the metadata.version of an already-installed skill
// directory's SKILL.md, returning "" when the file is missing or has no version.
func installedVersion(skillDir string) string {
	b, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		return ""
	}
	return frontmatterVersion(string(b))
}

// frontmatterVersion extracts metadata.version from a SKILL.md's YAML
// frontmatter without a YAML dependency (zero-dependency contract). The format
// is controlled — civ writes it — so a line scan suffices: find the frontmatter
// block (between the first two "---" lines), enter the top-level metadata:
// block, and read its indented version: key. Returns "" when absent.
func frontmatterVersion(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	inMeta := false
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			break // end of frontmatter
		}
		// A line with no leading whitespace is a top-level key; it sets or
		// clears the metadata context. Indented lines belong to the current
		// top-level block.
		if line == trimmed {
			inMeta = strings.HasPrefix(trimmed, "metadata:")
			continue
		}
		if inMeta && strings.HasPrefix(trimmed, "version:") {
			return unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "version:")))
		}
	}
	return ""
}

// unquote strips a single pair of matching surrounding quotes from a YAML
// scalar value (the template quotes the version string).
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
