package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"testing"
	"time"

	"ganigeorgiev.com/pocketbase/pocketbase"
	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	pbcore "ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/compdani/list_pocket/internal/flow/chatgpt"
	"github.com/compdani/list_pocket/internal/flow/llm"
	"github.com/compdani/list_pocket/internal/plugins"
	"github.com/compdani/pocketbase-mcp/chat/provider/catalog"
)

// chatProvidersTestApp is an App with the real ai_settings and
// oauth_credentials collections but no network: every model list the adapter
// reads must be pre-cached.
func chatProvidersTestApp(t *testing.T) *App {
	t.Helper()
	pbApp := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	if err := pbApp.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pbApp.ResetBootstrapState() })
	if err := pbApp.RunAppMigrations(); err != nil {
		t.Fatal(err)
	}
	a := &App{pb: pbApp, log: log.New(io.Discard, "", 0)}
	a.chatgpt = newChatGPTService(pbApp)
	// Cleanups run last-in-first-out, so stopping the app's background work
	// happens before the reset registered above: a leaked chat reload must
	// never reach a reset PocketBase.
	t.Cleanup(func() { shutdownApp(a) })
	return a
}

// cacheModels seeds the same 10-minute cache aiProviderModels reads.
func cacheModels(a *App, provider string, cached cachedAIModels) {
	cached.at = time.Now()
	a.chatgpt.modelsMu.Lock()
	a.chatgpt.models[provider] = cached
	a.chatgpt.modelsMu.Unlock()
}

func signedInCreds() *chatgpt.Credentials {
	return &chatgpt.Credentials{
		ClientID: "oaiapp_1", Sub: "user-1", Email: "ann@example.com",
		AccessToken: "at-1", RefreshToken: "rt-1",
		Scopes:    []string{chatgpt.PlanScope},
		ExpiresAt: time.Now().Add(time.Hour),
	}
}

func TestChatProviderStoreEmptyWithoutSettings(t *testing.T) {
	a := chatProvidersTestApp(t)
	t.Setenv("LISTPOCKET_AI_API_KEY", "")
	store := &chatProviderStore{app: a}

	providers, err := store.ListProviders(context.Background(), catalog.Scope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 0 {
		t.Fatalf("expected no providers, got %+v", providers)
	}
}

func TestChatProviderStoreAPIKey(t *testing.T) {
	a := chatProvidersTestApp(t)
	t.Setenv("LISTPOCKET_AI_API_KEY", "sk-test")
	t.Setenv("LISTPOCKET_AI_MODEL", "gpt-y")
	t.Setenv("LISTPOCKET_AI_BASE_URL", "")
	cacheModels(a, llm.ProviderAPI, cachedAIModels{models: []aiModel{
		{ID: "gpt-x", Name: "GPT X"},
		{ID: "gpt-y", Name: "GPT Y"},
	}})
	// An explicit Settings → AI default, so this test is not also exercising
	// the builder-model fallback.
	if err := a.setAIDefaults(aiChoice{Provider: llm.ProviderAPI, Model: "gpt-y"}); err != nil {
		t.Fatal(err)
	}
	store := &chatProviderStore{app: a}

	providers, err := store.ListProviders(context.Background(), catalog.Scope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 || providers[0].ID != llm.ProviderAPI {
		t.Fatalf("expected only the api provider, got %+v", providers)
	}
	p := providers[0]
	if !p.Enabled || p.Kind != catalog.KindOpenAI || p.WireAPI != "chat" || p.APIKey != "sk-test" {
		t.Errorf("unexpected provider: %+v", p)
	}
	if len(p.Models) != 2 || p.Models[0].ID != "gpt-x" || p.Models[1].Label != "GPT Y" {
		t.Errorf("models: %+v", p.Models)
	}
	if p.DefaultModel != "gpt-y" {
		t.Errorf("default model = %q, want gpt-y", p.DefaultModel)
	}
	if err := p.Validate(); err != nil {
		t.Errorf("returned provider must validate: %v", err)
	}
	if p.BaseURL != llm.DefaultBaseURL {
		t.Errorf("base URL = %q", p.BaseURL)
	}

	got, err := store.GetProvider(context.Background(), catalog.Scope{}, llm.ProviderAPI)
	if err != nil || got.ID != llm.ProviderAPI {
		t.Errorf("GetProvider: %+v %v", got, err)
	}
	if _, err := store.GetProvider(context.Background(), catalog.Scope{}, llm.ProviderChatGPT); !errors.Is(err, catalog.ErrProviderNotFound) {
		t.Errorf("expected ErrProviderNotFound, got %v", err)
	}
}

func TestChatProviderStoreAPIFallsBackWhenModelsFail(t *testing.T) {
	a := chatProvidersTestApp(t)
	t.Setenv("LISTPOCKET_AI_API_KEY", "sk-test")
	t.Setenv("LISTPOCKET_AI_MODEL", "")
	cacheModels(a, llm.ProviderAPI, cachedAIModels{err: "the provider returned HTTP 500"})
	store := &chatProviderStore{app: a}

	providers, err := store.ListProviders(context.Background(), catalog.Scope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 {
		t.Fatalf("expected the api provider, got %+v", providers)
	}
	p := providers[0]
	// With no Settings → AI default, the campaign builder's model is what the
	// side panel uses, so that is the one chat offers.
	if len(p.Models) != 1 || p.Models[0].ID != defaultAIBuilderModel {
		t.Fatalf("expected the fallback model %q, got %+v", defaultAIBuilderModel, p.Models)
	}
	if err := p.Validate(); err != nil {
		t.Errorf("fallback provider must validate: %v", err)
	}
}

func TestChatProviderStoreBuilderModelFallback(t *testing.T) {
	a := chatProvidersTestApp(t)
	t.Setenv("LISTPOCKET_AI_API_KEY", "sk-test")
	t.Setenv("LISTPOCKET_AI_MODEL", "")
	cacheModels(a, llm.ProviderAPI, cachedAIModels{models: []aiModel{{ID: "gpt-x", Name: "GPT X"}}})
	// No Settings → AI default: the builder's model on the API key, exactly like
	// a side-panel turn, and offered even when the gateway's list lacks it.
	if err := a.upsertTypedSettingsRecord("ai_builder", `{"model":"gpt-5.4-nano","timeout_seconds":300}`); err != nil {
		t.Fatal(err)
	}
	store := &chatProviderStore{app: a}

	providers, err := store.ListProviders(context.Background(), catalog.Scope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 {
		t.Fatalf("expected the api provider, got %+v", providers)
	}
	p := providers[0]
	if p.DefaultModel != "gpt-5.4-nano" {
		t.Errorf("default model = %q, want gpt-5.4-nano", p.DefaultModel)
	}
	if _, ok := p.Model("gpt-5.4-nano"); !ok {
		t.Errorf("the builder model must be offered: %+v", p.Models)
	}

	var choice aiChoice
	if err := store.LoadSetting(context.Background(), catalog.Scope{}, "defaultChoice", &choice); err != nil {
		t.Fatal(err)
	}
	if choice.Provider != llm.ProviderAPI || choice.Model != "gpt-5.4-nano" {
		t.Errorf("defaultChoice = %+v", choice)
	}
}

func TestChatProviderStoreChatGPT(t *testing.T) {
	a := chatProvidersTestApp(t)
	t.Setenv("LISTPOCKET_AI_API_KEY", "")
	t.Setenv("LISTPOCKET_CHATGPT_BASE_URL", "")
	cacheModels(a, llm.ProviderChatGPT, cachedAIModels{models: []aiModel{
		{ID: "codex-1", Name: "Codex One"},
	}})
	store := &chatProviderStore{app: a}

	// Signed out: absent, and no model fetch is attempted.
	providers, err := store.ListProviders(context.Background(), catalog.Scope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 0 {
		t.Fatalf("signed out should not offer chatgpt: %+v", providers)
	}
	if store.CodexSignedIn(context.Background(), catalog.Scope{}, llm.ProviderChatGPT) {
		t.Fatal("CodexSignedIn should be false while signed out")
	}

	if err := a.chatgpt.store.Save(signedInCreds()); err != nil {
		t.Fatal(err)
	}
	providers, err = store.ListProviders(context.Background(), catalog.Scope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 || providers[0].ID != llm.ProviderChatGPT {
		t.Fatalf("expected the chatgpt provider, got %+v", providers)
	}
	p := providers[0]
	if p.Kind != catalog.KindCodex || !p.Enabled || p.BaseURL != llm.ChatGPTBaseURL {
		t.Errorf("unexpected provider: %+v", p)
	}
	if len(p.Models) != 1 || p.Models[0].ID != "codex-1" || p.Models[0].Label != "Codex One" {
		t.Errorf("models: %+v", p.Models)
	}
	if err := p.Validate(); err != nil {
		t.Errorf("returned provider must validate: %v", err)
	}
	if !store.CodexSignedIn(context.Background(), catalog.Scope{}, llm.ProviderChatGPT) {
		t.Error("CodexSignedIn should be true after sign-in")
	}

	if err := a.chatgpt.store.Clear(); err != nil {
		t.Fatal(err)
	}
	providers, _ = store.ListProviders(context.Background(), catalog.Scope{})
	if len(providers) != 0 || store.CodexSignedIn(context.Background(), catalog.Scope{}, llm.ProviderChatGPT) {
		t.Errorf("signed out again: providers=%+v", providers)
	}
}

func TestChatProviderStoreSettings(t *testing.T) {
	a := chatProvidersTestApp(t)
	store := &chatProviderStore{app: a}
	ctx := context.Background()

	if err := a.setAIDefaults(aiChoice{Provider: llm.ProviderChatGPT, Model: "gpt-5-codex"}); err != nil {
		t.Fatal(err)
	}
	var choice aiChoice
	if err := store.LoadSetting(ctx, catalog.Scope{}, "defaultChoice", &choice); err != nil {
		t.Fatal(err)
	}
	if choice.Provider != llm.ProviderChatGPT || choice.Model != "gpt-5-codex" {
		t.Errorf("defaultChoice = %+v", choice)
	}

	rec, err := a.pb.FindFirstRecordByData("ai_settings", "key", aiDefaultSettingKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.pb.Delete(rec); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LISTPOCKET_AI_MODEL", "env-model")
	// With no Settings → AI default, chat uses the AI builder's model on the API
	// key, exactly like a side-panel turn (runAssistant).
	if err := a.upsertTypedSettingsRecord("ai_builder", `{"model":"gpt-5.4-nano","timeout_seconds":300}`); err != nil {
		t.Fatal(err)
	}
	choice = aiChoice{}
	if err := store.LoadSetting(ctx, catalog.Scope{}, "defaultChoice", &choice); err != nil {
		t.Fatal(err)
	}
	if choice.Provider != llm.ProviderAPI || choice.Model != "gpt-5.4-nano" {
		t.Errorf("builder fallback = %+v", choice)
	}

	var title struct {
		Mode  string `json:"mode"`
		Model string `json:"model"`
	}
	if err := store.LoadSetting(ctx, catalog.Scope{}, "titleChoice", &title); !errors.Is(err, catalog.ErrSettingNotFound) {
		t.Errorf("unknown setting must be ErrSettingNotFound, got %v", err)
	}
}

func TestChatProviderStoreRouter(t *testing.T) {
	a := chatProvidersTestApp(t)
	t.Setenv("LISTPOCKET_AI_API_KEY", "sk-test")
	t.Setenv("LISTPOCKET_AI_MODEL", "gpt-x")
	t.Setenv("LISTPOCKET_CHATGPT_BASE_URL", "")
	cacheModels(a, llm.ProviderAPI, cachedAIModels{models: []aiModel{
		{ID: "gpt-x", Name: "GPT X"},
		{ID: "gpt-y", Name: "GPT Y"},
	}})
	cacheModels(a, llm.ProviderChatGPT, cachedAIModels{models: []aiModel{
		{ID: "codex-1", Name: "Codex One"},
	}})
	if err := a.chatgpt.store.Save(signedInCreds()); err != nil {
		t.Fatal(err)
	}
	if err := a.setAIDefaults(aiChoice{Provider: llm.ProviderAPI, Model: "gpt-x"}); err != nil {
		t.Fatal(err)
	}

	router, err := catalog.NewRouter(context.Background(), catalog.RouterOptions{
		Scope:       catalog.Scope{Namespace: "listpocket", Mount: "/mcp"},
		Store:       &chatProviderStore{app: a},
		TitleChoice: "first-message",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	choices, err := router.Choices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, choice := range choices {
		ids[choice.ID] = true
	}
	for _, want := range []string{"api::gpt-x", "api::gpt-y", "chatgpt::codex-1"} {
		if !ids[want] {
			t.Errorf("missing choice %q in %+v", want, choices)
		}
	}
	dflt, err := router.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if dflt.ID != "api::gpt-x" {
		t.Errorf("default choice = %q", dflt.ID)
	}
	// LoadSetting must not answer titleChoice: a nil answer would turn titles off.
	if got := router.TitleChoice(); got != "first-message" {
		t.Errorf("title choice = %q, want first-message", got)
	}
}

func TestChatProviderStoreWritesAreHostOwned(t *testing.T) {
	a := chatProvidersTestApp(t)
	store := &chatProviderStore{app: a}
	ctx := context.Background()
	scope := catalog.Scope{}

	for name, err := range map[string]error{
		"SaveProvider":     store.SaveProvider(ctx, scope, catalog.Provider{}),
		"DeleteProvider":   store.DeleteProvider(ctx, scope, "api"),
		"SaveCredentials":  store.SaveCredentials(ctx, scope, "chatgpt", nil),
		"ClearCredentials": store.ClearCredentials(ctx, scope, "chatgpt"),
		"SaveSetting":      store.SaveSetting(ctx, scope, "defaultChoice", nil),
	} {
		if !errors.Is(err, errChatSettingsHostOwned) {
			t.Errorf("%s: expected the host-owned error, got %v", name, err)
		}
	}

	creds, err := store.LoadCredentials(ctx, scope, "chatgpt")
	if creds != nil || err != nil {
		t.Errorf("LoadCredentials should be unused: %+v %v", creds, err)
	}
}

// A cancelled reload must return before it reads Settings → AI (or the ChatGPT
// store), because shutdownApp may already have reset PocketBase.
func TestChatProviderStoreHonoursCancelledContext(t *testing.T) {
	a := chatProvidersTestApp(t)
	t.Setenv("LISTPOCKET_AI_API_KEY", "sk-test")
	store := &chatProviderStore{app: a}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	scope := catalog.Scope{}

	if _, err := store.ListProviders(ctx, scope); !errors.Is(err, context.Canceled) {
		t.Errorf("ListProviders: %v", err)
	}
	if _, err := store.GetProvider(ctx, scope, llm.ProviderAPI); !errors.Is(err, context.Canceled) {
		t.Errorf("GetProvider: %v", err)
	}
	var choice aiChoice
	if err := store.LoadSetting(ctx, scope, "defaultChoice", &choice); !errors.Is(err, context.Canceled) {
		t.Errorf("LoadSetting: %v", err)
	}
	if _, err := store.HostID(ctx, scope); !errors.Is(err, context.Canceled) {
		t.Errorf("HostID: %v", err)
	}
	if _, err := store.CodexToken(ctx, scope, llm.ProviderChatGPT, false); !errors.Is(err, context.Canceled) {
		t.Errorf("CodexToken: %v", err)
	}
	if store.CodexSignedIn(ctx, scope, llm.ProviderChatGPT) {
		t.Error("CodexSignedIn answered from a cancelled context")
	}
	// Nothing above touched the database: the app is still bootstrapped and a
	// live context still works.
	live := context.Background()
	if _, err := store.ListProviders(live, scope); err != nil {
		t.Errorf("live ListProviders: %v", err)
	}
}

// A reload stuck on an unanswering /models must be cancelled and joined by
// shutdownApp, before anything else is torn down, and it must not start
// another build afterwards.
func TestShutdownAppCancelsAndJoinsChatReload(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	// The handler reports the request's context and then never answers: only
	// cancellation can end the fetch.
	requested := make(chan context.Context, 1)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requested <- r.Context():
		default:
		}
		<-r.Context().Done()
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	a := chatProvidersTestApp(t)
	t.Setenv("LISTPOCKET_AI_API_KEY", "sk-test")
	t.Setenv("LISTPOCKET_AI_BASE_URL", "http://"+listener.Addr().String()+"/v1")
	t.Setenv("LISTPOCKET_CHAT_BACKEND", "module")
	a.plugins = &plugins.Manager{}
	// No cached models: EnableChat's first snapshot comes from the cache-only
	// path, and its background reload is the one that blocks.
	if err := a.setAIDefaults(aiChoice{Provider: llm.ProviderAPI, Model: "fake-model"}); err != nil {
		t.Fatal(err)
	}

	router, err := apis.NewRouter(a.pb)
	if err != nil {
		t.Fatal(err)
	}
	a.mountMCP(&pbcore.ServeEvent{App: a.pb, Router: router})
	if _, ok := a.mcp.ChatRouter(); !ok {
		t.Fatal("chat wasn't enabled")
	}

	var fetchCtx context.Context
	select {
	case fetchCtx = <-requested:
	case <-time.After(5 * time.Second):
		t.Fatal("the background reload never requested /models")
	}

	start := time.Now()
	done := make(chan struct{})
	go func() {
		shutdownApp(a)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("shutdownApp waited %s on a cancelled reload", time.Since(start))
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("shutdownApp took %s", elapsed)
	}

	// The fetch itself was cancelled, and its goroutine is gone: the scope's
	// WaitGroup is idle, so nothing can still reach the database.
	select {
	case <-fetchCtx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the /models request wasn't cancelled by shutdown")
	}
	idle := make(chan struct{})
	go func() {
		a.chatReloads.wg.Wait()
		close(idle)
	}()
	select {
	case <-idle:
	case <-time.After(2 * time.Second):
		t.Fatal("a chat reload goroutine outlived shutdown")
	}
	if _, ok := a.chatReloads.schedule(); ok {
		t.Fatal("a chat reload can still be scheduled after shutdown")
	}
	a.goReloadChatProviders() // must be a no-op, not a database read
}
