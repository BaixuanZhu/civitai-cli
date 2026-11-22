# Prompt assembly patterns

How to turn civ trigger words and example-image recipes into a working prompt.
Ground every choice in the data civ returns; do not invent triggers or parameters.

## Trigger-word placement

- Copy `trainedWords` verbatim. They are the exact strings the LoRA was trained on;
  paraphrasing, translating, or "cleaning them up" stops the LoRA from activating.
- Place trigger words near the front of the prompt so they are not diluted by a
  long subject description. A common order: trigger words, then subject, then
  style/quality tags.
- Include every required trigger word. Some LoRAs need more than one; missing any
  of them weakens or breaks the effect.
- Trigger words are case-sensitive in effect; reproduce the casing civ returns.

## LoRA weight syntax

The inline weight syntax differs by frontend:

- Automatic1111 / Forge (built-in LoRA support): `<lora:filename:weight>` inline in
  the prompt, e.g. `<lora:myStyle:0.8>`. `filename` is the LoRA file name without
  its extension (not necessarily the Civitai model name). Weight is typically
  0.6-1.0; start at the value the recipe implies and tune down if the effect is
  too strong or artifacts appear.
- ComfyUI: LoRAs are loaded with a LoraLoader node (`strength_model`,
  `strength_clip`), not inline prompt syntax. The prompt text usually carries only
  the trigger words; the weight lives on the node. Some custom-node packs accept
  `<lora:...>` in text, but do not assume it.
- When unsure of the frontend, provide the trigger words plus a recommended weight
  and let the user apply it in their tool's native mechanism.

## Adopting recipe parameters

Each recipe (`prompts[]`) is a real generation. Use it as the baseline:

- `steps`, `cfgScale`, `sampler`, `scheduler`: adopt these as the recommended
  settings, then adjust for the user's subject or speed/quality tradeoff. They are
  proven to work with this version, unlike generic defaults.
- `prompt`: study its structure (ordering, emphasis, quality tags) and reuse the
  skeleton with the user's subject substituted. Keep the trigger words intact.
- `negativePrompt`: reuse it as the starting negative prompt; it reflects what the
  example needed to suppress. Trim only what is irrelevant to the new subject.
- Match the base model: a recipe for an SDXL version assumes an SDXL checkpoint.
  Check `version.baseModel` before reusing parameters across ecosystems.

## Negative-prompt reuse

- Start from the recipe's `negativePrompt` rather than a generic block.
- Keep ecosystem-appropriate negatives (e.g. SD1.5 vs SDXL vs Flux differ; Flux
  often needs little or no negative prompt).
- Add subject-specific negatives only when the user's request calls for them.

## Common pitfalls

- Missing trigger word: the LoRA loads but does not activate. Always include every
  `trainedWords` entry verbatim.
- Over-stacking: too many LoRAs or conflicting trigger words muddy the result.
  Prefer one or two well-chosen LoRAs with compatible triggers.
- Base-model mismatch: an SD1.5 LoRA on an SDXL checkpoint (or vice versa) fails or
  degrades. Confirm `baseModel` matches the user's checkpoint.
- Inventing data: never guess trigger words or recipe parameters. If civ returns no
  trigger words or all example meta is null (`skipped` equals the image count), say
  the data is unavailable instead of fabricating it.
- Ignoring null meta: images with null meta carry no recipe; they are counted in
  `skipped`. Do not treat a skipped image as evidence of a prompt.
- Wrong weight syntax: `<lora:...>` inline works in A1111/Forge, not natively in
  ComfyUI. Provide the weight in the frontend's native form.