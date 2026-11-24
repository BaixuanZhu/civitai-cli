package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Community /images sampling contract (task 11-24-community-recipes).
// The CLI limit range is deliberately narrower than the API's documented
// 0-200: the feature is a bounded review sample, never a census, and
// limit=0 has special upstream visibility behavior. period=AllTime is
// fixed for both approved orders.
const (
	// CommunityMinLimit is the smallest accepted community sample size.
	CommunityMinLimit = 1
	// CommunityMaxLimit is the largest accepted community sample size.
	CommunityMaxLimit = 100
	// CommunityDefaultLimit is the default community sample size used by
	// the recipe command.
	CommunityDefaultLimit = 20
	// CommunitySortMostReactions is the default /images order. It is an
	// upstream discovery order, not a quality signal.
	CommunitySortMostReactions = "Most Reactions"
	// CommunitySortNewest is the only other approved /images order.
	CommunitySortNewest = "Newest"
	// CommunityPeriodAllTime is the fixed period sent with every community
	// list request so the sample is not mixed across windows.
	CommunityPeriodAllTime = "AllTime"

	communityMediaType     = "image"
	communityImagesPath    = "/images"
	communityImagesAPIPath = "/api/v1/images"
)

// MapCommunitySort maps a CLI community sort value (case-insensitive) to
// the approved /images enum. Only the two reviewed orders are registered;
// callers must reject an unregistered value before any request.
func MapCommunitySort(cli string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(cli)) {
	case "most-reactions", "most reactions":
		return CommunitySortMostReactions, true
	case "newest":
		return CommunitySortNewest, true
	}
	return "", false
}

// CommunitySortChoices lists the approved CLI community sort values in
// display order (default first) for usage-error messages and help text.
func CommunitySortChoices() []string { return []string{"most-reactions", "newest"} }

// ListImages fetches exactly one bounded GET /images response for one
// model version: modelVersionId, withMeta=true, type=image, limit, sort,
// and the fixed period=AllTime. It sends no modelId, page, cursor, nsfw,
// or browsingLevel and never follows continuation metadata. sort must be
// CommunitySortMostReactions or CommunitySortNewest; limit must be inside
// CommunityMinLimit..CommunityMaxLimit. Invalid input fails before HTTP.
func (c *Client) ListImages(ctx context.Context, modelVersionID, limit int, sort string) ([]byte, error) {
	if modelVersionID <= 0 {
		return nil, fmt.Errorf("api: images: invalid model version id %d", modelVersionID)
	}
	if limit < CommunityMinLimit || limit > CommunityMaxLimit {
		return nil, fmt.Errorf("api: images: invalid limit %d (want %d-%d)", limit, CommunityMinLimit, CommunityMaxLimit)
	}
	if sort != CommunitySortMostReactions && sort != CommunitySortNewest {
		return nil, fmt.Errorf("api: images: unsupported sort %q (want %q or %q)",
			sort, CommunitySortMostReactions, CommunitySortNewest)
	}
	q := url.Values{}
	q.Set("modelVersionId", strconv.Itoa(modelVersionID))
	q.Set("withMeta", "true")
	q.Set("type", communityMediaType)
	q.Set("limit", strconv.Itoa(limit))
	q.Set("sort", sort)
	q.Set("period", CommunityPeriodAllTime)
	return c.Get(ctx, communityImagesPath, q)
}

// GetImage fetches GET /images with exactly imageId, withMeta=true,
// type=image, and limit=1. It sends no list sort/period, model or version
// filter, page, cursor, nsfw, or browsingLevel. The response is projected
// by ParseCommunityImageDetail, which requires exactly one matching item.
func (c *Client) GetImage(ctx context.Context, imageID int) ([]byte, error) {
	if imageID <= 0 {
		return nil, fmt.Errorf("api: images: invalid image id %d", imageID)
	}
	q := url.Values{}
	q.Set("imageId", strconv.Itoa(imageID))
	q.Set("withMeta", "true")
	q.Set("type", communityMediaType)
	q.Set("limit", "1")
	return c.Get(ctx, communityImagesPath, q)
}

// imagesEnvelope is the tolerate-drift decode of one /images browse
// response. Items and Metadata stay raw so a single drifted item cannot
// reject the whole body and absent continuation metadata remains
// distinguishable from an explicit null token.
type imagesEnvelope struct {
	Items    json.RawMessage `json:"items"`
	Metadata json.RawMessage `json:"metadata"`
}

// decodeImagesEnvelope decodes the browse envelope. A non-object body
// (including a bare null) is a malformed envelope and therefore an error;
// unknown members are ignored.
func decodeImagesEnvelope(raw []byte) (*imagesEnvelope, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, fmt.Errorf("api: decode images envelope: response is not a JSON object")
	}
	var env imagesEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("api: decode images envelope: %w", err)
	}
	return &env, nil
}

// decodeImagesItems returns the raw items of a decodeImagesEnvelope
// result. Already validated envelope JSON makes the array decode
// infallible; absent/null items means "no items returned" (a successful
// empty sample) while any other non-array value is an envelope error.
func decodeImagesItems(env *imagesEnvelope) ([]json.RawMessage, error) {
	switch classifyRecipeRaw(env.Items) {
	case recipeRawAbsent, recipeRawNull:
		return []json.RawMessage{}, nil
	case recipeRawArray:
		var items []json.RawMessage
		if err := json.Unmarshal(env.Items, &items); err != nil {
			return nil, fmt.Errorf("api: decode images items: %w", err)
		}
		return items, nil
	default:
		return nil, fmt.Errorf("api: decode images envelope: items is not a JSON array")
	}
}

// continuationOffered reports whether the response metadata carried a
// nextCursor or nextPage value. Only presence is recorded: the opaque
// token is never exposed, emitted, or turned into a follow-up request.
func continuationOffered(metadata json.RawMessage) bool {
	if classifyRecipeRaw(metadata) != recipeRawObject {
		return false
	}
	var md struct {
		NextCursor json.RawMessage `json:"nextCursor"`
		NextPage   json.RawMessage `json:"nextPage"`
	}
	if err := json.Unmarshal(metadata, &md); err != nil {
		return false
	}
	return rawValuePresent(md.NextCursor) || rawValuePresent(md.NextPage)
}

// rawValuePresent reports a non-null JSON value that carries content. An
// empty or whitespace-only string is treated as absent so a blank
// continuation field does not claim an offered page.
func rawValuePresent(raw json.RawMessage) bool {
	switch classifyRecipeRaw(raw) {
	case recipeRawAbsent, recipeRawNull:
		return false
	case recipeRawString:
		s, ok := rawJSONString(raw)
		return ok && strings.TrimSpace(s) != ""
	default:
		return true
	}
}
