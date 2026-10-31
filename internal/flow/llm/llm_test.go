package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestMergeLayersAndModelRoles(t *testing.T) {
	global := map[string]any{
		"provider":    "openrouter",
		"model":       "big",
		"models":      map[string]any{"research": "big", "fast": "small"},
		"temperature": 0.2,
		"search":      map[string]any{"engine": "exa", "max_results": 5},
		"budget":      map[string]any{"max_usd_per_run": 0.5},
	}
	workflow := map[string]any{"temperature": 0.7, "search": map[string]any{"max_results": 8}, "models": map[string]any{"fast": "tiny"}}
	node := map[string]any{"model": "@fast", "top_p": nil}
	s, err := Merge(global, workflow, node)
	if err != nil {
		t.Fatal(err)
	}
	if s.Model != "tiny" {
		t.Errorf("model = %q, want tiny (workflow's fast role)", s.Model)
	}
	if *s.Temperature != 0.7 {
		t.Errorf("temperature = %v", *s.Temperature)
	}
	if s.Search.Engine != "exa" || s.Search.MaxResults != 8 {
		t.Errorf("search = %+v, want nested merge", s.Search)
	}
	if s.Budget.MaxUSDPerRun != 0.5 || s.Search.Mode != "openrouter" || s.Search.Strategy != "server_tool" {
		t.Errorf("defaults/budget not kept: %+v", s)
	}
	if _, err := Merge(map[string]any{"model": "@missing"}); err == nil {
		t.Error("undefined model role should error")
	}
}

func sse(chunks ...string) string {
	var b strings.Builder
	b.WriteString(": OPENROUTER PROCESSING\n\n")
	for _, c := range chunks {
		b.WriteString("data: " + c + "\n\n")
	}
	b.WriteString("data: [DONE]\n\n")
	return b.String()
}

func TestStreamingToolCallsCitationsUsage(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("bad request %s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sse(
			`{"model":"m1","choices":[{"delta":{"content":"Hel"}}]}`,
			`{"choices":[{"delta":{"content":"lo","annotations":[{"type":"url_citation","url_citation":{"url":"https://a.test/x","title":"A"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"web__fetch","arguments":"{\"url\":"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"https://a.test\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"cost":0.0012,"prompt_tokens_details":{"cached_tokens":7,"cache_write_tokens":3},"server_tool_use_details":{"web_search_requests":2}}}`,
		))
	}))
	defer srv.Close()

	ep := Endpoint{BaseURL: srv.URL + "/api/v1", APIKey: "k"}
	var deltas []string
	temp := 0.1
	resp, err := NewClient().Complete(context.Background(), ep, Request{
		Model: "m", Messages: []Message{{Role: "user", Content: "hi"}}, Stream: true,
		Tools:    []map[string]any{{"type": "openrouter:web_search"}},
		Settings: Settings{Temperature: &temp, ExtraBody: map[string]any{"transforms": []string{}}},
	}, func(d string) { deltas = append(deltas, d) })
	if err != nil {
		t.Fatal(err)
	}
	if resp.Message.Content != "Hello" || strings.Join(deltas, "") != "Hello" {
		t.Errorf("content = %q deltas=%v", resp.Message.Content, deltas)
	}
	if len(resp.Message.ToolCalls) != 1 || resp.Message.ToolCalls[0].Function.Arguments != `{"url":"https://a.test"}` {
		t.Errorf("tool calls = %+v", resp.Message.ToolCalls)
	}
	if len(resp.Citations) != 1 || resp.Citations[0].URL != "https://a.test/x" {
		t.Errorf("citations = %+v", resp.Citations)
	}
	if resp.Usage.CachedTokens != 7 || resp.Usage.CacheWriteTokens != 3 {
		t.Errorf("cache usage = %+v", resp.Usage)
	}
	if _, ok := gotBody["cache_control"]; ok {
		t.Error("cache_control should only be sent to OpenRouter")
	}
	if resp.Usage.WebSearchRequests != 2 || resp.Usage.Cost == nil || *resp.Usage.Cost != 0.0012 {
		t.Errorf("usage = %+v", resp.Usage)
	}
	if resp.FinishReason != "tool_calls" || resp.Model != "m1" {
		t.Errorf("finish=%q model=%q", resp.FinishReason, resp.Model)
	}
	if gotBody["stream"] != true || gotBody["temperature"] != 0.1 || gotBody["transforms"] == nil {
		t.Errorf("body = %v", gotBody)
	}
	if _, ok := gotBody["usage"]; ok {
		t.Error("usage accounting flag should only be sent to OpenRouter")
	}
}

func TestRetriesOn5xxThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			http.Error(w, `{"error":"overloaded"}`, http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1}}`)
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), Backoff: func(int) time.Duration { return time.Millisecond }}
	resp, err := c.Complete(context.Background(), Endpoint{BaseURL: srv.URL}, Request{Model: "m", Settings: Settings{Retries: 2}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Message.Content != "ok" || calls.Load() != 3 {
		t.Errorf("content=%q calls=%d", resp.Message.Content, calls.Load())
	}

	calls.Store(-10)
	_, err = c.Complete(context.Background(), Endpoint{BaseURL: srv.URL}, Request{Model: "m", Settings: Settings{Retries: 1}}, nil)
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Errorf("want 503 after retries exhausted, got %v", err)
	}
}

func TestOpenRouterBodyAndCost(t *testing.T) {
	ep := Endpoint{BaseURL: "https://openrouter.ai/api/v1", Prices: map[string]Price{"m": {Prompt: 3, Completion: 15}}}
	s, _ := Merge(map[string]any{"model": "m"})
	raw, err := buildBody(ep, Request{Model: "m", Settings: s})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"usage":{"include":true}`) {
		t.Errorf("openrouter body missing usage include: %s", raw)
	}
	if !strings.Contains(string(raw), `"cache_control":{"type":"ephemeral"}`) {
		t.Errorf("openrouter body missing default 5m cache_control: %s", raw)
	}
	for mode, want := range map[string]string{"1h": `"ttl":"1h"`, "off": ""} {
		raw, _ := buildBody(ep, Request{Model: "m", Settings: Settings{PromptCache: mode}})
		if want == "" && strings.Contains(string(raw), "cache_control") {
			t.Errorf("prompt_cache=off still sent cache_control: %s", raw)
		}
		if want != "" && !strings.Contains(string(raw), want) {
			t.Errorf("prompt_cache=%s body = %s", mode, raw)
		}
	}
	if got := ep.Cost("m", Usage{PromptTokens: 1_000_000, CompletionTokens: 100_000}); got != 4.5 {
		t.Errorf("computed cost = %v, want 4.5", got)
	}
	reported := 0.01
	if got := ep.Cost("m", Usage{Cost: &reported}); got != 0.01 {
		t.Errorf("reported cost should win, got %v", got)
	}
}
