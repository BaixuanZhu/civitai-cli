package api

import (
	"context"
	"net/url"
	"strconv"
)

// GetVersion fetches GET /model-versions/{id} and returns the raw version
// body (files + images with generation meta). Decode with ParseVersion.
func (c *Client) GetVersion(ctx context.Context, id int) ([]byte, error) {
	return c.Get(ctx, "/model-versions/"+strconv.Itoa(id), nil)
}

// GetVersionByHash fetches GET /model-versions/by-hash/{hash}. Accepts a
// full SHA256 (64 hex chars, case-insensitive) or AutoV2 (first 10 hex
// chars of SHA256, uppercase). Unknown hashes yield NotFoundError.
// Built for vet (M2); M1 ships the endpoint so the choke point covers it.
func (c *Client) GetVersionByHash(ctx context.Context, hash string) ([]byte, error) {
	return c.Get(ctx, "/model-versions/by-hash/"+url.PathEscape(hash), nil)
}
