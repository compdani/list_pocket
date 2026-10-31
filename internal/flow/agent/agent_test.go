package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	_ "github.com/compdani/list_pocket/internal/migrations"

	"github.com/compdani/list_pocket/internal/flow/agent"
	"github.com/compdani/list_pocket/internal/flow/llm"
	"github.com/compdani/list_pocket/internal/flow/tools"
	"github.com/compdani/list_pocket/internal/flow/web"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// mockLLM replays scripted streaming responses and records request bodies.
type mockLLM struct {
	mu       sync.Mutex
	bodies   []map[string]any
	replies  []string // SSE "data:" payloads, one reply per request, chunks split by \n
	pageURL  string
	hitCount int
}

func (m *mockLLM) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	m.mu.Lock()
	m.bodies = append(m.bodies, body)
	i := m.hitCount
	m.hitCount++
	m.mu.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	if i >= len(m.replies) {
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"}}]}\n\ndata: [DONE]\n\n")
		return
	}
	for _, chunk := range strings.Split(m.replies[i], "\n") {
		fmt.Fprintf(w, "data: %s\n\n", chunk)
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
}

var testLLMURL string

func newApp(t *testing.T, llmURL string) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestAppWithConfig(core.BaseAppConfig{DataDir: t.TempDir(), EncryptionEnv: "pb_test_env"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	if err := app.RunAllMigrations(); err != nil {
		t.Fatal(err)
	}
	testLLMURL = llmURL
	return app
}

func newRunner(app core.App) *agent.Runner {
	res := &llm.Resolver{Settings: map[string]any{"domain_blocklist": []any{"linkedin.com"}}, Getenv: func(k string) string {
		return map[string]string{"LISTPOCKET_AI_API_KEY": "test-key", "LISTPOCKET_AI_BASE_URL": testLLMURL, "LISTPOCKET_AI_MODEL": "m", "LISTPOCKET_AI_SEARCH_MODE": "openrouter"}[k]
	}}
	reg := tools.NewRegistry()
	f := web.NewFetcher(func() *web.Blocklist { return agent.LoadBlocklist(res) })
	f.AllowPrivate = true
	reg.Register(tools.WebFetch{Fetcher: f})
	return &agent.Runner{App: app, LLM: llm.NewClient(), Tools: reg, Resolver: res}
}

var leadSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"leads": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type":       "object",
				"properties": map[string]any{"name": map[string]any{"type": "string"}, "source_url": map[string]any{"type": "string"}},
				"required":   []any{"name", "source_url"},
			},
		},
	},
	"required": []any{"leads"},
}

func TestAgentSearchFetchSubmit(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<h1>Members</h1><p>Jane Doe — Doe Rentals LLC</p>`)
	}))
	defer page.Close()

	fetchArgs, _ := json.Marshal(map[string]string{"url": page.URL + "/members"})
	badSubmit := `{"leads":[{"name":"Jane Doe"}]}`
	goodSubmit, _ := json.Marshal(map[string]any{"leads": []any{
		map[string]any{"name": "Jane Doe", "source_url": page.URL + "/members"},
		map[string]any{"name": "Spam", "source_url": "https://www.linkedin.com/in/spam"},
	}})
	q := func(s string) string { b, _ := json.Marshal(s); return string(b) }

	m := &mockLLM{replies: []string{
		// 1: searched (citations incl. a blocklisted one), then calls web.fetch
		`{"model":"mock","choices":[{"delta":{"content":"Searching.","annotations":[{"type":"url_citation","url_citation":{"url":"` + page.URL + `/members","title":"REIA"}},{"type":"url_citation","url_citation":{"url":"https://linkedin.com/in/x","title":"LI"}}]}}]}` + "\n" +
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"web__fetch","arguments":` + q(string(fetchArgs)) + `}}]}}]}` + "\n" +
			`{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":20,"cost":0.002,"server_tool_use":{"web_search_requests":1}}}`,
		// 2: invalid submit (missing source_url) → repair
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c2","type":"function","function":{"name":"submit_result","arguments":` + q(badSubmit) + `}}]}}]}` + "\n" +
			`{"choices":[],"usage":{"prompt_tokens":150,"completion_tokens":10,"cost":0.001}}`,
		// 3: valid submit
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c3","type":"function","function":{"name":"submit_result","arguments":` + q(string(goodSubmit)) + `}}]}}]}` + "\n" +
			`{"choices":[],"usage":{"prompt_tokens":160,"completion_tokens":30,"cost":0.001}}`,
	}}
	llmSrv := httptest.NewServer(m)
	defer llmSrv.Close()

	app := newApp(t, llmSrv.URL)
	runner := newRunner(app)

	var events []agent.Event
	res, err := runner.Run(context.Background(), agent.Spec{
		Name:         "investor_search",
		SystemPrompt: "Find real estate investors in {{ .input.market }}.",
		Tools:        []string{"web.search", "web.fetch"},
		OutputSchema: leadSchema,
		AI:           map[string]any{"model": "@research"},
	}, agent.Options{
		Input:      map[string]any{"market": "Lake County, IN"},
		WorkflowAI: map[string]any{"models": map[string]any{"research": "test/research-model"}},
		OnEvent:    func(e agent.Event) { events = append(events, e) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "succeeded" {
		t.Fatalf("status = %s err=%s", res.Status, res.Error)
	}

	// Request 1: model role resolved, prompt rendered, server search tool + function tools present.
	first := m.bodies[0]
	if first["model"] != "test/research-model" {
		t.Errorf("model = %v", first["model"])
	}
	msgs := first["messages"].([]any)
	if sys := msgs[0].(map[string]any)["content"].(string); !strings.Contains(sys, "Lake County, IN") {
		t.Errorf("system prompt not rendered: %q", sys)
	}
	var names []string
	var search map[string]any
	for _, tl := range first["tools"].([]any) {
		tm := tl.(map[string]any)
		if tm["type"] == web.ServerToolType {
			search = tm
			continue
		}
		names = append(names, tm["function"].(map[string]any)["name"].(string))
	}
	if strings.Join(names, ",") != "web__fetch,submit_result" {
		t.Errorf("function tools = %v", names)
	}
	if search == nil {
		t.Fatal("openrouter:web_search tool missing")
	}
	params := search["parameters"].(map[string]any)
	if !strings.Contains(fmt.Sprint(params["excluded_domains"]), "linkedin.com") {
		t.Errorf("search params = %v", params)
	}

	// Request 2 carried the fetched page back as a tool message.
	toolMsg := m.bodies[1]["messages"].([]any)[3].(map[string]any)
	if toolMsg["role"] != "tool" || !strings.Contains(toolMsg["content"].(string), "Jane Doe") {
		t.Errorf("tool message = %v", toolMsg)
	}
	// Request 3 carried the validation error.
	repair := m.bodies[2]["messages"].([]any)
	if c := repair[len(repair)-1].(map[string]any)["content"].(string); !strings.Contains(c, "source_url") {
		t.Errorf("repair message = %q", c)
	}

	// Output: blocked lead + citation dropped.
	leads := res.Output["result"].(map[string]any)["leads"].([]any)
	if len(leads) != 1 || leads[0].(map[string]any)["name"] != "Jane Doe" {
		t.Errorf("leads = %v", leads)
	}
	if cits := res.Output["_citations"].([]llm.Citation); len(cits) != 1 {
		t.Errorf("citations = %v", cits)
	}
	if res.SearchRequests != 1 || res.CostUSD < 0.0039 || res.CostUSD > 0.0041 {
		t.Errorf("search=%d cost=%v", res.SearchRequests, res.CostUSD)
	}

	// Persistence.
	rec, err := app.FindRecordById("flow_agent_runs", res.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.GetString("status") != "succeeded" || rec.GetInt("search_requests") != 1 || rec.GetInt("prompt_tokens") != 410 {
		t.Errorf("agent_run = %v", rec.PublicExport())
	}
	steps, _ := app.FindAllRecords("flow_run_steps")
	// model, tool(fetch), model, tool(bad submit), model, tool(good submit)
	if len(steps) != 6 {
		t.Errorf("steps = %d, want 6", len(steps))
	}
	var sawDelta bool
	for _, e := range events {
		if e.Kind == "delta" && e.Text == "Searching." {
			sawDelta = true
		}
	}
	if !sawDelta {
		t.Error("expected streamed delta event")
	}
}

func TestAgentBudgetAndUnknownTool(t *testing.T) {
	m := &mockLLM{replies: []string{
		`{"choices":[{"delta":{"content":"thinking"}}]}` + "\n" + `{"choices":[],"usage":{"cost":1.0}}`,
	}}
	llmSrv := httptest.NewServer(m)
	defer llmSrv.Close()
	app := newApp(t, llmSrv.URL)
	runner := newRunner(app)

	res, _ := runner.Run(context.Background(), agent.Spec{SystemPrompt: "x", Tools: []string{"nope.tool"}}, agent.Options{})
	if res.Status != "failed" || !strings.Contains(res.Error, "unknown tool") {
		t.Errorf("unknown tool: %+v", res)
	}

	res, _ = runner.Run(context.Background(), agent.Spec{SystemPrompt: "x", OutputSchema: leadSchema},
		agent.Options{RunAI: map[string]any{"budget": map[string]any{"max_usd_per_run": 0.5}}})
	if res.Status != "failed" || !strings.Contains(res.Error, "budget") {
		t.Errorf("budget: %+v", res)
	}
}

// A model that keeps researching gets a wrap-up warning, then is forced to
// submit on its last step; searches stop at the per-run cap.
func TestAgentWrapUpAndSearchCap(t *testing.T) {
	q := func(s string) string { b, _ := json.Marshal(s); return string(b) }
	fetch := `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"f","type":"function","function":{"name":"web__fetch","arguments":` + q(`{"url":"http://127.0.0.1:1/x"}`) + `}}]}}]}` + "\n" +
		`{"choices":[],"usage":{"prompt_tokens":10,"server_tool_use_details":{"web_search_requests":15}}}`
	submit := `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"s","type":"function","function":{"name":"submit_result","arguments":` + q(`{"leads":[]}`) + `}}]}}]}`
	m := &mockLLM{replies: []string{fetch, fetch, fetch, submit}}
	llmSrv := httptest.NewServer(m)
	defer llmSrv.Close()
	app := newApp(t, llmSrv.URL)
	runner := newRunner(app)

	res, err := runner.Run(context.Background(), agent.Spec{
		SystemPrompt: "Find leads.", Tools: []string{"web.search", "web.fetch"}, MaxSteps: 4, OutputSchema: leadSchema,
	}, agent.Options{RunAI: map[string]any{"search": map[string]any{"max_searches_per_run": 20}}})
	if err != nil || res.Status != "succeeded" {
		t.Fatalf("run: %v %+v", err, res)
	}
	body := func(i int) string { raw, _ := json.Marshal(m.bodies[i]); return string(raw) }
	if !strings.Contains(body(0), "openrouter:web_search") {
		t.Error("first request should offer search")
	}
	if strings.Contains(body(2), "openrouter:web_search") || !strings.Contains(body(2), "Web search limit reached (30 searches)") {
		t.Errorf("after the cap, search should be withdrawn with a note: %s", body(2))
	}
	if !strings.Contains(body(2), "You have 2 steps left") {
		t.Error("wrap-up warning missing two steps before the limit")
	}
	if !strings.Contains(body(3), `"tool_choice":{"function":{"name":"submit_result"},"type":"function"}`) {
		t.Errorf("last step should force submit_result: %s", body(3))
	}
}

// humanBodyTool needs a person to have written its body when format is html.
type humanBodyTool struct {
	mu    sync.Mutex
	calls int
}

func (t *humanBodyTool) Name() string                { return "test.human" }
func (t *humanBodyTool) Description() string         { return "Sends a body." }
func (t *humanBodyTool) RequiresApproval() bool      { return false }
func (t *humanBodyTool) InputSchema() map[string]any { return map[string]any{"type": "object"} }
func (t *humanBodyTool) HumanWrittenField(args map[string]any) string {
	if args["format"] == "html" {
		return "body"
	}
	return ""
}
func (t *humanBodyTool) Invoke(context.Context, tools.Call) (any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.calls++
	return map[string]any{"sent": true}, nil
}

// An agent's arguments are always a model's writing, so it can't use a field that
// needs a person, even when the text looks fixed.
func TestAgentCannotFillHumanWrittenField(t *testing.T) {
	call := func(id, args string) string {
		quoted, _ := json.Marshal(args)
		return `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"` + id + `","type":"function","function":{"name":"test__human","arguments":` + string(quoted) + `}}]}}]}`
	}
	m := &mockLLM{replies: []string{
		call("c1", `{"format":"html","body":"<p>hi</p>"}`),
		call("c2", `{"format":"plain","body":"hi"}`),
	}}
	llmSrv := httptest.NewServer(m)
	defer llmSrv.Close()
	runner := newRunner(newApp(t, llmSrv.URL))
	tool := &humanBodyTool{}
	runner.Tools.Register(tool)

	res, err := runner.Run(context.Background(), agent.Spec{SystemPrompt: "x", Tools: []string{"test.human"}}, agent.Options{})
	if err != nil || res.Status != "succeeded" {
		t.Fatalf("run: %+v %v", res, err)
	}
	if tool.calls != 1 {
		t.Fatalf("only the plain call should run, got %d", tool.calls)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	raw, _ := json.Marshal(m.bodies[1]["messages"])
	if !strings.Contains(string(raw), "a person has to check body") {
		t.Fatalf("the model should be told why: %s", raw)
	}
}
