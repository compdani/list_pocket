package llm

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func testResolver(env map[string]string) *Resolver {
	return &Resolver{Getenv: func(k string) string { return env[k] }}
}

func TestResolveUsesAPIKeyByDefault(t *testing.T) {
	r := testResolver(map[string]string{"LISTPOCKET_AI_API_KEY": "k", "LISTPOCKET_AI_MODEL": "m1", "LISTPOCKET_AI_FAST_MODEL": "m2"})
	s, ep, err := r.Resolve(map[string]any{"model": "@fast"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Model != "m2" || ep.APIKey != "k" || ep.Kind != "" {
		t.Fatalf("got %+v %+v", s, ep)
	}
}

func TestResolveExplainsMissingSetup(t *testing.T) {
	if _, _, err := testResolver(nil).Resolve(); err == nil || !strings.Contains(err.Error(), "Settings → AI") {
		t.Fatalf("got %v", err)
	}
}

func TestResolveUsesPlanWhenChosen(t *testing.T) {
	r := testResolver(map[string]string{"LISTPOCKET_AI_MODEL": "api-model"})
	r.Tokens = func(context.Context) (string, error) { return "plan-token", nil }
	r.Defaults = func() (string, string) { return ProviderChatGPT, "gpt-codex" }

	s, ep, err := r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if s.Model != "gpt-codex" || ep.Kind != KindResponses || ep.APIKey != "plan-token" || ep.BaseURL != ChatGPTBaseURL {
		t.Fatalf("got %+v %+v", s, ep)
	}

	// A plan with no model chosen says so, rather than sending an API model's name.
	r.Defaults = func() (string, string) { return ProviderChatGPT, "" }
	if _, _, err := r.Resolve(); err == nil || !strings.Contains(err.Error(), "no model") {
		t.Fatalf("got %v", err)
	}

	// One request can pick the plan even when the API key is the default.
	r.Defaults = nil
	if _, ep, err = r.Resolve(map[string]any{"provider": ProviderChatGPT, "model": "gpt-codex"}); err != nil || ep.Kind != KindResponses {
		t.Fatalf("got %+v, %v", ep, err)
	}
}

func TestResolvePassesOnSignedOutPlan(t *testing.T) {
	r := testResolver(nil)
	r.Tokens = func(context.Context) (string, error) { return "", errors.New("not signed in") }
	if _, _, err := r.Resolve(map[string]any{"provider": ProviderChatGPT, "model": "m"}); err == nil || err.Error() != "not signed in" {
		t.Fatalf("got %v", err)
	}
	if _, _, err := r.Resolve(map[string]any{"provider": "other", "model": "m"}); err == nil {
		t.Fatal("expected an unknown provider to be refused")
	}
}

func TestAttachmentsOnBothRoutes(t *testing.T) {
	msg := Message{Role: "user", Content: "look", Attachments: []Attachment{
		{Filename: "a.png", Mime: "image/png", Data: "AA"},
		{Filename: "b.pdf", Mime: "application/pdf", Data: "BB"},
	}}
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"type":"image_url"`, `data:image/png;base64,AA`, `"file_data":"data:application/pdf;base64,BB"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("chat body missing %s: %s", want, raw)
		}
	}
	raw, err = buildResponsesBody(Request{Model: "m", Messages: []Message{msg}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"type":"input_image"`, `"image_url":"data:image/png;base64,AA"`, `"type":"input_file"`, `"filename":"b.pdf"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("responses body missing %s: %s", want, raw)
		}
	}
	// A plain message keeps its usual shape.
	raw, _ = json.Marshal(Message{Role: "user", Content: "hi"})
	if string(raw) != `{"role":"user","content":"hi"}` {
		t.Errorf("plain message changed: %s", raw)
	}
}
