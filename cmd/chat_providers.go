package main

// chatProviderStore adapts Settings → AI to the chat module's provider catalog.
// List Pocket keeps the only copy of the settings: the module reads providers,
// models, the default and the ChatGPT plan token from here, and never signs in
// or refreshes by itself (catalog.CodexTokenStore).

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/compdani/list_pocket/internal/flow/chatgpt"
	"github.com/compdani/list_pocket/internal/flow/llm"
	"github.com/compdani/pocketbase-mcp/chat/provider/catalog"
	"github.com/compdani/pocketbase-mcp/chat/provider/codex"
)

// errChatSettingsHostOwned explains why the module's write paths are refused.
var errChatSettingsHostOwned = errors.New("AI providers are managed in Settings → AI")

var (
	_ catalog.Store           = (*chatProviderStore)(nil)
	_ catalog.CodexTokenStore = (*chatProviderStore)(nil)
)

type chatProviderStore struct {
	app *App
}

// ListProviders returns the providers Settings → AI has ready, keyed by List
// Pocket's own names so stored choices and the UI keep working. A provider that
// isn't set up is omitted rather than returned disabled: the catalog only needs
// what a turn can use.
func (s *chatProviderStore) ListProviders(ctx context.Context, _ catalog.Scope) ([]catalog.Provider, error) {
	if s == nil || s.app == nil || s.app.chatgpt == nil {
		return nil, nil
	}
	var out []catalog.Provider
	for _, build := range []func(context.Context) (catalog.Provider, bool){
		s.codexProvider,
		s.apiProvider,
	} {
		p, ok := build(ctx)
		if !ok {
			continue
		}
		p = p.Normalized()
		if err := p.Validate(); err != nil {
			// A bad env URL or an unusable model list must not take chat down.
			s.app.logf("chat: provider %q unavailable: %v", p.ID, err)
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// codexProvider is the signed-in ChatGPT plan. Models come from the same live
// cache Settings → AI uses, after checking the sign-in so a signed-out install
// doesn't touch the network.
func (s *chatProviderStore) codexProvider(ctx context.Context) (catalog.Provider, bool) {
	a := s.app
	if !codexSignedIn(a.chatgpt.store) {
		return catalog.Provider{}, false
	}
	list := a.aiProviderModels(ctx, llm.ProviderChatGPT, false)
	models := catalogModels(list.models)
	if list.err != "" || len(models) == 0 {
		return catalog.Provider{}, false
	}
	// chatgpt.ListModels doesn't parse supported reasoning levels, so these
	// models carry no efforts and the model's own default applies.
	p := catalog.Provider{
		ID:      llm.ProviderChatGPT,
		Label:   "ChatGPT plan",
		Kind:    catalog.KindCodex,
		Enabled: true,
		BaseURL: chatEnvOr("LISTPOCKET_CHATGPT_BASE_URL", llm.ChatGPTBaseURL),
		Models:  models,
	}
	if provider, model := a.aiDefaults(); provider == p.ID {
		if _, ok := p.Model(model); ok {
			p.DefaultModel = model
		}
	}
	return p, true
}

// apiProvider is the OpenAI-compatible API key from the environment. When the
// gateway has no usable /models list it still offers the one model every AI
// feature already falls back to, so chat works exactly like today.
func (s *chatProviderStore) apiProvider(ctx context.Context) (catalog.Provider, bool) {
	a := s.app
	if strings.TrimSpace(os.Getenv("LISTPOCKET_AI_API_KEY")) == "" {
		return catalog.Provider{}, false
	}
	list := a.aiProviderModels(ctx, llm.ProviderAPI, false)
	models := catalogModels(list.models)
	if list.err != "" || len(models) == 0 {
		fallback := a.aiAPIDefaultModel()
		models = []catalog.Model{{ID: fallback, Label: fallback}}
	}
	p := catalog.Provider{
		ID:       llm.ProviderAPI,
		Label:    "API key",
		Kind:     catalog.KindOpenAI,
		Enabled:  true,
		BaseURL:  chatEnvOr("LISTPOCKET_AI_BASE_URL", llm.DefaultBaseURL),
		WireAPI:  "chat",
		APIKey:   strings.TrimSpace(os.Getenv("LISTPOCKET_AI_API_KEY")),
		Models:   models,
	}
	wanted := a.aiAPIDefaultModel()
	if _, ok := p.Model(wanted); ok {
		p.DefaultModel = wanted
	}
	return p, true
}

// aiAPIDefaultModel is the model the resolver would use on the API key.
func (a *App) aiAPIDefaultModel() string {
	if a != nil {
		if provider, model := a.aiResolver().DefaultChoice(); provider == llm.ProviderAPI {
			if model = strings.TrimSpace(model); model != "" {
				return model
			}
		}
	}
	if model := strings.TrimSpace(os.Getenv("LISTPOCKET_AI_MODEL")); model != "" {
		return model
	}
	return llm.DefaultModel
}

func catalogModels(models []aiModel) []catalog.Model {
	out := make([]catalog.Model, 0, len(models))
	for _, m := range models {
		if strings.TrimSpace(m.ID) == "" {
			continue
		}
		out = append(out, catalog.Model{ID: m.ID, Label: m.Name})
	}
	return out
}

func chatEnvOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func (s *chatProviderStore) GetProvider(ctx context.Context, scope catalog.Scope, id string) (catalog.Provider, error) {
	providers, err := s.ListProviders(ctx, scope)
	if err != nil {
		return catalog.Provider{}, err
	}
	for _, p := range providers {
		if p.ID == id {
			return p, nil
		}
	}
	return catalog.Provider{}, catalog.ErrProviderNotFound
}

// The write methods exist so the store satisfies catalog.Store; the host's
// settings page is the only writer, and catalog.ManageProviders stays off.
func (s *chatProviderStore) SaveProvider(context.Context, catalog.Scope, catalog.Provider) error {
	return errChatSettingsHostOwned
}

func (s *chatProviderStore) DeleteProvider(context.Context, catalog.Scope, string) error {
	return errChatSettingsHostOwned
}

func (s *chatProviderStore) SaveCredentials(context.Context, catalog.Scope, string, *codex.Credentials) error {
	return errChatSettingsHostOwned
}

func (s *chatProviderStore) ClearCredentials(context.Context, catalog.Scope, string) error {
	return errChatSettingsHostOwned
}

func (s *chatProviderStore) SaveSetting(context.Context, catalog.Scope, string, any) error {
	return errChatSettingsHostOwned
}

// LoadCredentials is unused: host-owned codex tokens skip the router's own
// credential store entirely (catalog.CodexTokenStore).
func (s *chatProviderStore) LoadCredentials(context.Context, catalog.Scope, string) (*codex.Credentials, error) {
	return nil, nil
}

// LoadSetting serves only defaultChoice. Any other key must be ErrSettingNotFound:
// the router treats a nil answer to titleChoice as "off", which would silently
// switch titles back off.
func (s *chatProviderStore) LoadSetting(_ context.Context, _ catalog.Scope, key string, dst any) error {
	if key != "defaultChoice" {
		return catalog.ErrSettingNotFound
	}
	provider, model := s.app.aiDefaults()
	if provider == "" && model == "" {
		provider, model = s.app.aiResolver().DefaultChoice()
	}
	raw, err := json.Marshal(aiChoice{Provider: provider, Model: model})
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}

func (s *chatProviderStore) HostID(context.Context, catalog.Scope) (string, error) {
	if s == nil || s.app == nil || s.app.chatgpt == nil || s.app.chatgpt.store == nil {
		return "", errors.New("chat: no ChatGPT store")
	}
	return s.app.chatgpt.store.HostID(), nil
}

// CodexToken hands the router a fresh plan token. Refreshes stay inside
// chatgpt.Store, so the rotating refresh token has exactly one owner.
func (s *chatProviderStore) CodexToken(ctx context.Context, _ catalog.Scope, _ string, forceRefresh bool) (string, error) {
	if s == nil || s.app == nil || s.app.chatgpt == nil || s.app.chatgpt.store == nil {
		return "", errors.New("chat: no ChatGPT store")
	}
	if forceRefresh {
		return s.app.chatgpt.store.Refresh(ctx)
	}
	return s.app.chatgpt.store.Token(ctx)
}

func (s *chatProviderStore) CodexSignedIn(context.Context, catalog.Scope, string) bool {
	if s == nil || s.app == nil || s.app.chatgpt == nil {
		return false
	}
	return codexSignedIn(s.app.chatgpt.store)
}

func codexSignedIn(store *chatgpt.Store) bool {
	if store == nil {
		return false
	}
	st := store.Status()
	return st.SignedIn && st.Plan
}

// reloadChatProviders refreshes the chat catalog after Settings → AI changes.
// It's a no-op until step 3 enables chat, and a failure only leaves the old
// snapshot in place.
func (a *App) reloadChatProviders(ctx context.Context) {
	if a == nil || a.mcp == nil {
		return
	}
	router, ok := a.mcp.ChatRouter()
	if !ok || router == nil {
		return
	}
	if err := router.Reload(ctx); err != nil {
		a.logf("chat: reloading providers: %v", err)
	}
}

// reloadChatProvidersSoon keeps a settings response fast: Reload may fetch
// /models through the cache, and the response doesn't depend on it.
func (a *App) reloadChatProvidersSoon() {
	go a.reloadChatProviders(context.Background())
}
