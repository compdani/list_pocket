package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/compdani/list_pocket/internal/apperr"
	"github.com/compdani/list_pocket/internal/flow/chatgpt"
	"github.com/compdani/list_pocket/internal/flow/llm"
	"github.com/pocketbase/pocketbase"
	pbcore "github.com/pocketbase/pocketbase/core"
)

const (
	chatgptLoginTimeout = 10 * time.Minute
	aiModelsCacheFor    = 10 * time.Minute
	aiDefaultSettingKey = "default_ai"

	chatgptHostingNote = "OpenAI offers ChatGPT plan usage for open-source and locally hosted apps. For a paid or remotely hosted deployment, they ask you to apply through their interest form."
)

// aiChoice is a provider and model picked for a request.
type aiChoice struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

func (c *aiChoice) validate() error {
	if c == nil {
		return nil
	}
	if c.Provider != llm.ProviderAPI && c.Provider != llm.ProviderChatGPT {
		return apperr.BadRequest("unknown AI provider")
	}
	if c.Model == "" || len(c.Model) > 200 {
		return apperr.BadRequest("choose a model")
	}
	return nil
}

// chatgptService keeps one signed-in ChatGPT account and the sign-in in progress.
type chatgptService struct {
	pb    *pocketbase.PocketBase
	store *chatgpt.Store

	mu      sync.Mutex
	pending bool
	lastErr string
	flow    *chatgptFlow
	cancel  context.CancelFunc

	modelsMu sync.Mutex
	models   map[string]cachedAIModels
}

type chatgptFlow struct {
	lb *chatgpt.Loopback
	p  *chatgpt.Pending
}

type cachedAIModels struct {
	models []aiModel
	err    string
	at     time.Time
}

type aiModel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func newChatGPTService(pb *pocketbase.PocketBase) *chatgptService {
	return &chatgptService{
		pb:     pb,
		store:  &chatgpt.Store{App: pb, OAuth: &chatgpt.OAuth{}},
		models: map[string]cachedAIModels{},
	}
}

// aiDefaults returns the provider and model chosen in Settings → AI, if any.
func (a *App) aiDefaults() (string, string) {
	if a == nil || a.pb == nil {
		return "", ""
	}
	rec, err := a.pb.FindFirstRecordByData("ai_settings", "key", aiDefaultSettingKey)
	if err != nil {
		return "", ""
	}
	var out aiChoice
	if err := rec.UnmarshalJSONField("value", &out); err != nil {
		return "", ""
	}
	return out.Provider, out.Model
}

func (a *App) setAIDefaults(c aiChoice) error {
	rec, err := a.pb.FindFirstRecordByData("ai_settings", "key", aiDefaultSettingKey)
	if err != nil {
		col, err := a.pb.FindCollectionByNameOrId("ai_settings")
		if err != nil {
			return err
		}
		rec = pbcore.NewRecord(col)
		rec.Set("key", aiDefaultSettingKey)
	}
	rec.Set("value", c)
	return a.pb.Save(rec)
}

// aiResolver is how every AI feature finds its provider.
func (a *App) aiResolver() *llm.Resolver {
	r := &llm.Resolver{Defaults: a.aiDefaults}
	if a != nil && a.chatgpt != nil {
		r.Tokens = a.chatgpt.store.Token
	}
	return r
}

func (c *chatgptService) status() map[string]any {
	c.mu.Lock()
	pending, lastErr := c.pending, c.lastErr
	c.mu.Unlock()
	return map[string]any{"status": c.store.Status(), "pending": pending, "error": lastErr, "note": chatgptHostingNote}
}

// GetAISettings returns which providers are set up and the default. It never includes
// tokens or keys.
func (a *App) GetAISettings(re *pbcore.RequestEvent) error {
	provider, model := a.aiDefaults()
	out := a.chatgpt.status()
	out["default"] = aiChoice{Provider: provider, Model: model}
	out["api_configured"] = strings.TrimSpace(os.Getenv("LISTPOCKET_AI_API_KEY")) != ""
	return okJSON(re, out)
}

// SetAIDefault chooses what every AI feature uses unless told otherwise. An empty
// provider goes back to the API key from the environment.
func (a *App) SetAIDefault(re *pbcore.RequestEvent) error {
	var in aiChoice
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	if in.Provider != "" || in.Model != "" {
		if err := in.validate(); err != nil {
			return err
		}
	}
	if err := a.setAIDefaults(in); err != nil {
		return apperr.Internal(err.Error())
	}
	return okJSON(re, in)
}

// ChatGPTLogin starts sign-in. The callback arrives on this server's loopback address,
// which works when the browser is on the same machine. Otherwise the person pastes the
// address they land on into ChatGPTCallback.
func (a *App) ChatGPTLogin(re *pbcore.RequestEvent) error {
	c := a.chatgpt
	c.mu.Lock()
	if c.pending {
		c.mu.Unlock()
		return apperr.BadRequest("a sign-in is already in progress")
	}
	c.pending, c.lastErr = true, ""
	c.mu.Unlock()

	urlCh := make(chan string, 1)
	go func() {
		cred, err := c.signIn(context.Background(), func(u string) { urlCh <- u })
		if err == nil {
			err = c.store.Save(cred)
		}
		c.mu.Lock()
		c.pending = false
		if err != nil {
			c.lastErr = err.Error()
			a.log.Printf("ChatGPT sign-in failed: %v", err)
		}
		c.mu.Unlock()
		c.forgetModels()
	}()
	select {
	case u := <-urlCh:
		return okJSON(re, map[string]any{"auth_url": u, "expires_in": int(chatgptLoginTimeout.Seconds())})
	case <-time.After(5 * time.Second):
		return apperr.Internal("couldn't start the sign-in listener")
	}
}

func (c *chatgptService) signIn(ctx context.Context, open func(string)) (*chatgpt.Credentials, error) {
	lb, err := chatgpt.Listen(0)
	if err != nil {
		return nil, err
	}
	existing, _ := c.store.Load()
	p := c.store.OAuth.Begin(c.store.HostID(), lb.RedirectURI, existing)
	c.mu.Lock()
	c.flow = &chatgptFlow{lb: lb, p: p}
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.flow = nil
		c.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(ctx, chatgptLoginTimeout)
	defer cancel()
	c.mu.Lock()
	c.cancel = cancel
	c.mu.Unlock()
	open(p.AuthURL)
	q, err := lb.Wait(ctx)
	if err != nil {
		return nil, err
	}
	return c.store.OAuth.Complete(ctx, p, q)
}

func (c *chatgptService) deliver(q url.Values) error {
	c.mu.Lock()
	f := c.flow
	c.mu.Unlock()
	if f == nil {
		return errors.New("no sign-in in progress; start again from Settings → AI")
	}
	if q.Get("state") != f.p.State {
		return errors.New("this belongs to a different sign-in attempt; start again")
	}
	if !f.lb.Deliver(q) {
		return errors.New("this sign-in has already finished")
	}
	return nil
}

// ChatGPTCancel gives up on a sign-in in progress, so a closed tab doesn't block another
// attempt until it times out.
func (a *App) ChatGPTCancel(re *pbcore.RequestEvent) error {
	c := a.chatgpt
	c.mu.Lock()
	cancel := c.cancel
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return okJSON(re, true)
}

// ChatGPTCallback finishes a sign-in from a pasted callback address.
func (a *App) ChatGPTCallback(re *pbcore.RequestEvent) error {
	var in struct {
		URL string `json:"url"`
	}
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	u, err := url.Parse(strings.TrimSpace(in.URL))
	if err != nil || u.Query().Get("state") == "" {
		return apperr.BadRequest("paste the full address: it contains code= and state=")
	}
	if err := a.chatgpt.deliver(u.Query()); err != nil {
		return apperr.BadRequest(err.Error())
	}
	return okJSON(re, true)
}

// ChatGPTCallbackPage catches the callback on this server. It's public because the
// browser arrives from OpenAI, and it only acts on the state of a sign-in in progress.
func (a *App) ChatGPTCallbackPage(re *pbcore.RequestEvent) error {
	msg := "Signed in. You can close this tab and return to List Pocket."
	if err := a.chatgpt.deliver(re.Request.URL.Query()); err != nil {
		msg = "Couldn't complete sign-in: " + err.Error()
	} else if v := re.Request.URL.Query().Get("error"); v != "" {
		msg = "Sign-in didn't complete: " + v
	}
	return re.HTML(http.StatusOK, `<!doctype html><meta charset="utf-8"><title>List Pocket</title><body style="font-family:system-ui;padding:40px">`+html.EscapeString(msg)+`</body>`)
}

// ChatGPTImport loads a session exported from another install, for servers a browser
// can't reach.
func (a *App) ChatGPTImport(re *pbcore.RequestEvent) error {
	var cred chatgpt.Credentials
	if err := bindJSON(re, &cred); err != nil {
		return err
	}
	if err := a.chatgpt.store.Import(&cred); err != nil {
		return apperr.BadRequest(err.Error())
	}
	a.chatgpt.forgetModels()
	return okJSON(re, a.chatgpt.store.Status())
}

// ChatGPTLogout revokes the session and forgets it. Anything defaulting to the plan goes
// back to the API key, so it fails on a missing key rather than a signed-out account.
func (a *App) ChatGPTLogout(re *pbcore.RequestEvent) error {
	out := map[string]any{"signed_out": true}
	if cred, err := a.chatgpt.store.Load(); err == nil {
		if err := a.chatgpt.store.OAuth.Revoke(re.Request.Context(), cred); err != nil {
			out["warning"] = "Signed out here, but OpenAI didn't confirm it (" + err.Error() + "). Disconnect the app in ChatGPT → Settings → Security and login."
		}
	}
	if err := a.chatgpt.store.Clear(); err != nil {
		return apperr.Internal(err.Error())
	}
	if provider, _ := a.aiDefaults(); provider == llm.ProviderChatGPT {
		if err := a.setAIDefaults(aiChoice{}); err != nil {
			return apperr.Internal(err.Error())
		}
	}
	a.chatgpt.forgetModels()
	return okJSON(re, out)
}

func (c *chatgptService) forgetModels() {
	c.modelsMu.Lock()
	defer c.modelsMu.Unlock()
	c.models = map[string]cachedAIModels{}
}

func (a *App) aiProviderModels(ctx context.Context, provider string, refresh bool) cachedAIModels {
	c := a.chatgpt
	c.modelsMu.Lock()
	cached, ok := c.models[provider]
	c.modelsMu.Unlock()
	if ok && !refresh && time.Since(cached.at) < aiModelsCacheFor {
		return cached
	}
	out := cachedAIModels{at: time.Now()}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	ep, err := a.aiResolver().Endpoint(provider)
	switch {
	case err != nil:
		out.err = err.Error()
	case provider == llm.ProviderChatGPT:
		list, err := chatgpt.ListModels(ctx, nil, ep.BaseURL, ep.APIKey)
		if err != nil {
			out.err = err.Error()
		}
		for _, m := range list {
			out.models = append(out.models, aiModel{ID: m.Slug, Name: m.DisplayName})
		}
	default:
		out.models, err = listAPIModels(ctx, ep)
		if err != nil {
			out.err = err.Error()
		}
	}
	c.modelsMu.Lock()
	c.models[provider] = out
	c.modelsMu.Unlock()
	return out
}

func listAPIModels(ctx context.Context, ep llm.Endpoint) ([]aiModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ep.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+ep.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the provider returned HTTP %d", resp.StatusCode)
	}
	var body struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	out := make([]aiModel, 0, len(body.Data))
	for _, m := range body.Data {
		name := m.Name
		if name == "" {
			name = m.ID
		}
		out = append(out, aiModel{ID: m.ID, Name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// GetAgentModels lists what can be picked for a conversation, by provider.
func (a *App) GetAgentModels(re *pbcore.RequestEvent) error {
	refresh := queryParam(re, "refresh") == "1"
	provider, model := a.aiResolver().DefaultChoice()
	providers := []map[string]any{}
	for _, p := range []struct{ name, label string }{{llm.ProviderChatGPT, "ChatGPT plan"}, {llm.ProviderAPI, "API key"}} {
		list := a.aiProviderModels(re.Request.Context(), p.name, refresh)
		models := list.models
		if models == nil {
			models = []aiModel{}
		}
		providers = append(providers, map[string]any{"name": p.name, "label": p.label, "models": models, "error": list.err})
	}
	return okJSON(re, map[string]any{"default": aiChoice{Provider: provider, Model: model}, "providers": providers})
}

// aiBuilderRouter sends campaign generation through the ChatGPT plan when it's the
// default, and otherwise to the provider chosen at boot.
type aiBuilderRouter struct {
	app      *App
	fallback aiBuilderProvider
}

func (r *aiBuilderRouter) Generate(ctx context.Context, req aiBuilderGenerateReq) (aiBuilderResult, error) {
	resolver := r.app.aiResolver()
	provider, _ := resolver.DefaultChoice()
	if provider != llm.ProviderChatGPT {
		return r.fallback.Generate(ctx, req)
	}
	settings, endpoint, err := resolver.Resolve()
	if err != nil {
		return aiBuilderResult{}, err
	}
	system := strings.TrimSpace(req.SystemPrompt)
	if system == "" {
		system = defaultAIBuilderSystemPrompt
	}
	user := llm.Message{Role: "user", Content: toJSONString(aiBuilderUserPayload(req))}
	for _, att := range req.StatusAttachments {
		user.Attachments = append(user.Attachments, llm.Attachment{Filename: att.Filename, Mime: att.MimeType, Data: strings.TrimSpace(att.Data)})
	}
	resp, err := (&llm.Client{HTTP: http.DefaultClient}).Complete(ctx, endpoint, llm.Request{
		Model: settings.Model, Settings: settings, Stream: true,
		Messages: []llm.Message{{Role: "system", Content: system}, user},
		Extra:    map[string]any{"response_format": aiBuilderOpenAIJSONResponseFormat()},
	}, nil)
	if err != nil {
		return aiBuilderResult{}, assistantProviderError(err)
	}
	var result aiBuilderResult
	if err := json.Unmarshal([]byte(resp.Message.Content), &result); err != nil {
		return aiBuilderResult{}, fmt.Errorf("invalid provider JSON: %w", err)
	}
	return result, nil
}
