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
// approved views: a prompt-free first-N official summary, an explicit
// singular official text detail selected by source index, one bounded
// community sample, and the independent exact-image detail command.
const recipeHelp = `show compact, read-only version context and example recipes

Usage:
  civ recipe <versionId> [--examples <count>] [--json]
  civ recipe <versionId> --example <sourceIndex> [--json]
  civ recipe <versionId> --source community [--sort <order>] [--limit <count>] [--json]
  civ recipe image <imageId> [--json]

Options:
  --examples <count>  Number of prompt-free example summaries to return from
                      the start of the version's example list (0-10;
                      default 3). Count 1 is still a summary: prompt and
                      negative-prompt text are not included at any count.
                      Valid only for the official source.
  --example <index>   Return generation detail for exactly one official
                      example, selected by its 1-based source index. This
                      view includes prompt and negativePrompt text. Cannot
                      be combined with --examples. Valid only for the
                      official source.
  --source <source>   Recipe source: official (default) or community.
                      Official projects the version's own example images
                      and never requests /images. Community samples exactly
                      one bounded GET /images response for the version.
  --sort <order>      Community sample order: most-reactions (default) or
                      newest, both with the fixed AllTime period. Neither
                      order is a quality signal. Valid only with
                      --source community.
  --limit <count>     Community sample size (1-100; default 20): one
                      bounded response with no pagination. Valid only with
                      --source community.

Examples:
  civ recipe 3263843
  civ recipe 3263843 --examples 5 --json
  civ recipe 3263843 --example 1 --json
  civ recipe 3263843 --source community --limit 10 --json
  civ recipe image 123456 --json

Notes:
  Recipes are source evidence, not recommended settings or a guarantee of
  reproducibility. Missing metadata is reported as null plus an issue,
  never as an invented value. Official mode fetches one version response
  and does not download models or images, generate content, or export
  workflows. Prompt text is omitted from summaries, including a summary of
  one; use --example <sourceIndex> for one official example's detail.

  Community mode reports coverage as returned/withMeta/usable counts for
  one bounded response, plus whether a continuation was offered (it is
  never followed). An empty sample is a valid visibility outcome, not
  proof that a version has no community images, and no alternate sort is
  retried automatically. Every community list is prompt-free; use
  civ recipe image <imageId> for one image's prompt and negativePrompt.
  Neither mode downloads media or recommends settings.
`

// Approved recipe source values. Official stays the default, so the
// compatibility baseline performs no /images traffic. The community
// source is a single bounded sample; combining sources is deferred.
const (
	recipeSourceOfficial  = "official"
	recipeSourceCommunity = "community"
)

// recipeOptions carries the recipe-specific flags. The integer selectors
// and string selectors all track explicit presence: the summary default
// (3) must not conflict with --example, an explicit --example 0 is an
// invalid index rather than "no detail requested", community-only options
// must be rejected in official mode, and `civ recipe image` must reject
// any option that belongs to another view.
type recipeOptions struct {
	examples recipeIntFlag
	example  recipeIntFlag
	source   recipeStringFlag
	sort     recipeStringFlag
	limit    recipeIntFlag
}

// register binds the recipe flags to fs.
func (o *recipeOptions) register(fs *flag.FlagSet) {
	o.source.value = recipeSourceOfficial
	fs.Var(&o.examples, "examples", "prompt-free example summaries to return, first source entries first (0-10; default 3)")
	fs.Var(&o.example, "example", "prompt text detail for one example, by 1-based source index")
	fs.Var(&o.source, "source", "recipe source: official or community (default official)")
	fs.Var(&o.sort, "sort", "community sample order: most-reactions or newest (default most-reactions)")
	fs.Var(&o.limit, "limit", "community sample size, 1-100 (default 20)")
}

// recipeIntFlag is an integer flag.Value that remembers whether the user
// supplied it. The stdlib flag package cannot distinguish "absent" from
// "explicitly set to the zero value", and recipe selection needs that
// distinction for --examples, --example, and --limit.
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

// recipeStringFlag is a string flag.Value that remembers whether the user
// supplied it, so an explicitly empty community sort is a usage error
// instead of silently becoming the default, and `civ recipe image` can
// reject a source option it does not accept.
type recipeStringFlag struct {
	value string
	set   bool
}

// String implements flag.Value; see recipeIntFlag.String.
func (f *recipeStringFlag) String() string { return f.value }

// Set implements flag.Value and records explicit presence.
func (f *recipeStringFlag) Set(s string) error {
	f.value = s
	f.set = true
	return nil
}

// recipeSourceValue validates the --source value (case-insensitive) and
// returns the canonical approved source name.
func recipeSourceValue(o *recipeOptions) (string, error) {
	source := strings.ToLower(strings.TrimSpace(o.source.value))
	switch source {
	case recipeSourceOfficial, recipeSourceCommunity:
		return source, nil
	default:
		return "", fmt.Errorf("invalid --source %q (want %s or %s)", o.source.value, recipeSourceOfficial, recipeSourceCommunity)
	}
}

// Recipe runs the recipe command:
// civ recipe <versionId> [--examples <count> | --example <sourceIndex>],
// civ recipe <versionId> --source community [--sort <order>] [--limit <count>],
// or civ recipe image <imageId>, all with [--json]. Official views are the
// default and never request /images; community sampling and exact-image
// detail are explicit. Errors use the shared 1/2/4 mapping (exit 3 stays
// vet-only).
func Recipe(args []string) {
	var o recipeOptions
	execute("recipe", recipeHelp, args, o.register, func(ctx context.Context, g *globals, pos []string) error {
		return runRecipe(ctx, g, pos, &o)
	})
}

// runRecipe is the pure recipe implementation (testable without os.Exit).
// All selector, source, and range validation happens before the single
// request each mode makes; an out-of-range official detail index is
// detected after that fetch and returns a selection error with no partial
// output. The `image` keyword selects the independent exact-image detail
// command.
func runRecipe(ctx context.Context, g *globals, pos []string, o *recipeOptions) error {
	if len(pos) > 0 && pos[0] == "image" {
		if len(pos) != 2 {
			return errors.New("recipe image requires exactly one image id, e.g. civ recipe image 123456")
		}
		return runRecipeImage(ctx, g, pos[1], o)
	}
	if len(pos) != 1 {
		return errors.New("recipe requires exactly one version id, e.g. civ recipe 3263843")
	}
	source, err := recipeSourceValue(o)
	if err != nil {
		return err
	}
	id, err := strconv.Atoi(pos[0])
	if err != nil || id <= 0 {
		return fmt.Errorf("invalid version id %q (want a positive integer)", pos[0])
	}
	if source == recipeSourceCommunity {
		return runRecipeCommunity(ctx, g, id, o)
	}
	if o.sort.set {
		return errors.New("--sort is valid only with --source community")
	}
	if o.limit.set {
		return errors.New("--limit is valid only with --source community")
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

// runRecipeCommunity performs the one bounded community sample: one
// /images list request with the exact version, approved order, fixed
// AllTime period, and the validated limit. No pagination, alternate-sort
// retry, detail fan-out, or resource lookup follows.
func runRecipeCommunity(ctx context.Context, g *globals, versionID int, o *recipeOptions) error {
	if o.examples.set || o.example.set {
		return errors.New("--examples and --example are official-source selectors; community samples use --limit, and `civ recipe image <imageId>` returns one image's detail")
	}
	sortCLI := "most-reactions"
	if o.sort.set {
		sortCLI = o.sort.value
	}
	sort, ok := api.MapCommunitySort(sortCLI)
	if !ok {
		return fmt.Errorf("invalid --sort %q (want %s)", o.sort.value, strings.Join(api.CommunitySortChoices(), " or "))
	}
	limit := api.CommunityDefaultLimit
	if o.limit.set {
		if o.limit.value < api.CommunityMinLimit || o.limit.value > api.CommunityMaxLimit {
			return fmt.Errorf("invalid --limit %d (want %d-%d)", o.limit.value, api.CommunityMinLimit, api.CommunityMaxLimit)
		}
		limit = o.limit.value
	}

	c := newAPIClient(g)
	raw, err := c.ListImages(ctx, versionID, limit, sort)
	if err != nil {
		return err
	}
	sample, err := api.ParseCommunitySample(raw, versionID, limit, sort)
	if err != nil {
		return err
	}
	if g.JSON {
		return report.WriteIndent(g.stdout(), sample)
	}
	return renderCommunitySample(g.stdout(), sample)
}

// runRecipeImage performs the independent exact-image detail: one /images
// request with exactly imageId, withMeta=true, type=image, and limit=1,
// then projects only that image's detail. It reads no list state, writes
// nothing, and never fetches media or model-version metadata.
func runRecipeImage(ctx context.Context, g *globals, arg string, o *recipeOptions) error {
	if o.examples.set || o.example.set || o.sort.set || o.limit.set || o.source.set {
		return errors.New("civ recipe image accepts only an image id and --json; --source, --sort, --limit, --examples, and --example are not applicable")
	}
	id, err := strconv.Atoi(arg)
	if err != nil || id <= 0 {
		return fmt.Errorf("invalid image id %q (want a positive integer)", arg)
	}

	c := newAPIClient(g)
	raw, err := c.GetImage(ctx, id)
	if err != nil {
		return err
	}
	detail, err := api.ParseCommunityImageDetail(raw, id)
	if err != nil {
		return err
	}
	if g.JSON {
		return report.WriteIndent(g.stdout(), detail)
	}
	return renderCommunityImageDetail(g.stdout(), detail)
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

// renderCommunitySample writes the bounded community list view: exact
// query and coverage first, then one large-field-free block per image in
// upstream order. Prompt and negative-prompt text is never printed here,
// at any count or in any mode.
func renderCommunitySample(w io.Writer, s *api.CommunitySample) error {
	fmt.Fprintf(w, "COMMUNITY  version %d   sort %s   period %s   limit %d\n",
		s.Query.VersionID, s.Query.Sort, s.Query.Period, s.Query.Limit)
	fmt.Fprintf(w, "SOURCE     %s   %s\n", s.Provenance.Source, s.Provenance.APIPath)

	continuation := "no"
	if s.Coverage.ContinuationOffered {
		continuation = "yes (not followed)"
	}
	fmt.Fprintf(w, "COVERAGE   returned %d of %d requested   with meta %s   usable %s   continuation %s\n",
		s.Coverage.Returned, s.Coverage.RequestedLimit,
		communityCoverageText(s.Coverage.WithMeta, s.Coverage.Returned),
		communityCoverageText(s.Coverage.Usable, s.Coverage.Returned),
		continuation)

	if len(s.Observations) == 0 {
		fmt.Fprintln(w, "\n  (no community images returned for this bounded sample)")
	}
	for _, obs := range s.Observations {
		fmt.Fprintln(w)
		writeCommunityObservation(w, obs)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Community samples are bounded evidence from one /images response, not recommendations.")
	fmt.Fprintln(w, "Prompt text is omitted from samples; use civ recipe image <imageId> for one image's detail.")
	return nil
}

// renderCommunityImageDetail writes the explicit single-image detail: the
// selected image's provenance, visibility, reactions, normalized
// parameters, resources, and verbatim prompt text.
func renderCommunityImageDetail(w io.Writer, d *api.CommunityImageDetail) error {
	if d.Image == nil {
		return errors.New("recipe: community image detail is missing its image")
	}
	image := d.Image
	fmt.Fprintf(w, "IMAGE      %s\n", recipeIntValue(image.ImageID))
	fmt.Fprintf(w, "PROVENANCE %s   %s   imageId %d\n",
		d.Provenance.Source, d.Provenance.APIPath, d.Provenance.ImageID)
	fmt.Fprintf(w, "POST       %s   by %s   %s\n",
		recipeIntValue(image.PostID), recipeHumanText(image.Username), recipeHumanText(image.CreatedAt))
	fmt.Fprintf(w, "PAGE       %s\n", recipeHumanText(image.PageURL))
	fmt.Fprintf(w, "VISIBILITY nsfw %s   browsing %s\n",
		recipeHumanText(image.NSFWLevel), recipeHumanText(image.BrowsingLevel))
	fmt.Fprintf(w, "TYPE       %s   baseModel %s\n",
		recipeHumanText(image.Type), recipeHumanText(image.BaseModel))
	fmt.Fprintf(w, "REACTIONS  %s\n", communityReactionsText(image.Reactions))
	fmt.Fprintf(w, "METADATA   %s   prompt %s   negativePrompt %s\n",
		image.MetadataStatus, image.PromptStatus, image.NegativePromptStatus)
	writeCommunityParameters(w, image.Parameters)
	writeCommunityResources(w, image.Resources)
	if len(image.Issues) > 0 {
		fmt.Fprintf(w, "ISSUES     %s\n", recipeIssuesText(image.Issues))
	}
	if image.TextParameters == nil {
		fmt.Fprintln(w, "\n  textParameters: not provided (no generation metadata)")
		return nil
	}
	writeRecipePromptText(w, "prompt", image.TextParameters.Prompt)
	writeRecipePromptText(w, "negativePrompt", image.TextParameters.NegativePrompt)
	return nil
}

// writeCommunityObservation prints one compact community list row:
// provenance, visibility, separate reactions, normalized parameters,
// origin-labeled resources, and per-image issues. It has no large-field
// parameter by construction.
func writeCommunityObservation(w io.Writer, obs api.CommunityObservation) {
	fmt.Fprintf(w, "  [%d] image %s   post %s   by %s   %s\n",
		obs.UpstreamPosition, recipeIntValue(obs.ImageID), recipeIntValue(obs.PostID),
		recipeHumanText(obs.Username), recipeHumanText(obs.CreatedAt))
	fmt.Fprintf(w, "      page %s   type %s   baseModel %s\n",
		recipeHumanText(obs.PageURL), recipeHumanText(obs.Type), recipeHumanText(obs.BaseModel))
	fmt.Fprintf(w, "      nsfw %s   browsing %s   metadata %s   usable %s\n",
		recipeHumanText(obs.NSFWLevel), recipeHumanText(obs.BrowsingLevel),
		obs.MetadataStatus, report.YesNo(obs.Usable))
	fmt.Fprintf(w, "      reactions %s\n", communityReactionsText(obs.Reactions))
	writeCommunityParameters(w, obs.Parameters)
	writeCommunityResources(w, obs.Resources)
	if len(obs.Issues) > 0 {
		fmt.Fprintf(w, "      issues: %s\n", recipeIssuesText(obs.Issues))
	}
}

// communityCoverageText renders one coverage count over the returned
// denominator, with "n/a" when this response returned no items (never a
// division by zero).
func communityCoverageText(count, returned int) string {
	if returned == 0 {
		return fmt.Sprintf("%d/%d (n/a)", count, returned)
	}
	return fmt.Sprintf("%d/%d (%d%%)", count, returned, count*100/returned)
}

// communityReactionsText renders the separate reaction/comment counters.
// A nil block means the response carried no stats; individual "-" values
// mean that counter was absent. No combined score is produced.
func communityReactionsText(r *api.CommunityReactions) string {
	if r == nil {
		return "not provided"
	}
	return fmt.Sprintf("like %s   dislike %s   heart %s   laugh %s   cry %s   comment %s",
		recipeIntValue(r.Like), recipeIntValue(r.Dislike), recipeIntValue(r.Heart),
		recipeIntValue(r.Laugh), recipeIntValue(r.Cry), recipeIntValue(r.Comment))
}

// writeCommunityParameters prints normalized scalar observations as
// origin.name=value groups, four per line, with each value's exact
// normalized form ("-" for null, "(empty)" for an empty string).
func writeCommunityParameters(w io.Writer, params []api.ObservedParameter) {
	if len(params) == 0 {
		fmt.Fprintln(w, "      parameters none observed")
		return
	}
	const perLine = 4
	for i := 0; i < len(params); i += perLine {
		end := min(i+perLine, len(params))
		parts := make([]string, 0, end-i)
		for _, param := range params[i:end] {
			parts = append(parts, param.Origin+"."+param.Name+"="+communityParameterText(param.Value))
		}
		label := "                 "
		if i == 0 {
			label = "      parameters "
		}
		fmt.Fprintf(w, "%s%s\n", label, strings.Join(parts, "   "))
	}
}

// communityParameterText renders one normalized parameter value. Values
// are only int64, float64, or string (never containers), so the fallback
// is defensive rather than a coercion path.
func communityParameterText(value any) string {
	switch v := value.(type) {
	case nil:
		return "-"
	case string:
		if v == "" {
			return "(empty)"
		}
		return v
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}

// writeCommunityResources prints one line per origin-labeled resource
// entry, preserving origin, position, explicit identity fields, and the
// observed weight ("-" when absent, never an inferred 1).
func writeCommunityResources(w io.Writer, resources []api.ObservedResource) {
	if len(resources) == 0 {
		fmt.Fprintln(w, "      resources none observed")
		return
	}
	for _, resource := range resources {
		fmt.Fprintf(w, "      resource %s[%d] type %s   modelVersionId %s   name %s   hash %s   weight %s\n",
			resource.Origin, resource.Position,
			recipeHumanText(resource.Type), recipeIntValue(resource.ModelVersionID),
			recipeHumanText(resource.Name), recipeHumanText(resource.Hash),
			recipeFloatValue(resource.Weight))
	}
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
