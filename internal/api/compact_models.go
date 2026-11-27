package api

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// CompactModelSummary is the shared model projection used by compact search
// items and compact model show. It deliberately excludes description HTML,
// files, images, generation metadata, and download URLs.
//
// Presence contract: every key is always present. Scalar members are null
// when the upstream value was absent or null; an explicit upstream
// zero/false/empty string is preserved as that value. Array members are
// null when not provided and [] when the source supplied an empty array.
// Versions keeps every embedded version in source order (no latest-only
// selection), so callers can pick a version without another request.
type CompactModelSummary struct {
	// ID is the model ID; null when upstream identity is absent. It is the
	// value show/recipe/compare need for the next command.
	ID *json.Number `json:"id"`
	// Name is the source display name, verbatim.
	Name *string `json:"name"`
	// URL is the canonical https://civitai.com/models/{id} page, built only
	// when ID is present.
	URL *string `json:"url"`
	// Type preserves unknown upstream type strings verbatim.
	Type *string `json:"type"`
	// Creator is the creator username only.
	Creator *string `json:"creator"`
	// Tags preserves source order; null when not provided.
	Tags []string `json:"tags"`
	// BaseModels preserves source order with no family inference.
	BaseModels []string `json:"baseModels"`
	// NSFW is the source model flag only.
	NSFW *bool `json:"nsfw"`
	// Availability is the model availability, never inferred from versions.
	Availability *string `json:"availability"`
	// License is null only when all four model-level grants are absent.
	License *CompactLicense `json:"license"`
	// Stats is null when upstream stats were not provided; absent stats are
	// never converted into observed zero counters.
	Stats *CompactModelStats `json:"stats"`
	// Versions keeps every embedded version in source order.
	Versions []CompactVersionRef `json:"versions"`
}

// CompactLicense carries the four model-level license grants. Members are
// null when the corresponding source grant was absent or null.
type CompactLicense struct {
	AllowNoCredit         *bool    `json:"allowNoCredit"`
	AllowCommercialUse    []string `json:"allowCommercialUse"`
	AllowDerivatives      *bool    `json:"allowDerivatives"`
	AllowDifferentLicense *bool    `json:"allowDifferentLicense"`
}

// CompactModelStats carries the selection counters of a model. Tips,
// ratios, and derived scores are intentionally omitted.
type CompactModelStats struct {
	DownloadCount   *json.Number `json:"downloadCount"`
	ThumbsUpCount   *json.Number `json:"thumbsUpCount"`
	ThumbsDownCount *json.Number `json:"thumbsDownCount"`
	CommentCount    *json.Number `json:"commentCount"`
}

// CompactVersionRef is the minimal version reference embedded in compact
// search items and compact model show. It excludes trained words, files,
// images, hashes, and per-version download URLs: embedded versions may omit
// those fields or carry incomplete data, so they are not presented as
// canonical version detail. Version show is the detail path.
type CompactVersionRef struct {
	ID            *json.Number         `json:"id"`
	Name          *string              `json:"name"`
	BaseModel     *string              `json:"baseModel"`
	BaseModelType *string              `json:"baseModelType"`
	PublishedAt   *string              `json:"publishedAt"`
	Availability  *string              `json:"availability"`
	Stats         *CompactVersionStats `json:"stats"`
}

// CompactVersionStats carries the adoption counters shared by version
// references and the version summary. Members are null when absent.
type CompactVersionStats struct {
	DownloadCount   *json.Number `json:"downloadCount"`
	ThumbsUpCount   *json.Number `json:"thumbsUpCount"`
	ThumbsDownCount *json.Number `json:"thumbsDownCount"`
}

// ParseCompactModels decodes a /models response body into the versioned
// compact search envelope. It uses the same response the legacy parse path
// uses and adds no request. An absent or null items member is a decode
// error (not a valid search envelope); an explicit empty array yields
// items: [].
func ParseCompactModels(raw []byte) (*CompactSearchEnvelope, error) {
	if err := compactRequireObject(raw); err != nil {
		return nil, err
	}
	var env struct {
		Items json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("api: decode compact search: %w", err)
	}
	if len(env.Items) == 0 {
		return nil, fmt.Errorf("api: compact search response has no items array")
	}
	var items []compactRawModel
	if err := json.Unmarshal(env.Items, &items); err != nil {
		return nil, fmt.Errorf("api: decode compact search items: %w", err)
	}
	if items == nil { // upstream sent items: null
		return nil, fmt.Errorf("api: compact search response has no items array")
	}
	out := &CompactSearchEnvelope{
		SchemaVersion: CompactSearchSchemaVersion,
		Kind:          compactKindModelSearch,
		Items:         make([]CompactModelSummary, len(items)),
	}
	evidence := make([]searchBaseEvidence, len(items))
	for i := range items {
		out.Items[i] = compactProjectModel(&items[i])
		evidence[i] = compactModelBaseEvidence(&items[i])
	}
	out.Summary = summarizeSearchBaseEvidence(evidence)
	return out, nil
}

// ParseCompactModel decodes a /models/{id} response body into the compact
// model envelope. The model is the same summary projection used by compact
// search items; no extra endpoint or enrichment request is made.
func ParseCompactModel(raw []byte) (*CompactModelEnvelope, error) {
	if err := compactRequireObject(raw); err != nil {
		return nil, err
	}
	var src compactRawModel
	if err := json.Unmarshal(raw, &src); err != nil {
		return nil, fmt.Errorf("api: decode compact model: %w", err)
	}
	return &CompactModelEnvelope{
		SchemaVersion: CompactShowSchemaVersion,
		Kind:          compactKindModelSummary,
		Model:         compactProjectModel(&src),
	}, nil
}

// searchBaseEvidence is the raw summary input for one returned model: the
// model-level baseModels strings and the baseModel strings of every
// embedded version, in source order. Adapters only extract these fields;
// the fallback and deduplication policy lives solely in
// summarizeSearchBaseEvidence so compact JSON and human output can never
// disagree.
type searchBaseEvidence struct {
	modelBases   []string
	versionBases []string
}

// compactModelBaseEvidence extracts summary evidence from one compact raw
// model, preserving nullable version-base presence: an absent or null
// version baseModel contributes nothing.
func compactModelBaseEvidence(m *compactRawModel) searchBaseEvidence {
	ev := searchBaseEvidence{modelBases: m.BaseModels}
	for i := range m.ModelVersions {
		if b := m.ModelVersions[i].BaseModel; b != nil {
			ev.versionBases = append(ev.versionBases, *b)
		}
	}
	return ev
}

// legacyModelBaseEvidence extracts summary evidence from one legacy
// decoded Model. Legacy version bases are plain strings; an absent base is
// the empty string, which the core discards as blank.
func legacyModelBaseEvidence(m *Model) searchBaseEvidence {
	ev := searchBaseEvidence{modelBases: m.BaseModels}
	for i := range m.ModelVersions {
		ev.versionBases = append(ev.versionBases, m.ModelVersions[i].BaseModel)
	}
	return ev
}

// SummarizeSearchModels projects the legacy decoded search models into the
// same SearchResultSummary the compact envelope carries, so the human
// footer and compact JSON report identical facts for one response. It is a
// thin adapter over the shared evidence core: no request, no pagination,
// and no primary/default/latest base selection.
func SummarizeSearchModels(models []Model) SearchResultSummary {
	evidence := make([]searchBaseEvidence, len(models))
	for i := range models {
		evidence[i] = legacyModelBaseEvidence(&models[i])
	}
	return summarizeSearchBaseEvidence(evidence)
}

// summarizeSearchBaseEvidence is the single owner of the base-evidence
// policy. Per model: usable model-level bases win; only when none remain
// (absent, null, empty, or all blank) does it fall back to the bases of
// ALL embedded versions; a still-empty set counts the model as without
// base. Selected names are deduplicated by exact original value — blank
// strings are rejected, but retained names are never trimmed, case-folded,
// or canonicalized. Coverage counts models, not occurrences: each distinct
// base contributes at most one per model, and a model with several
// distinct bases increments each of them plus ModelsWithMultipleBases
// once. Entries are sorted by count descending, then exact name ascending,
// so map iteration and input order cannot leak into the output.
func summarizeSearchBaseEvidence(evidence []searchBaseEvidence) SearchResultSummary {
	summary := SearchResultSummary{
		Returned:     len(evidence),
		BaseCoverage: []SearchBaseCoverage{},
	}
	coverage := map[string]int{}
	for _, ev := range evidence {
		bases := usableBaseSet(ev.modelBases)
		if len(bases) == 0 {
			bases = usableBaseSet(ev.versionBases)
		}
		if len(bases) == 0 {
			summary.ModelsWithoutBase++
			continue
		}
		if len(bases) > 1 {
			summary.ModelsWithMultipleBases++
		}
		for base := range bases {
			coverage[base]++
		}
	}
	for base, models := range coverage {
		summary.BaseCoverage = append(summary.BaseCoverage, SearchBaseCoverage{BaseModel: base, Models: models})
	}
	sort.Slice(summary.BaseCoverage, func(i, j int) bool {
		a, b := summary.BaseCoverage[i], summary.BaseCoverage[j]
		if a.Models != b.Models {
			return a.Models > b.Models
		}
		return a.BaseModel < b.BaseModel
	})
	return summary
}

// usableBaseSet deduplicates base names by their exact original value.
// Values whose TrimSpace is empty are unusable evidence; every other value
// is kept verbatim, whitespace and case included.
func usableBaseSet(values []string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			continue
		}
		set[v] = struct{}{}
	}
	return set
}

// compactRawModel declares only the fields the model summary contracts.
// Pointer members distinguish an absent/null source value from an explicit
// zero, false, or empty value.
type compactRawModel struct {
	ID                    *json.Number           `json:"id"`
	Name                  *string                `json:"name"`
	Type                  *string                `json:"type"`
	NSFW                  *bool                  `json:"nsfw"`
	Tags                  []string               `json:"tags"`
	BaseModels            []string               `json:"baseModels"`
	Availability          *string                `json:"availability"`
	Creator               *compactRawCreator     `json:"creator"`
	AllowNoCredit         *bool                  `json:"allowNoCredit"`
	AllowCommercialUse    []string               `json:"allowCommercialUse"`
	AllowDerivatives      *bool                  `json:"allowDerivatives"`
	AllowDifferentLicense *bool                  `json:"allowDifferentLicense"`
	Stats                 *compactRawModelStats  `json:"stats"`
	ModelVersions         []compactRawVersionRef `json:"modelVersions"`
}

// compactRawCreator is the creator block reduced to the username.
type compactRawCreator struct {
	Username *string `json:"username"`
}

// compactRawModelStats is the model stats source with absent-preserving
// members. Only counters that appear in the compact registry are declared.
type compactRawModelStats struct {
	DownloadCount   *json.Number `json:"downloadCount"`
	ThumbsUpCount   *json.Number `json:"thumbsUpCount"`
	ThumbsDownCount *json.Number `json:"thumbsDownCount"`
	CommentCount    *json.Number `json:"commentCount"`
}

// compactRawVersionRef is the embedded-version source for the versionRef
// projection.
type compactRawVersionRef struct {
	ID            *json.Number            `json:"id"`
	Name          *string                 `json:"name"`
	BaseModel     *string                 `json:"baseModel"`
	BaseModelType *string                 `json:"baseModelType"`
	PublishedAt   *string                 `json:"publishedAt"`
	Availability  *string                 `json:"availability"`
	Stats         *compactRawVersionStats `json:"stats"`
}

// compactRawVersionStats is the shared version-stats source.
type compactRawVersionStats struct {
	DownloadCount   *json.Number `json:"downloadCount"`
	ThumbsUpCount   *json.Number `json:"thumbsUpCount"`
	ThumbsDownCount *json.Number `json:"thumbsDownCount"`
}

// compactProjectModel builds the shared modelSummary projection. No field
// is inferred: missing values stay null, and URLs are derived only from a
// supplied ID.
func compactProjectModel(src *compactRawModel) CompactModelSummary {
	out := CompactModelSummary{
		ID:           src.ID,
		Name:         src.Name,
		Type:         src.Type,
		NSFW:         src.NSFW,
		Tags:         src.Tags,
		BaseModels:   src.BaseModels,
		Availability: src.Availability,
	}
	if src.Creator != nil {
		out.Creator = src.Creator.Username
	}
	if src.ID != nil {
		url := "https://civitai.com/models/" + src.ID.String()
		out.URL = &url
	}
	if src.AllowNoCredit != nil || src.AllowCommercialUse != nil ||
		src.AllowDerivatives != nil || src.AllowDifferentLicense != nil {
		out.License = &CompactLicense{
			AllowNoCredit:         src.AllowNoCredit,
			AllowCommercialUse:    src.AllowCommercialUse,
			AllowDerivatives:      src.AllowDerivatives,
			AllowDifferentLicense: src.AllowDifferentLicense,
		}
	}
	if src.Stats != nil {
		out.Stats = &CompactModelStats{
			DownloadCount:   src.Stats.DownloadCount,
			ThumbsUpCount:   src.Stats.ThumbsUpCount,
			ThumbsDownCount: src.Stats.ThumbsDownCount,
			CommentCount:    src.Stats.CommentCount,
		}
	}
	if src.ModelVersions != nil {
		out.Versions = make([]CompactVersionRef, len(src.ModelVersions))
		for i := range src.ModelVersions {
			out.Versions[i] = compactProjectVersionRef(&src.ModelVersions[i])
		}
	}
	return out
}

// compactProjectVersionRef builds one versionRef, preserving source order
// at the caller.
func compactProjectVersionRef(src *compactRawVersionRef) CompactVersionRef {
	out := CompactVersionRef{
		ID:            src.ID,
		Name:          src.Name,
		BaseModel:     src.BaseModel,
		BaseModelType: src.BaseModelType,
		PublishedAt:   src.PublishedAt,
		Availability:  src.Availability,
	}
	if src.Stats != nil {
		out.Stats = compactProjectVersionStats(src.Stats)
	}
	return out
}

// compactProjectVersionStats copies the three contracted version counters,
// keeping per-member absence.
func compactProjectVersionStats(src *compactRawVersionStats) *CompactVersionStats {
	return &CompactVersionStats{
		DownloadCount:   src.DownloadCount,
		ThumbsUpCount:   src.ThumbsUpCount,
		ThumbsDownCount: src.ThumbsDownCount,
	}
}
