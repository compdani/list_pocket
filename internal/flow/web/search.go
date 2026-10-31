package web

import (
	"github.com/compdani/list_pocket/internal/flow/llm"
)

const ServerToolType = "openrouter:web_search"

// engineCanExclude reports whether an OpenRouter search engine honors
// excluded_domains (PLAN.md §3.3.1). OpenAI native ignores it; Google native
// has no domain filtering at all.
func engineCanExclude(engine string) bool {
	switch engine {
	case "", "auto", "exa", "parallel", "firecrawl", "perplexity":
		return true
	}
	return false // "native": depends on the model's provider
}

// SearchWarnings lists ways the configured search can't enforce the
// blocklist at the source (the core backstop still applies).
func SearchWarnings(s llm.SearchSettings, kinds ...string) []string {
	for _, k := range kinds {
		if k == llm.KindResponses && s.Mode != "off" {
			return []string{"ChatGPT-plan search can't exclude domains up front; blocked results are removed after the response"}
		}
	}
	var w []string
	if s.Mode == "openai_native" || (s.Mode == "openrouter" && !engineCanExclude(s.Engine)) {
		w = append(w, "search engine may ignore excluded_domains; blocked results are only removed after the response")
	}
	return w
}

// ApplySearch adds provider-side web search to req according to settings.
// Returns false when search is off. With strategy "subrequest" the caller
// exposes a function tool instead and uses ApplySearch on the sub-request.
func ApplySearch(req *llm.Request, ep llm.Endpoint, s llm.SearchSettings, block *Blocklist) bool {
	if s.Mode == "off" {
		return false
	}
	if ep.Kind == llm.KindResponses {
		// ChatGPT-plan route: OpenAI's hosted search (subject to model and
		// account policy). It can't take an exclude list; the core backstop
		// drops blocklisted citations and sources.
		req.Tools = append(req.Tools, map[string]any{"type": "web_search"})
		return true
	}
	switch s.Mode {
	case "off":
		return false
	case "openai_native":
		if !ep.IsOpenRouter() {
			// Direct OpenAI: search-capable models take web_search_options.
			opts := map[string]any{}
			if s.SearchContextSize != "" {
				opts["search_context_size"] = s.SearchContextSize
			}
			if req.Extra == nil {
				req.Extra = map[string]any{}
			}
			req.Extra["web_search_options"] = opts
			return true
		}
		s.Engine = "native"
	}
	req.Tools = append(req.Tools, ServerTool(s, block))
	return true
}

// ServerTool builds the openrouter:web_search tool definition.
func ServerTool(s llm.SearchSettings, block *Blocklist) map[string]any {
	params := map[string]any{}
	if s.Engine != "" {
		params["engine"] = s.Engine
	}
	if s.MaxResults > 0 {
		params["max_results"] = s.MaxResults
	}
	if s.MaxTotalResults > 0 {
		params["max_total_results"] = s.MaxTotalResults
	}
	if s.SearchContextSize != "" {
		params["search_context_size"] = s.SearchContextSize
	}
	if len(s.AllowedDomains) > 0 {
		// Engines differ on mixing allow + exclude; an allowlist is stricter,
		// so it wins and the blocklist is enforced by the backstop.
		params["allowed_domains"] = s.AllowedDomains
	} else if excluded := mergeDomains(s.ExcludedDomains, block.Domains()); len(excluded) > 0 {
		params["excluded_domains"] = excluded
	}
	tool := map[string]any{"type": ServerToolType}
	if len(params) > 0 {
		tool["parameters"] = params
	}
	return tool
}

func mergeDomains(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, list := range [][]string{a, b} {
		for _, d := range list {
			d = normalizeDomain(d)
			if d != "" && !seen[d] {
				seen[d] = true
				out = append(out, d)
			}
		}
	}
	return out
}

// FilterCitations drops citations whose URL is blocked; returns kept and
// dropped.
func FilterCitations(cs []llm.Citation, block *Blocklist) (kept, dropped []llm.Citation) {
	seen := map[string]bool{}
	for _, c := range cs {
		if block.BlockedURL(c.URL) {
			dropped = append(dropped, c)
			continue
		}
		if seen[c.URL] {
			continue
		}
		seen[c.URL] = true
		kept = append(kept, c)
	}
	return kept, dropped
}

// DropBlockedSources walks v and removes, from any array, objects whose
// "source_url" is blocked. Returns the cleaned value and removed URLs.
func DropBlockedSources(v any, block *Blocklist) (any, []string) {
	var removed []string
	var walk func(any) any
	walk = func(v any) any {
		switch t := v.(type) {
		case map[string]any:
			for k, child := range t {
				t[k] = walk(child)
			}
			return t
		case []any:
			out := t[:0]
			for _, item := range t {
				if m, ok := item.(map[string]any); ok {
					if src, ok := m["source_url"].(string); ok && src != "" && block.BlockedURL(src) {
						removed = append(removed, src)
						continue
					}
				}
				out = append(out, walk(item))
			}
			return out
		}
		return v
	}
	return walk(v), removed
}
