package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Community /images projection constants (task 11-24-community-recipes).
// The schema identifier is shared with the official recipe family: both
// sources are views of the same recipe contract, and an incompatible
// field/type change requires a new identifier. Kind values are distinct
// so consumers can never confuse a bounded list sample with the singular
// large-field-bearing detail view.
const (
	communityImageSource      = "community-images"
	communityImagePageBaseURL = "https://civitai.com/images/"
	communityOriginMeta       = "meta"
	communityOriginTopLevel   = "topLevel"
	communityResourceCivitai  = "civitaiResources"
	communityResourceLegacy   = "resources"
	communitySampleKind       = "communityRecipeSample"
	communityImageDetailKind  = "communityImageDetail"
)

// CommunityImageProvenance identifies the independent exact-image call.
// It deliberately carries no list version/sort/period: the detail view
// only claims what its own response provides.
type CommunityImageProvenance struct {
	Source  string `json:"source"`
	APIPath string `json:"apiPath"`
	ImageID int    `json:"imageId"`
}

// CommunityQuery is the exact bounded list query semantics reported with
// every sample: requested version, media type, order, fixed period, and
// sample size.
type CommunityQuery struct {
	VersionID int    `json:"versionId"`
	Type      string `json:"type"`
	Sort      string `json:"sort"`
	Period    string `json:"period"`
	Limit     int    `json:"limit"`
}

// CommunityCoverage reports facts about this one bounded response, never
// estimates of all community content. WithMeta counts items whose meta is
// a non-null JSON object (including {}); Usable counts items with at least
// one supported non-null/non-empty scalar observation or one identified
// resource entry. Both denominators are Returned.
type CommunityCoverage struct {
	RequestedLimit      int  `json:"requestedLimit"`
	Returned            int  `json:"returned"`
	WithMeta            int  `json:"withMeta"`
	Usable              int  `json:"usable"`
	ContinuationOffered bool `json:"continuationOffered"`
}

// CommunityImageIdentity is the compact provenance shared by list rows and
// the explicit single-image detail. Every pointer distinguishes an absent
// value from an observed one; PageURL is derived from the image id and is
// never fetched.
type CommunityImageIdentity struct {
	ImageID       *int64  `json:"imageId"`
	PostID        *int64  `json:"postId"`
	Username      *string `json:"username"`
	CreatedAt     *string `json:"createdAt"`
	PageURL       *string `json:"pageUrl"`
	Type          *string `json:"type"`
	BaseModel     *string `json:"baseModel"`
	NSFWLevel     *string `json:"nsfwLevel"`
	BrowsingLevel *string `json:"browsingLevel"`
}

// CommunityReactions preserves the separate upstream reaction counters.
// Each pointer distinguishes absent stats from an observed zero; no
// combined engagement or quality score is derived from them.
type CommunityReactions struct {
	Like    *int64 `json:"likeCount"`
	Dislike *int64 `json:"dislikeCount"`
	Heart   *int64 `json:"heartCount"`
	Laugh   *int64 `json:"laughCount"`
	Cry     *int64 `json:"cryCount"`
	Comment *int64 `json:"commentCount"`
}

// ObservedParameter is one normalized scalar observation. Origin is
// "meta" for a free-form metadata field or "topLevel" for a response-level
// dimension. Value is an int64, a float64, or a string; it is nil when the
// source key was null or unrepresentable (the accompanying issue explains
// the unrepresentable case). Unknown fields, units, and defaults are never
// inferred.
type ObservedParameter struct {
	Name   string `json:"name"`
	Origin string `json:"origin"`
	Value  any    `json:"value"`
}

// ObservedResource is one origin-labeled resource-stack entry. Origin is
// "civitaiResources" or "resources" and Position is the zero-based index
// within that array, so order and duplicates survive. Weight is null when
// absent or invalid; it is never defaulted to 1.
type ObservedResource struct {
	Origin         string   `json:"origin"`
	Position       int      `json:"position"`
	Type           *string  `json:"type"`
	ModelVersionID *int64   `json:"modelVersionId"`
	Name           *string  `json:"name"`
	Hash           *string  `json:"hash"`
	Weight         *float64 `json:"weight"`
}

// CommunityObservation is one compact, large-field-free list row. It has
// no prompt, negativePrompt, raw meta, workflow, or media-location member:
// summary redaction is a type-level property, not a renderer convention.
// UpstreamPosition is the one-based position in the response; Parameters
// and Resources keep upstream order and are always non-nil.
type CommunityObservation struct {
	Source           string `json:"source"`
	UpstreamPosition int    `json:"upstreamPosition"`
	CommunityImageIdentity
	ModelVersionIDs []int64             `json:"modelVersionIds"`
	MetadataStatus  string              `json:"metadataStatus"`
	Usable          bool                `json:"usable"`
	Reactions       *CommunityReactions `json:"reactions"`
	Parameters      []ObservedParameter `json:"parameters"`
	Resources       []ObservedResource  `json:"resources"`
	Issues          []RecipeIssue       `json:"issues"`
}

// CommunitySample is the bounded community list view. Provenance is the
// shared recipe envelope provenance (source "community-images", logical
// /api/v1/images path, requested version id). The sample contains only
// public-safe observations and coverage facts; raw list metadata is
// released after projection and is never reachable through this type.
type CommunitySample struct {
	SchemaVersion string                 `json:"schemaVersion"`
	Kind          string                 `json:"kind"`
	Provenance    RecipeProvenance       `json:"provenance"`
	Query         CommunityQuery         `json:"query"`
	Coverage      CommunityCoverage      `json:"coverage"`
	Observations  []CommunityObservation `json:"observations"`
}

// CommunityTextParameters is the detail-only large-text block. It is
// reachable only through CommunityDetailImage; list DTOs have no path to
// it.
type CommunityTextParameters struct {
	Prompt         *string `json:"prompt"`
	NegativePrompt *string `json:"negativePrompt"`
}

// CommunityDetailImage is the singular detail of the requested image,
// including that image's large-text parameters. It shares the compact
// identity and normalized observations with the list projection but
// carries no list position or sample coverage.
type CommunityDetailImage struct {
	CommunityImageIdentity
	ModelVersionIDs      []int64                  `json:"modelVersionIds"`
	MetadataStatus       string                   `json:"metadataStatus"`
	PromptStatus         string                   `json:"promptStatus"`
	NegativePromptStatus string                   `json:"negativePromptStatus"`
	Reactions            *CommunityReactions      `json:"reactions"`
	Parameters           []ObservedParameter      `json:"parameters"`
	Resources            []ObservedResource       `json:"resources"`
	TextParameters       *CommunityTextParameters `json:"textParameters"`
	Issues               []RecipeIssue            `json:"issues"`
}

// CommunityImageDetail is the explicit single-image detail view produced
// by `civ recipe image <imageId>`. It is independent of any list sample
// and is never persisted or handed to another command.
type CommunityImageDetail struct {
	SchemaVersion string                   `json:"schemaVersion"`
	Kind          string                   `json:"kind"`
	Provenance    CommunityImageProvenance `json:"provenance"`
	Image         *CommunityDetailImage    `json:"image"`
}

// communitySourceImage is the narrow, presence-aware decode of one browse
// image. CDN url, BlurHash, and thumbnail fields are deliberately absent:
// the sampler never requests or emits media locations. Meta stays raw so
// the projector distinguishes absent, null, object, and drifted shapes.
type communitySourceImage struct {
	ID              json.RawMessage `json:"id"`
	PostID          json.RawMessage `json:"postId"`
	Username        json.RawMessage `json:"username"`
	CreatedAt       json.RawMessage `json:"createdAt"`
	Type            json.RawMessage `json:"type"`
	BaseModel       json.RawMessage `json:"baseModel"`
	NSFWLevel       json.RawMessage `json:"nsfwLevel"`
	BrowsingLevel   json.RawMessage `json:"browsingLevel"`
	Width           json.RawMessage `json:"width"`
	Height          json.RawMessage `json:"height"`
	ModelVersionIDs json.RawMessage `json:"modelVersionIds"`
	Stats           json.RawMessage `json:"stats"`
	Meta            json.RawMessage `json:"meta"`
}

// communitySourceMeta is the presence-aware decode of the allowlisted
// generation fields plus both resource arrays. Prompt text is declared
// only because the explicit detail projection needs it; the list
// projector never reads these members.
type communitySourceMeta struct {
	Prompt            json.RawMessage `json:"prompt"`
	NegativePrompt    json.RawMessage `json:"negativePrompt"`
	Seed              json.RawMessage `json:"seed"`
	Steps             json.RawMessage `json:"steps"`
	CFGScale          json.RawMessage `json:"cfgScale"`
	Sampler           json.RawMessage `json:"sampler"`
	Scheduler         json.RawMessage `json:"scheduler"`
	ClipSkip          json.RawMessage `json:"clipSkip"`
	DenoisingStrength json.RawMessage `json:"denoisingStrength"`
	Width             json.RawMessage `json:"width"`
	Height            json.RawMessage `json:"height"`
	CivitaiResources  json.RawMessage `json:"civitaiResources"`
	Resources         json.RawMessage `json:"resources"`
}

// communitySourceStats is the presence-aware decode of the separate
// reaction/comment counters.
type communitySourceStats struct {
	LikeCount    json.RawMessage `json:"likeCount"`
	DislikeCount json.RawMessage `json:"dislikeCount"`
	HeartCount   json.RawMessage `json:"heartCount"`
	LaughCount   json.RawMessage `json:"laughCount"`
	CryCount     json.RawMessage `json:"cryCount"`
	CommentCount json.RawMessage `json:"commentCount"`
}

// communitySourceResource is the presence-aware decode of one resource
// entry. Unknown fields and unknown type strings are tolerated.
type communitySourceResource struct {
	Type           json.RawMessage `json:"type"`
	ModelVersionID json.RawMessage `json:"modelVersionId"`
	Name           json.RawMessage `json:"name"`
	Hash           json.RawMessage `json:"hash"`
	Weight         json.RawMessage `json:"weight"`
}

// ParseCommunitySample decodes one bounded GET /images list body and
// projects the compact, large-field-free community sample. versionID,
// limit, and sort must already be validated by the command; invalid
// arguments are contract violations and return an error rather than a
// clamped sample.
func ParseCommunitySample(raw []byte, versionID, limit int, sort string) (*CommunitySample, error) {
	if versionID <= 0 {
		return nil, fmt.Errorf("api: community: invalid model version id %d", versionID)
	}
	if limit < CommunityMinLimit || limit > CommunityMaxLimit {
		return nil, fmt.Errorf("api: community: invalid limit %d (want %d-%d)", limit, CommunityMinLimit, CommunityMaxLimit)
	}
	if sort != CommunitySortMostReactions && sort != CommunitySortNewest {
		return nil, fmt.Errorf("api: community: unsupported sort %q", sort)
	}
	env, err := decodeImagesEnvelope(raw)
	if err != nil {
		return nil, err
	}
	items, err := decodeImagesItems(env)
	if err != nil {
		return nil, err
	}

	sample := &CommunitySample{
		SchemaVersion: recipeSchemaVersion,
		Kind:          communitySampleKind,
		Provenance: RecipeProvenance{
			Source:    communityImageSource,
			APIPath:   communityImagesAPIPath,
			VersionID: versionID,
		},
		Query: CommunityQuery{
			VersionID: versionID,
			Type:      communityMediaType,
			Sort:      sort,
			Period:    CommunityPeriodAllTime,
			Limit:     limit,
		},
		Coverage: CommunityCoverage{
			RequestedLimit:      limit,
			Returned:            len(items),
			ContinuationOffered: continuationOffered(env.Metadata),
		},
		Observations: make([]CommunityObservation, 0, len(items)),
	}
	for i, item := range items {
		obs := projectCommunityObservation(item, i+1)
		if obs.MetadataStatus == recipeStatusAvailable {
			sample.Coverage.WithMeta++
		}
		if obs.Usable {
			sample.Coverage.Usable++
		}
		sample.Observations = append(sample.Observations, obs)
	}
	return sample, nil
}

// ParseCommunityImageDetail decodes one exact-image GET /images body and
// projects the singular detail for imageID. The response must contain
// exactly one item whose id is imageID; zero, multiple, mismatched, or
// non-object items make the detail unavailable. No other request is made
// and no list state is consulted.
func ParseCommunityImageDetail(raw []byte, imageID int) (*CommunityImageDetail, error) {
	if imageID <= 0 {
		return nil, fmt.Errorf("api: community: invalid image id %d", imageID)
	}
	env, err := decodeImagesEnvelope(raw)
	if err != nil {
		return nil, err
	}
	items, err := decodeImagesItems(env)
	if err != nil {
		return nil, err
	}
	if len(items) != 1 {
		return nil, fmt.Errorf("api: community image %d unavailable: response returned %d items (want exactly 1)", imageID, len(items))
	}
	image := projectCommunityDetailImage(items[0])
	if image.ImageID == nil || *image.ImageID != int64(imageID) {
		return nil, fmt.Errorf("api: community image %d unavailable: response item does not match the requested id", imageID)
	}
	return &CommunityImageDetail{
		SchemaVersion: recipeSchemaVersion,
		Kind:          communityImageDetailKind,
		Provenance: CommunityImageProvenance{
			Source:  communityImageSource,
			APIPath: communityImagesAPIPath,
			ImageID: imageID,
		},
		Image: &image,
	}, nil
}

// projectCommunityObservation projects one raw items[] entry into the
// compact observation. A null or non-object entry keeps its upstream
// position with an issue and contributes no metadata or usability.
func projectCommunityObservation(raw json.RawMessage, position int) CommunityObservation {
	issues := make([]RecipeIssue, 0)
	obs := CommunityObservation{
		Source:           communityImageSource,
		UpstreamPosition: position,
		ModelVersionIDs:  []int64{},
		MetadataStatus:   recipeStatusNotProvided,
		Parameters:       []ObservedParameter{},
		Resources:        []ObservedResource{},
	}

	var src communitySourceImage
	switch classifyRecipeRaw(raw) {
	case recipeRawObject:
		if err := json.Unmarshal(raw, &src); err != nil {
			issues = append(issues, RecipeIssue{Field: "item", Reason: recipeReasonUnsupportedShape})
			src = communitySourceImage{}
		}
	case recipeRawNull:
		issues = append(issues, RecipeIssue{Field: "item", Reason: recipeReasonNotProvided})
	default:
		issues = append(issues, RecipeIssue{Field: "item", Reason: recipeReasonUnsupportedShape})
	}

	meta, hasMeta := decodeCommunityMeta(src.Meta)
	obs.CommunityImageIdentity = buildCommunityImageIdentity(src, &issues)
	obs.ModelVersionIDs = projectCommunityModelVersionIDs(src.ModelVersionIDs, &issues)
	obs.Reactions = projectCommunityReactions(src.Stats, &issues)
	obs.MetadataStatus = communityMetadataStatus(src.Meta)
	if obs.MetadataStatus == recipeStatusUnsupported {
		issues = append(issues, RecipeIssue{Field: "meta", Reason: recipeReasonUnsupportedShape})
	}
	obs.Parameters = projectCommunityParameters(meta, hasMeta, src.Width, src.Height, &issues)
	obs.Resources = projectCommunityResources(meta, hasMeta, &issues)
	obs.Usable = communityParametersUsable(obs.Parameters) || communityResourcesIdentified(obs.Resources)
	obs.Issues = issues
	return obs
}

// projectCommunityDetailImage projects the singular detail from the one
// exact-image response item. It shares the list projection helpers so
// normalized values cannot drift between views, and additionally reads
// prompt and negativePrompt because this is the only large-field view.
func projectCommunityDetailImage(raw json.RawMessage) CommunityDetailImage {
	issues := make([]RecipeIssue, 0)
	image := CommunityDetailImage{
		ModelVersionIDs:      []int64{},
		MetadataStatus:       recipeStatusNotProvided,
		PromptStatus:         recipeStatusNotProvided,
		NegativePromptStatus: recipeStatusNotProvided,
		Parameters:           []ObservedParameter{},
		Resources:            []ObservedResource{},
	}

	var src communitySourceImage
	if classifyRecipeRaw(raw) == recipeRawObject {
		if err := json.Unmarshal(raw, &src); err != nil {
			issues = append(issues, RecipeIssue{Field: "item", Reason: recipeReasonUnsupportedShape})
			src = communitySourceImage{}
		}
	} else {
		issues = append(issues, RecipeIssue{Field: "item", Reason: recipeReasonUnsupportedShape})
	}

	image.CommunityImageIdentity = buildCommunityImageIdentity(src, &issues)
	image.ModelVersionIDs = projectCommunityModelVersionIDs(src.ModelVersionIDs, &issues)
	image.Reactions = projectCommunityReactions(src.Stats, &issues)
	meta, hasMeta := decodeCommunityMeta(src.Meta)
	image.MetadataStatus = communityMetadataStatus(src.Meta)
	if image.MetadataStatus == recipeStatusUnsupported {
		issues = append(issues, RecipeIssue{Field: "meta", Reason: recipeReasonUnsupportedShape})
	}
	if hasMeta {
		promptStatus, prompt := recipeTextValue(meta.Prompt)
		negativeStatus, negative := recipeTextValue(meta.NegativePrompt)
		image.PromptStatus = promptStatus
		image.NegativePromptStatus = negativeStatus
		appendRecipeTextIssue(&issues, "prompt", promptStatus, true)
		appendRecipeTextIssue(&issues, "negativePrompt", negativeStatus, true)
		image.TextParameters = &CommunityTextParameters{Prompt: prompt, NegativePrompt: negative}
	}
	image.Parameters = projectCommunityParameters(meta, hasMeta, src.Width, src.Height, &issues)
	image.Resources = projectCommunityResources(meta, hasMeta, &issues)
	image.Issues = issues
	return image
}

// decodeCommunityMeta decodes the metadata allowlist once per item. The
// second return value is false unless meta is a JSON object, so callers
// cannot mistake absent, null, or drifted shapes for an empty recipe.
func decodeCommunityMeta(raw json.RawMessage) (communitySourceMeta, bool) {
	if classifyRecipeRaw(raw) != recipeRawObject {
		return communitySourceMeta{}, false
	}
	var meta communitySourceMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return communitySourceMeta{}, false
	}
	return meta, true
}

// communityMetadataStatus classifies the raw meta member: a JSON object is
// available evidence, absent/null is not provided, and any other shape is
// reported as unsupported without failing the sample.
func communityMetadataStatus(raw json.RawMessage) string {
	switch classifyRecipeRaw(raw) {
	case recipeRawObject:
		return recipeStatusAvailable
	case recipeRawAbsent, recipeRawNull:
		return recipeStatusNotProvided
	default:
		return recipeStatusUnsupported
	}
}

// buildCommunityImageIdentity projects the item-level provenance scalars.
// Issues are recorded only for present-but-unrepresentable values; simple
// absence stays null without an issue because only the image id is
// structurally required.
func buildCommunityImageIdentity(src communitySourceImage, issues *[]RecipeIssue) CommunityImageIdentity {
	identity := CommunityImageIdentity{}
	appendCommunityInt(&identity.ImageID, src.ID, "imageId", issues)
	appendCommunityInt(&identity.PostID, src.PostID, "postId", issues)
	appendCommunityString(&identity.Username, src.Username, "username", issues)
	appendCommunityString(&identity.CreatedAt, src.CreatedAt, "createdAt", issues)
	appendCommunityString(&identity.Type, src.Type, "type", issues)
	appendCommunityString(&identity.BaseModel, src.BaseModel, "baseModel", issues)
	appendCommunityLabel(&identity.NSFWLevel, src.NSFWLevel, "nsfwLevel", issues)
	appendCommunityLabel(&identity.BrowsingLevel, src.BrowsingLevel, "browsingLevel", issues)
	if identity.ImageID != nil && *identity.ImageID > 0 {
		page := fmt.Sprintf("%s%d", communityImagePageBaseURL, *identity.ImageID)
		identity.PageURL = &page
	}
	return identity
}

// projectCommunityParameters projects the approved metadata allowlist in a
// fixed order, then top-level dimensions with origin "topLevel". A field
// appears whenever the source key exists, so an explicit null stays null
// while an absent key is omitted. Invalid numeric text and non-scalar
// shapes stay null and add a structured issue. Nothing is parsed from
// prompts, no unit is inferred, and no default value is supplied.
func projectCommunityParameters(meta communitySourceMeta, hasMeta bool, width, height json.RawMessage, issues *[]RecipeIssue) []ObservedParameter {
	params := make([]ObservedParameter, 0, 11)
	if !hasMeta {
		meta = communitySourceMeta{}
	}
	appendCommunityScalar(&params, issues, "seed", communityOriginMeta, "parameters.seed",
		meta.Seed, recipeSeedValue(meta.Seed))
	appendCommunityScalar(&params, issues, "steps", communityOriginMeta, "parameters.steps",
		meta.Steps, recipeIntegerValue(meta.Steps))
	appendCommunityScalar(&params, issues, "cfgScale", communityOriginMeta, "parameters.cfgScale",
		meta.CFGScale, recipeFloatValue(meta.CFGScale))
	appendCommunityScalar(&params, issues, "sampler", communityOriginMeta, "parameters.sampler",
		meta.Sampler, recipeStringValue(meta.Sampler))
	appendCommunityScalar(&params, issues, "scheduler", communityOriginMeta, "parameters.scheduler",
		meta.Scheduler, recipeStringValue(meta.Scheduler))
	appendCommunityScalar(&params, issues, "clipSkip", communityOriginMeta, "parameters.clipSkip",
		meta.ClipSkip, recipeIntegerValue(meta.ClipSkip))
	appendCommunityScalar(&params, issues, "denoisingStrength", communityOriginMeta, "parameters.denoisingStrength",
		meta.DenoisingStrength, recipeFloatValue(meta.DenoisingStrength))
	appendCommunityScalar(&params, issues, "width", communityOriginMeta, "parameters.width",
		meta.Width, recipeIntegerValue(meta.Width))
	appendCommunityScalar(&params, issues, "height", communityOriginMeta, "parameters.height",
		meta.Height, recipeIntegerValue(meta.Height))
	appendCommunityScalar(&params, issues, "width", communityOriginTopLevel, "width",
		width, recipeIntegerValue(width))
	appendCommunityScalar(&params, issues, "height", communityOriginTopLevel, "height",
		height, recipeIntegerValue(height))
	return params
}

// projectCommunityResources projects both origin-labeled resource arrays
// in metadata order: civitaiResources first, then legacy resources. Entry
// order and duplicates survive; nothing is sorted, deduplicated, resolved,
// or inferred from top-level modelVersionIds.
func projectCommunityResources(meta communitySourceMeta, hasMeta bool, issues *[]RecipeIssue) []ObservedResource {
	resources := []ObservedResource{}
	if !hasMeta {
		return resources
	}
	resources = append(resources, projectCommunityResourceArray(communityResourceCivitai, meta.CivitaiResources, issues)...)
	resources = append(resources, projectCommunityResourceArray(communityResourceLegacy, meta.Resources, issues)...)
	return resources
}

// projectCommunityResourceArray projects one resource array. A non-array
// value or non-object element degrades to an issue at that path; null
// elements are skipped as absence. Every projected entry keeps its own
// field-level issue paths.
func projectCommunityResourceArray(origin string, raw json.RawMessage, issues *[]RecipeIssue) []ObservedResource {
	resources := []ObservedResource{}
	switch classifyRecipeRaw(raw) {
	case recipeRawAbsent, recipeRawNull:
		return resources
	case recipeRawArray:
		var entries []json.RawMessage
		if err := json.Unmarshal(raw, &entries); err != nil {
			*issues = append(*issues, RecipeIssue{Field: origin, Reason: recipeReasonUnsupportedShape})
			return resources
		}
		for i, entry := range entries {
			path := fmt.Sprintf("%s[%d]", origin, i)
			switch classifyRecipeRaw(entry) {
			case recipeRawNull:
				continue
			case recipeRawObject:
				var src communitySourceResource
				if err := json.Unmarshal(entry, &src); err != nil {
					*issues = append(*issues, RecipeIssue{Field: path, Reason: recipeReasonUnsupportedShape})
					continue
				}
				resources = append(resources, projectCommunityResource(src, origin, i, path, issues))
			default:
				*issues = append(*issues, RecipeIssue{Field: path, Reason: recipeReasonUnsupportedShape})
			}
		}
		return resources
	default:
		*issues = append(*issues, RecipeIssue{Field: origin, Reason: recipeReasonUnsupportedShape})
		return resources
	}
}

// projectCommunityResource projects one resource entry, keeping every
// known field independently. Unknown type strings are preserved verbatim;
// the weight is the shared normalized float (null when absent or invalid)
// and is never defaulted.
func projectCommunityResource(src communitySourceResource, origin string, position int, path string, issues *[]RecipeIssue) ObservedResource {
	resource := ObservedResource{Origin: origin, Position: position}
	appendCommunityString(&resource.Type, src.Type, path+".type", issues)
	appendCommunityInt(&resource.ModelVersionID, src.ModelVersionID, path+".modelVersionId", issues)
	appendCommunityString(&resource.Name, src.Name, path+".name", issues)
	appendCommunityString(&resource.Hash, src.Hash, path+".hash", issues)
	if classifyRecipeRaw(src.Weight) != recipeRawAbsent {
		r := recipeFloatValue(src.Weight)
		if r.Value != nil {
			resource.Weight = r.Value
		} else if r.Reason != recipeReasonNotProvided {
			*issues = append(*issues, RecipeIssue{Field: path + ".weight", Reason: r.Reason})
		}
	}
	return resource
}

// projectCommunityReactions projects the separate counters. Absent/null
// stats stay nil; a present object yields a non-nil block whose individual
// pointers distinguish absent from observed zero.
func projectCommunityReactions(raw json.RawMessage, issues *[]RecipeIssue) *CommunityReactions {
	switch classifyRecipeRaw(raw) {
	case recipeRawAbsent, recipeRawNull:
		return nil
	case recipeRawObject:
		var src communitySourceStats
		if err := json.Unmarshal(raw, &src); err != nil {
			*issues = append(*issues, RecipeIssue{Field: "stats", Reason: recipeReasonUnsupportedShape})
			return nil
		}
		reactions := &CommunityReactions{}
		appendCommunityInt(&reactions.Like, src.LikeCount, "stats.likeCount", issues)
		appendCommunityInt(&reactions.Dislike, src.DislikeCount, "stats.dislikeCount", issues)
		appendCommunityInt(&reactions.Heart, src.HeartCount, "stats.heartCount", issues)
		appendCommunityInt(&reactions.Laugh, src.LaughCount, "stats.laughCount", issues)
		appendCommunityInt(&reactions.Cry, src.CryCount, "stats.cryCount", issues)
		appendCommunityInt(&reactions.Comment, src.CommentCount, "stats.commentCount", issues)
		return reactions
	default:
		*issues = append(*issues, RecipeIssue{Field: "stats", Reason: recipeReasonUnsupportedShape})
		return nil
	}
}

// projectCommunityModelVersionIDs projects the top-level modelVersionIds
// as provenance only. It never becomes a resource stack: it lacks entry
// types, weights, duplicates, and order, so it cannot establish identity
// or usability.
func projectCommunityModelVersionIDs(raw json.RawMessage, issues *[]RecipeIssue) []int64 {
	ids := []int64{}
	switch classifyRecipeRaw(raw) {
	case recipeRawAbsent, recipeRawNull:
		return ids
	case recipeRawArray:
		var entries []json.RawMessage
		if err := json.Unmarshal(raw, &entries); err != nil {
			*issues = append(*issues, RecipeIssue{Field: "modelVersionIds", Reason: recipeReasonUnsupportedShape})
			return ids
		}
		for i, entry := range entries {
			r := recipeIntegerValue(entry)
			if r.Value != nil {
				ids = append(ids, *r.Value)
				continue
			}
			if r.Reason != recipeReasonNotProvided {
				*issues = append(*issues, RecipeIssue{
					Field:  fmt.Sprintf("modelVersionIds[%d]", i),
					Reason: r.Reason,
				})
			}
		}
		return ids
	default:
		*issues = append(*issues, RecipeIssue{Field: "modelVersionIds", Reason: recipeReasonUnsupportedShape})
		return ids
	}
}

// appendCommunityScalar records one normalized parameter entry. Absent
// keys are omitted entirely; present keys are emitted even when null or
// unrepresentable so "null stays null" survives JSON output. Only
// present-but-invalid reasons add an issue.
func appendCommunityScalar[T any](params *[]ObservedParameter, issues *[]RecipeIssue, name, origin, field string, raw json.RawMessage, result recipeScalarResult[T]) {
	if classifyRecipeRaw(raw) == recipeRawAbsent {
		return
	}
	var value any
	if result.Value != nil {
		value = *result.Value
	} else if result.Reason != recipeReasonNotProvided {
		*issues = append(*issues, RecipeIssue{Field: field, Reason: result.Reason})
	}
	*params = append(*params, ObservedParameter{Name: name, Origin: origin, Value: value})
}

// appendCommunityInt records a nullable integer field. Absent and explicit
// null stay nil without an issue; present-but-invalid values add one.
func appendCommunityInt(dst **int64, raw json.RawMessage, field string, issues *[]RecipeIssue) {
	if classifyRecipeRaw(raw) == recipeRawAbsent {
		return
	}
	r := recipeIntegerValue(raw)
	if r.Value != nil {
		*dst = r.Value
		return
	}
	if r.Reason != recipeReasonNotProvided {
		*issues = append(*issues, RecipeIssue{Field: field, Reason: r.Reason})
	}
}

// appendCommunityString records a nullable string field under the same
// absence rule: a present empty string stays empty, non-strings degrade to
// null plus an issue.
func appendCommunityString(dst **string, raw json.RawMessage, field string, issues *[]RecipeIssue) {
	if classifyRecipeRaw(raw) == recipeRawAbsent {
		return
	}
	r := recipeStringValue(raw)
	if r.Value != nil {
		*dst = r.Value
		return
	}
	if r.Reason != recipeReasonNotProvided {
		*issues = append(*issues, RecipeIssue{Field: field, Reason: r.Reason})
	}
}

// appendCommunityLabel records a visibility label. The images endpoint
// returns nsfwLevel in string form and browsingLevel as a bitmask number;
// both are kept as their original text so drift stays visible and neither
// is interpreted as an instruction to widen access.
func appendCommunityLabel(dst **string, raw json.RawMessage, field string, issues *[]RecipeIssue) {
	switch classifyRecipeRaw(raw) {
	case recipeRawAbsent, recipeRawNull:
		return
	case recipeRawString:
		if s, ok := rawJSONString(raw); ok {
			*dst = &s
			return
		}
		*issues = append(*issues, RecipeIssue{Field: field, Reason: recipeReasonUnsupportedShape})
	case recipeRawNumber:
		s := string(bytes.TrimSpace(raw))
		*dst = &s
	default:
		*issues = append(*issues, RecipeIssue{Field: field, Reason: recipeReasonUnsupportedShape})
	}
}

// communityParametersUsable reports whether any parameter is supported
// observed evidence: a non-null number (zero counts) or a non-empty
// string. Invalid/null entries and whitespace-only strings do not count.
func communityParametersUsable(params []ObservedParameter) bool {
	for _, param := range params {
		switch value := param.Value.(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				return true
			}
		case int64, float64:
			return true
		}
	}
	return false
}

// communityResourcesIdentified reports whether any resource entry carries
// identity evidence: a modelVersionId, or a non-empty name or hash. A bare
// type or weight is not identity.
func communityResourcesIdentified(resources []ObservedResource) bool {
	for _, resource := range resources {
		if resource.ModelVersionID != nil {
			return true
		}
		if resource.Name != nil && strings.TrimSpace(*resource.Name) != "" {
			return true
		}
		if resource.Hash != nil && strings.TrimSpace(*resource.Hash) != "" {
			return true
		}
	}
	return false
}
