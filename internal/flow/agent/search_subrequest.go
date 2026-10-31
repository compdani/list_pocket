package agent

import (
	"context"
	"fmt"

	"github.com/compdani/list_pocket/internal/flow/llm"
	"github.com/compdani/list_pocket/internal/flow/tools"
	"github.com/compdani/list_pocket/internal/flow/web"
)

// searchSubrequest is the web.search fallback (search.strategy=subrequest)
// for models that can't mix the provider's search server tool with function
// tools: each call makes a separate search-only request and returns its
// cited answer to the main loop.
type searchSubrequest struct {
	st *run
}

func (*searchSubrequest) Name() string { return tools.WebSearch }

func (*searchSubrequest) Description() string {
	return "Search the web. Returns a short answer with source citations (url, title). " +
		"Ask focused queries; call again to dig deeper."
}

func (*searchSubrequest) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "What to search for."},
		},
		"required":             []string{"query"},
		"additionalProperties": false,
	}
}

func (*searchSubrequest) RequiresApproval() bool { return false }

func (t *searchSubrequest) Invoke(ctx context.Context, call tools.Call) (any, error) {
	st := t.st
	if st.searches >= st.searchCap() {
		return nil, fmt.Errorf("web search limit reached (%d searches); finish with what you have", st.searches)
	}
	query, _ := call.Args["query"].(string)
	req := llm.Request{
		Model: st.settings.Model,
		Messages: []llm.Message{
			{Role: "system", Content: "Search the web and answer the query concisely. Cite every source you rely on."},
			{Role: "user", Content: query},
		},
		Settings: st.settings,
	}
	req.Settings.ToolChoice = ""
	web.ApplySearch(&req, st.endpoint, st.settings.Search, st.block)

	resp, err := st.r.LLM.Complete(ctx, st.endpoint, req, nil)
	if err != nil {
		return nil, err
	}
	st.account(resp)
	kept, _ := web.FilterCitations(resp.Citations, st.block)
	return map[string]any{"answer": resp.Message.Content, "citations": kept}, nil
}
