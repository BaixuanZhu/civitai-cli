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
	"civitai-cli/internal/report"
)

// recipeHelp is the curated `civ recipe help` / `-h` page (without the
// shared global block, which writeCommandHelp appends). It documents the
// two approved views: a prompt-free first-N summary and an explicit
// singular text detail selected by source index.
const recipeHelp = `show compact, read-only version context and example recipes

Usage:
  civ recipe <versionId> [--examples <count>] [--json]
  civ recipe <versionId> --example <sourceIndex> [--json]

Options:
  --examples <count>  Number of prompt-free example summaries to return from
                      the start of the version's example list (0-10;
                      default 3). Count 1 is still a summary: prompt and
                      negative-prompt text are not included at any count.
  --example <index>   Return generation detail for exactly one example,
                      selected by its 1-based source index. Detail is the
                      only view that includes prompt and negativePrompt
                      text. Cannot be combined with --examples.

Examples:
  civ recipe 3263843
  civ recipe 3263843 --examples 5 --json
  civ recipe 3263843 --example 1 --json

Notes:
  Recipes are source evidence, not recommended settings or a guarantee of
  reproducibility. Missing metadata is reported as null plus an issue,
  never as an invented value. The command fetches one version response and
  does not download models or images, generate content, or export
  workflows. Prompt text is omitted from summaries, including a summary of
  one; use --example <sourceIndex> for one example's detail.
`

// recipeOptions carries the recipe-specific flags. Both selectors track
// whether they were supplied explicitly: the summary default (3) must not
// conflict with --example, and an explicit --example 0 is an invalid index
// rather than "no detail requested".
type recipeOptions struct {
	examples recipeIntFlag
	example  recipeIntFlag
}

// register binds the recipe flags to fs.
func (o *recipeOptions) register(fs *flag.FlagSet) {
	fs.Var(&o.examples, "examples", "prompt-free example summaries to return, first source entries first (0-10; default 3)")
	fs.Var(&o.example, "example", "prompt text detail for one example, by 1-based source index")
}

// recipeIntFlag is an integer flag.Value that remembers whether the user
// supplied it. The stdlib flag package cannot distinguish "absent" from
// "explicitly set to the zero value", and recipe selection needs that
// distinction for both --examples and --example.
type recipeIntFlag struct {
	value int
	set   bool
}

// String implements flag.Value; it is only used for default display, which
// the command runner suppresses in favor of the curated help page.
func (f *recipeIntFlag) String() string { return strconv.Itoa(f.value) }

// Set implements flag.Value and records explicit presence.
func (f *recipeIntFlag) Set(s string) error {
	v, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("want an integer, got %q", s)
	}
	f.value = v
	f.set = true
	return nil
}

// Recipe runs the recipe command:
// civ recipe <versionId> [--examples <count> | --example <sourceIndex>].
// It prints prompt-free context and example summaries by default; only the
// explicit singular detail selector returns prompt text. Errors use the
// shared 1/2/4 mapping (exit 3 stays vet-only).
func Recipe(args []string) {
	var o recipeOptions
	execute("recipe", recipeHelp, args, o.register, func(ctx context.Context, g *globals, pos []string) error {
		return runRecipe(ctx, g, pos, &o)
	})
}

// runRecipe is the pure recipe implementation (testable without os.Exit).
// All selector and range validation happens before the single version
// request; an out-of-range detail index is detected after that fetch and
// returns a selection error with no partial output.
func runRecipe(ctx context.Context, g *globals, pos []string, o *recipeOptions) error {
	if len(pos) != 1 {
		return errors.New("recipe requires exactly one version id, e.g. civ recipe 3263843")
	}
	id, err := strconv.Atoi(pos[0])
	if err != nil || id <= 0 {
		return fmt.Errorf("invalid version id %q (want a positive integer)", pos[0])
	}
	if o.example.set && o.examples.set {
		return errors.New("--example and --examples cannot be combined: --examples returns prompt-free summaries, --example returns one example's text")
	}
	if o.example.set && o.example.value <= 0 {
		return fmt.Errorf("invalid example index %d (want a positive 1-based source index)", o.example.value)
	}
	count := 3
	if o.examples.set {
		if o.examples.value < 0 || o.examples.value > 10 {
			return fmt.Errorf("invalid --examples count %d (want 0-10)", o.examples.value)
		}
		count = o.examples.value
	}

	c := newAPIClient(g)
	raw, err := c.GetVersion(ctx, id)
	if err != nil {
		return err
	}

	if o.example.set {
		detail, err := api.ParseRecipeDetail(raw, id, o.example.value)
		if err != nil {
			return err
		}
		if g.JSON {
			return report.WriteIndent(g.stdout(), detail)
		}
		return renderRecipeDetail(g.stdout(), detail)
	}

	summary, err := api.ParseRecipeSummary(raw, id, count)
	if err != nil {
		return err
	}
	if g.JSON {
		return report.WriteIndent(g.stdout(), summary)
	}
	return renderRecipeSummary(g.stdout(), summary)
}

// renderRecipeSummary writes the human summary view: version context,
// selection counts, and one prompt-free block per returned example. Prompt
// and negativePrompt values are never printed here, at any count.
func renderRecipeSummary(w io.Writer, s *api.RecipeSummary) error {
	writeRecipeHeader(w, s.Provenance, s.Subject, s.Issues)
	fmt.Fprintf(w, "\nEXAMPLES  %s\n", recipeSummarySelectionText(s.Selection))
	if len(s.Examples) == 0 {
		fmt.Fprintln(w, "  (none returned)")
	}
	for _, ex := range s.Examples {
		fmt.Fprintln(w)
		writeRecipeExampleSummary(w, ex)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Prompt text is omitted from summaries; use --example <sourceIndex> for one example's detail.")
	return nil
}

// renderRecipeDetail writes the human singular detail view: the shared
// context plus the selected example's small parameters and verbatim prompt
// and negative-prompt text.
func renderRecipeDetail(w io.Writer, d *api.RecipeDetail) error {
	writeRecipeHeader(w, d.Provenance, d.Subject, d.Issues)
	fmt.Fprintf(w, "\nEXAMPLE  %s\n", recipeDetailSelectionText(d.Selection))
	if d.Example == nil {
		return errors.New("recipe: detail view is missing its example")
	}
	fmt.Fprintln(w)
	writeRecipeExampleSummary(w, d.Example.RecipeSummaryExample)

	if d.Example.TextParameters == nil {
		fmt.Fprintln(w, "\n  textParameters: not provided (no generation metadata)")
		return nil
	}
	writeRecipePromptText(w, "prompt", d.Example.TextParameters.Prompt)
	writeRecipePromptText(w, "negativePrompt", d.Example.TextParameters.NegativePrompt)
	return nil
}

// writeRecipeHeader prints the model/version context shared by both views.
// Issues are listed once, as stable field/reason pairs.
func writeRecipeHeader(w io.Writer, p api.RecipeProvenance, s api.RecipeSubject, issues []api.RecipeIssue) {
	width := report.ConsoleWidth()
	fmt.Fprintf(w, "MODEL      %s (%s)\n",
		report.Truncate(recipeHumanText(s.Model.Name), max(width-24, 16)),
		recipeIntValue(s.Model.ID))

	published := "-"
	if s.Version.PublishedAt != nil {
		published = report.DateText(*s.Version.PublishedAt)
		if published == "" {
			published = "-"
		}
	}
	fmt.Fprintf(w, "VERSION    %s (%s)   base: %s / %s   published: %s\n",
		report.Truncate(recipeHumanText(s.Version.Name), max(width-76, 12)),
		recipeIntValue(s.Version.ID),
		recipeHumanText(s.Version.BaseModel),
		recipeHumanText(s.Version.BaseModelType),
		published)
	fmt.Fprintf(w, "PROVENANCE %s   %s\n", p.Source, p.APIPath)

	fmt.Fprintf(w, "TRIGGER WORDS (%d)\n", len(s.TrainedWords))
	for _, word := range s.TrainedWords {
		fmt.Fprintln(w, word)
	}
	if len(issues) > 0 {
		fmt.Fprintf(w, "ISSUES     %s\n", recipeIssuesText(issues))
	}
}

// writeRecipeExampleSummary prints one prompt-free example block: source
// index, metadata/text availability, the small-parameter whitelist, the
// provenance URL, and any per-example issues.
func writeRecipeExampleSummary(w io.Writer, ex api.RecipeSummaryExample) {
	fmt.Fprintf(w, "  [%d]  metadata %s   prompt %s   negativePrompt %s\n",
		ex.SourceIndex, ex.MetadataStatus, ex.PromptStatus, ex.NegativePromptStatus)
	if p := ex.Parameters; p != nil {
		fmt.Fprintf(w, "       steps %s   cfgScale %s   seed %s   denoise %s\n",
			recipeIntValue(p.Steps), recipeFloatValue(p.CFGScale), recipeHumanText(p.Seed), recipeFloatValue(p.Denoise))
		fmt.Fprintf(w, "       sampler %s   scheduler %s   width %s   height %s\n",
			recipeHumanText(p.Sampler), recipeHumanText(p.Scheduler), recipeIntValue(p.Width), recipeIntValue(p.Height))
	}
	url := "-"
	if ex.Provenance.ImageURL != nil {
		url = *ex.Provenance.ImageURL
		if url == "" {
			url = "(empty)"
		}
	}
	fmt.Fprintf(w, "       url %s\n", url)
	if len(ex.Issues) > 0 {
		fmt.Fprintf(w, "       issues: %s\n", recipeIssuesText(ex.Issues))
	}
}

// writeRecipePromptText prints one verbatim large-text value. Only the
// explicit singular detail view calls this; multi-line prompts are
// preserved unchanged with no quotes or bullets.
func writeRecipePromptText(w io.Writer, label string, value *string) {
	switch {
	case value == nil:
		fmt.Fprintf(w, "\n  %s: not provided\n", label)
	case *value == "":
		fmt.Fprintf(w, "\n  %s: (empty)\n", label)
	default:
		fmt.Fprintf(w, "\n  %s:\n%s\n", label, *value)
	}
}

// recipeSummarySelectionText renders the summary's bounded-selection
// counts, including the unknown-source state when images were not
// provided.
func recipeSummarySelectionText(sel api.RecipeSelection) string {
	if sel.Total == nil {
		return fmt.Sprintf("source not provided (counts unknown); requested %d", sel.Requested)
	}
	truncation := "complete"
	switch {
	case sel.Truncated == nil:
		truncation = "truncation unknown"
	case *sel.Truncated:
		truncation = "truncated"
	}
	return fmt.Sprintf("returned %d of %s (%s with meta, %s without); %s",
		sel.Returned, recipeIntValue(sel.Total), recipeIntValue(sel.WithMetadata), recipeIntValue(sel.WithoutMetadata), truncation)
}

// recipeDetailSelectionText renders the singular selection's source index
// and full-source counts.
func recipeDetailSelectionText(sel api.RecipeSelection) string {
	if sel.Total == nil {
		return fmt.Sprintf("source index %d; source counts unknown", sel.Requested)
	}
	return fmt.Sprintf("source index %d of %s (%s with meta, %s without)",
		sel.Requested, recipeIntValue(sel.Total), recipeIntValue(sel.WithMetadata), recipeIntValue(sel.WithoutMetadata))
}

// recipeIssuesText renders issues as "field reason" pairs for human output.
func recipeIssuesText(issues []api.RecipeIssue) string {
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		parts = append(parts, issue.Field+" "+issue.Reason)
	}
	return strings.Join(parts, ", ")
}

// recipeHumanText renders a nullable small string: "-" for absent/null and
// "(empty)" for an explicit empty value, so the two source states stay
// distinguishable in human output.
func recipeHumanText(value *string) string {
	switch {
	case value == nil:
		return "-"
	case *value == "":
		return "(empty)"
	default:
		return *value
	}
}

// recipeIntValue renders a nullable integer as text ("-" for null).
func recipeIntValue[T int | int64](value *T) string {
	if value == nil {
		return "-"
	}
	return strconv.FormatInt(int64(*value), 10)
}

// recipeFloatValue renders a nullable float with the shortest exact
// representation ("-" for null), so 1 stays "1" and 7.5 stays "7.5".
func recipeFloatValue(value *float64) string {
	if value == nil {
		return "-"
	}
	return strconv.FormatFloat(*value, 'f', -1, 64)
}
