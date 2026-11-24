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

Each example summary (`examples[]`) is a real generation; its prompt text is
available only through an explicit `civ recipe <versionId> --example <sourceIndex>
--json` detail call. Use the summary plus, when needed, that detail as the
baseline:

- `parameters.steps`, `cfgScale`, `sampler`, `scheduler`: adopt these as the
  recommended settings, then adjust for the user's subject or speed/quality
  tradeoff. They are proven to work with this version, unlike generic defaults.
- `example.textParameters.prompt` (detail only): study its structure (ordering,
  emphasis, quality tags) and reuse the skeleton with the user's subject
  substituted. Keep the trigger words intact.
- `example.textParameters.negativePrompt` (detail only): reuse it as the starting
  negative prompt; it reflects what the example needed to suppress. Trim only what
  is irrelevant to the new subject.
- Match the base model: an example from an SDXL version assumes an SDXL checkpoint.
  Check `subject.version.baseModel` before reusing parameters across ecosystems.

## Negative-prompt reuse

- Start from the selected example's `textParameters.negativePrompt` (explicit
  detail) rather than a generic block.
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
- Inventing data: never guess trigger words or recipe parameters. If
  `subject.trainedWords` is empty, or an example's `promptStatus` is `notProvided`
  / `unsupportedJsonShape`, say the data is unavailable instead of fabricating it.
- Ignoring missing metadata: an example whose `metadataStatus` is `notProvided`
  carries no readable generation metadata and is counted in
  `selection.withoutMetadata`; `promptStatus`/`negativePromptStatus` say whether
  text exists. Do not treat a metadata-less example as evidence of a prompt.
- Wrong weight syntax: `<lora:...>` inline works in A1111/Forge, not natively in
  ComfyUI. Provide the weight in the frontend's native form.