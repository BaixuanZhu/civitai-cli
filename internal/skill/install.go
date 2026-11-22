package skill

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"civitai-cli/internal/app"
)

// Result actions reported per platform. They are stable strings consumed by the
// cmd layer's human summary and --json output.
const (
	// ActionInstalled marks a fresh write or an overwrite.
	ActionInstalled = "installed"
	// ActionUpdated marks an update that rewrote a differing or forced version.
	ActionUpdated = "updated"
	// ActionUpToDate marks an update that found the same version and wrote nothing.
	ActionUpToDate = "up-to-date"
	// ActionSkipped marks a conflict skipped, or a remove that found nothing.
	ActionSkipped = "skipped"
	// ActionRemoved marks a remove that deleted the skill directory.
	ActionRemoved = "removed"
	// ActionBackedUp marks a conflict backed up to .bak, then installed.
	ActionBackedUp = "backed-up"
	// ActionNotFound marks an update that found no installed skill.
	ActionNotFound = "not-found"
)

// ConflictFunc decides how install handles an existing skill directory. It
// receives the target path and the installed metadata.version ("" when unknown)
// and returns one of "overwrite", "skip", or "backup". The cmd layer injects an
// interactive prompt; non-interactive callers pass nil (skip) or set Force.
// Keeping the decision behind a callback leaves this package free of stdin.
type ConflictFunc func(path, installedVersion string) (action string)

// Options configures an Install/Remove/Update run. Out receives progress and
// warning lines (stderr in production; nil discards). Conflict is consulted only
// by Install when a target exists and Force is false.
type Options struct {
	Platforms []Platform
	Scope     Scope
	Force     bool
	Out       io.Writer
	Conflict  ConflictFunc
}

// Result is the per-platform outcome of a skill operation, serialized for --json.
type Result struct {
	Platform Platform `json:"platform"`
	Path     string   `json:"path"`
	Action   string   `json:"action"`
}

// Install writes the bundled civ-prompt skill into each platform's native root.
// A fresh target is created (mkdir -p) and populated from the embedded tree with
// the version rendered. An existing target without Force consults Conflict
// (overwrite/skip/backup); with Force it is overwritten. backup renames the old
// directory to <path>.bak before writing. Each platform yields one Result.
func Install(opts Options) ([]Result, error) {
	var results []Result
	for _, p := range opts.Platforms {
		dir, err := targetDir(p, opts.Scope)
		if err != nil {
			return results, err
		}
		res := Result{Platform: p, Path: dir}
		switch {
		case !dirExists(dir):
			if err := writeSkill(dir); err != nil {
				return results, err
			}
			res.Action = ActionInstalled
		case opts.Force:
			if err := writeSkill(dir); err != nil {
				return results, err
			}
			res.Action = ActionInstalled
		default:
			action := "skip"
			if opts.Conflict != nil {
				action = opts.Conflict(dir, installedVersion(dir))
			}
			switch action {
			case "overwrite":
				if err := writeSkill(dir); err != nil {
					return results, err
				}
				res.Action = ActionInstalled
			case "backup":
				if err := backupDir(dir); err != nil {
					return results, err
				}
				progressf(opts.Out, "backed up existing skill to %s.bak\n", dir)
				if err := writeSkill(dir); err != nil {
					return results, err
				}
				res.Action = ActionBackedUp
			default: // skip
				res.Action = ActionSkipped
			}
		}
		results = append(results, res)
	}
	return results, nil
}

// Remove deletes each platform's civ-prompt directory (os.RemoveAll). A missing
// target is a no-op (skipped). Only <root>/civ-prompt is removed; sibling files
// such as zcode's .skills_store_lock.json are never touched.
func Remove(opts Options) ([]Result, error) {
	var results []Result
	for _, p := range opts.Platforms {
		dir, err := targetDir(p, opts.Scope)
		if err != nil {
			return results, err
		}
		res := Result{Platform: p, Path: dir}
		if !dirExists(dir) {
			res.Action = ActionSkipped
		} else {
			if err := os.RemoveAll(dir); err != nil {
				return results, fmt.Errorf("skill: remove %s: %w", dir, err)
			}
			res.Action = ActionRemoved
		}
		results = append(results, res)
	}
	return results, nil
}

// Update rewrites an installed skill when its metadata.version differs from the
// running app.Version (or Force is set), preserving the original name/directory.
// A matching version is reported up-to-date and left untouched. A missing target
// is reported not-found (the cmd layer hints to install first).
func Update(opts Options) ([]Result, error) {
	var results []Result
	for _, p := range opts.Platforms {
		dir, err := targetDir(p, opts.Scope)
		if err != nil {
			return results, err
		}
		res := Result{Platform: p, Path: dir}
		switch {
		case !dirExists(dir):
			res.Action = ActionNotFound
		case installedVersion(dir) == app.Version && !opts.Force:
			res.Action = ActionUpToDate
		default:
			if err := writeSkill(dir); err != nil {
				return results, err
			}
			res.Action = ActionUpdated
		}
		results = append(results, res)
	}
	return results, nil
}

// targetDir resolves the full skill directory (<root>/civ-prompt) for a platform
// and scope, propagating RootFor's validation errors.
func targetDir(p Platform, s Scope) (string, error) {
	root, err := RootFor(p, s)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, SkillName), nil
}

// writeSkill creates dir and copies the embedded skill tree into it, rendering
// the version placeholder in SKILL.md and copying references/ verbatim. Real
// copies are written (no symlinks) so Windows needs no elevated privileges.
func writeSkill(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("skill: create %s: %w", dir, err)
	}
	return fs.WalkDir(FS, embedRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, embedRoot), "/")
		if rel == "" {
			return nil // the embedded root directory itself
		}
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if d.IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("skill: create %s: %w", target, err)
			}
			return nil
		}
		data, err := FS.ReadFile(p)
		if err != nil {
			return fmt.Errorf("skill: read embedded %s: %w", p, err)
		}
		if rel == "SKILL.md" {
			data = renderVersion(data)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("skill: create %s: %w", filepath.Dir(target), err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return fmt.Errorf("skill: write %s: %w", target, err)
		}
		return nil
	})
}

// backupDir renames an existing skill directory to <dir>.bak, clearing any stale
// backup first so the rename never collides.
func backupDir(dir string) error {
	bak := dir + ".bak"
	if err := os.RemoveAll(bak); err != nil {
		return fmt.Errorf("skill: clear stale backup %s: %w", bak, err)
	}
	if err := os.Rename(dir, bak); err != nil {
		return fmt.Errorf("skill: backup %s: %w", dir, err)
	}
	return nil
}

// dirExists reports whether dir is an existing directory.
func dirExists(dir string) bool {
	fi, err := os.Stat(dir)
	return err == nil && fi.IsDir()
}

// progressf writes a progress/warning line to w when non-nil (nil discards), so
// the package never writes to os.Stderr directly and stays test-injectable.
func progressf(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, format, args...)
}
