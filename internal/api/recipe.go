package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// Recipe contract constants (task 11-24-recipe-context, RT2). The schema
// identifier is part of the public machine contract: incompatible field or
// type changes require a new identifier, and no schema bump may ever add
// prompt text to a summary.
const (
	recipeSchemaVersion     = "civ.recipe/v1"
	recipeSummaryKind       = "recipeSummary"
	recipeExampleDetailKind = "recipeExampleDetail"
	recipeSourceOfficialAPI = "civitai.officialApi"
	recipeSourceExample     = "civitai.versionExample"

	recipeStatusAvailable   = "available"
	recipeStatusProvided    = "provided"
	recipeStatusEmpty       = "empty"
	recipeStatusNotProvided = "notProvided"
	recipeStatusUnsupported = "unsupportedJsonShape"

	recipeModeFirst       = "first"
	recipeModeSourceIndex = "sourceIndex"

	recipeReasonNotProvided      = "notProvided"
	recipeReasonInvalidNumber    = "invalidNumber"
	recipeReasonUnsupportedShape = "unsupportedJsonShape"
)

// RecipeIssue is one structured source-availability or shape problem.
// Field is a stable dotted path relative to the owning object (for example
// "parameters.width", "model.name", or "trainedWords"). Reason is one of
// "notProvided", "invalidNumber", or "unsupportedJsonShape". Issues carry
// no source text, so they are safe in summaries.
type RecipeIssue struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

// RecipeProvenance identifies the first-party source of a recipe view.
// VersionID is the validated requested identity; APIPath is the logical
// endpoint without host or credentials, so test servers never leak in.
type RecipeProvenance struct {
	Source    string `json:"source"`
	APIPath   string `json:"apiPath"`
	VersionID int    `json:"versionId"`
}

// RecipeModelIdentity is the owning-model context of a version. Fields are
// nullable because a standalone version may omit modelId or the embedded
// model summary; absence is reported instead of fetched from /models/{id}.
type RecipeModelIdentity struct {
	ID   *int64  `json:"id"`
	Name *string `json:"name"`
	Type *string `json:"type"`
}

// RecipeVersionIdentity is the model-version context. Values are preserved
// verbatim (unknown base models and types stay strings); a missing value is
// null with a corresponding issue.
type RecipeVersionIdentity struct {
	ID            *int64  `json:"id"`
	Name          *string `json:"name"`
	BaseModel     *string `json:"baseModel"`
	BaseModelType *string `json:"baseModelType"`
	PublishedAt   *string `json:"publishedAt"`
}

// RecipeSubject is the model/version context shared by both recipe views.
// TrainedWords is always an array: absent/null becomes [] with a
// notProvided issue, and an explicit [] stays empty without one. Values
// keep source order and are never trimmed, deduplicated, or invented.
type RecipeSubject struct {
	Model        RecipeModelIdentity   `json:"model"`
	Version      RecipeVersionIdentity `json:"version"`
	TrainedWords []string              `json:"trainedWords"`
}

// RecipeSelection describes how examples were chosen. Counts cover the full
// provided source array, not only the returned window. Total, WithMetadata,
// and WithoutMetadata are null when the source did not provide an images
// array; Truncated is null when the total is unknown and always null for
// singular detail selection.
type RecipeSelection struct {
	Mode            string `json:"mode"`
	Requested       int    `json:"requested"`
	Returned        int    `json:"returned"`
	Total           *int   `json:"total"`
	WithMetadata    *int   `json:"withMetadata"`
	WithoutMetadata *int   `json:"withoutMetadata"`
	Truncated       *bool  `json:"truncated"`
}

// RecipeExampleProvenance identifies one official example image. ImageURL
// is an evidence location only; it is never fetched.
type RecipeExampleProvenance struct {
	Source    string  `json:"source"`
	VersionID int     `json:"versionId"`
	ImageURL  *string `json:"imageUrl"`
}

// RecipeParameters is the approved small-parameter whitelist. Every field
// is null when absent, null, invalid, or an unsupported shape; the
// accompanying issue explains why. Values are direct source values (or
// numeric strings normalized by the shared recipe normalizer) — never
// inferred from comfy, outer image dimensions, other examples, or engine
// defaults. Seed is a decimal string so large integers survive JavaScript
// and other IEEE-754 consumers exactly.
type RecipeParameters struct {
	Steps     *int64   `json:"steps"`
	CFGScale  *float64 `json:"cfgScale"`
	Sampler   *string  `json:"sampler"`
	Scheduler *string  `json:"scheduler"`
	Seed      *string  `json:"seed"`
	Width     *int64   `json:"width"`
	Height    *int64   `json:"height"`
	Denoise   *float64 `json:"denoise"`
}

// RecipeSummaryExample is the prompt-free projection of one source
// example. It deliberately has no prompt, negativePrompt, raw meta, or
// workflow member: summary redaction is a type-level property, not a
// renderer convention.
type RecipeSummaryExample struct {
	SourceIndex          int                     `json:"sourceIndex"`
	Provenance           RecipeExampleProvenance `json:"provenance"`
	MetadataStatus       string                  `json:"metadataStatus"`
	PromptStatus         string                  `json:"promptStatus"`
	NegativePromptStatus string                  `json:"negativePromptStatus"`
	Parameters           *RecipeParameters       `json:"parameters"`
	Issues               []RecipeIssue           `json:"issues"`
}

// RecipeSummary is the bounded, prompt-free recipe summary view. Examples
// is always present (possibly []) and never carries a singular example.
type RecipeSummary struct {
	SchemaVersion string                 `json:"schemaVersion"`
	Kind          string                 `json:"kind"`
	Provenance    RecipeProvenance       `json:"provenance"`
	Subject       RecipeSubject          `json:"subject"`
	Selection     RecipeSelection        `json:"selection"`
	Examples      []RecipeSummaryExample `json:"examples"`
	Issues        []RecipeIssue          `json:"issues"`
}

// RecipeTextParameters is the detail-only large-text member. It is only
// reachable through the singular detail view.
type RecipeTextParameters struct {
	Prompt         *string `json:"prompt"`
	NegativePrompt *string `json:"negativePrompt"`
}

// RecipeDetailExample embeds every prompt-free summary field and adds the
// singular text parameters for exactly one selected source example.
type RecipeDetailExample struct {
	RecipeSummaryExample
	TextParameters *RecipeTextParameters `json:"textParameters"`
}

// RecipeDetail is the explicit singular example-detail view. It has no
// examples array; only the selected source index's text is projected.
type RecipeDetail struct {
	SchemaVersion string               `json:"schemaVersion"`
	Kind          string               `json:"kind"`
	Provenance    RecipeProvenance     `json:"provenance"`
	Subject       RecipeSubject        `json:"subject"`
	Selection     RecipeSelection      `json:"selection"`
	Example       *RecipeDetailExample `json:"example"`
	Issues        []RecipeIssue        `json:"issues"`
}

// recipeSource is the narrow presence-aware decode of one version-detail
// body. Every recipe-relevant field stays a json.RawMessage so the
// projector can distinguish absent, null, empty, and unsupported shapes —
// the legacy value-typed ImageMeta/ModelVersion cannot express those
// states. Unknown fields (description, files, stats, comfy, ...) are
// ignored by design.
type recipeSource struct {
	ID            json.RawMessage `json:"id"`
	ModelID       json.RawMessage `json:"modelId"`
	Name          json.RawMessage `json:"name"`
	BaseModel     json.RawMessage `json:"baseModel"`
	BaseModelType json.RawMessage `json:"baseModelType"`
	PublishedAt   json.RawMessage `json:"publishedAt"`
	TrainedWords  json.RawMessage `json:"trainedWords"`
	Model         json.RawMessage `json:"model"`
	Images        json.RawMessage `json:"images"`
}

// recipeSourceModel is the owning-model summary embedded in a standalone
// version response.
type recipeSourceModel struct {
	Name json.RawMessage `json:"name"`
	Type json.RawMessage `json:"type"`
}

// recipeSourceImage is the narrow decode of one images[] entry. HasMeta is
// deliberately not read: it is a hint, and null meta stays withoutMetadata
// even when hasMeta is true.
type recipeSourceImage struct {
	URL  json.RawMessage `json:"url"`
	Meta json.RawMessage `json:"meta"`
}

// recipeSourceMeta is the presence-aware decode of one image's generation
// metadata. Only the approved fields are declared; comfy, models,
// resources, engine, and other large bags are ignored.
type recipeSourceMeta struct {
	Prompt         json.RawMessage `json:"prompt"`
	NegativePrompt json.RawMessage `json:"negativePrompt"`
	Steps          json.RawMessage `json:"steps"`
	CFGScale       json.RawMessage `json:"cfgScale"`
	Sampler        json.RawMessage `json:"sampler"`
	Scheduler      json.RawMessage `json:"scheduler"`
	Seed           json.RawMessage `json:"seed"`
	Width          json.RawMessage `json:"width"`
	Height         json.RawMessage `json:"height"`
	Denoise        json.RawMessage `json:"denoise"`
}

// recipeRawKind classifies the JSON shape of a raw field for presence and
// drift handling. Absent and null are distinct from an explicit empty
// string, and container shapes are distinct from scalars.
type recipeRawKind int

const (
	recipeRawAbsent recipeRawKind = iota
	recipeRawNull
	recipeRawString
	recipeRawNumber
	recipeRawObject
	recipeRawArray
	recipeRawOther
)

// classifyRecipeRaw inspects the first byte of a raw JSON value. The input
// always comes from encoding/json, so the non-null keywords can only be
// booleans; anything else starting a number is a number literal.
func classifyRecipeRaw(raw json.RawMessage) recipeRawKind {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return recipeRawAbsent
	}
	switch trimmed[0] {
	case 'n':
		return recipeRawNull
	case '"':
		return recipeRawString
	case '{':
		return recipeRawObject
	case '[':
		return recipeRawArray
	case 't', 'f':
		return recipeRawOther
	default:
		return recipeRawNumber
	}
}

// recipeScalarResult is one normalized scalar: Value is nil when the
// source was absent, null, invalid, or an unsupported shape, and Reason
// carries the issue reason in that case (empty on success).
type recipeScalarResult[T any] struct {
	Value  *T
	Reason string
}

// recipeNumberText extracts the decimal text of a numeric field,
// accepting both a JSON number literal and a numeric string (the approved
// shared normalizer's string drift rule). Malformed numeric text yields
// invalidNumber; containers and booleans yield unsupportedJsonShape.
func recipeNumberText(raw json.RawMessage) (*string, string) {
	switch classifyRecipeRaw(raw) {
	case recipeRawAbsent, recipeRawNull:
		return nil, recipeReasonNotProvided
	case recipeRawString:
		s, ok := rawJSONString(raw)
		if !ok || !isDecimalNumberText(s) {
			return nil, recipeReasonInvalidNumber
		}
		return &s, ""
	case recipeRawNumber:
		s := string(bytes.TrimSpace(raw))
		if !isDecimalNumberText(s) {
			return nil, recipeReasonInvalidNumber
		}
		return &s, ""
	default:
		return nil, recipeReasonUnsupportedShape
	}
}

// isDecimalNumberText rejects non-decimal numeric syntaxes (hex, binary,
// octal prefixes, and fractions). json.RawMessage numbers are already
// decimal-only, but numeric strings can carry arbitrary text.
func isDecimalNumberText(s string) bool {
	if s == "" {
		return false
	}
	return !strings.ContainsAny(s, "/xXbBoOpP")
}

// recipeIntegerValue normalizes a field the contract defines as an
// integer: JSON numbers with no fractional part and numeric strings are
// accepted; fractions are rejected rather than rounded.
func recipeIntegerValue(raw json.RawMessage) recipeScalarResult[int64] {
	text, reason := recipeNumberText(raw)
	if text == nil {
		return recipeScalarResult[int64]{Reason: reason}
	}
	if v, err := strconv.ParseInt(*text, 10, 64); err == nil {
		return recipeScalarResult[int64]{Value: &v}
	}
	f, err := strconv.ParseFloat(*text, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || math.Trunc(f) != f {
		return recipeScalarResult[int64]{Reason: recipeReasonInvalidNumber}
	}
	if f < math.MinInt64 || f > math.MaxInt64 {
		return recipeScalarResult[int64]{Reason: recipeReasonInvalidNumber}
	}
	v := int64(f)
	return recipeScalarResult[int64]{Value: &v}
}

// recipeFloatValue normalizes a field the contract defines as a free
// numeric value (cfgScale, denoise). Non-finite values cannot appear in
// JSON output and are rejected.
func recipeFloatValue(raw json.RawMessage) recipeScalarResult[float64] {
	text, reason := recipeNumberText(raw)
	if text == nil {
		return recipeScalarResult[float64]{Reason: reason}
	}
	f, err := strconv.ParseFloat(*text, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return recipeScalarResult[float64]{Reason: recipeReasonInvalidNumber}
	}
	return recipeScalarResult[float64]{Value: &f}
}

// recipeStringValue normalizes a small string field: explicit empty stays
// "", absent/null becomes null plus notProvided, and non-string shapes
// become null plus unsupportedJsonShape.
func recipeStringValue(raw json.RawMessage) recipeScalarResult[string] {
	switch classifyRecipeRaw(raw) {
	case recipeRawAbsent, recipeRawNull:
		return recipeScalarResult[string]{Reason: recipeReasonNotProvided}
	case recipeRawString:
		s, ok := rawJSONString(raw)
		if !ok {
			return recipeScalarResult[string]{Reason: recipeReasonUnsupportedShape}
		}
		return recipeScalarResult[string]{Value: &s}
	default:
		return recipeScalarResult[string]{Reason: recipeReasonUnsupportedShape}
	}
}

// recipeSeedValue normalizes meta.seed to an exact decimal integer string,
// preserving every digit of 64-bit-range seeds (a float64 round trip would
// corrupt values above 2^53). Non-integer text and fractions are invalid.
func recipeSeedValue(raw json.RawMessage) recipeScalarResult[string] {
	text, reason := recipeNumberText(raw)
	if text == nil {
		return recipeScalarResult[string]{Reason: reason}
	}
	r, ok := new(big.Rat).SetString(*text)
	if !ok || !r.IsInt() {
		return recipeScalarResult[string]{Reason: recipeReasonInvalidNumber}
	}
	v := r.Num().String()
	return recipeScalarResult[string]{Value: &v}
}

// rawJSONString decodes a raw JSON value known to be a string.
func rawJSONString(raw json.RawMessage) (string, bool) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

// recipeTextValue derives a prompt-like text field's availability status
// and, for strings, its verbatim value. Whitespace is not trimmed, so a
// whitespace-only prompt remains provided.
func recipeTextValue(raw json.RawMessage) (string, *string) {
	switch classifyRecipeRaw(raw) {
	case recipeRawAbsent, recipeRawNull:
		return recipeStatusNotProvided, nil
	case recipeRawString:
		s, ok := rawJSONString(raw)
		if !ok {
			return recipeStatusUnsupported, nil
		}
		if s == "" {
			return recipeStatusEmpty, &s
		}
		return recipeStatusProvided, &s
	default:
		return recipeStatusUnsupported, nil
	}
}

// recipeTextField names the issue field for a text status problem. Detail
// issues point at the detail-only keys; summary issues point at the status
// fields' source names.
func recipeTextField(name string, detail bool) string {
	if detail {
		return "textParameters." + name
	}
	return name
}

// recipeProvenance builds the shared envelope provenance for one request.
func recipeProvenance(requestedID int) RecipeProvenance {
	return RecipeProvenance{
		Source:    recipeSourceOfficialAPI,
		APIPath:   fmt.Sprintf("/api/v1/model-versions/%d", requestedID),
		VersionID: requestedID,
	}
}

// decodeRecipeSource decodes the narrow recipe source view. Malformed
// top-level JSON (including a non-object body) is an error.
func decodeRecipeSource(raw []byte) (*recipeSource, error) {
	var src recipeSource
	if err := json.Unmarshal(raw, &src); err != nil {
		return nil, fmt.Errorf("api: decode recipe version: %w", err)
	}
	return &src, nil
}

// checkRecipeVersionID rejects a conflicting nonzero returned version id.
// A missing/nil returned id is reported through the subject's nullable id
// plus issue instead of failing the command.
func checkRecipeVersionID(returned *int64, requestedID int) error {
	if returned == nil || *returned == 0 || *returned == int64(requestedID) {
		return nil
	}
	return fmt.Errorf("api: recipe: response version id %d does not match requested %d", *returned, requestedID)
}

// buildRecipeSubject projects the model/version identity and trained
// words, returning the returned version id (nil when unreadable) and the
// envelope/subject issues in a deterministic order.
func buildRecipeSubject(src *recipeSource) (RecipeSubject, *int64, []RecipeIssue) {
	issues := make([]RecipeIssue, 0)
	subject := RecipeSubject{TrainedWords: []string{}}

	modelID := recipeIntegerValue(src.ModelID)
	if modelID.Value != nil {
		subject.Model.ID = modelID.Value
	} else {
		issues = append(issues, RecipeIssue{Field: "model.id", Reason: modelID.Reason})
	}

	var model recipeSourceModel
	modelReadable, modelMalformed := false, false
	switch classifyRecipeRaw(src.Model) {
	case recipeRawObject:
		if err := json.Unmarshal(src.Model, &model); err != nil {
			modelMalformed = true
		} else {
			modelReadable = true
		}
	case recipeRawAbsent, recipeRawNull:
		// No summary object: name/type are not provided.
	default:
		modelMalformed = true
	}
	modelField := func(field string, raw json.RawMessage) *string {
		name := "model." + field
		switch {
		case modelMalformed:
			issues = append(issues, RecipeIssue{Field: name, Reason: recipeReasonUnsupportedShape})
			return nil
		case !modelReadable:
			issues = append(issues, RecipeIssue{Field: name, Reason: recipeReasonNotProvided})
			return nil
		}
		r := recipeStringValue(raw)
		if r.Value == nil {
			issues = append(issues, RecipeIssue{Field: name, Reason: r.Reason})
		}
		return r.Value
	}
	subject.Model.Name = modelField("name", model.Name)
	subject.Model.Type = modelField("type", model.Type)

	versionID := recipeIntegerValue(src.ID)
	if versionID.Value != nil {
		subject.Version.ID = versionID.Value
	} else {
		issues = append(issues, RecipeIssue{Field: "version.id", Reason: versionID.Reason})
	}

	versionField := func(field string, raw json.RawMessage) *string {
		r := recipeStringValue(raw)
		if r.Value == nil {
			issues = append(issues, RecipeIssue{Field: "version." + field, Reason: r.Reason})
		}
		return r.Value
	}
	subject.Version.Name = versionField("name", src.Name)
	subject.Version.BaseModel = versionField("baseModel", src.BaseModel)
	subject.Version.BaseModelType = versionField("baseModelType", src.BaseModelType)
	subject.Version.PublishedAt = versionField("publishedAt", src.PublishedAt)

	words, wordIssues := buildRecipeTrainedWords(src.TrainedWords)
	subject.TrainedWords = words
	issues = append(issues, wordIssues...)

	return subject, versionID.Value, issues
}

// buildRecipeTrainedWords normalizes the trainedWords array: absent/null
// becomes [] plus notProvided, explicit [] stays empty, unknown shapes
// become [] plus unsupportedJsonShape, and non-string elements are dropped
// with a per-index issue so the rest stays usable.
func buildRecipeTrainedWords(raw json.RawMessage) ([]string, []RecipeIssue) {
	words := []string{}
	issues := make([]RecipeIssue, 0)

	switch classifyRecipeRaw(raw) {
	case recipeRawAbsent, recipeRawNull:
		return words, []RecipeIssue{{Field: "trainedWords", Reason: recipeReasonNotProvided}}
	case recipeRawArray:
		var elems []json.RawMessage
		if err := json.Unmarshal(raw, &elems); err != nil {
			return words, []RecipeIssue{{Field: "trainedWords", Reason: recipeReasonUnsupportedShape}}
		}
		for i, elem := range elems {
			s, ok := rawJSONString(elem)
			if !ok {
				issues = append(issues, RecipeIssue{
					Field:  fmt.Sprintf("trainedWords[%d]", i),
					Reason: recipeReasonUnsupportedShape,
				})
				continue
			}
			words = append(words, s)
		}
		return words, issues
	default:
		return words, []RecipeIssue{{Field: "trainedWords", Reason: recipeReasonUnsupportedShape}}
	}
}

// recipeImages is the decoded images array plus its presence. provided is
// false when the key was absent or null; entries is nil then. An explicit
// [] is provided with zero entries.
type recipeImages struct {
	provided bool
	entries  []json.RawMessage
}

// decodeRecipeImages decodes the images array. A non-array container is a
// command error; absent/null is an unknown source, not proof of emptiness.
func decodeRecipeImages(raw json.RawMessage) (*recipeImages, error) {
	switch classifyRecipeRaw(raw) {
	case recipeRawAbsent, recipeRawNull:
		return &recipeImages{}, nil
	case recipeRawArray:
		var entries []json.RawMessage
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, fmt.Errorf("api: decode recipe images: %w", err)
		}
		return &recipeImages{provided: true, entries: entries}, nil
	default:
		return nil, fmt.Errorf("api: recipe images is not a JSON array")
	}
}

// recipeHasMetadata reports whether one images[] entry carries a meta
// object. Null/absent meta and non-object shapes count as without
// metadata, matching the contract's hasMeta-is-only-a-hint rule.
func recipeHasMetadata(raw json.RawMessage) bool {
	var im recipeSourceImage
	if err := json.Unmarshal(raw, &im); err != nil {
		return false
	}
	if classifyRecipeRaw(im.Meta) != recipeRawObject {
		return false
	}
	var meta recipeSourceMeta
	return json.Unmarshal(im.Meta, &meta) == nil
}

// recipeMetadataCounts tallies full-source metadata availability. The
// counts cover every provided entry, not only a summary window.
func recipeMetadataCounts(entries []json.RawMessage) (withMeta, withoutMeta int) {
	for _, entry := range entries {
		if recipeHasMetadata(entry) {
			withMeta++
			continue
		}
		withoutMeta++
	}
	return withMeta, withoutMeta
}

// setRecipeScalar stores a normalized scalar or records its issue.
func setRecipeScalar[T any](dst **T, result recipeScalarResult[T], issues *[]RecipeIssue, field string) {
	if result.Value != nil {
		*dst = result.Value
		return
	}
	*issues = append(*issues, RecipeIssue{Field: field, Reason: result.Reason})
}

// buildRecipeParameters projects the approved small-parameter whitelist.
// Values are direct meta fields only; absent/null/invalid/shape problems
// become null plus a stable issue path.
func buildRecipeParameters(meta recipeSourceMeta, issues *[]RecipeIssue) *RecipeParameters {
	p := &RecipeParameters{}
	setRecipeScalar(&p.Steps, recipeIntegerValue(meta.Steps), issues, "parameters.steps")
	setRecipeScalar(&p.CFGScale, recipeFloatValue(meta.CFGScale), issues, "parameters.cfgScale")
	setRecipeScalar(&p.Sampler, recipeStringValue(meta.Sampler), issues, "parameters.sampler")
	setRecipeScalar(&p.Scheduler, recipeStringValue(meta.Scheduler), issues, "parameters.scheduler")
	setRecipeScalar(&p.Seed, recipeSeedValue(meta.Seed), issues, "parameters.seed")
	setRecipeScalar(&p.Width, recipeIntegerValue(meta.Width), issues, "parameters.width")
	setRecipeScalar(&p.Height, recipeIntegerValue(meta.Height), issues, "parameters.height")
	setRecipeScalar(&p.Denoise, recipeFloatValue(meta.Denoise), issues, "parameters.denoise")
	return p
}

// projectRecipeExample builds the prompt-free projection of one source
// entry. When detail is true it also returns the singular verbatim text
// parameters; the summary view never receives text values.
func projectRecipeExample(raw json.RawMessage, sourceIndex, versionID int, detail bool) (RecipeSummaryExample, *RecipeTextParameters) {
	issues := make([]RecipeIssue, 0)
	ex := RecipeSummaryExample{
		SourceIndex: sourceIndex,
		Provenance: RecipeExampleProvenance{
			Source:    recipeSourceExample,
			VersionID: versionID,
		},
		PromptStatus:         recipeStatusNotProvided,
		NegativePromptStatus: recipeStatusNotProvided,
	}

	var im recipeSourceImage
	if err := json.Unmarshal(raw, &im); err != nil {
		im = recipeSourceImage{}
	}

	url := recipeStringValue(im.URL)
	if url.Value != nil {
		ex.Provenance.ImageURL = url.Value
	} else {
		issues = append(issues, RecipeIssue{Field: "imageUrl", Reason: url.Reason})
	}

	var meta recipeSourceMeta
	metaKind := classifyRecipeRaw(im.Meta)
	if metaKind == recipeRawObject {
		if err := json.Unmarshal(im.Meta, &meta); err != nil {
			metaKind = recipeRawOther
		}
	}

	var text *RecipeTextParameters
	switch metaKind {
	case recipeRawObject:
		ex.MetadataStatus = recipeStatusAvailable
		ex.Parameters = buildRecipeParameters(meta, &issues)

		promptStatus, promptText := recipeTextValue(meta.Prompt)
		negativeStatus, negativeText := recipeTextValue(meta.NegativePrompt)
		ex.PromptStatus = promptStatus
		ex.NegativePromptStatus = negativeStatus
		appendRecipeTextIssue(&issues, "prompt", promptStatus, detail)
		appendRecipeTextIssue(&issues, "negativePrompt", negativeStatus, detail)
		if detail {
			text = &RecipeTextParameters{Prompt: promptText, NegativePrompt: negativeText}
		}
	case recipeRawAbsent, recipeRawNull:
		ex.MetadataStatus = recipeStatusNotProvided
		issues = append(issues, RecipeIssue{Field: "parameters", Reason: recipeReasonNotProvided})
	default:
		ex.MetadataStatus = recipeStatusNotProvided
		issues = append(issues, RecipeIssue{Field: "parameters", Reason: recipeReasonUnsupportedShape})
	}

	ex.Issues = issues
	return ex, text
}

// appendRecipeTextIssue records a text-status problem. Summaries only flag
// unsupported shapes (absence is already visible through the status
// fields); detail additionally requires a notProvided issue whenever the
// text value is null.
func appendRecipeTextIssue(issues *[]RecipeIssue, name, status string, detail bool) {
	switch status {
	case recipeStatusUnsupported:
		*issues = append(*issues, RecipeIssue{Field: recipeTextField(name, detail), Reason: recipeReasonUnsupportedShape})
	case recipeStatusNotProvided:
		if detail {
			*issues = append(*issues, RecipeIssue{Field: recipeTextField(name, detail), Reason: recipeReasonNotProvided})
		}
	}
}

// ParseRecipeSummary decodes one /model-versions/{id} response body and
// projects a bounded, prompt-free summary of its first count examples in
// source order. count is clamped to >= 0; the approved 0-10 range is
// validated by the command layer before any request. A missing images
// array is a successful summary with unknown counts and a notProvided
// issue; a non-array images value is a decode error.
func ParseRecipeSummary(raw []byte, requestedID, count int) (*RecipeSummary, error) {
	if requestedID <= 0 {
		return nil, fmt.Errorf("api: recipe: invalid version id %d", requestedID)
	}
	if count < 0 {
		count = 0
	}
	src, err := decodeRecipeSource(raw)
	if err != nil {
		return nil, err
	}
	subject, returnedID, subjectIssues := buildRecipeSubject(src)
	if err := checkRecipeVersionID(returnedID, requestedID); err != nil {
		return nil, err
	}
	images, err := decodeRecipeImages(src.Images)
	if err != nil {
		return nil, err
	}

	summary := &RecipeSummary{
		SchemaVersion: recipeSchemaVersion,
		Kind:          recipeSummaryKind,
		Provenance:    recipeProvenance(requestedID),
		Subject:       subject,
		Selection:     RecipeSelection{Mode: recipeModeFirst, Requested: count},
		Examples:      make([]RecipeSummaryExample, 0),
		Issues:        subjectIssues,
	}
	if !images.provided {
		summary.Issues = append(summary.Issues, RecipeIssue{Field: "images", Reason: recipeReasonNotProvided})
		return summary, nil
	}

	total := len(images.entries)
	withMeta, withoutMeta := recipeMetadataCounts(images.entries)
	truncated := count < total
	summary.Selection.Total = &total
	summary.Selection.WithMetadata = &withMeta
	summary.Selection.WithoutMetadata = &withoutMeta
	summary.Selection.Truncated = &truncated

	n := min(count, total)
	for i := 0; i < n; i++ {
		example, _ := projectRecipeExample(images.entries[i], i+1, requestedID, false)
		summary.Examples = append(summary.Examples, example)
	}
	summary.Selection.Returned = len(summary.Examples)
	return summary, nil
}

// ParseRecipeDetail decodes one /model-versions/{id} response body and
// projects the singular detail of the one-based sourceIndex example. It is
// the only recipe entry point that returns prompt and negativePrompt text.
// An unavailable or out-of-range source index is an error after the single
// fetch; no summary fallback and no partial detail are produced.
func ParseRecipeDetail(raw []byte, requestedID, sourceIndex int) (*RecipeDetail, error) {
	if requestedID <= 0 {
		return nil, fmt.Errorf("api: recipe: invalid version id %d", requestedID)
	}
	src, err := decodeRecipeSource(raw)
	if err != nil {
		return nil, err
	}
	subject, returnedID, subjectIssues := buildRecipeSubject(src)
	if err := checkRecipeVersionID(returnedID, requestedID); err != nil {
		return nil, err
	}
	images, err := decodeRecipeImages(src.Images)
	if err != nil {
		return nil, err
	}
	if !images.provided {
		return nil, fmt.Errorf("api: recipe: version %d did not provide an example image list", requestedID)
	}
	total := len(images.entries)
	if sourceIndex < 1 || sourceIndex > total {
		return nil, fmt.Errorf("api: recipe: example index %d is out of range (version %d has %d example images)", sourceIndex, requestedID, total)
	}

	withMeta, withoutMeta := recipeMetadataCounts(images.entries)
	example, text := projectRecipeExample(images.entries[sourceIndex-1], sourceIndex, requestedID, true)
	returned := 1
	return &RecipeDetail{
		SchemaVersion: recipeSchemaVersion,
		Kind:          recipeExampleDetailKind,
		Provenance:    recipeProvenance(requestedID),
		Subject:       subject,
		Selection: RecipeSelection{
			Mode:            recipeModeSourceIndex,
			Requested:       sourceIndex,
			Returned:        returned,
			Total:           &total,
			WithMetadata:    &withMeta,
			WithoutMetadata: &withoutMeta,
		},
		Example: &RecipeDetailExample{RecipeSummaryExample: example, TextParameters: text},
		Issues:  subjectIssues,
	}, nil
}
