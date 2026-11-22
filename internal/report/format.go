// Package report renders command results: aligned tables (text/tabwriter),
// machine-readable JSON, and the shared formatting vocabulary (k/M
// abbreviations, YYYY-MM-DD dates, ratios, NSFW bitmask text, file sizes).
// Human output goes to stdout; this package never logs. All formatting is
// English-only by project convention (zh-CN consoles mojibake).
package report

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// CompactNumber abbreviates counts: 999 → "999", 45200 → "45.2k",
// 1234567 → "1.2M" (one decimal place for k/M).
func CompactNumber(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return strconv.FormatInt(n, 10)
	}
}

// DateText extracts YYYY-MM-DD from an ISO8601 timestamp. Anything that
// does not look like a date (including "") is returned unchanged so
// upstream drift stays visible instead of being silently reformatted.
func DateText(s string) string {
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		return s[:10]
	}
	return s
}

// RatioText renders a thumbs-up:thumbs-down ratio as "N:1", rounded to
// the nearest integer. Zero downvotes are treated as 1 (no division by
// zero; a model with only upvotes reads "N:1").
func RatioText(up, down int64) string {
	if down < 1 {
		down = 1
	}
	r := (up + down/2) / down
	return fmt.Sprintf("%d:1", r)
}

// nsfwBits are the known bitmask positions of the models-endpoint
// nsfwLevel (verified live 2026-11-22, appendix C of the plan). Unknown
// higher bits render as "L<bit>" instead of failing.
var nsfwBits = []struct {
	bit  int64
	name string
}{
	{1, "None"},
	{2, "Soft"},
	{4, "Mature"},
	{8, "X"},
}

// NSFWLevelText renders the numeric nsfwLevel bitmask by joining known
// bit names with "+" (7 → "None+Soft+Mature"); unknown bits append
// "L<bit>". Non-numeric content (string-form drift like "Soft") is
// returned as-is, and "" stays "" (missing → render layer shows "-").
func NSFWLevelText(level json.Number) string {
	if level == "" {
		return ""
	}
	v, err := level.Int64()
	if err != nil {
		return level.String()
	}
	if v == 0 {
		return "None"
	}
	var parts []string
	for _, k := range nsfwBits {
		if v&k.bit != 0 {
			parts = append(parts, k.name)
			v &^= k.bit
		}
	}
	// Remaining unknown bits, lowest first (rem & -rem isolates the
	// lowest set bit; works for every position including the top one).
	for rem := v; rem > 0; {
		bit := rem & -rem
		parts = append(parts, fmt.Sprintf("L%d", bit))
		rem &^= bit
	}
	return strings.Join(parts, "+")
}

// FormatSizeKB renders a file size given in KB as a human string with
// one decimal: 231400 → "226.0 MB", 4084209.625 → "3.9 GB".
func FormatSizeKB(kb float64) string {
	switch {
	case kb >= 1024*1024:
		return fmt.Sprintf("%.1f GB", kb/(1024*1024))
	case kb >= 1024:
		return fmt.Sprintf("%.1f MB", kb/1024)
	default:
		return fmt.Sprintf("%.1f KB", kb)
	}
}

// typeDisplay maps API type enums to compact table labels. Unknown
// values fall through unchanged (design D4: never fail on enum drift).
var typeDisplay = map[string]string{
	"Checkpoint":        "CKPT",
	"TextualInversion":  "TI",
	"Hypernetwork":      "Hyper",
	"AestheticGradient": "AesGr",
	"LORA":              "LORA",
	"LoCon":             "LoCon",
	"DoRA":              "DoRA",
	"Controlnet":        "Ctrl",
	"Upscaler":          "Upscl",
	"MotionModule":      "Motion",
	"VAE":               "VAE",
	"TextEncoder":       "TxtEnc",
	"UNet":              "UNet",
	"CLIPVision":        "CLIPV",
	"Poses":             "Poses",
	"Wildcards":         "Wild",
	"Workflows":         "Wflow",
	"ComfyWorkflows":    "CWflow",
	"Detection":         "Detect",
	"VisionLanguage":    "VLM",
	"CLIP":              "CLIP",
	"LLM":               "LLM",
	"Other":             "Other",
}

// DisplayType returns the compact table label for an API model type,
// preserving unknown values as-is.
func DisplayType(apiType string) string {
	if d, ok := typeDisplay[apiType]; ok {
		return d
	}
	return apiType
}

// YesNo renders a bool as "yes"/"no" for license and scan fields.
func YesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
