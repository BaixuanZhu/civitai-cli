package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"civitai-cli/internal/api"
	"civitai-cli/internal/app"
	"civitai-cli/internal/report"
)

// stringListFlag accumulates a repeatable, comma-separated string flag
// (--type lora --type ti, or --type lora,ti).
type stringListFlag struct{ values []string }

// String renders the accumulated values (flag.Value interface).
func (f *stringListFlag) String() string { return strings.Join(f.values, ",") }

// Set splits one flag occurrence on commas and appends the non-empty parts.
func (f *stringListFlag) Set(s string) error {
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			f.values = append(f.values, part)
		}
	}
	return nil
}

// searchOptions carries the search-specific flags.
type searchOptions struct {
	types  stringListFlag
	base   string
	sort   string
	period string
	tag    string
	user   string
	limit  int
}

// register binds the search flags to fs.
func (o *searchOptions) register(fs *flag.FlagSet) {
	fs.Var(&o.types, "type", "model type filter; repeatable or comma-separated (lora, checkpoint, ti, locon, ...)")
	fs.StringVar(&o.base, "base", "", "baseModel filter, comma-separated; unregistered values warn (API applies only the first)")
	fs.StringVar(&o.sort, "sort", "hot", "sort order: hot, highest-rated, most-downloaded, most-liked, most-discussed, most-collected, most-images, newest, oldest, recently-added")
	fs.StringVar(&o.period, "period", "all", "ranking period: day, week, month, year, all")
	fs.StringVar(&o.tag, "tag", "", "exact tag filter")
	fs.StringVar(&o.user, "user", "", "creator username filter")
	fs.IntVar(&o.limit, "limit", 20, "max results (1-100)")
}

// Search runs the search command: civ search <query> [flags].
func Search(args []string) {
	var o searchOptions
	execute("search", args, o.register, func(ctx context.Context, g *globals, pos []string) error {
		return runSearch(ctx, g, pos, &o)
	})
}

// runSearch is the pure search implementation (testable without os.Exit).
func runSearch(ctx context.Context, g *globals, pos []string, o *searchOptions) error {
	if len(pos) == 0 {
		return errors.New("search requires a query, e.g. civ search stocking")
	}
	query := strings.Join(pos, " ")
	if o.limit < 1 || o.limit > 100 {
		return fmt.Errorf("--limit must be 1-100 (got %d); the API hard-caps at 100", o.limit)
	}
	sortVal, ok := api.MapSort(o.sort)
	if !ok {
		return fmt.Errorf("unknown --sort %q; valid values: %s", o.sort, strings.Join(api.SortChoices(), ", "))
	}
	periodVal, ok := api.MapPeriod(o.period)
	if !ok {
		return fmt.Errorf("unknown --period %q; valid values: %s", o.period, strings.Join(api.PeriodChoices(), ", "))
	}
	var types []string
	for _, t := range o.types.values {
		v, ok := api.MapType(t)
		if !ok {
			return fmt.Errorf("unknown --type %q; valid values: %s", t, strings.Join(api.TypeChoices(), ", "))
		}
		types = append(types, v)
	}
	bases, err := resolveBaseFilters(o.base)
	if err != nil {
		return err
	}

	raw, err := newAPIClient(g).SearchModels(ctx, api.SearchParams{
		Query:      query,
		Types:      types,
		BaseModels: bases,
		Tag:        o.tag,
		Username:   o.user,
		Sort:       sortVal,
		Period:     periodVal,
		Limit:      o.limit,
	})
	if err != nil {
		return err
	}

	if g.JSON {
		items, err := api.ModelsRawItems(raw)
		if err != nil {
			return err
		}
		return report.WriteRaw(g.stdout(), items)
	}
	models, err := api.ParseModels(raw)
	if err != nil {
		return err
	}
	return renderSearchTable(g.stdout(), models)
}

// resolveBaseFilters canonicalizes the comma-separated --base list.
// Unregistered values warn but pass through (the API returns no matches
// for them rather than erroring). Verified live 2026-11-22: the API
// applies only the FIRST baseModels value, so extras are dropped with a
// warning instead of silently misleading the user.
func resolveBaseFilters(base string) ([]string, error) {
	var bases []string
	for _, b := range strings.Split(base, ",") {
		b = strings.TrimSpace(b)
		if b == "" {
			continue
		}
		if canon, ok := api.CanonicalBaseModel(b); ok {
			bases = append(bases, canon)
		} else {
			app.Warnf("baseModel %q is not in the verified platform list (see civ search --help); the API will likely return no matches", b)
			bases = append(bases, b)
		}
	}
	if len(bases) > 1 {
		app.Warnf("the API applies only the first baseModel filter (%q); ignoring %s", bases[0], strings.Join(bases[1:], ", "))
		bases = bases[:1]
	}
	return bases, nil
}

// searchCaps holds the flexible column budgets of the search table.
type searchCaps struct {
	name    int
	typ     int
	base    int
	creator int
}

// searchLayout budgets the nine search columns for the terminal width.
// Fixed columns: ID 7, DL 6, RATIO 6, UPDATED 10, NSFW 4; tabwriter adds
// padding 2 between the nine columns (16 total). NAME gets the remainder
// (capped at 48); on narrow terminals CREATOR/BASE/TYPE shrink to their
// floors (7/8/4) so NAME keeps at least 12 cells. Below ~68 columns the
// table cannot fit and mild overflow is accepted (nine columns is the
// plan's fixed contract).
func searchLayout(width int) searchCaps {
	const fixed = 7 + 6 + 6 + 10 + 4 + 16 // ID DL RATIO UPDATED NSFW + gaps
	c := searchCaps{name: 48, typ: 6, base: 11, creator: 12}
	rem := width - fixed - c.typ - c.base - c.creator
	if rem >= 12 {
		c.name = min(rem, 48)
		return c
	}
	c.creator, c.base, c.typ = 7, 8, 4
	c.name = max(width-fixed-c.typ-c.base-c.creator, 12)
	return c
}

// renderSearchTable prints the plan's search table:
// ID NAME TYPE BASE DL RATIO UPDATED NSFW CREATOR.
// UPDATED is the newest version publishedAt (model items carry no
// updatedAt field — verified live 2026-11-22).
func renderSearchTable(w io.Writer, models []api.Model) error {
	if len(models) == 0 {
		_, err := fmt.Fprintln(w, "No models found.")
		return err
	}
	caps := searchLayout(report.ConsoleWidth())
	tw := report.NewTableWriter(w)
	fmt.Fprintln(tw, "ID\tNAME\tTYPE\tBASE\tDL\tRATIO\tUPDATED\tNSFW\tCREATOR")
	for _, m := range models {
		base := "-"
		if len(m.BaseModels) > 0 {
			base = strings.Join(m.BaseModels, ",")
		} else if len(m.ModelVersions) > 0 {
			base = m.ModelVersions[0].BaseModel
		}
		nsfw := "-"
		if m.NSFW {
			nsfw = "Yes"
		}
		updated := report.DateText(latestPublishedAt(m))
		if updated == "" {
			updated = "-"
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			m.ID,
			report.Truncate(m.Name, caps.name),
			report.Truncate(report.DisplayType(m.Type), caps.typ),
			report.Truncate(base, caps.base),
			report.CompactNumber(m.Stats.DownloadCount),
			report.RatioText(m.Stats.ThumbsUpCount, m.Stats.ThumbsDownCount),
			updated,
			nsfw,
			report.Truncate(m.Creator.Username, caps.creator),
		)
	}
	return tw.Flush()
}

// latestPublishedAt returns the newest version publishedAt of a model
// (ISO8601 strings compare lexicographically), or "" when none exists.
func latestPublishedAt(m api.Model) string {
	latest := ""
	for _, v := range m.ModelVersions {
		if v.PublishedAt > latest {
			latest = v.PublishedAt
		}
	}
	return latest
}
