package tools

import (
	"context"
	"errors"

	"github.com/compdani/list_pocket/internal/flow/web"
)

// WebFetch is the built-in web.fetch tool.
type WebFetch struct {
	Fetcher *web.Fetcher
}

func (WebFetch) Name() string { return "web.fetch" }

func (WebFetch) Description() string {
	return "Fetch a public web page and return its readable text (links shown as 'text (url)', " +
		"email links as the address). Use it to read pages found by search and to confirm facts " +
		"such as a published email address. Blocked domains, robots.txt-disallowed pages, and " +
		"private addresses are refused."
}

func (WebFetch) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url":       map[string]any{"type": "string", "description": "Absolute http(s) URL."},
			"max_chars": map[string]any{"type": "integer", "description": "Maximum characters of text to return (default 8000, max 30000). Ask for more only when the page was truncated and you need the rest."},
		},
		"required":             []string{"url"},
		"additionalProperties": false,
	}
}

func (WebFetch) RequiresApproval() bool { return false }

func (WebFetch) ReadOnly() bool { return true }

func (t WebFetch) Invoke(ctx context.Context, call Call) (any, error) {
	url, _ := call.Args["url"].(string)
	maxChars := 0
	if v, ok := call.Args["max_chars"].(float64); ok {
		maxChars = int(v)
	}
	return t.Fetcher.Fetch(ctx, url, maxChars)
}

// WebSearchInfo is the catalog entry for web.search. Search runs inside the
// model provider (PLAN.md §3.3.1); the agent loop wires it into requests, so
// this tool is never invoked directly.
type WebSearchInfo struct{}

func (WebSearchInfo) Name() string { return WebSearch }

func (WebSearchInfo) Description() string {
	return "Web search performed by the model provider (OpenRouter web search or OpenAI native). " +
		"Give it to an agent and the model searches when it needs to; results come back as citations. " +
		"Configured per workflow under ai.search."
}

func (WebSearchInfo) InputSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func (WebSearchInfo) RequiresApproval() bool { return false }

func (WebSearchInfo) Invoke(context.Context, Call) (any, error) {
	return nil, errors.New("web.search is provider-side and cannot be invoked directly")
}
