package cmd

import (
	"encoding/json"
	"fmt"
	"io"

	"civitai-cli/internal/api"
	"civitai-cli/internal/report"
)

// promptRecipe is one example image's generation recipe. Numeric fields
// stay json.Number so steps/cfgScale survive string/number drift and
// marshal back as numbers. Empty fields are omitted (design D4: absent
// meta fields are skipped, never rendered as blank holes). Index is the
// 1-based example number; URL identifies the image in machine output.
type promptRecipe struct {
	Index          int         `json:"index"`
	URL            string      `json:"url,omitempty"`
	Prompt         string      `json:"prompt,omitempty"`
	NegativePrompt string      `json:"negativePrompt,omitempty"`
	Steps          json.Number `json:"steps,omitempty"`
	CFGScale       json.Number `json:"cfgScale,omitempty"`
	Sampler        string      `json:"sampler,omitempty"`
	Scheduler      string      `json:"scheduler,omitempty"`
}

// promptsReport is the --json shape: the recipe array plus the count of
// images skipped because their meta was null (parent design D7/D4).
type promptsReport struct {
	Prompts []promptRecipe `json:"prompts"`
	Skipped int            `json:"skipped"`
}

// renderPrompts writes the example-image generation recipes shared by
// show -v --prompts and vet --prompts. Images whose Meta is null are
// skipped and counted. Text mode prints a copyable plain-text block per
// image — [n] header then prompt/negativePrompt/steps/cfgScale/sampler/
// scheduler in that order, absent fields skipped, multi-line prompt text
// preserved verbatim (no quotes, no bullets, no line collapsing — D7).
// JSON mode emits {prompts:[{index,url,...}], skipped:N}.
func renderPrompts(w io.Writer, images []api.Image, jsonMode bool) error {
	recipes := make([]promptRecipe, 0, len(images))
	skipped := 0
	for _, im := range images {
		if im.Meta == nil {
			skipped++
			continue
		}
		recipes = append(recipes, promptRecipe{
			Index:          len(recipes) + 1,
			URL:            im.URL,
			Prompt:         im.Meta.Prompt,
			NegativePrompt: im.Meta.NegativePrompt,
			Steps:          im.Meta.Steps,
			CFGScale:       im.Meta.CFGScale,
			Sampler:        im.Meta.Sampler,
			Scheduler:      im.Meta.Scheduler,
		})
	}

	if jsonMode {
		return report.WriteIndent(w, promptsReport{Prompts: recipes, Skipped: skipped})
	}

	fmt.Fprintf(w, "\nPROMPTS (%d)\n", len(recipes))
	for _, r := range recipes {
		fmt.Fprintf(w, "\n[%d]\n", r.Index)
		if r.Prompt != "" {
			fmt.Fprintf(w, "prompt: %s\n", r.Prompt)
		}
		if r.NegativePrompt != "" {
			fmt.Fprintf(w, "negativePrompt: %s\n", r.NegativePrompt)
		}
		if r.Steps != "" {
			fmt.Fprintf(w, "steps: %s\n", r.Steps)
		}
		if r.CFGScale != "" {
			fmt.Fprintf(w, "cfgScale: %s\n", r.CFGScale)
		}
		if r.Sampler != "" {
			fmt.Fprintf(w, "sampler: %s\n", r.Sampler)
		}
		if r.Scheduler != "" {
			fmt.Fprintf(w, "scheduler: %s\n", r.Scheduler)
		}
	}
	if skipped > 0 {
		fmt.Fprintf(w, "\nskipped %d images without meta\n", skipped)
	}
	return nil
}
