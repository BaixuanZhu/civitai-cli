package api

import (
	"context"
	"net/url"
	"strconv"
)

// ListTags fetches GET /tags?query=&limit= for tag-name completion and
// returns the raw response body. Decode with ParseTags.
func (c *Client) ListTags(ctx context.Context, query string, limit int) ([]byte, error) {
	q := url.Values{}
	if query != "" {
		q.Set("query", query)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	return c.Get(ctx, "/tags", q)
}
