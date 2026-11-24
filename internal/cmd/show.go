package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"civitai-cli/internal/api"
	"civitai-cli/internal/report"
)

// showOptions carries the show-specific flags. -v/--ver drill into a
// version; the global debug flag is --verbose (no short alias) so the
// two never collide (design D6). --prompts appends example-image recipes
// to the version drill-down (M3). --view selects the JSON projection and
// is meaningful only with --json (compact is the implicit default).
type showOptions struct {
	ver     int
	prompts bool
	view    jsonViewFlag
}

// register binds the show flags to fs.
func (o *showOptions) register(fs *flag.FlagSet) {
	o.view.defaultCompact()
	fs.Var(&o.view, "view", "JSON view: compact (default, summary) or full (legacy decoded data); requires --json and conflicts with --prompts")
	fs.IntVar(&o.ver, "v", 0, "drill into this version id (files, hashes, trigger words)")
	fs.IntVar(&o.ver, "ver", 0, "alias of -v")
	fs.BoolVar(&o.prompts, "prompts", false, "with -v, append example-image generation recipes (prompt/negativePrompt/steps/cfgScale/sampler/scheduler)")
}

// showHelp is the curated `civ show help` / `-h` page (without the shared
// global block, which writeCommandHelp appends). It documents the -v/--ver
// drill-down and that --prompts requires -v (runShow enforces the pair).
const showHelp = `show a model's header and version table

Usage:
  civ show <modelId> [-v <versionId>] [flags]

Options:
  -v <id>          Drill into this version: files, full hashes, trigger
                   words, and scan results
  --ver <id>       Alias of -v
  --prompts        With -v, append example-image generation recipes
                   (prompt, negative prompt, steps, cfgScale, sampler,
                   scheduler); requires -v <versionId>

Examples:
  civ show 182296
  civ show 182296 -v 482391
  civ show 182296 --ver 482391 --prompts --json
`

// Show runs the show command: civ show <modelId> [-v <versionId>].
func Show(args []string) {
	var o showOptions
	execute("show", showHelp, args, o.register, func(ctx context.Context, g *globals, pos []string) error {
		return runShow(ctx, g, pos, &o)
	})
}

// runShow is the pure show implementation (testable without os.Exit).
func runShow(ctx context.Context, g *globals, pos []string, o *showOptions) error {
	if len(pos) != 1 {
		return errors.New("show requires exactly one model id, e.g. civ show 2458426")
	}
	id, err := strconv.Atoi(pos[0])
	if err != nil || id <= 0 {
		return fmt.Errorf("invalid model id %q (want a positive integer)", pos[0])
	}
	view, err := o.view.selected()
	if err != nil {
		return err
	}
	switch {
	case o.view.set && o.prompts:
		return errors.New("--view cannot be combined with --prompts; --prompts is its own recipe view")
	case o.view.set && !g.JSON:
		return errors.New("--view requires --json; human output is already a curated summary")
	case o.prompts && o.ver <= 0:
		return errors.New("--prompts requires -v <versionId> to select a version")
	}
	c := newAPIClient(g)

	if o.ver > 0 {
		raw, err := c.GetVersion(ctx, o.ver)
		if err != nil {
			return err
		}
		v, err := api.ParseVersion(raw)
		if err != nil {
			return err
		}
		if v.ModelID != 0 && v.ModelID != id {
			return fmt.Errorf("version %d belongs to model %d, not %d", o.ver, v.ModelID, id)
		}
		if g.JSON {
			// --prompts focuses the machine output on the recipe view; the
			// full version JSON (which already embeds images[].meta) remains
			// available without --prompts.
			if o.prompts {
				return renderPrompts(g.stdout(), v.Images, true)
			}
			if view == "full" {
				// Legacy escape path: the decoded version object exactly as
				// the pre-compact default rendered it.
				return report.WriteIndent(g.stdout(), v)
			}
			env, err := api.ParseCompactVersion(raw)
			if err != nil {
				return err
			}
			return report.WriteIndent(g.stdout(), env)
		}
		if err := renderVersionDetail(g.stdout(), id, v); err != nil {
			return err
		}
		if o.prompts {
			return renderPrompts(g.stdout(), v.Images, false)
		}
		return nil
	}

	raw, err := c.GetModel(ctx, id)
	if err != nil {
		return err
	}
	m, err := api.ParseModel(raw)
	if err != nil {
		return err
	}
	if g.JSON {
		if view == "full" {
			// Legacy escape path: the decoded model object exactly as the
			// pre-compact default rendered it.
			return report.WriteIndent(g.stdout(), m)
		}
		env, err := api.ParseCompactModel(raw)
		if err != nil {
			return err
		}
		return report.WriteIndent(g.stdout(), env)
	}
	return renderModel(g.stdout(), m)
}

// renderModel prints the model header block plus the version table.
func renderModel(w io.Writer, m *api.Model) error {
	width := report.ConsoleWidth()
	kv := report.NewTableWriter(w)
	fmt.Fprintf(kv, "MODEL\t%s\n", m.Name)
	fmt.Fprintf(kv, "CREATOR\t%s\n", m.Creator.Username)
	fmt.Fprintf(kv, "TYPE\t%s\n", m.Type)
	// Tags can be dozens; keep the header line within the console budget
	// (the full list remains available via --json).
	fmt.Fprintf(kv, "TAGS\t%s\n", report.Truncate(strings.Join(m.Tags, ", "), max(width-9, 20)))
	for i, line := range licenseLines(m, width-9) {
		if i == 0 {
			fmt.Fprintf(kv, "LICENSE\t%s\n", line)
		} else {
			// Continuation lines align under the value column (key
			// column width 9); they carry no tab and do not disturb
			// tabwriter alignment.
			fmt.Fprintf(kv, "%s%s\n", strings.Repeat(" ", 9), line)
		}
	}
	fmt.Fprintf(kv, "STATS\tDL %s   ratio %s   comments %d   tips %s\n",
		report.CompactNumber(m.Stats.DownloadCount),
		report.RatioText(m.Stats.ThumbsUpCount, m.Stats.ThumbsDownCount),
		m.Stats.CommentCount,
		report.CompactNumber(m.Stats.TippedAmountCount))
	updated := report.DateText(latestPublishedAt(*m))
	if updated == "" {
		updated = "-"
	}
	fmt.Fprintf(kv, "UPDATED\t%s\n", updated)
	fmt.Fprintf(kv, "URL\thttps://civitai.com/models/%d\n", m.ID)
	if err := kv.Flush(); err != nil {
		return err
	}

	fmt.Fprintf(w, "\nVERSIONS (%d)\n", len(m.ModelVersions))
	// Fixed columns: ID 8, BASE 12, PUBLISHED 10, DL 7, RATIO 7,
	// TRIGGERS 8, plus 2*6 padding = 64; NAME gets the remainder.
	nameCap := max(width-64, 12)
	tw := report.NewTableWriter(w)
	fmt.Fprintln(tw, "ID\tNAME\tBASE\tPUBLISHED\tDL\tRATIO\tTRIGGERS")
	for _, v := range m.ModelVersions {
		triggers := "-" // embedded versions may omit trainedWords (nil ≠ empty)
		if v.TrainedWords != nil {
			triggers = strconv.Itoa(len(v.TrainedWords))
		}
		published := report.DateText(v.PublishedAt)
		if published == "" {
			published = "-"
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
			v.ID,
			report.Truncate(v.Name, nameCap),
			report.Truncate(v.BaseModel, 12),
			published,
			report.CompactNumber(v.Stats.DownloadCount),
			report.RatioText(v.Stats.ThumbsUpCount, v.Stats.ThumbsDownCount),
			triggers,
		)
	}
	return tw.Flush()
}

// licenseLines renders the four license flags, greedily packed into
// lines of at most budget cells so the header stays inside the console
// width. "credit: yes" means allowNoCredit (no credit required);
// commercial lists the allowCommercialUse grants (None/empty → "none").
func licenseLines(m *api.Model, budget int) []string {
	commercial := "none"
	if len(m.AllowCommercialUse) > 0 {
		joined := strings.Join(m.AllowCommercialUse, ",")
		if joined != "None" {
			commercial = joined
		}
	}
	segs := []string{
		"credit: " + report.YesNo(m.AllowNoCredit),
		"commercial: " + report.Truncate(commercial, max(budget-12, 10)),
		"derivatives: " + report.YesNo(m.AllowDerivatives),
		"relicensing: " + report.YesNo(m.AllowDifferentLicense),
	}
	var lines []string
	cur := ""
	for _, s := range segs {
		switch {
		case cur == "":
			cur = s
		case len(cur)+3+len(s) <= budget:
			cur += "   " + s
		default:
			lines = append(lines, cur)
			cur = s
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// renderVersionDetail prints the version drill-down: header, file table,
// full hashes per file, scan results, and the trigger-word block (plain
// lines, no quotes or bullets — directly copyable into a registry).
func renderVersionDetail(w io.Writer, modelID int, v *api.ModelVersion) error {
	modelName := strconv.Itoa(modelID)
	if v.Model != nil && v.Model.Name != "" {
		modelName = v.Model.Name
	}
	published := report.DateText(v.PublishedAt)
	if published == "" {
		published = "-"
	}
	// Keep header lines within the console budget: the MODEL line carries
	// the id and URL (~58 cells), the VERSION line the base and date.
	width := report.ConsoleWidth()
	modelName = report.Truncate(modelName, max(width-58, 16))
	versionName := report.Truncate(v.Name, max(width-56-len(v.BaseModel), 12))
	fmt.Fprintf(w, "MODEL     %s (%d)   https://civitai.com/models/%d\n", modelName, modelID, modelID)
	fmt.Fprintf(w, "VERSION   %s (%d)   base: %s   published: %s\n", versionName, v.ID, v.BaseModel, published)
	fmt.Fprintf(w, "STATS     DL %s   ratio %s\n",
		report.CompactNumber(v.Stats.DownloadCount),
		report.RatioText(v.Stats.ThumbsUpCount, v.Stats.ThumbsDownCount))

	fmt.Fprintf(w, "\nFILES (%d)\n", len(v.Files))
	// Fixed columns: SIZE 9, FORMAT 10, FP 5, PRIMARY 7, PICKLE 7,
	// VIRUS 7, plus 2*6 padding = 57; NAME gets the remainder.
	nameCap := max(report.ConsoleWidth()-57, 16)
	tw := report.NewTableWriter(w)
	fmt.Fprintln(tw, "NAME\tSIZE\tFORMAT\tFP\tPRIMARY\tPICKLE\tVIRUS")
	for _, f := range v.Files {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			report.Truncate(f.Name, nameCap),
			report.Truncate(report.FormatSizeKB(f.SizeKB), 9),
			report.Truncate(f.Metadata.Format, 10),
			report.Truncate(f.Metadata.FP, 5),
			report.YesNo(f.Primary),
			f.PickleScanResult,
			f.VirusScanResult,
		)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	for _, f := range v.Files {
		if len(f.Hashes) == 0 {
			continue
		}
		fmt.Fprintf(w, "\nHASHES (%s)\n", f.Name)
		for _, algo := range hashOrder(f.Hashes) {
			fmt.Fprintf(w, "  %-9s %s\n", algo, f.Hashes[algo])
		}
	}

	fmt.Fprintf(w, "\nTRIGGER WORDS (%d)\n", len(v.TrainedWords))
	for _, word := range v.TrainedWords {
		fmt.Fprintln(w, word)
	}
	return nil
}

// hashOrder lists hash algorithms for display: SHA256 and AutoV2 first
// (the vetting workhorses), then the remaining keys in lexical order.
func hashOrder(hashes map[string]string) []string {
	keys := make([]string, 0, len(hashes))
	for k := range hashes {
		if k != "SHA256" && k != "AutoV2" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	out := make([]string, 0, len(hashes))
	if _, ok := hashes["SHA256"]; ok {
		out = append(out, "SHA256")
	}
	if _, ok := hashes["AutoV2"]; ok {
		out = append(out, "AutoV2")
	}
	return append(out, keys...)
}
