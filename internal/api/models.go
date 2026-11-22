package api

import (
	"context"
	"net/url"
	"strconv"
)

// SearchParams carries /models query parameters. Enum fields must already
// hold API values (the cmd layer maps CLI vocabulary via enums.go).
//
// Multi-value semantics verified live 2026-11-22:
//   - Types: sent as repeated types= params (the comma-joined form is a
//     ZodError; the API union accepts a single value or an array).
//   - BaseModels: sent as repeated baseModels= params, but the API keeps
//     the FIRST value only — the cmd layer warns when more than one is
//     given. The singular baseModel param is silently ignored for ALL
//     values and must not be used.
type SearchParams struct {
	Query      string
	Types      []string
	BaseModels []string
	Tag        string
	Username   string
	Sort       string
	Period     string
	Limit      int    // 1..100; 0 omits the param (server default 100)
	Cursor     string // metadata.nextCursor from a previous page
}

// SearchModels fetches GET /models and returns the raw response body
// (envelope with items + metadata). Decode with ParseModels for human
// output or ModelsRawItems for --json passthrough.
func (c *Client) SearchModels(ctx context.Context, p SearchParams) ([]byte, error) {
	q := url.Values{}
	if p.Query != "" {
		q.Set("query", p.Query)
	}
	for _, t := range p.Types {
		if t != "" {
			q.Add("types", t)
		}
	}
	for _, b := range p.BaseModels {
		if b != "" {
			q.Add("baseModels", b)
		}
	}
	if p.Tag != "" {
		q.Set("tag", p.Tag)
	}
	if p.Username != "" {
		q.Set("username", p.Username)
	}
	if p.Sort != "" {
		q.Set("sort", p.Sort)
	}
	if p.Period != "" {
		q.Set("period", p.Period)
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	if p.Cursor != "" {
		q.Set("cursor", p.Cursor)
	}
	return c.Get(ctx, "/models", q)
}

// GetModel fetches GET /models/{id} and returns the raw model body
// (same shape as a search item, with all versions). Decode with ParseModel.
func (c *Client) GetModel(ctx context.Context, id int) ([]byte, error) {
	return c.Get(ctx, "/models/"+strconv.Itoa(id), nil)
}
