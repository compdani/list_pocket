package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildResponsesBody(t *testing.T) {
	temp := 0.3
	par := true
	raw, err := buildResponsesBody(Request{
		Model: "gpt-x",
		Messages: []Message{
			{Role: "system", Content: "Be brief."},
			{Role: "user", Content: "Find leads"},
			{Role: "assistant", Content: "Looking.", ToolCalls: []ToolCall{{ID: "c1", Type: "function", Function: FunctionCall{Name: "web__fetch", Arguments: `{"url":"x"}`}}}},
			{Role: "tool", ToolCallID: "c1", Content: `{"text":"page"}`},
		},
		Tools: []map[string]any{
			FunctionTool("web__fetch", "Fetch", map[string]any{"type": "object"}),
			{"type": "openrouter:web_search"},
			{"type": "web_search"},
		},
		Settings: Settings{Temperature: &temp, MaxTokens: 100, ToolChoice: "auto", ParallelToolCalls: &par, ReasoningEffort: "low"},
		Extra:    map[string]any{"tool_choice": map[string]any{"type": "function", "function": map[string]any{"name": "submit_result"}}, "usage": map[string]any{"include": true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var b map[string]any
	_ = json.Unmarshal(raw, &b)
	if b["store"] != false || b["stream"] != true || b["instructions"] != "Be brief." || b["model"] != "gpt-x" {
		t.Errorf("basics: %s", raw)
	}
	for _, banned := range []string{"temperature", "max_tokens", "max_output_tokens", "top_p", "messages", "usage"} {
		if _, ok := b[banned]; ok {
			t.Errorf("%s must not be sent on the plan route", banned)
		}
	}
	input := b["input"].([]any)
	types := []string{}
	for _, it := range input {
		m := it.(map[string]any)
		types = append(types, fmt.Sprint(m["type"], ":", m["role"]))
	}
	if strings.Join(types, ",") != "message:user,message:assistant,function_call:<nil>,function_call_output:<nil>" {
		t.Errorf("input items: %v", types)
	}
	if fc := input[2].(map[string]any); fc["call_id"] != "c1" || fc["name"] != "web__fetch" {
		t.Errorf("function_call: %v", fc)
	}
	tools := b["tools"].([]any)
	if len(tools) != 2 || tools[0].(map[string]any)["name"] != "web__fetch" || tools[1].(map[string]any)["type"] != "web_search" {
		t.Errorf("tools: %v", tools)
	}
	if tc := b["tool_choice"].(map[string]any); tc["name"] != "submit_result" || tc["type"] != "function" {
		t.Errorf("forced tool_choice not translated: %v", b["tool_choice"])
	}
	if b["reasoning"].(map[string]any)["effort"] != "low" {
		t.Error("reasoning effort")
	}
}

func sseEvents(events ...string) string {
	var sb strings.Builder
	for _, e := range events {
		var m map[string]any
		_ = json.Unmarshal([]byte(e), &m)
		var compact bytes.Buffer
		_ = json.Compact(&compact, []byte(e)) // one SSE data line per event
		fmt.Fprintf(&sb, "event: %s\ndata: %s\n\n", m["type"], compact.String())
	}
	return sb.String()
}

func TestResponsesStream(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseEvents(
			`{"type":"response.created","response":{"status":"in_progress"}}`,
			`{"type":"response.output_text.delta","delta":"Hel"}`,
			`{"type":"response.output_text.delta","delta":"lo"}`,
			`{"type":"response.completed","response":{"model":"gpt-x","status":"completed","output":[
				{"type":"web_search_call","id":"ws1","status":"completed"},
				{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello","annotations":[{"type":"url_citation","url":"https://a.test","title":"A"}]}]},
				{"type":"function_call","id":"fc1","call_id":"call_9","name":"submit_result","arguments":"{\"leads\":[]}"}
			],"usage":{"input_tokens":120,"output_tokens":30,"total_tokens":150,"input_tokens_details":{"cached_tokens":100}}}}`,
		))
	}))
	defer srv.Close()

	var deltas []string
	ep := Endpoint{BaseURL: srv.URL + "/v1", APIKey: "plan-token", Kind: KindResponses}
	resp, err := NewClient().Complete(context.Background(), ep, Request{Model: "gpt-x", Messages: []Message{{Role: "user", Content: "hi"}}}, func(d string) { deltas = append(deltas, d) })
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/responses" || gotAuth != "Bearer plan-token" || gotBody["stream"] != true {
		t.Errorf("request: %s %s %v", gotPath, gotAuth, gotBody["stream"])
	}
	if resp.Message.Content != "Hello" || strings.Join(deltas, "") != "Hello" {
		t.Errorf("text: %q %v", resp.Message.Content, deltas)
	}
	if len(resp.Message.ToolCalls) != 1 || resp.Message.ToolCalls[0].ID != "call_9" || resp.Message.ToolCalls[0].Function.Name != "submit_result" {
		t.Errorf("tool calls: %+v", resp.Message.ToolCalls)
	}
	if len(resp.Citations) != 1 || resp.Usage.WebSearchRequests != 1 || resp.Usage.PromptTokens != 120 || resp.Usage.CachedTokens != 100 {
		t.Errorf("citations/usage: %+v %+v", resp.Citations, resp.Usage)
	}
	if ep.Cost("gpt-x", resp.Usage) != 0 {
		t.Error("plan usage has no per-token cost")
	}
}

// The ChatGPT-plan route (store:false) streams items via output_item.done and
// sends response.completed with an empty output.
func TestResponsesStreamItemsWithoutCompletedOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, sseEvents(
			`{"type":"response.output_item.done","item":{"type":"web_search_call","id":"ws1","status":"completed"}}`,
			`{"type":"response.output_item.done","item":{"type":"function_call","call_id":"call_1","name":"ping","arguments":"{\"text\":\"x\"}"}}`,
			`{"type":"response.completed","response":{"model":"m","status":"completed","output":[],"usage":{"input_tokens":10,"output_tokens":5}}}`,
		))
	}))
	defer srv.Close()
	resp, err := NewClient().Complete(context.Background(), Endpoint{BaseURL: srv.URL, Kind: KindResponses}, Request{Model: "m"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage.WebSearchRequests != 1 || len(resp.Message.ToolCalls) != 1 || resp.Message.ToolCalls[0].Function.Name != "ping" {
		t.Errorf("streamed items not used: %+v %+v", resp.Usage, resp.Message.ToolCalls)
	}
}

func TestResponsesStreamFailures(t *testing.T) {
	for name, body := range map[string]string{
		"usage limit": sseEvents(`{"type":"response.output_text.delta","delta":"x"}`,
			`{"type":"response.failed","response":{"status":"failed","error":{"code":"subscription_sharing_usage_limit_exceeded","message":"Weekly limit reached"}}}`),
		"cut off": sseEvents(`{"type":"response.output_text.delta","delta":"x"}`),
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		_, err := NewClient().Complete(context.Background(), Endpoint{BaseURL: srv.URL, Kind: KindResponses}, Request{Model: "m"}, nil)
		srv.Close()
		if err == nil {
			t.Errorf("%s: expected error", name)
		}
		if name == "usage limit" && !strings.Contains(err.Error(), "subscription_sharing_usage_limit_exceeded") {
			t.Errorf("usage limit error should carry the code: %v", err)
		}
	}
}
