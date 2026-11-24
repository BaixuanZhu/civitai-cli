---
name: civ-prompt
description: Craft text-to-image prompts (文生图提示词) for Stable Diffusion, SDXL, Flux, Illustrious, and Anima using real Civitai data through the civ CLI. Use when writing or refining a prompt, choosing a LoRA or checkpoint, needing trigger words (触发词) to activate a LoRA, or reusing example-image recipes (prompt, negative prompt 负向提示词, steps, cfgScale, sampler, scheduler). Runs civ recipe/search/show/vet to ground prompts in verified trigger words and generation recipes instead of guessing.
license: MIT
metadata:
  version: "__CIV_VERSION__"
allowed-tools: Bash(civ:*)
---

# civ-prompt

## Overview

Ground text-to-image prompts in real Civitai data instead of guessing. The `civ`
CLI exposes each model version's trigger words (`trainedWords`) and example-image
generation recipes (prompt, negative prompt, steps, cfgScale, sampler, scheduler).
Use that data to assemble a prompt that actually activates the chosen LoRA and
reproduces a proven look.

`civ` only reads registry data. It does not download models, download images, or
generate images. You assemble the prompt; the user's image tool renders it.

## When to use

- Writing or refining a prompt for Stable Diffusion, SDXL, Flux, Illustrious, or Anima.
- Choosing a LoRA or checkpoint and needing the trigger words that activate it.
- Reproducing a specific example image's look (recipe-driven prompting).
- Auditing a local model file to find its registry version and prompts.

## Workflow

1. Locate the model version. Pick the entry point that matches what you have:
   - Local file (or hash): `civ vet <path> --json`, then use the reported
     `version.id` with `civ recipe <versionId> --json`.
   - Known model and version id: `civ recipe <versionId> --json` (one request,
     prompt-free summary with trigger words, counts, and example summaries).
   - A concept with no id yet: `civ search <concept> --type lora --json` (compact
     envelope by default), then `civ recipe <versionId> --json` on the chosen hit.
2. Extract the data from the JSON:
   - Trigger words: `subject.trainedWords` — the exact strings that activate the LoRA.
   - Example summaries: `examples[]` with `sourceIndex`, `parameters` (steps,
     cfgScale, sampler, scheduler, seed, size, denoise), and text-availability
     statuses. Summaries never contain prompt text, including a summary of one.
   - Full prompt text: only after choosing a `sourceIndex`, run
     `civ recipe <versionId> --example <sourceIndex> --json` and read
     `example.textParameters.prompt` / `.negativePrompt`.
3. Assemble the prompt:
   - Include every required trigger word verbatim, usually near the front.
   - Adopt a recipe's structure and parameters (steps, cfgScale, sampler,
     scheduler) as the recommended baseline, then adjust to the user's subject.
   - Reuse the recipe's negativePrompt as the starting negative prompt.
   - Keep the base model consistent with the version's `baseModel`.
4. Output the final prompt, the negative prompt, the recommended parameters, and
   the trigger-word block. State which version and recipe you grounded it in.

## Key facts

- Trigger words must appear verbatim to activate a LoRA. Never paraphrase,
  translate, or invent them; copy `trainedWords` exactly.
- Recipes are real generations from the version's example images — evidence, not
  suggestions. Prefer them over generic prompt boilerplate.
- Anonymous by default: no token is needed for search/show/vet/tags. A token
  (`civ login`) only unlocks personal-state and gated NSFW queries.
- `--json` is machine-readable; parse it rather than scraping the human table.
- Images whose meta is null are skipped and counted (`skipped`); they carry no recipe.
- `civ recipe` summaries are prompt-free by design. Do not expect or invent prompt
  text from the summary; select one example explicitly for its text.
- `civ search --json` and `civ show --json` default to compact envelopes; use
  `--view full` only when you deliberately need a field the compact view omits.
  `civ show -v --prompts --json` remains a legacy compatibility detail view.
- `civ` does not download models or images and does not generate images.

## Resources

- `references/civ-commands.md` — command syntax, `--json` shapes, and exit codes.
- `references/prompt-patterns.md` — trigger-word placement, LoRA weight syntax,
  recipe adoption, negative-prompt reuse, and common pitfalls.

## Boundaries

- Do not fabricate trigger words or recipe parameters; if data is missing, say so.
- Skip example images whose meta is null (no recipe is available for them).
- Anonymous NSFW results are clamped by the API, so mature recipes may be absent
  without a token. Note this rather than assuming the version has no examples.
- Respect the model's license grants (reported by vet/show) before commercial use.