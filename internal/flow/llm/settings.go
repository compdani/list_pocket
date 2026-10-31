package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Settings is the resolved AI settings shape (PLAN.md §3.2).
type Settings struct {
	Provider          string            `json:"provider"`
	Model             string            `json:"model"`
	Models            map[string]string `json:"models,omitempty"`
	Temperature       *float64          `json:"temperature,omitempty"`
	MaxTokens         int               `json:"max_tokens,omitempty"`
	TopP              *float64          `json:"top_p,omitempty"`
	ReasoningEffort   string            `json:"reasoning_effort,omitempty"`
	ResponseFormat    string            `json:"response_format,omitempty"` // "" or "json_schema"
	ToolChoice        string            `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool             `json:"parallel_tool_calls,omitempty"`
	TimeoutS          int               `json:"timeout_s,omitempty"`
	Retries           int               `json:"retries,omitempty"`
	ExtraBody         map[string]any    `json:"extra_body,omitempty"`
	PromptCache       string            `json:"prompt_cache,omitempty"` // "5m" (default) | "1h" | "off"
	Search            SearchSettings    `json:"search"`
	Budget            Budget            `json:"budget"`
}

type SearchSettings struct {
	Mode              string   `json:"mode,omitempty"`     // "openrouter" (default) | "openai_native" | "off"
	Strategy          string   `json:"strategy,omitempty"` // "server_tool" (default) | "subrequest"
	Engine            string   `json:"engine,omitempty"`
	MaxResults        int      `json:"max_results,omitempty"`
	MaxTotalResults   int      `json:"max_total_results,omitempty"`
	MaxSearchesPerRun int      `json:"max_searches_per_run,omitempty"` // per agent run; default 20
	SearchContextSize string   `json:"search_context_size,omitempty"`
	ExcludedDomains   []string `json:"excluded_domains,omitempty"`
	AllowedDomains    []string `json:"allowed_domains,omitempty"`
}

type Budget struct {
	MaxUSDPerRun float64 `json:"max_usd_per_run,omitempty"`
	MaxUSDPerDay float64 `json:"max_usd_per_day,omitempty"`
}

// Merge resolves settings layers field by field; later layers win. Nested
// objects (models, search, budget, extra_body) merge recursively; JSON null
// in a later layer means "not set here" and keeps the earlier value.
func Merge(layers ...map[string]any) (Settings, error) {
	merged := map[string]any{}
	for _, layer := range layers {
		mergeInto(merged, layer)
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return Settings{}, err
	}
	var s Settings
	if err := json.Unmarshal(raw, &s); err != nil {
		return Settings{}, fmt.Errorf("invalid AI settings: %w", err)
	}
	if err := s.resolveModelRole(); err != nil {
		return Settings{}, err
	}
	s.applyDefaults()
	return s, nil
}

func mergeInto(dst, src map[string]any) {
	for k, v := range src {
		if v == nil {
			continue
		}
		if sv, ok := v.(map[string]any); ok {
			dv, ok := dst[k].(map[string]any)
			if !ok {
				dv = map[string]any{}
			}
			mergeInto(dv, sv)
			dst[k] = dv
			continue
		}
		dst[k] = v
	}
}

// resolveModelRole turns "@fast" into the model named by models.fast.
func (s *Settings) resolveModelRole() error {
	if !strings.HasPrefix(s.Model, "@") {
		return nil
	}
	role := strings.TrimPrefix(s.Model, "@")
	model, ok := s.Models[role]
	if !ok || model == "" {
		return fmt.Errorf("model role %q is not defined in models", s.Model)
	}
	s.Model = model
	return nil
}

func (s *Settings) applyDefaults() {
	if s.TimeoutS <= 0 {
		s.TimeoutS = 120
	}
	if s.Retries < 0 {
		s.Retries = 0
	}
	if s.PromptCache == "" {
		s.PromptCache = "5m"
	}
	if s.Search.Mode == "" {
		s.Search.Mode = "openrouter"
	}
	if s.Search.Strategy == "" {
		s.Search.Strategy = "server_tool"
	}
}

// ToMap is the JSON-object form of s, for storing as resolved_ai.
func (s Settings) ToMap() map[string]any {
	raw, _ := json.Marshal(s)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}
