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
	"sync/atomic"

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
	// cacheOnly keeps the catalog's first build off the network. EnableChat
	// builds a snapshot synchronously, and ListProviders may otherwise fetch
	// /models (up to 20s per provider) before the app can serve anything. The
	// first build reads only the shared model cache; a background reload then
	// fills it in.
	cacheOnly atomic.Bool
}

// ListProviders returns the providers Settings → AI has ready, keyed by List
// Pocket's own names so stored choices and the UI keep working. A provider that
// isn't set up is omitted rather than returned disabled: the catalog only needs
// what a turn can use.
func (s *chatProviderStore) ListProviders(ctx context.Context, _ catalog.Scope) ([]catalog.Provider, error) {
	// A cancelled reload must never reach the database: aiDefaults below reads
	// PocketBase, which shutdownApp may already have reset.
	if err := contextDone(ctx); err != nil {
		return nil, err
	}
	if s == nil || s.app == nil || s.app.chatgpt == nil {
		return nil, nil
	}
	var out []catalog.Provider
	for _, build := range []func(context.Context) (catalog.Provider, bool){
		s.codexProvider,
		s.apiProvider,
	} {
		if err := contextDone(ctx); err != nil {
			return nil, err
		}
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

// providerModels returns one provider's model list. The startup snapshot reads
// the cache only; the background reload after EnableChat does the fetching.
func (s *chatProviderStore) providerModels(ctx context.Context, provider string) cachedAIModels {
	if s.cacheOnly.Load() {
		if cached, ok := s.app.cachedProviderModels(provider); ok {
			return cached
		}
		return cachedAIModels{err: "the model list hasn't been fetched yet"}
	}
	return s.app.aiProviderModels(ctx, provider, false)
}

// cachedProviderModels reads the 10-minute model cache without any network call.
func (a *App) cachedProviderModels(provider string) (cachedAIModels, bool) {
	if a == nil || a.chatgpt == nil {
		return cachedAIModels{}, false
	}
	a.chatgpt.modelsMu.Lock()
	defer a.chatgpt.modelsMu.Unlock()
	cached, ok := a.chatgpt.models[provider]
	return cached, ok
}

// codexProvider is the signed-in ChatGPT plan. Models come from the same live
// cache Settings → AI uses, after checking the sign-in so a signed-out install
// doesn't touch the network.
func (s *chatProviderStore) codexProvider(ctx context.Context) (catalog.Provider, bool) {
	a := s.app
	if !codexSignedIn(a.chatgpt.store) {
		return catalog.Provider{}, false
	}
	list := s.providerModels(ctx, llm.ProviderChatGPT)
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
	list := s.providerModels(ctx, llm.ProviderAPI)
	models := catalogModels(list.models)
	if list.err != "" || len(models) == 0 {
		fallback := a.aiAPIDefaultModel()
		models = []catalog.Model{{ID: fallback, Label: fallback}}
	}
	p := catalog.Provider{
		ID:      llm.ProviderAPI,
		Label:   "API key",
		Kind:    catalog.KindOpenAI,
		Enabled: true,
		BaseURL: chatEnvOr("LISTPOCKET_AI_BASE_URL", llm.DefaultBaseURL),
		WireAPI: "chat",
		APIKey:  strings.TrimSpace(os.Getenv("LISTPOCKET_AI_API_KEY")),
		Models:  models,
	}
	wanted := a.aiAPIDefaultModel()
	if provider, model := a.chatDefaultChoice(); provider == llm.ProviderAPI && strings.TrimSpace(model) != "" {
		wanted = model
		// The builder's model may not be in the gateway's /models list. Chat
		// still has to offer it, exactly like today's side panel.
		if _, ok := p.Model(wanted); !ok {
			p.Models = append(p.Models, catalog.Model{ID: wanted, Label: wanted})
		}
	}
	if _, ok := p.Model(wanted); ok {
		p.DefaultModel = wanted
	}
	return p, true
}

// chatDefaultChoice is the provider and model a turn uses when it names neither.
// With no choice in Settings → AI, it is the campaign builder's model on the API
// key, which is what the assistant uses today (runAssistant).
func (a *App) chatDefaultChoice() (string, string) {
	if a == nil {
		return "", ""
	}
	provider, model := a.aiDefaults()
	if provider != "" || model != "" {
		return provider, model
	}
	if builder, _, _ := a.getAIBuilderSettingsValues(); strings.TrimSpace(builder) != "" {
		return llm.ProviderAPI, builder
	}
	return a.aiResolver().DefaultChoice()
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
func (s *chatProviderStore) LoadSetting(ctx context.Context, _ catalog.Scope, key string, dst any) error {
	if err := contextDone(ctx); err != nil {
		return err
	}
	if key != "defaultChoice" {
		return catalog.ErrSettingNotFound
	}
	provider, model := s.app.chatDefaultChoice()
	raw, err := json.Marshal(aiChoice{Provider: provider, Model: model})
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}

func (s *chatProviderStore) HostID(ctx context.Context, _ catalog.Scope) (string, error) {
	if err := contextDone(ctx); err != nil {
		return "", err
	}
	if s == nil || s.app == nil || s.app.chatgpt == nil || s.app.chatgpt.store == nil {
		return "", errors.New("chat: no ChatGPT store")
	}
	return s.app.chatgpt.store.HostID(), nil
}

// CodexToken hands the router a fresh plan token. Refreshes stay inside
// chatgpt.Store, so the rotating refresh token has exactly one owner.
func (s *chatProviderStore) CodexToken(ctx context.Context, _ catalog.Scope, _ string, forceRefresh bool) (string, error) {
	if err := contextDone(ctx); err != nil {
		return "", err
	}
	if s == nil || s.app == nil || s.app.chatgpt == nil || s.app.chatgpt.store == nil {
		return "", errors.New("chat: no ChatGPT store")
	}
	if forceRefresh {
		return s.app.chatgpt.store.Refresh(ctx)
	}
	return s.app.chatgpt.store.Token(ctx)
}

func (s *chatProviderStore) CodexSignedIn(ctx context.Context, _ catalog.Scope, _ string) bool {
	if contextDone(ctx) != nil {
		return false
	}
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

// contextDone returns ctx's error, treating a nil context as live: the catalog
// always passes one, and the nil check keeps the store safe when called
// directly.
func contextDone(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

// reloadChatProviders refreshes the chat catalog after Settings → AI changes.
// It's a no-op until step 3 enables chat, and a failure only leaves the old
// snapshot in place. A cancelled context means the app is shutting down, so it
// never starts a build.
func (a *App) reloadChatProviders(ctx context.Context) {
	if a == nil || a.mcp == nil || contextDone(ctx) != nil {
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

// goReloadChatProviders keeps a settings response fast and ties the reload to
// the app's lifetime: Reload may fetch /models (and then read the database), so
// it runs in the app's chat reload scope and shutdownApp cancels and joins it.
// After shutdown nothing starts, which is what keeps a late reload away from a
// reset PocketBase.
func (a *App) goReloadChatProviders() {
	if a == nil {
		return
	}
	ctx, ok := a.chatReloads.schedule()
	if !ok {
		return
	}
	go func() {
		defer a.chatReloads.done()
		a.reloadChatProviders(ctx)
	}()
}
