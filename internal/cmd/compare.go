package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"civitai-cli/internal/api"
	"civitai-cli/internal/compare"
	"civitai-cli/internal/report"
)

// compareHelp is the curated `civ compare help` / `-h` page (without the
// shared global block, which writeCommandHelp appends). It documents the
// approved kind-first syntax, the 2-5 subject bound, the first-subject
// baseline rule, and the --diff/--json interaction.
const compareHelp = `compare two to five models or model versions side by side

Usage:
  civ compare models <modelId> <modelId> [<modelId>...] [flags]
  civ compare versions <versionId> <versionId> [<versionId>...] [flags]

Options:
  --diff  Hide confirmed-equal fields; show only changes and fields whose
          values could not be fully determined (not valid with --json)

Examples:
  civ compare models 2458426 260267
  civ compare versions 3108589 3263843 --diff
  civ compare models 2458426 260267 --json

Notes:
  Subjects keep input order and the first subject is the directional
  baseline for added/removed/from-to changes; it is not a recommendation.
  A version comparison fetches each selected version plus each distinct
  owning model (N+1 GETs when every version shares a parent). Missing API
  fields are shown as missing and compared as indeterminate, never as an
  empty value. JSON always carries facts plus derived changes and reports
  schemaVersion 1.

Exit codes:
  0 success   1 usage error or not found   2 network failure   4 token rejected
`

// Minimum and maximum subject counts (approved decisions D2).
const (
	compareMinSubjects = 2
	compareMaxSubjects = 5
)

// compareOptions carries the compare-specific flags.
type compareOptions struct {
	diff bool
}

// register binds the compare flags to fs.
func (o *compareOptions) register(fs *flag.FlagSet) {
	fs.BoolVar(&o.diff, "diff", false, "hide confirmed-equal fields; show only changes and indeterminate fields")
}

// Compare runs the compare command: civ compare models|versions <id> <id>
// [...]. It fetches canonical detail for every explicitly selected subject,
// builds one aligned comparison result, and renders either the full human
// view, the --diff view, or the stable --json contract. Subject order is
// input order and the first subject is the directional baseline.
func Compare(args []string) {
	var o compareOptions
	execute("compare", compareHelp, args, o.register, func(ctx context.Context, g *globals, pos []string) error {
		return runCompare(ctx, g, pos, &o)
	})
}

// runCompare is the pure compare implementation (testable without os.Exit):
// validate the complete invocation before any request, fetch sequentially
// with invocation-local parent deduplication, build the full Result, then
// render. Any selected or required-parent failure returns an error before
// stdout is written.
func runCompare(ctx context.Context, g *globals, pos []string, o *compareOptions) error {
	if len(pos) == 0 {
		return errors.New("compare requires a kind: civ compare models <id> <id> [...] or civ compare versions <id> <id> [...]")
	}
	kind := pos[0]
	ids, err := parseCompareIDs(kind, pos[1:])
	if err != nil {
		return err
	}
	if o.diff && g.JSON {
		return errors.New("--diff cannot be combined with --json; JSON always includes facts and derived changes")
	}

	c := newAPIClient(g)
	var res *compare.Result
	switch kind {
	case "models":
		res, err = fetchCompareModels(ctx, c, ids)
	case "versions":
		res, err = fetchCompareVersions(ctx, c, ids)
	}
	if err != nil {
		return err
	}
	if g.JSON {
		return report.WriteIndent(g.stdout(), res)
	}
	renderCompareResult(g.stdout(), res, o.diff)
	return nil
}

// parseCompareIDs validates the kind and the complete subject list before
// any network work: exactly one supported kind, 2-5 positive decimal IDs,
// and no duplicates. Errors are usage-class (exit 1).
func parseCompareIDs(kind string, args []string) ([]int64, error) {
	singular := ""
	switch kind {
	case "models":
		singular = "model"
	case "versions":
		singular = "version"
	default:
		return nil, fmt.Errorf("unknown comparison kind %q (want models or versions)", kind)
	}
	if len(args) < compareMinSubjects {
		return nil, fmt.Errorf("compare %s requires at least %d ids, e.g. civ compare %s <id> <id>", kind, compareMinSubjects, kind)
	}
	if len(args) > compareMaxSubjects {
		return nil, fmt.Errorf("compare %s accepts at most %d ids, got %d", kind, compareMaxSubjects, len(args))
	}
	ids := make([]int64, 0, len(args))
	seen := make(map[int64]bool, len(args))
	for _, arg := range args {
		id, err := strconv.ParseInt(arg, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid %s id %q (want a positive integer)", singular, arg)
		}
		if seen[id] {
			return nil, fmt.Errorf("duplicate %s id %d; compare subjects must be unique", singular, id)
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}

// fetchCompareModels fetches each selected model in input order and builds
// the model comparison Result.
func fetchCompareModels(ctx context.Context, c *api.Client, ids []int64) (*compare.Result, error) {
	subjects := make([]compare.ModelSubject, 0, len(ids))
	for _, id := range ids {
		raw, err := c.GetModel(ctx, int(id))
		if err != nil {
			return nil, fmt.Errorf("compare: model %d: %w", id, err)
		}
		snap, err := api.ParseCompareModel(raw)
		if err != nil {
			return nil, fmt.Errorf("compare: model %d: %w", id, err)
		}
		subjects = append(subjects, compare.ModelSubject{ID: id, Snapshot: snap})
	}
	return compare.BuildModels(subjects), nil
}

// fetchCompareVersions fetches each selected version plus each distinct
// owning model exactly once (invocation-local deduplication) and builds the
// version comparison Result. A version whose standalone detail carries no
// positive owning model id is an error under the strict fetch policy.
func fetchCompareVersions(ctx context.Context, c *api.Client, ids []int64) (*compare.Result, error) {
	parents := make(map[int64]*api.CompareModel)
	subjects := make([]compare.VersionSubject, 0, len(ids))
	for _, id := range ids {
		raw, err := c.GetVersion(ctx, int(id))
		if err != nil {
			return nil, fmt.Errorf("compare: version %d: %w", id, err)
		}
		snap, err := api.ParseCompareVersion(raw)
		if err != nil {
			return nil, fmt.Errorf("compare: version %d: %w", id, err)
		}
		if !snap.ModelID.HasValue() || snap.ModelID.Value <= 0 {
			return nil, fmt.Errorf("compare: version %d: response carries no owning model id", id)
		}
		parentID := snap.ModelID.Value
		parent, ok := parents[parentID]
		if !ok {
			parentRaw, err := c.GetModel(ctx, int(parentID))
			if err != nil {
				return nil, fmt.Errorf("compare: parent model %d for version %d: %w", parentID, id, err)
			}
			parent, err = api.ParseCompareModel(parentRaw)
			if err != nil {
				return nil, fmt.Errorf("compare: parent model %d for version %d: %w", parentID, id, err)
			}
			parents[parentID] = parent
		}
		subjects = append(subjects, compare.VersionSubject{ID: id, Snapshot: snap, Parent: parent})
	}
	return compare.BuildVersions(subjects), nil
}

// renderCompareResult writes the human view. The full view shows every
// registry field with per-value sources; the diff view keeps only changed
// and indeterminate fields.
func renderCompareResult(w io.Writer, res *compare.Result, diffOnly bool) {
	fmt.Fprintf(w, "COMPARE %s  (schemaVersion %d)\n", res.Kind, res.SchemaVersion)
	fmt.Fprintln(w, "Baseline: subject 1 (input order; not a recommendation)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "SUBJECTS")
	for _, s := range res.Subjects {
		fmt.Fprintf(w, "  %d  %s\n", s.Index+1, compareSubjectLine(res.Kind, s))
	}
	if diffOnly {
		renderCompareDiff(w, res)
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "FIELDS")
	for _, f := range res.Fields {
		writeCompareField(w, f)
	}
}

// compareSubjectLine renders one subject header line.
func compareSubjectLine(kind compare.Kind, s compare.Subject) string {
	name := s.Name
	if name == "" {
		name = "-"
	}
	if kind == compare.KindVersion {
		return fmt.Sprintf("version %d  %s  model %d  %s", s.VersionID, name, s.ModelID, s.URL)
	}
	return fmt.Sprintf("model %d  %s  %s", s.ModelID, name, s.URL)
}

// renderCompareDiff writes the --diff view: identity and confirmed-equal
// fields are hidden, changed fields and indeterminate fields are shown in
// separate sections.
func renderCompareDiff(w io.Writer, res *compare.Result) {
	var changed, indeterminate []compare.Field
	for _, f := range res.Fields {
		if compare.IsIdentityField(res.Kind, f.Key) {
			continue
		}
		switch f.Comparison {
		case compare.ComparisonDifferent:
			changed = append(changed, f)
		case compare.ComparisonIndeterminate:
			indeterminate = append(indeterminate, f)
		}
	}
	if len(changed) == 0 && len(indeterminate) == 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "No observed differences.")
		return
	}
	if len(changed) == 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "No confirmed differences; some fields could not be fully determined.")
	} else {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "CHANGES")
		for _, f := range changed {
			writeCompareField(w, f)
		}
	}
	if len(indeterminate) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "INDETERMINATE (missing data; not a confirmed difference)")
		for _, f := range indeterminate {
			writeCompareField(w, f)
		}
	}
}

// writeCompareField prints one aligned field: comparison/completeness, one
// value line per subject with its source, nested file facts, and any
// derived operation over the same values.
func writeCompareField(w io.Writer, f compare.Field) {
	note := ""
	if !f.Complete {
		note = " (incomplete)"
	}
	fmt.Fprintf(w, "\n  %s  [%s]%s\n", f.Key, f.Comparison, note)
	for i, v := range f.Values {
		fmt.Fprintf(w, "    %d: %s [%s]\n", i+1, compareValueText(v), v.Source)
	}
	if files, ok := compareFilesValue(f); ok {
		for i, list := range files {
			writeCompareFileFacts(w, i+1, list)
		}
	}
	if coverages, ok := compareRecipeValues(f); ok {
		for i, cov := range coverages {
			writeCompareRecipeFacts(w, i+1, cov)
		}
	}
	writeCompareDerived(w, f)
}

// compareFilesValue extracts the per-subject file lists of a fileList field
// so the human view can print nested facts.
func compareFilesValue(f compare.Field) ([][]compare.FileValue, bool) {
	out := make([][]compare.FileValue, len(f.Values))
	found := false
	for i, v := range f.Values {
		if list, ok := v.Value.([]compare.FileValue); ok {
			out[i] = list
			found = true
		}
	}
	return out, found
}

// compareRecipeValues extracts the per-subject recipe coverage objects so
// the human view can print them as readable key groups.
func compareRecipeValues(f compare.Field) ([]compare.RecipeCoverage, bool) {
	out := make([]compare.RecipeCoverage, len(f.Values))
	found := false
	for i, v := range f.Values {
		if cov, ok := v.Value.(compare.RecipeCoverage); ok {
			out[i] = cov
			found = true
		}
	}
	return out, found
}

// compareValueText renders one field value for the aligned fact lines.
func compareValueText(v compare.Value) string {
	switch v.State {
	case compare.StateMissing:
		return "<missing>"
	case compare.StateNull:
		return "null"
	}
	switch x := v.Value.(type) {
	case []compare.FileValue:
		return fmt.Sprintf("%d file(s)", len(x))
	case compare.RecipeCoverage:
		return fmt.Sprintf("%d image(s)", x.ImagesTotal)
	default:
		return compareScalarText(x)
	}
}

// compareScalarText renders a scalar/set value without quotes except for an
// empty string, which is otherwise invisible.
func compareScalarText(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		if x == "" {
			return `""`
		}
		return x
	case bool:
		return report.YesNo(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.Itoa(x)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case []string:
		return "[" + strings.Join(x, ", ") + "]"
	default:
		return fmt.Sprint(x)
	}
}

// compareOptionalText renders one presence-aware file attribute.
func compareOptionalText(o compare.Optional) string {
	if o.State != compare.StateValue {
		return "<" + string(o.State) + ">"
	}
	return compareScalarText(o.Value)
}

// writeCompareRecipeFacts prints recipe coverage as two readable groups so
// every count is attributable and absent hasMeta flags stay distinct.
func writeCompareRecipeFacts(w io.Writer, subject int, cov compare.RecipeCoverage) {
	fmt.Fprintf(w, "      subject %d recipe coverage:\n", subject)
	fmt.Fprintf(w, "        images %d  meta %d  declaredMissing %d  hasMeta true=%d false=%d null=%d missing=%d\n",
		cov.ImagesTotal, cov.MetaPresent, cov.DeclaredMetaMissing,
		cov.HasMetaTrue, cov.HasMetaFalse, cov.HasMetaNull, cov.HasMetaMissing)
	fmt.Fprintf(w, "        nonEmpty prompt=%d negativePrompt=%d steps=%d cfgScale=%d sampler=%d scheduler=%d seed=%d width=%d height=%d\n",
		cov.NonEmpty.Prompt, cov.NonEmpty.NegativePrompt, cov.NonEmpty.Steps, cov.NonEmpty.CFGScale,
		cov.NonEmpty.Sampler, cov.NonEmpty.Scheduler, cov.NonEmpty.Seed, cov.NonEmpty.Width, cov.NonEmpty.Height)
}

// writeCompareFileFacts prints the curated facts of one subject's files.
func writeCompareFileFacts(w io.Writer, subject int, files []compare.FileValue) {
	if len(files) == 0 {
		return
	}
	fmt.Fprintf(w, "      subject %d files:\n", subject)
	for _, f := range files {
		fmt.Fprintf(w, "        file %s  %s  %s\n",
			compareOptionalText(f.ID), compareOptionalText(f.Name), compareOptionalText(f.Type))
		size := compareOptionalText(f.SizeKB)
		if kb, ok := f.SizeKB.Value.(float64); f.SizeKB.State == compare.StateValue && ok {
			size = report.FormatSizeKB(kb)
		}
		fmt.Fprintf(w, "          primary %s  size %s  format %s  fp %s  sizeMetadata %s\n",
			compareOptionalText(f.Primary), size, compareOptionalText(f.Format),
			compareOptionalText(f.FP), compareOptionalText(f.Size))
		fmt.Fprintf(w, "          scan pickle %s  virus %s  at %s\n",
			compareOptionalText(f.PickleScanResult), compareOptionalText(f.VirusScanResult),
			compareOptionalText(f.ScannedAt))
		if f.HashesState != compare.StateValue {
			fmt.Fprintf(w, "          hashes <%s>\n", f.HashesState)
			continue
		}
		if len(f.Hashes) == 0 {
			fmt.Fprintln(w, "          hashes []")
			continue
		}
		for _, h := range f.Hashes {
			fmt.Fprintf(w, "          hash %s %s\n", h.Algorithm, h.Value)
		}
	}
}

// writeCompareDerived prints the derived operation stored on the field;
// the diff view never recalculates, it only reformats this same data.
func writeCompareDerived(w io.Writer, f compare.Field) {
	switch d := f.Derived.(type) {
	case nil:
		return
	case *compare.ScalarDiff:
		if d.State != compare.DerivedApplied {
			fmt.Fprintln(w, "      derived: indeterminate (operand missing or null)")
			return
		}
		for _, c := range d.Changes {
			fmt.Fprintf(w, "      derived: subject %d: %s -> %s\n",
				c.Subject+1, compareScalarText(c.From), compareScalarText(c.To))
		}
	case *compare.SetDiff:
		if d.State != compare.DerivedApplied {
			fmt.Fprintln(w, "      derived: indeterminate (operand missing or null)")
			return
		}
		for _, c := range d.Changes {
			var parts []string
			if len(c.Added) > 0 {
				parts = append(parts, "added "+strings.Join(c.Added, ", "))
			}
			if len(c.Removed) > 0 {
				parts = append(parts, "removed "+strings.Join(c.Removed, ", "))
			}
			if len(parts) == 0 {
				continue
			}
			fmt.Fprintf(w, "      derived: subject %d: %s\n", c.Subject+1, strings.Join(parts, "; "))
		}
	case *compare.NumberDiff:
		if d.State != compare.DerivedApplied {
			fmt.Fprintln(w, "      derived: indeterminate (operand missing or null)")
			return
		}
		for _, c := range d.Changes {
			fmt.Fprintf(w, "      derived: subject %d: %d -> %d (delta %+d)\n",
				c.Subject+1, c.From, c.To, c.Delta)
		}
	case *compare.FilesDiff:
		writeCompareFilesDerived(w, d)
	case *compare.RecipeDiff:
		if d.State != compare.DerivedApplied {
			fmt.Fprintln(w, "      derived: indeterminate (operand missing or null)")
			return
		}
		for _, c := range d.Changes {
			fmt.Fprintf(w, "      derived: subject %d: %s %d -> %d (delta %+d)\n",
				c.Subject+1, c.Field, c.From, c.To, c.Delta)
		}
	}
}

// writeCompareFilesDerived prints file pairing, metadata/hash changes, and
// unmatched registry files for every later subject.
func writeCompareFilesDerived(w io.Writer, d *compare.FilesDiff) {
	if len(d.Changes) == 0 {
		fmt.Fprintln(w, "      derived: indeterminate (files operand missing or null)")
		return
	}
	for _, ch := range d.Changes {
		if ch.State != compare.DerivedApplied {
			fmt.Fprintf(w, "      derived: subject %d: files indeterminate (missing or null)\n", ch.Subject+1)
			continue
		}
		if len(ch.Pairs) == 0 && len(ch.Added) == 0 && len(ch.Removed) == 0 {
			fmt.Fprintf(w, "      derived: subject %d: no file changes\n", ch.Subject+1)
			continue
		}
		fmt.Fprintf(w, "      derived: subject %d:\n", ch.Subject+1)
		for _, p := range ch.Pairs {
			fmt.Fprintf(w, "        match %s %s -> %s\n", p.MatchBasis,
				compareFileIDLabel(p.BaselineFileID), compareFileIDLabel(p.CandidateFileID))
			for _, c := range p.Changes {
				fmt.Fprintf(w, "          %s: %s -> %s\n", c.Field, compareScalarText(c.From), compareScalarText(c.To))
			}
			writeCompareHashDiff(w, p.Hashes)
		}
		for _, r := range ch.Added {
			fmt.Fprintf(w, "        added file %s %s (%s)%s\n",
				compareFileIDLabel(r.ID), r.Name, r.Type, compareReasonSuffix(r.Reason))
		}
		for _, r := range ch.Removed {
			fmt.Fprintf(w, "        removed file %s %s (%s)%s\n",
				compareFileIDLabel(r.ID), r.Name, r.Type, compareReasonSuffix(r.Reason))
		}
	}
}

// compareFileIDLabel renders a registry file ID, using "-" when the source
// did not supply a positive ID.
func compareFileIDLabel(id int64) string {
	if id <= 0 {
		return "-"
	}
	return strconv.FormatInt(id, 10)
}

// writeCompareHashDiff prints per-algorithm hash evidence of one pair.
func writeCompareHashDiff(w io.Writer, d compare.HashDiff) {
	if d.State != compare.DerivedApplied {
		fmt.Fprintln(w, "          hashes indeterminate (missing or null)")
		return
	}
	if len(d.Added) == 0 && len(d.Removed) == 0 && len(d.Changed) == 0 {
		return
	}
	for _, h := range d.Changed {
		fmt.Fprintf(w, "          hash %s: %s -> %s\n", h.Algorithm, h.From, h.To)
	}
	for _, h := range d.Added {
		fmt.Fprintf(w, "          hash %s added: %s\n", h.Algorithm, h.Value)
	}
	for _, h := range d.Removed {
		fmt.Fprintf(w, "          hash %s removed: %s\n", h.Algorithm, h.Value)
	}
}

// compareReasonSuffix renders an ambiguity reason for an unmatched file.
func compareReasonSuffix(reason string) string {
	if reason == "" {
		return ""
	}
	return " [" + reason + "]"
}
