package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/compdani/list_pocket/internal/auth"
	"github.com/compdani/list_pocket/internal/flow/llm"
	"github.com/compdani/list_pocket/internal/plugins"
	pbcore "github.com/pocketbase/pocketbase/core"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestAssistantRejectsIncompleteProviderReply(t *testing.T) {
	t.Setenv("LISTPOCKET_AI_API_KEY", "fixture-key")
	original := http.DefaultClient
	defer func() { http.DefaultClient = original }()
	for name, stream := range map[string]string{
		"ended early":  `data: {"choices":[{"delta":{"content":"partial"}}]}` + "\n",
		"stream error": `data: {"error":{"message":"failure"}}` + "\n",
		"cut short":    `data: {"choices":[{"finish_reason":"length"}]}` + "\n",
	} {
		http.DefaultClient = &http.Client{Transport: assistantRoundTrip(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(stream)), Header: http.Header{}}, nil
		})}
		app := &App{auth: &auth.Auth{}}
		re := &pbcore.RequestEvent{}
		re.Request = httptest.NewRequest("POST", "/", nil)
		re.Set(auth.UserHTTPCtxKey, auth.User{})
		turn := &assistantTurn{proposals: map[string]*assistantProposal{}}
		err := app.runAssistant(context.Background(), re, turn, assistantRequest{Messages: []assistantMessage{{Role: "user", Content: "hi"}}, Context: json.RawMessage(`{}`)}, "m")
		if err == nil {
			t.Errorf("%s: expected a failure", name)
		}
	}
}
func TestAssistantEventsHaveMonotonicReplayIDs(t *testing.T) {
	turn := &assistantTurn{}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); turn.emit("text_delta", "text") }()
	}
	wg.Wait()
	for i, event := range turn.events {
		if event.ID != i+1 {
			t.Fatal("unordered event IDs")
		}
	}
}
func TestAssistantTurnOwnership(t *testing.T) {
	collection := pbcore.NewBaseCollection("users")
	record := pbcore.NewRecord(collection)
	record.Id = "owner"
	re := &pbcore.RequestEvent{Auth: record}
	re.Request = httptest.NewRequest("GET", "/", nil)
	re.Request.SetPathValue("id", "ownership-test")
	assistantState.Lock()
	assistantState.turns["ownership-test"] = &assistantTurn{owner: "someone-else"}
	assistantState.Unlock()
	defer func() { assistantState.Lock(); delete(assistantState.turns, "ownership-test"); assistantState.Unlock() }()
	if _, err := ownedAssistantTurn(re); err == nil {
		t.Fatal("cross-user turn access allowed")
	}
	assistantState.Lock()
	assistantState.turns["ownership-test"].owner = "owner"
	assistantState.Unlock()
	if _, err := ownedAssistantTurn(re); err != nil {
		t.Fatal(err)
	}
}
func TestAssistantProposalDecisionIsIdempotent(t *testing.T) {
	collection := pbcore.NewBaseCollection("users")
	record := pbcore.NewRecord(collection)
	record.Id = "owner"
	turn := &assistantTurn{owner: "owner", proposals: map[string]*assistantProposal{"p": {ID: "p", Tool: "propose_editor_patch", Status: "pending"}}}
	assistantState.Lock()
	assistantState.turns["decision-test"] = turn
	assistantState.Unlock()
	defer func() { assistantState.Lock(); delete(assistantState.turns, "decision-test"); assistantState.Unlock() }()
	for i := 0; i < 2; i++ {
		re := &pbcore.RequestEvent{Auth: record}
		re.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"proposal":"p","approve":true}`))
		re.Request.Header.Set("Content-Type", "application/json")
		re.Request.SetPathValue("id", "decision-test")
		re.Response = httptest.NewRecorder()
		if err := (&App{}).DecideAssistantProposal(re); err != nil {
			t.Fatal(err)
		}
	}
	if turn.proposals["p"].Status != "approved" {
		t.Fatal(fmt.Sprint(turn.proposals["p"]))
	}
}

func TestAssistantToolsEnforcePermissionsAndApproval(t *testing.T) {
	app := &App{auth: &auth.Auth{}}
	re := &pbcore.RequestEvent{}
	re.Request = httptest.NewRequest("POST", "/", nil)
	re.Set(auth.UserHTTPCtxKey, auth.User{})
	if _, err := app.executeAssistantTool(re, "campaigns", json.RawMessage(`{}`), false); err == nil {
		t.Fatal("read without campaign permissions succeeded")
	}
	if _, err := app.executeAssistantTool(re, "create_campaign_draft", json.RawMessage(`{}`), false); err == nil {
		t.Fatal("write without approval succeeded")
	}
	if _, err := app.executeAssistantTool(re, "subscribers", json.RawMessage(`{"query":"sql_query=SELECT+1"}`), false); err == nil {
		t.Fatal("arbitrary SQL argument accepted")
	}
	if _, err := app.executeAssistantTool(re, "publish_workflow", json.RawMessage(`{}`), true); err == nil {
		t.Fatal("unregistered side effect allowed")
	}
}
func TestAssistantCancellationAndReplay(t *testing.T) {
	collection := pbcore.NewBaseCollection("users")
	record := pbcore.NewRecord(collection)
	record.Id = "owner"
	ctx, cancel := context.WithCancel(context.Background())
	turn := &assistantTurn{owner: "owner", cancel: cancel, done: true}
	turn.emit("text_delta", map[string]string{"text": "first"})
	turn.emit("done", nil)
	assistantState.Lock()
	assistantState.turns["replay-test"] = turn
	assistantState.Unlock()
	defer func() { assistantState.Lock(); delete(assistantState.turns, "replay-test"); assistantState.Unlock() }()
	re := &pbcore.RequestEvent{Auth: record}
	re.Request = httptest.NewRequest("GET", "/stream?after=1", nil)
	re.Request.SetPathValue("id", "replay-test")
	rec := httptest.NewRecorder()
	re.Response = rec
	if err := (&App{}).StreamAssistantTurn(re); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rec.Body.String(), "first") || !strings.Contains(rec.Body.String(), `"type":"done"`) {
		t.Fatal("cursor replay duplicated or dropped events")
	}
	if err := (&App{}).CancelAssistantTurn(re); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != context.Canceled {
		t.Fatal("cancel did not reach provider context")
	}
}

type assistantRoundTrip func(*http.Request) (*http.Response, error)

func (f assistantRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
func TestAssistantToolLoopStreamsAndRecoversFromPermissionFailure(t *testing.T) {
	t.Setenv("LISTPOCKET_AI_API_KEY", "fixture-key")
	original := http.DefaultClient
	defer func() { http.DefaultClient = original }()
	requests := 0
	http.DefaultClient = &http.Client{Transport: assistantRoundTrip(func(req *http.Request) (*http.Response, error) {
		requests++
		var payload map[string]any
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		for _, definition := range payload["tools"].([]any) {
			function := definition.(map[string]any)["function"].(map[string]any)
			parameters := function["parameters"].(map[string]any)
			if _, ok := parameters["required"].([]any); !ok {
				t.Fatal("tool schema required must be an array")
			}
		}
		stream := ""
		if requests == 1 {
			stream = `data: {"choices":[{"delta":{"content":"Looking up campaigns.","tool_calls":[{"index":0,"id":"call1","function":{"name":"campaigns","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n"
		} else {
			messages := payload["messages"].([]any)
			last := messages[len(messages)-1].(map[string]any)
			if last["role"] != "tool" || !strings.Contains(last["content"].(string), "permission denied") {
				t.Fatal("tool failure missing from model continuation")
			}
			stream = `data: {"choices":[{"delta":{"content":"Campaign access is unavailable."},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n"
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(stream)), Header: http.Header{}}, nil
	})}
	app := &App{auth: &auth.Auth{}}
	re := &pbcore.RequestEvent{}
	re.Request = httptest.NewRequest("POST", "/", nil)
	re.Set(auth.UserHTTPCtxKey, auth.User{})
	turn := &assistantTurn{proposals: map[string]*assistantProposal{}}
	if err := app.runAssistant(context.Background(), re, turn, assistantRequest{Messages: []assistantMessage{{Role: "user", Content: "List campaigns"}}, Context: json.RawMessage(`{}`)}, "fixture-model"); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("got %d provider rounds", requests)
	}
	deltas := 0
	for _, event := range turn.events {
		if event.Type == "text_delta" {
			deltas++
		}
	}
	if deltas != 2 {
		t.Fatal("text was not streamed between tool calls")
	}
}

func TestValidateAssistantProposal(t *testing.T) {
	valid := `{"revision":"r1","operations":[{"op":"set","field":"subject","after":"New"},{"op":"replace_body","after":"<p>x</p>"},{"op":"replace_text","before":"a","after":"b"}]}`
	if err := validateAssistantProposal("propose_editor_patch", json.RawMessage(valid)); err != nil {
		t.Fatalf("valid proposal rejected: %v", err)
	}
	for name, raw := range map[string]string{
		"no revision":         `{"operations":[{"op":"replace_body","after":"x"}]}`,
		"no operations":       `{"revision":"r1","operations":[]}`,
		"body without after":  `{"revision":"r1","operations":[{"op":"replace_body"}]}`,
		"text without before": `{"revision":"r1","operations":[{"op":"replace_text","after":"b"}]}`,
		"unknown field":       `{"revision":"r1","operations":[{"op":"set","field":"body","after":"b"}]}`,
		"unknown op":          `{"revision":"r1","operations":[{"op":"rewrite","after":"b"}]}`,
	} {
		if err := validateAssistantProposal("propose_editor_patch", json.RawMessage(raw)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	if err := validateAssistantProposal("create_campaign_draft", json.RawMessage(`{}`)); err != nil {
		t.Errorf("other tools are not validated here: %v", err)
	}
}

// Plugins run with full access, so other roles can't see or call these tools, whatever
// the model asks for.
func TestAssistantPluginToolsAreSuperAdminOnly(t *testing.T) {
	app := &App{auth: &auth.Auth{}, plugins: &plugins.Manager{}}
	staff := &pbcore.RequestEvent{}
	staff.Request = httptest.NewRequest("POST", "/", nil)
	staff.Set(auth.UserHTTPCtxKey, auth.User{UserRoleID: auth.SuperAdminRoleID + 1})
	admin := &pbcore.RequestEvent{}
	admin.Request = httptest.NewRequest("POST", "/", nil)
	admin.Set(auth.UserHTTPCtxKey, auth.User{UserRoleID: auth.SuperAdminRoleID})

	names := func(re *pbcore.RequestEvent) map[string]bool {
		out := map[string]bool{}
		for _, tool := range app.assistantToolsFor(re) {
			out[tool.name] = true
		}
		return out
	}
	for _, name := range []string{"plugin_reference", "plugins", "plugin_files", "plugin_draft_write", "plugin_draft_edit", "propose_plugin"} {
		if names(staff)[name] {
			t.Errorf("%s is visible to staff", name)
		}
		if !names(admin)[name] {
			t.Errorf("%s is missing for a super admin", name)
		}
		if _, err := app.executeAssistantTool(staff, name, json.RawMessage(`{}`), true); err == nil || err.Error() != "unknown tool" {
			t.Errorf("%s ran for staff: %v", name, err)
		}
	}
	// A super admin still needs a person's approval to install.
	if _, err := app.executeAssistantTool(admin, "propose_plugin", json.RawMessage(`{}`), false); err == nil || err.Error() != "approval required" {
		t.Errorf("install without approval: %v", err)
	}
	if out, err := app.executeAssistantTool(admin, "plugin_reference", json.RawMessage(`{}`), false); err != nil || !strings.Contains(fmt.Sprint(out), "plugin.tool") {
		t.Errorf("the reference should hold the plugin guide: %v", err)
	}
}

func TestAssistantTokenLimit(t *testing.T) {
	t.Setenv("LISTPOCKET_AI_MAX_OUTPUT_TOKENS", "")
	if got := assistantTokenLimit()["max_completion_tokens"]; got != assistantMaxOutputTokens {
		t.Fatalf("default: %v", got)
	}
	t.Setenv("LISTPOCKET_AI_MAX_OUTPUT_TOKENS", "500000")
	if got := assistantTokenLimit()["max_completion_tokens"]; got != 500000 {
		t.Fatalf("configured: %v", got)
	}
	t.Setenv("LISTPOCKET_AI_MAX_OUTPUT_TOKENS", "0")
	if assistantTokenLimit() != nil {
		t.Fatal("0 should ask for no limit")
	}
	t.Setenv("LISTPOCKET_AI_MAX_OUTPUT_TOKENS", "lots")
	if got := assistantTokenLimit()["max_completion_tokens"]; got != assistantMaxOutputTokens {
		t.Fatalf("invalid: %v", got)
	}
}

// A model that allows fewer tokens than we ask for must still work.
func TestAssistantFallsBackWhenModelRefusesTokenLimit(t *testing.T) {
	t.Setenv("LISTPOCKET_AI_API_KEY", "fixture-key")
	t.Setenv("LISTPOCKET_AI_MAX_OUTPUT_TOKENS", "")
	original := http.DefaultClient
	defer func() { http.DefaultClient = original }()
	var sent []bool
	http.DefaultClient = &http.Client{Transport: assistantRoundTrip(func(req *http.Request) (*http.Response, error) {
		var payload map[string]any
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		_, limited := payload["max_completion_tokens"]
		sent = append(sent, limited)
		if limited {
			body := `{"error":{"message":"max_completion_tokens is too large: 128000. This model supports at most 16384"}}`
			return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
		}
		stream := `data: {"choices":[{"delta":{"content":"Done."},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n"
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(stream)), Header: http.Header{}}, nil
	})}
	app := &App{auth: &auth.Auth{}}
	re := &pbcore.RequestEvent{}
	re.Request = httptest.NewRequest("POST", "/", nil)
	re.Set(auth.UserHTTPCtxKey, auth.User{})
	turn := &assistantTurn{proposals: map[string]*assistantProposal{}}
	if err := app.runAssistant(context.Background(), re, turn, assistantRequest{Messages: []assistantMessage{{Role: "user", Content: "hi"}}}, ""); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(sent) != "[true false]" {
		t.Fatalf("requests with a limit: %v", sent)
	}
}

func TestExpandPluginProposal(t *testing.T) {
	app := &App{plugins: nil}
	raw := json.RawMessage(`{"summary":"s"}`)
	if got, err := app.expandAssistantProposal("propose_reply", raw); err != nil || string(got) != string(raw) {
		t.Fatalf("other proposals pass through: %s, %v", got, err)
	}
	if got, err := app.expandAssistantProposal("propose_plugin", raw); err != nil || string(got) != string(raw) {
		t.Fatalf("files sent directly pass through: %s, %v", got, err)
	}
	if _, err := app.expandAssistantProposal("propose_plugin", json.RawMessage(`{"plugin":"a","files":[{"path":"x.js","content":""}]}`)); err == nil {
		t.Fatal("a draft and files together should be refused")
	}
}

func TestAssistantProviderErrorExplainsCredentials(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		err := assistantProviderError(&llm.StatusError{Status: status, Body: "secret detail from the provider"})
		if !strings.Contains(err.Error(), "credentials") || strings.Contains(err.Error(), "secret detail") {
			t.Errorf("%d: %v", status, err)
		}
	}
	if err := assistantProviderError(&llm.StatusError{Status: 500}); err.Error() != "AI provider returned HTTP 500" {
		t.Errorf("other statuses: %v", err)
	}
}
