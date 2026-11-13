package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func connect(t *testing.T, cfg Config) (*mcp.ClientSession, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a, b := mcp.NewInMemoryTransports()
	ss, err := s.Connect(ctx, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs, ctx
}

func TestDiscoveryAndRead(t *testing.T) {
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" || r.URL.Path != "/prefix/mailapi/subscribers/abc123/notes" {
			t.Errorf("unexpected route: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing bearer auth")
		}
		if got := r.URL.Query()["tag"]; len(got) != 2 || got[1] != "b" {
			t.Errorf("query lost: %v", r.URL.Query())
		}
		fmt.Fprint(w, `{"data":[{"body":"note"}]}`)
	}))
	defer api.Close()
	cs, ctx := connect(t, Config{URL: api.URL + "/prefix/", Token: "secret"})
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Tools) != len(reads) {
		t.Fatalf("got %d tools", len(list.Tools))
	}
	for _, tool := range list.Tools {
		if !tool.Annotations.ReadOnlyHint || strings.HasPrefix(tool.Name, "create_") {
			t.Errorf("unexpected write tool: %s", tool.Name)
		}
	}
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "list_contact_notes", Arguments: map[string]any{"id": "abc123", "query": "tag=a&tag=b"}})
	if err != nil || result.IsError {
		t.Fatalf("read: %v %v", result, err)
	}
	if result.Content[0].(*mcp.TextContent).Text != `{"data":[{"body":"note"}]}` {
		t.Fatal("response changed")
	}
	if calls.Load() != 1 {
		t.Fatal("unexpected API calls")
	}
}

func TestRejectedCallsNeverReachAPI(t *testing.T) {
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer api.Close()
	cs, ctx := connect(t, Config{URL: api.URL, Token: "secret"})
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"get_subscriber", map[string]any{}},
		{"get_subscriber", map[string]any{"id": "../settings"}},
		{"get_subscriber", map[string]any{"id": "x?access_token=bad"}},
		{"list_lists", map[string]any{"url": "https://example.com"}},
		{"list_lists", map[string]any{"query": "access_token=other"}},
		{"list_lists", map[string]any{"query": "broken=%zz"}},
		{"create_list", map[string]any{"name": "no writes"}},
		{"update_subscriber", map[string]any{"id": "contact123", "phone": "123"}},
	} {
		t.Run(tc.name+fmt.Sprint(tc.args), func(t *testing.T) {
			result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if err == nil && !result.IsError {
				t.Fatal("call was not rejected")
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatal("invalid call reached API")
	}
}

func TestUpdateSubscriberSendsOnlySuppliedFields(t *testing.T) {
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "PATCH" || r.URL.Path != "/mailapi/subscribers/contact123" {
			t.Errorf("unexpected route: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body) != 1 || body["phone"] != "" {
			t.Errorf("omitted fields were sent: %v", body)
		}
		fmt.Fprint(w, `{"data":{"id":"contact123"}}`)
	}))
	defer api.Close()
	cs, ctx := connect(t, Config{URL: api.URL, Token: "secret", AllowWrites: true})
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "update_subscriber", Arguments: map[string]any{"id": "contact123", "phone": ""}})
	if err != nil || result.IsError {
		t.Fatalf("update: %+v %v", result, err)
	}
	result, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "update_subscriber", Arguments: map[string]any{"id": "contact123"}})
	if err == nil && !result.IsError {
		t.Fatal("empty update accepted")
	}
	result, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "update_subscriber", Arguments: map[string]any{"id": "../settings", "phone": ""}})
	if err == nil && !result.IsError {
		t.Fatal("invalid ID accepted")
	}
	if calls.Load() != 1 {
		t.Fatalf("unexpected API requests: %d", calls.Load())
	}
}

func TestWrites(t *testing.T) {
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "POST" || r.URL.Path != "/mailapi/subscribers/contact123/notes" {
			t.Errorf("unexpected route: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body) != 1 || body["body"] != "Follow up Friday" {
			t.Errorf("unexpected body: %v", body)
		}
		fmt.Fprint(w, `{"data":{"id":"note123"}}`)
	}))
	defer api.Close()
	cs, ctx := connect(t, Config{URL: api.URL, Token: "secret", AllowWrites: true})
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Tools) != len(reads)+5 {
		t.Fatalf("got %d tools", len(list.Tools))
	}
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "create_contact_note", Arguments: map[string]any{"id": "contact123", "body": "Follow up Friday"}})
	if err != nil || result.IsError {
		t.Fatalf("write: %v %v", result, err)
	}
	if calls.Load() != 1 {
		t.Fatal("write was retried or skipped")
	}
}

func TestBackendErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"forbidden", 403, `{"message":"denied secret"}`, "HTTP 403"},
		{"expired", 401, `{"message":"expired"}`, "HTTP 401"},
		{"redirect", 302, "redirect", "HTTP 302"},
		{"html", 200, "<html>Login</html>", "non-JSON"},
		{"oversize", 200, strings.Repeat("x", maxResponseBytes+1), "exceeds 8 MiB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Location", "/redirect-target")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer api.Close()
			cs, ctx := connect(t, Config{URL: api.URL, Token: "secret"})
			result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "list_lists", Arguments: map[string]any{}})
			if err != nil {
				t.Fatal(err)
			}
			text := result.Content[0].(*mcp.TextContent).Text
			if !result.IsError || !strings.Contains(text, tc.want) || strings.Contains(text, "secret") {
				t.Fatalf("unexpected result: %s", text)
			}
			if calls.Load() != 1 {
				t.Fatal("request was retried or redirected")
			}
		})
	}
}

func TestInvalidConfiguration(t *testing.T) {
	for _, base := range []string{"", "file:///tmp/db", "http://user:pass@localhost", "http://localhost?token=x", "http://localhost/#fragment"} {
		if _, err := New(Config{URL: base, Token: "secret"}); err == nil {
			t.Errorf("accepted %q", base)
		}
	}
	if _, err := New(Config{URL: "http://localhost"}); err == nil {
		t.Error("accepted empty token")
	}
}
