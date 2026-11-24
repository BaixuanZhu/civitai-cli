package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"civitai-cli/internal/api"
	"civitai-cli/internal/report"
	"civitai-cli/internal/resolver"
)

// resolveHelp is the curated `civ resolve help` / `-h` page (without the
// shared global block, which writeCommandHelp appends). It documents the
// four accepted reference forms, the narrow meaning of READY, and the
// dedicated not-ready exit code. It has no command-specific flags: the
// only flag is the global --json.
const resolveHelp = `evaluate whether an explicitly declared set of model resources can work together

Usage:
  civ resolve <ref> [<ref>...] [flags]

  Accepted refs (one identity claim each; refs are independent):
    <path>            An explicit local regular file. Path syntax is
                      required: a separator, a leading "." or "~", a
                      Windows drive prefix, or a filename with an
                      extension (e.g. model.safetensors).
    <sha256>          A full 64-character SHA256 (content reference).
    <air>             An AIR URN with an exact @<versionId>, e.g.
                      urn:air:anima:checkpoint:civitai:2458426@3263843.
    version:<id>      An exact positive numeric version id.

  Bare ids, model ids, display names, AutoV2/short hashes, and AIR
  without a version are usage errors. No file is downloaded, scanned,
  rewritten, or indexed.

Examples:
  civ resolve ./models/checkpoint.safetensors ./models/lora.safetensors
  civ resolve C:\models\checkpoint.safetensors --json
  civ resolve version:3263843
  civ resolve urn:air:anima:checkpoint:civitai:2458426@3263843

Notes:
  READY requires every ref to be an explicit, present local file whose
  exact SHA256 matches one visible registry file, exactly one resolved
  Checkpoint anchor, and a reviewed compatible outcome for every
  dependent resource (LORA, TextualInversion, Controlnet, VAE). A
  remote-only ref (hash, AIR, or version:<id>) is diagnostic only: it is
  reported unverified and can never make the set ready. Missing base
  metadata and uncovered model-type tuples are unknown, never compatible.

Exit codes:
  0 ready   1 usage/local I/O   2 network failure   4 token rejected   5 not ready
`

// Resolve runs the resolve command: civ resolve <ref>... [--json]. It
// validates the four approved reference forms before any I/O, gathers
// read-only local and registry evidence, evaluates the compiled resolver
// policy, and prints a deterministic READY / NOT READY report. A
// completed not-ready evaluation exits 5 with the report still on stdout;
// usage and operational failures keep the shared 1/2/4 classes.
func Resolve(args []string) {
	executeWithExitCode("resolve", resolveHelp, args, nil, func(ctx context.Context, g *globals, pos []string) error {
		return runResolve(ctx, g, pos)
	}, resolveExitCode, resolveErrorHint)
}

// runResolve is the pure resolve implementation (testable without
// os.Exit): parse refs first (usage errors do zero I/O), build the
// resolver environment from the shared API client and hashing seam, run
// the pure evaluation, render, then signal a completed not-ready verdict
// with the dedicated sentinel error.
func runResolve(ctx context.Context, g *globals, pos []string) error {
	requests, err := resolver.ParseRequests(pos)
	if err != nil {
		return err
	}

	c := newAPIClient(g)
	env := resolver.Environment{
		Stat:     resolveStat,
		HashFile: computeHash,
		VersionByID: func(ctx context.Context, id int) (resolver.VersionMetadata, error) {
			raw, err := c.GetVersion(ctx, id)
			if err != nil {
				return resolver.VersionMetadata{}, err
			}
			return parseResolveVersion(raw)
		},
		VersionByHash: func(ctx context.Context, hash string) (resolver.VersionMetadata, error) {
			raw, err := c.GetVersionByHash(ctx, hash)
			if err != nil {
				return resolver.VersionMetadata{}, err
			}
			return parseResolveVersion(raw)
		},
		IsNotFound: func(err error) bool {
			var nfe *api.NotFoundError
			return errors.As(err, &nfe)
		},
	}

	rep, err := resolver.Resolve(ctx, requests, resolver.DefaultPolicy(), env)
	if err != nil {
		return err
	}

	if g.JSON {
		if err := report.WriteIndent(g.stdout(), rep); err != nil {
			return err
		}
	} else if err := renderResolveReport(g.stdout(), rep); err != nil {
		return err
	}
	if !rep.Ready {
		return &resolveNotReadyError{blocking: rep.Summary.BlockingFindingCount}
	}
	return nil
}

// resolveStat adapts os.Stat to the resolver's file-status contract: a
// missing path is a completed missing-resource result (nil error), while
// other stat failures stay operational.
func resolveStat(path string) (resolver.FileStatus, error) {
	fi, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return resolver.FileStatus{}, nil
		}
		return resolver.FileStatus{}, fmt.Errorf("resolve: stat %q: %w", path, err)
	}
	return resolver.FileStatus{Exists: true, Regular: fi.Mode().IsRegular()}, nil
}

// parseResolveVersion projects an api.ModelVersion into the resolver's
// metadata contract. Only the SHA256 hash entry is projected: resolve
// binds file identity by full SHA256, never by AutoV2 or file names.
func parseResolveVersion(raw []byte) (resolver.VersionMetadata, error) {
	v, err := api.ParseVersion(raw)
	if err != nil {
		return resolver.VersionMetadata{}, err
	}
	meta := resolver.VersionMetadata{
		ModelID:       v.ModelID,
		VersionID:     v.ID,
		AIR:           v.AIR,
		Name:          v.Name,
		BaseModel:     v.BaseModel,
		BaseModelType: v.BaseModelType,
	}
	if v.Model != nil {
		meta.ModelName = v.Model.Name
		meta.ModelType = v.Model.Type
	}
	for _, f := range v.Files {
		meta.Files = append(meta.Files, resolver.FileMetadata{
			FileID:  f.ID,
			Name:    f.Name,
			Type:    f.Type,
			Primary: f.Primary,
			SHA256:  f.Hashes["SHA256"],
		})
	}
	return meta, nil
}

// renderResolveReport writes the deterministic human report: the verdict
// first, then per-resource identity/local evidence in input order,
// compatibility outcomes, and every blocking finding with its code and
// structured expected/actual evidence. Full ids and hashes are never
// truncated.
func renderResolveReport(w io.Writer, rep resolver.Report) error {
	var b strings.Builder
	verdict := "READY"
	if !rep.Ready {
		verdict = "NOT READY"
	}
	fmt.Fprintf(&b, "%s\n", verdict)
	fmt.Fprintf(&b, "policy %s   schema %d   resources %d   blocking findings %d\n\n",
		rep.PolicyVersion, rep.SchemaVersion, rep.Summary.ResourceCount, rep.Summary.BlockingFindingCount)

	fmt.Fprintf(&b, "RESOURCES (%d)\n", len(rep.Resources))
	for _, r := range rep.Resources {
		fmt.Fprintf(&b, "  [%d] %s\n", r.InputIndex, r.RequestedRef)
		switch {
		case r.Resolution == resolver.ResolutionResolved || r.Identity.VersionID > 0:
			fmt.Fprintf(&b, "      identity: model %d / version %d", r.Identity.ModelID, r.Identity.VersionID)
			if r.Identity.FileID > 0 {
				fmt.Fprintf(&b, " / file %d", r.Identity.FileID)
			}
			b.WriteString("\n")
		default:
			fmt.Fprintf(&b, "      identity: %s\n", r.Resolution)
		}
		if r.Names.Model != "" || r.Names.Version != "" || r.Names.File != "" {
			fmt.Fprintf(&b, "      names: %s / %s / %s\n",
				orDash(r.Names.Model), orDash(r.Names.Version), orDash(r.Names.File))
		}
		if r.Identity.ModelType != "" || r.Identity.BaseModel != "" || r.Identity.BaseModelType != "" {
			base := orDash(r.Identity.BaseModel)
			if r.Identity.BaseModelType != "" {
				base += " (" + r.Identity.BaseModelType + ")"
			}
			fmt.Fprintf(&b, "      type: %s   base: %s\n", orDash(r.Identity.ModelType), base)
		}
		if r.Identity.AIR != "" {
			fmt.Fprintf(&b, "      air: %s\n", r.Identity.AIR)
		}
		local := string(r.Local.State)
		if r.Local.Path != "" {
			local += " (" + r.Local.Path + ")"
		}
		fmt.Fprintf(&b, "      local: %s\n", local)
		if r.Local.ComputedSHA256 != "" {
			fmt.Fprintf(&b, "      sha256: %s\n", r.Local.ComputedSHA256)
		}
		if len(r.VisibleFiles) > 0 {
			b.WriteString("      visible files:\n")
			for _, f := range r.VisibleFiles {
				fmt.Fprintf(&b, "        %d  name=%s  type=%s  primary=%s  sha256=%s\n",
					f.FileID, orDash(f.Name), orDash(f.Type), report.YesNo(f.Primary), orDash(f.SHA256))
			}
		}
	}

	fmt.Fprintf(&b, "\nCOMPATIBILITY (%d)\n", len(rep.Compatibility))
	for _, c := range rep.Compatibility {
		fmt.Fprintf(&b, "  [%d] %s (base %s) -> [%d] %s (base %s): %s   rule %s\n",
			c.LeftIndex, orDash(c.Left.ModelType), orDash(c.Left.BaseModel),
			c.RightIndex, orDash(c.Right.ModelType), orDash(c.Right.BaseModel),
			c.Status, c.RuleID)
		if c.Reason != "" {
			fmt.Fprintf(&b, "      %s\n", c.Reason)
		}
	}

	fmt.Fprintf(&b, "\nFINDINGS (%d)\n", len(rep.Findings))
	for _, f := range rep.Findings {
		fmt.Fprintf(&b, "  %s  resources %v: %s", f.Code, f.ResourceIndexes, f.Message)
		if f.RuleID != "" {
			fmt.Fprintf(&b, "  (rule %s)", f.RuleID)
		}
		b.WriteString("\n")
		if f.Evidence.Expected != nil || f.Evidence.Actual != nil {
			fmt.Fprintf(&b, "      expected %s; actual %s\n",
				resolveIdentityText(f.Evidence.Expected), resolveIdentityText(f.Evidence.Actual))
		}
	}

	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("resolve: write report: %w", err)
	}
	return nil
}

// resolveIdentityText renders one identity for human finding evidence
// (full ids and hashes, never truncated).
func resolveIdentityText(id *resolver.Identity) string {
	if id == nil {
		return "-"
	}
	var parts []string
	if id.ModelID > 0 {
		parts = append(parts, fmt.Sprintf("model %d", id.ModelID))
	}
	if id.VersionID > 0 {
		parts = append(parts, fmt.Sprintf("version %d", id.VersionID))
	}
	if id.FileID > 0 {
		parts = append(parts, fmt.Sprintf("file %d", id.FileID))
	}
	if id.SHA256 != "" {
		parts = append(parts, "sha256 "+id.SHA256)
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " / ")
}

// resolveNotReadyError signals a completed evaluation whose report was
// already written: NOT READY is a result, not a usage or operational
// failure. The dedicated exit code keeps it distinct from vet's 3 and
// the shared 1/2/4 classes.
type resolveNotReadyError struct {
	blocking int
}

// Error reports the concise verdict line for the runner's stderr
// diagnostic; the report itself stays on stdout.
func (e *resolveNotReadyError) Error() string {
	return fmt.Sprintf("resolve: set is not ready (%d blocking finding(s))", e.blocking)
}

// resolveExitCode maps a completed NOT READY evaluation to exit 5 and
// delegates every operational class to the unchanged shared mapping.
func resolveExitCode(err error, hasToken bool) int {
	var nr *resolveNotReadyError
	if errors.As(err, &nr) {
		return 5
	}
	return exitCodeFor(err, hasToken)
}

// resolveErrorHint prints no extra hint for a completed NOT READY verdict
// (the report on stdout is the actionable result) and keeps the shared
// hints for operational failures.
func resolveErrorHint(err error, hasToken bool) {
	var nr *resolveNotReadyError
	if errors.As(err, &nr) {
		return
	}
	printErrorHint(err, hasToken)
}
