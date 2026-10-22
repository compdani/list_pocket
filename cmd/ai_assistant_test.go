package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/compdani/list_pocket/internal/auth"
	pbcore "github.com/pocketbase/pocketbase/core"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestAssistantStreamDeltasAndFragmentedCalls(t *testing.T) {
	stream := `data: {"choices":[{"delta":{"content":"Hello "}}]}

data: {"choices":[{"delta":{"content":"there","tool_calls":[{"index":0,"id":"call1","function":{"name":"campaigns","arguments":"{\"id\":"}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"123\"}"}}]},"finish_reason":"tool_calls"}]}

data: [DONE]

`
	var deltas []string
	calls, text, err := consumeAssistantStream(strings.NewReader(stream), func(d string) { deltas = append(deltas, d) })
	if err != nil {
		t.Fatal(err)
	}
	if len(deltas) != 2 || text != "Hello there" || len(calls) != 1 || calls[0].Function.Arguments != `{"id":"123"}` {
		t.Fatalf("unexpected result: %q %#v", text, calls)
	}
}
func TestAssistantRejectsIncompleteProviderStream(t *testing.T) {
	for _, stream := range []string{`data: {"choices":[{"delta":{"content":"partial"}}]}` + "\n", `data: {"error":{"message":"failure"}}` + "\n", `data: {"choices":[{"finish_reason":"length"}]}` + "\n"} {
		if _, _, err := consumeAssistantStream(strings.NewReader(stream), func(string) {}); err == nil {
			t.Fatal("expected incomplete/error stream to fail")
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
