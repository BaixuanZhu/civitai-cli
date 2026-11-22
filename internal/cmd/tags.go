package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"

	"civitai-cli/internal/api"
	"civitai-cli/internal/report"
)

// tagsOptions carries the tags-specific flags.
type tagsOptions struct {
	limit int
}

// register binds the tags flags to fs.
func (o *tagsOptions) register(fs *flag.FlagSet) {
	fs.IntVar(&o.limit, "limit", 20, "max tag names (1-100)")
}

// tagsHelp is the curated `civ tags help` / `-h` page (without the shared
// global block, which writeCommandHelp appends).
const tagsHelp = `tag-name completion for prompt engineering

Usage:
  civ tags <query> [flags]

Options:
  --limit <n>      Max tag names, 1-100 (default 20)

Examples:
  civ tags anima
  civ tags "hatsune miku" --limit 50
  civ tags masterpiece --json
`

// Tags runs the tags command: civ tags <query> — tag-name completion for
// prompt engineering.
func Tags(args []string) {
	var o tagsOptions
	execute("tags", tagsHelp, args, o.register, func(ctx context.Context, g *globals, pos []string) error {
		return runTags(ctx, g, pos, &o)
	})
}

// runTags is the pure tags implementation (testable without os.Exit).
func runTags(ctx context.Context, g *globals, pos []string, o *tagsOptions) error {
	if len(pos) == 0 {
		return errors.New("tags requires a query, e.g. civ tags anima")
	}
	query := strings.Join(pos, " ")
	if o.limit < 1 || o.limit > 100 {
		return fmt.Errorf("--limit must be 1-100 (got %d)", o.limit)
	}

	raw, err := newAPIClient(g).ListTags(ctx, query, o.limit)
	if err != nil {
		return err
	}
	tags, err := api.ParseTags(raw)
	if err != nil {
		return err
	}

	names := make([]string, 0, len(tags))
	for _, t := range tags {
		names = append(names, t.Name)
	}
	if g.JSON {
		return report.WriteIndent(g.stdout(), names)
	}
	if len(names) == 0 {
		_, err := fmt.Fprintln(g.stdout(), "No tags found.")
		return err
	}
	for _, n := range names {
		if _, err := fmt.Fprintln(g.stdout(), n); err != nil {
			return err
		}
	}
	return nil
}
