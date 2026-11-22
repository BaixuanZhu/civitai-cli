package api

import (
	"sort"
	"strings"
)

// Enum mapping tables: CLI values (lowercase, hyphenated) → API values.
// All tables were pinned against live ZodError enum leaks on 2026-11-22
// (testdata/probes/enum_sort.json, enum_types.json, enum_period.json).
// Lookups are case-insensitive; unknown sort/type/period are usage errors
// at the CLI boundary, while unknown baseModel values only warn because
// the API silently ignores them (design D5).

// sortMap maps CLI --sort values to API sort enums (spaces included —
// URLs are always built via url.Values.Encode, never hand-concatenated).
// Note: "Recently Added" is listed by the API enum but requires a
// collectionId on /models (HTTP 400 otherwise); the mapping is kept so
// the server's explanatory error reaches the user.
var sortMap = map[string]string{
	"hot":             "Hot",
	"highest-rated":   "Highest Rated",
	"most-downloaded": "Most Downloaded",
	"most-liked":      "Most Liked",
	"most-discussed":  "Most Discussed",
	"most-collected":  "Most Collected",
	"most-images":     "Most Images",
	"newest":          "Newest",
	"oldest":          "Oldest",
	"recently-added":  "Recently Added",
}

// periodMap maps CLI --period values to API period enums.
var periodMap = map[string]string{
	"day":   "Day",
	"week":  "Week",
	"month": "Month",
	"year":  "Year",
	"all":   "AllTime",
}

// typeMap maps CLI --type values (and aliases) to API type enums.
// The API accepts all 23 registered values; short aliases follow the
// plan's UX vocabulary (ti, hypernet, ...).
var typeMap = map[string]string{
	"checkpoint":        "Checkpoint",
	"textualinversion":  "TextualInversion",
	"ti":                "TextualInversion",
	"hypernetwork":      "Hypernetwork",
	"hypernet":          "Hypernetwork",
	"aestheticgradient": "AestheticGradient",
	"lora":              "LORA",
	"locon":             "LoCon",
	"dora":              "DoRA",
	"controlnet":        "Controlnet",
	"upscaler":          "Upscaler",
	"motionmodule":      "MotionModule",
	"vae":               "VAE",
	"textencoder":       "TextEncoder",
	"unet":              "UNet",
	"clipvision":        "CLIPVision",
	"poses":             "Poses",
	"wildcards":         "Wildcards",
	"workflows":         "Workflows",
	"comfyworkflows":    "ComfyWorkflows",
	"detection":         "Detection",
	"visionlanguage":    "VisionLanguage",
	"clip":              "CLIP",
	"llm":               "LLM",
	"other":             "Other",
}

// baseModelWhitelist maps lowercase baseModel names to their canonical
// platform spelling. Verified live 2026-11-22: the singular baseModel
// query param is silently ignored for ALL values, so the CLI sends the
// plural baseModels param; this whitelist was pinned by per-value filter
// probes union values harvested from live model data (63 values, see
// testdata/probes/base_model_verified.json). Unknown values warn but do
// not block: the API returns empty results for them rather than erroring.
var baseModelWhitelist = map[string]string{
	"anima":                  "Anima",
	"auraflow":               "AuraFlow",
	"chroma":                 "Chroma",
	"cogvideox":              "CogVideoX",
	"flux.1 d":               "Flux.1 D",
	"flux.1 kontext":         "Flux.1 Kontext",
	"flux.1 krea":            "Flux.1 Krea",
	"flux.1 s":               "Flux.1 S",
	"flux.2 d":               "Flux.2 D",
	"flux.2 klein 4b-base":   "Flux.2 Klein 4B-base",
	"flux.2 klein 9b-base":   "Flux.2 Klein 9B-base",
	"hidream":                "HiDream",
	"hunyuan 1":              "Hunyuan 1",
	"hunyuan video":          "Hunyuan Video",
	"ideogram 4.0":           "Ideogram 4.0",
	"illustrious":            "Illustrious",
	"kolors":                 "Kolors",
	"krea 2":                 "Krea 2",
	"krea 2 turbo":           "Krea 2 Turbo",
	"ltxv":                   "LTXV",
	"ltxv 2.3":               "LTXV 2.3",
	"ltxv 2.5":               "LTXV 2.5",
	"ltxv2":                  "LTXV2",
	"lumina":                 "Lumina",
	"moss image":             "MOSS Image",
	"minimax h3":             "MiniMax H3",
	"mochi":                  "Mochi",
	"noobai":                 "NoobAI",
	"odor":                   "ODOR",
	"other":                  "Other",
	"pixart e":               "PixArt E",
	"pixart a":               "PixArt a",
	"pony":                   "Pony",
	"qwen":                   "Qwen",
	"qwen 2.1":               "Qwen 2.1",
	"sd 1.4":                 "SD 1.4",
	"sd 1.5":                 "SD 1.5",
	"sd 1.5 hyper":           "SD 1.5 Hyper",
	"sd 1.5 lcm":             "SD 1.5 LCM",
	"sd 2.0":                 "SD 2.0",
	"sd 2.0 768":             "SD 2.0 768",
	"sd 2.1":                 "SD 2.1",
	"sd 2.1 768":             "SD 2.1 768",
	"sd 2.1 unclip":          "SD 2.1 Unclip",
	"sdxl 0.9":               "SDXL 0.9",
	"sdxl 1.0":               "SDXL 1.0",
	"sdxl 1.0 lcm":           "SDXL 1.0 LCM",
	"sdxl distilled":         "SDXL Distilled",
	"sdxl hyper":             "SDXL Hyper",
	"sdxl lightning":         "SDXL Lightning",
	"svd xt":                 "SVD XT",
	"stable cascade":         "Stable Cascade",
	"upscaler":               "Upscaler",
	"wan video":              "Wan Video",
	"wan video 1.3b t2v":     "Wan Video 1.3B t2v",
	"wan video 14b i2v 480p": "Wan Video 14B i2v 480p",
	"wan video 14b i2v 720p": "Wan Video 14B i2v 720p",
	"wan video 14b t2v":      "Wan Video 14B t2v",
	"wan video 2.2 i2v-a14b": "Wan Video 2.2 I2V-A14B",
	"wan video 2.2 t2v-a14b": "Wan Video 2.2 T2V-A14B",
	"wan video 2.2 ti2v-5b":  "Wan Video 2.2 TI2V-5B",
	"zimagebase":             "ZImageBase",
	"zimageturbo":            "ZImageTurbo",
}

// MapSort maps a CLI sort value (case-insensitive) to its API enum.
// The bool reports whether the value is registered.
func MapSort(cli string) (string, bool) {
	v, ok := sortMap[strings.ToLower(strings.TrimSpace(cli))]
	return v, ok
}

// MapPeriod maps a CLI period value (case-insensitive) to its API enum.
func MapPeriod(cli string) (string, bool) {
	v, ok := periodMap[strings.ToLower(strings.TrimSpace(cli))]
	return v, ok
}

// MapType maps a CLI model-type value or alias (case-insensitive) to its
// API enum.
func MapType(cli string) (string, bool) {
	v, ok := typeMap[strings.ToLower(strings.TrimSpace(cli))]
	return v, ok
}

// CanonicalBaseModel maps a baseModel filter value (case-insensitive) to
// its canonical platform spelling. Unknown values return ok=false; the
// caller should warn and pass the original value through (the API will
// silently return no matches for it).
func CanonicalBaseModel(cli string) (string, bool) {
	v, ok := baseModelWhitelist[strings.ToLower(strings.TrimSpace(cli))]
	return v, ok
}

// SortChoices lists the registered CLI --sort values in lexical order,
// for usage-error messages.
func SortChoices() []string { return sortedKeys(sortMap) }

// PeriodChoices lists the registered CLI --period values in lexical order.
func PeriodChoices() []string { return sortedKeys(periodMap) }

// TypeChoices lists the registered CLI --type values in lexical order.
func TypeChoices() []string { return sortedKeys(typeMap) }

// BaseModelChoices lists the whitelisted canonical baseModel values in
// lexical order, for warning messages and documentation.
func BaseModelChoices() []string {
	out := make([]string, 0, len(baseModelWhitelist))
	for _, v := range baseModelWhitelist {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// sortedKeys returns the map keys in lexical order.
func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
