package llm

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// Providers a request can use.
const (
	// ProviderAPI is the API key configured in the environment.
	ProviderAPI = "api"
	// ProviderChatGPT is a signed-in ChatGPT plan, which pays through Codex usage.
	ProviderChatGPT = "chatgpt"
)

// ChatGPTBaseURL is where plan requests go.
const ChatGPTBaseURL = "https://api.openai.com/v1"

// Resolver turns settings layers into Settings + Endpoint. The API key comes from the
// same environment variables as the rest of the app, and the plan from Tokens.
type Resolver struct {
	Getenv func(string) string // defaults to os.Getenv
	// Settings holds instance-wide values: domain_blocklist and budgets.
	Settings map[string]any
	// Tokens returns a current access token for the signed-in ChatGPT plan.
	Tokens func(ctx context.Context) (string, error)
	// Defaults returns the provider and model chosen in Settings → AI, either of which
	// can be empty.
	Defaults func() (provider, model string)
}

const (
	DefaultModel   = "gpt-5.4-mini"
	DefaultBaseURL = "https://api.openai.com/v1"
)

func (r *Resolver) env(key string) string {
	getenv := r.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	return strings.TrimSpace(getenv(key))
}

// DefaultChoice returns the provider and model used when nothing else says otherwise.
func (r *Resolver) DefaultChoice() (provider, model string) {
	if r.Defaults != nil {
		provider, model = r.Defaults()
	}
	if provider == "" {
		provider = ProviderAPI
	}
	if model == "" && provider == ProviderAPI {
		if model = r.env("LISTPOCKET_AI_MODEL"); model == "" {
			model = DefaultModel
		}
	}
	return provider, model
}

// BaseLayer is the bottom AI settings layer.
//
//	LISTPOCKET_AI_MODEL           default model on the API key
//	LISTPOCKET_AI_RESEARCH_MODEL  the @research role (default: the default model)
//	LISTPOCKET_AI_FAST_MODEL      the @fast role (default: the default model)
func (r *Resolver) BaseLayer() map[string]any {
	or := func(key, fallback string) string {
		if v := r.env(key); v != "" {
			return v
		}
		return fallback
	}
	provider, model := r.DefaultChoice()
	search := "openai_native"
	if strings.Contains(r.baseURL(), "openrouter.ai") {
		search = "openrouter"
	}
	search = or("LISTPOCKET_AI_SEARCH_MODE", search)
	return map[string]any{
		"provider": provider,
		"model":    model,
		"models": map[string]any{
			"research": or("LISTPOCKET_AI_RESEARCH_MODEL", model),
			"fast":     or("LISTPOCKET_AI_FAST_MODEL", model),
		},
		"search": map[string]any{"mode": search},
	}
}

func (r *Resolver) baseURL() string {
	if v := r.env("LISTPOCKET_AI_BASE_URL"); v != "" {
		return v
	}
	return DefaultBaseURL
}

// Resolve merges the base layer with the given layers (workflow, node, run, in that
// order) and returns the provider endpoint.
func (r *Resolver) Resolve(layers ...map[string]any) (Settings, Endpoint, error) {
	s, err := Merge(append([]map[string]any{r.BaseLayer()}, layers...)...)
	if err != nil {
		return Settings{}, Endpoint{}, err
	}
	if s.Model == "" {
		return Settings{}, Endpoint{}, fmt.Errorf("no model chosen: pick one in Settings → AI")
	}
	ep, err := r.Endpoint(s.Provider)
	if err != nil {
		return Settings{}, Endpoint{}, err
	}
	return s, ep, nil
}

// Endpoint returns where to send requests for a provider, and how to authorize them.
func (r *Resolver) Endpoint(provider string) (Endpoint, error) {
	switch provider {
	case ProviderChatGPT:
		if r.Tokens == nil {
			return Endpoint{}, fmt.Errorf("ChatGPT plan sign-in isn't available")
		}
		token, err := r.Tokens(context.Background())
		if err != nil {
			return Endpoint{}, err
		}
		base := r.env("LISTPOCKET_CHATGPT_BASE_URL")
		if base == "" {
			base = ChatGPTBaseURL
		}
		return Endpoint{Name: provider, Kind: KindResponses, BaseURL: strings.TrimRight(base, "/"), APIKey: token}, nil
	case ProviderAPI, "":
		key := r.env("LISTPOCKET_AI_API_KEY")
		if key == "" {
			return Endpoint{}, fmt.Errorf("AI isn't set up: configure LISTPOCKET_AI_API_KEY, or sign in with ChatGPT in Settings → AI")
		}
		return Endpoint{Name: ProviderAPI, BaseURL: strings.TrimRight(r.baseURL(), "/"), APIKey: key}, nil
	}
	return Endpoint{}, fmt.Errorf("unknown AI provider %q", provider)
}

// Setting returns an instance-wide setting.
func (r *Resolver) Setting(key string) (any, error) {
	if v, ok := r.Settings[key]; ok {
		return v, nil
	}
	return nil, fmt.Errorf("setting %q is not set", key)
}
