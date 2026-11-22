package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"civitai-cli/internal/api"
	"civitai-cli/internal/app"
	"civitai-cli/internal/hashcache"
	"civitai-cli/internal/report"
)

// Vet runs the vet command: civ vet <path|sha256|autov2>. It looks the
// hash up in the Civitai registry (by-hash endpoint) and prints a review
// report: model/version/creator header, trigger words, files with full
// hashes and scan results, license grants, and example-image counts.
// Exit codes (plan §4.3): 0 hit, 3 hash not registered, 1 usage error,
// 2 network failure, 4 rejected token. --prompts appends the example-image
// generation recipes (M3).
func Vet(args []string) {
	var o vetOptions
	executeWithExitCode("vet", args, o.register, func(ctx context.Context, g *globals, pos []string) error {
		return runVet(ctx, g, pos, &o)
	}, vetExitCode, vetErrorHint)
}

// vetOptions carries the vet-specific flags.
type vetOptions struct {
	prompts bool
}

// register binds the vet flags to fs.
func (o *vetOptions) register(fs *flag.FlagSet) {
	fs.BoolVar(&o.prompts, "prompts", false, "append example-image generation recipes (prompt/negativePrompt/steps/cfgScale/sampler/scheduler)")
}

// vetInputKind labels the three accepted forms of a vet argument.
type vetInputKind int

const (
	vetInputPath   vetInputKind = iota // local file path (default form)
	vetInputSHA256                     // full SHA256: 64 hex chars
	vetInputAutoV2                     // AutoV2: first 10 hex chars of SHA256
)

// classifyInput labels a vet argument and normalizes hash forms to
// uppercase hex (the API's canonical form): 64 hex chars → SHA256,
// 10 hex chars → AutoV2, anything else → local path. Hex matching is
// case-insensitive; wrong-length hex strings fall through to path
// semantics (the caller's existence check rejects them, exit 1). The
// function is pure — no filesystem access — so it is table-testable.
func classifyInput(arg string) (vetInputKind, string) {
	if isHex(arg) {
		switch len(arg) {
		case 64:
			return vetInputSHA256, strings.ToUpper(arg)
		case 10:
			return vetInputAutoV2, strings.ToUpper(arg)
		}
	}
	return vetInputPath, arg
}

// isHex reports whether s is a non-empty run of hex digits (either case).
func isHex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

// runVet is the pure vet implementation (testable without os.Exit):
// classify the argument, hash local files streaming, query by-hash,
// fetch the owning model for creator/license context, render the report.
// --prompts appends the example-image recipes (text) or focuses the JSON
// output on the recipe view.
func runVet(ctx context.Context, g *globals, pos []string, o *vetOptions) error {
	if len(pos) != 1 {
		return errors.New("vet requires exactly one argument: a file path, a 64-char SHA256, or a 10-char AutoV2")
	}
	arg := pos[0]
	kind, hash := classifyInput(arg)
	if kind == vetInputPath {
		fi, err := os.Stat(arg)
		if err != nil {
			return fmt.Errorf("%q is neither an existing file nor a valid hash (want a file path, a 64-char SHA256, or a 10-char AutoV2)", arg)
		}
		if fi.IsDir() {
			return fmt.Errorf("%q is a directory; vet needs a file", arg)
		}
		app.Infof("hashing %s", arg)
		hash, err = hashcache.ComputeSHA256(arg)
		if err != nil {
			return err
		}
		app.Debugf("computed sha256 %s", hash)
	}

	c := newAPIClient(g)
	raw, err := c.GetVersionByHash(ctx, hash)
	if err != nil {
		return err // NotFoundError → vetExitCode → exit 3 with the vet hint
	}
	v, err := api.ParseVersion(raw)
	if err != nil {
		return err
	}
	m := fetchVetModel(ctx, c, v.ModelID)

	if g.JSON {
		if o.prompts {
			return renderPrompts(g.stdout(), v.Images, true)
		}
		return report.WriteIndent(g.stdout(), newVetReport(hash, v, m))
	}
	if err := renderVetReport(g.stdout(), v, m); err != nil {
		return err
	}
	if o.prompts {
		return renderPrompts(g.stdout(), v.Images, false)
	}
	return nil
}

// vetExitCode maps vet failures to exit codes (plan §4.3): a 404 means
// the hash is not registered on Civitai → exit 3. Every other class
// keeps the global mapping (1 usage, 2 network, 4 rejected token).
func vetExitCode(err error, hasToken bool) int {
	var nfe *api.NotFoundError
	if errors.As(err, &nfe) {
		return 3
	}
	return exitCodeFor(err, hasToken)
}

// vetErrorHint replaces the generic not-found guidance with vet-specific
// advice (the input is a path/hash, not an id); other error classes keep
// the shared hints.
func vetErrorHint(err error, hasToken bool) {
	var nfe *api.NotFoundError
	if errors.As(err, &nfe) {
		fmt.Fprintln(os.Stderr, "hint: this hash is not registered on Civitai; the file may not originate from Civitai or may have been modified")
		return
	}
	printErrorHint(err, hasToken)
}

// fetchVetModel best-effort fetches the owning model for the CREATOR and
// LICENSE blocks: by-hash returns a version object whose model summary
// carries only {name,type,nsfw,poi} — creator, license grants, and
// comment counts live on the model (verified live 2026-11-22, see
// testdata/FIXTURES.md). The by-hash hit is vet's source of truth, so a
// failure here degrades the report with a stderr warning instead of
// failing the command (a model-endpoint 404 must not turn a hit into
// exit 3). Returns nil when unavailable.
func fetchVetModel(ctx context.Context, c *api.Client, id int) *api.Model {
	if id <= 0 {
		app.Warnf("by-hash response carries no modelId; creator and license will be omitted")
		return nil
	}
	raw, err := c.GetModel(ctx, id)
	if err != nil {
		app.Warnf("model %d details unavailable (%v); creator and license will be omitted", id, err)
		return nil
	}
	m, err := api.ParseModel(raw)
	if err != nil {
		app.Warnf("model %d details undecodable (%v); creator and license will be omitted", id, err)
		return nil
	}
	return m
}

// renderVetReport prints the plan §4.3 review report: MODEL/VERSION/
// CREATOR header, TRIGGER WORDS plain-text block (one word per line, no
// quotes or bullets — directly copyable), FILES with full SHA256/AutoV2
// and scan results, the LICENSE four-grant line, and EXAMPLES counts.
// DL/ratio describe the vetted version; comments are model-level (version
// stats carry no commentCount). m may be nil (degraded model fetch).
func renderVetReport(w io.Writer, v *api.ModelVersion, m *api.Model) error {
	width := report.ConsoleWidth()

	modelName := "-"
	if m != nil && m.Name != "" {
		modelName = m.Name
	} else if v.Model != nil && v.Model.Name != "" {
		modelName = v.Model.Name
	}
	modelURL := "-"
	if v.ModelID > 0 {
		modelURL = fmt.Sprintf("https://civitai.com/models/%d", v.ModelID)
	}
	creator, comments := "-", "-"
	if m != nil {
		if m.Creator.Username != "" {
			creator = m.Creator.Username
		}
		comments = strconv.FormatInt(m.Stats.CommentCount, 10)
	}
	versionName := orDash(v.Name)
	published := orDash(report.DateText(v.PublishedAt))
	base := orDash(v.BaseModel)

	fmt.Fprintf(w, "MODEL     %s   %s\n", report.Truncate(modelName, max(width-50, 16)), modelURL)
	fmt.Fprintf(w, "VERSION   %s (%s)   base: %s\n", report.Truncate(versionName, max(width-34-len(base), 12)), published, base)
	fmt.Fprintf(w, "CREATOR   %s   DL %s   ratio %s   comments %s\n",
		report.Truncate(creator, max(width-48, 12)),
		report.CompactNumber(v.Stats.DownloadCount),
		report.RatioText(v.Stats.ThumbsUpCount, v.Stats.ThumbsDownCount),
		comments)

	fmt.Fprintf(w, "\nTRIGGER WORDS (%d)\n", len(v.TrainedWords))
	for _, word := range v.TrainedWords {
		fmt.Fprintln(w, word)
	}

	fmt.Fprintf(w, "\nFILES (%d)\n", len(v.Files))
	nameCap := max(width-40, 16)
	for _, f := range v.Files {
		formatFP := strings.TrimSpace(f.Metadata.Format + " " + f.Metadata.FP)
		primary := "-"
		if f.Primary {
			primary = "primary"
		}
		fmt.Fprintf(w, "  %s  %s  %s  %s\n",
			report.Truncate(orDash(f.Name), nameCap),
			report.FormatSizeKB(f.SizeKB),
			orDash(formatFP),
			primary)
		fmt.Fprintf(w, "  %-8s %s\n", "sha256", orDash(f.Hashes["SHA256"]))
		fmt.Fprintf(w, "  %-8s %s\n", "autov2", orDash(f.Hashes["AutoV2"]))
		fmt.Fprintf(w, "  %-8s pickle: %s   virus: %s\n", "scan", orDash(f.PickleScanResult), orDash(f.VirusScanResult))
	}

	if m != nil {
		// licenseLines (show.go) packs the four grants into console-width
		// lines; continuations align under the value column (key width 10).
		for i, line := range licenseLines(m, width-10) {
			if i == 0 {
				fmt.Fprintf(w, "\nLICENSE   %s\n", line)
			} else {
				fmt.Fprintf(w, "          %s\n", line)
			}
		}
	} else {
		fmt.Fprint(w, "\nLICENSE   unknown (model details unavailable)\n")
	}

	ex := countExamples(v.Images)
	fmt.Fprintf(w, "\nEXAMPLES  %d (mature %d)   --prompts for example recipes\n", ex.Total, ex.Mature)
	return nil
}

// orDash renders missing values as "-" so drifted/absent fields stay
// visible instead of leaving blank holes in the report.
func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// nsfwAdultBits masks the Mature (4) and X (8) bits of the numeric
// nsfwLevel bitmask (plan appendix C). Unknown higher bits are not
// counted as mature.
const nsfwAdultBits int64 = 4 | 8

// countExamples tallies a version's example images: total, mature-or-X
// (numeric nsfwLevel bitmask; string-form drift is not counted), and
// images carrying generation meta (meta can be null even when hasMeta
// is true — design D4).
func countExamples(images []api.Image) vetExamples {
	ex := vetExamples{Total: len(images)}
	for _, im := range images {
		if lvl, err := im.NSFWLevel.Int64(); err == nil && lvl&nsfwAdultBits != 0 {
			ex.Mature++
		}
		if im.Meta != nil {
			ex.WithMeta++
		}
	}
	return ex
}

// vetReport is the --json review object: the queried hash (uppercase;
// the computed SHA256 for path input), the full decoded version (files
// with every hash under version.files, trigger words, example images),
// the owning-model summary (creator, license grants, stats, URL), and
// example-image counts. Model is null when the model fetch degraded.
type vetReport struct {
	Hash     string            `json:"hash"`
	Version  *api.ModelVersion `json:"version"`
	Model    *vetModelSummary  `json:"model"`
	Examples vetExamples       `json:"examples"`
}

// vetModelSummary is the owning-model context of a vet hit.
type vetModelSummary struct {
	ID      int        `json:"id"`
	Name    string     `json:"name"`
	Type    string     `json:"type"`
	URL     string     `json:"url"`
	Creator string     `json:"creator"`
	License vetLicense `json:"license"`
	Stats   api.Stats  `json:"stats"`
}

// vetLicense carries the four license grants under their API field names
// so scripts can consume them without a vocabulary translation.
type vetLicense struct {
	AllowNoCredit         bool     `json:"allowNoCredit"`
	AllowCommercialUse    []string `json:"allowCommercialUse"`
	AllowDerivatives      bool     `json:"allowDerivatives"`
	AllowDifferentLicense bool     `json:"allowDifferentLicense"`
}

// vetExamples is the example-image tally of a version.
type vetExamples struct {
	Total    int `json:"total"`
	Mature   int `json:"mature"`
	WithMeta int `json:"withMeta"`
}

// newVetReport assembles the --json review object from the by-hash hit
// and the optional owning-model details.
func newVetReport(hash string, v *api.ModelVersion, m *api.Model) vetReport {
	r := vetReport{Hash: hash, Version: v, Examples: countExamples(v.Images)}
	if m != nil {
		r.Model = &vetModelSummary{
			ID:      m.ID,
			Name:    m.Name,
			Type:    m.Type,
			URL:     fmt.Sprintf("https://civitai.com/models/%d", m.ID),
			Creator: m.Creator.Username,
			License: vetLicense{
				AllowNoCredit:         m.AllowNoCredit,
				AllowCommercialUse:    m.AllowCommercialUse,
				AllowDerivatives:      m.AllowDerivatives,
				AllowDifferentLicense: m.AllowDifferentLicense,
			},
			Stats: m.Stats,
		}
	}
	return r
}
