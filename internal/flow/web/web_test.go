package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/compdani/list_pocket/internal/flow/llm"
)

func TestBlocklist(t *testing.T) {
	b := NewBlocklist([]string{"linkedin.com", "*.facebook.com", " BiggerPockets.com "})
	for host, want := range map[string]bool{
		"linkedin.com": true, "www.linkedin.com": true, "facebook.com": true, "m.facebook.com": true,
		"biggerpockets.com": true, "notlinkedin.com": false, "example.com": false,
	} {
		if got := b.BlockedHost(host); got != want {
			t.Errorf("BlockedHost(%q) = %v, want %v", host, got, want)
		}
	}
	if !b.BlockedURL("not a url") {
		t.Error("unparseable URLs must count as blocked")
	}
}

func TestServerToolAndWarnings(t *testing.T) {
	b := NewBlocklist([]string{"*.linkedin.com"})
	tool := ServerTool(llm.SearchSettings{Engine: "exa", MaxResults: 5, ExcludedDomains: []string{"reddit.com"}}, b)
	params := tool["parameters"].(map[string]any)
	if tool["type"] != ServerToolType || !reflect.DeepEqual(params["excluded_domains"], []string{"reddit.com", "linkedin.com"}) {
		t.Errorf("tool = %v", tool)
	}

	req := &llm.Request{}
	ApplySearch(req, llm.Endpoint{BaseURL: "https://api.openai.com/v1"}, llm.SearchSettings{Mode: "openai_native", SearchContextSize: "low"}, b)
	if len(req.Tools) != 0 || req.Extra["web_search_options"] == nil {
		t.Errorf("direct OpenAI should use web_search_options, got %+v", req)
	}
	req = &llm.Request{}
	ApplySearch(req, llm.Endpoint{BaseURL: "https://openrouter.ai/api/v1"}, llm.SearchSettings{Mode: "openai_native"}, b)
	if p := req.Tools[0]["parameters"].(map[string]any); p["engine"] != "native" {
		t.Errorf("openai_native via OpenRouter should use engine native: %v", req.Tools)
	}
	if len(SearchWarnings(llm.SearchSettings{Mode: "openrouter", Engine: "exa"})) != 0 {
		t.Error("exa supports exclusions; no warning expected")
	}
	if len(SearchWarnings(llm.SearchSettings{Mode: "openrouter", Engine: "native"})) == 0 {
		t.Error("native engine should warn")
	}
}

func TestDropBlockedSources(t *testing.T) {
	b := NewBlocklist([]string{"linkedin.com"})
	v := map[string]any{"leads": []any{
		map[string]any{"name": "A", "source_url": "https://www.linkedin.com/in/a"},
		map[string]any{"name": "B", "source_url": "https://reia.test/members"},
	}}
	cleaned, removed := DropBlockedSources(v, b)
	leads := cleaned.(map[string]any)["leads"].([]any)
	if len(leads) != 1 || leads[0].(map[string]any)["name"] != "B" || len(removed) != 1 {
		t.Errorf("leads=%v removed=%v", leads, removed)
	}
}

func TestParseRobots(t *testing.T) {
	body := "User-agent: *\nDisallow: /private\n\nUser-agent: GoHarnessBot\nUser-agent: other\nDisallow: /nobots\nDisallow: /tmp*\n"
	if got := ParseRobots(body, "GoHarnessBot"); !reflect.DeepEqual(got, []string{"/nobots", "/tmp"}) {
		t.Errorf("own group = %v", got)
	}
	if got := ParseRobots(body, "SomeoneElse"); !reflect.DeepEqual(got, []string{"/private"}) {
		t.Errorf("star group = %v", got)
	}
}

func TestExtractText(t *testing.T) {
	html := `<html><head><title> Lake County REIA </title><style>.x{}</style></head><body>
	<nav>Home | About</nav><h1>Members</h1><p>Jane Doe, <b>Doe Rentals LLC</b></p>
	<p>Contact: <a href="mailto:jane@doerentals.test?subject=hi">jane@doerentals.test</a></p>
	<ul><li><a href="/team">Team</a></li></ul><script>alert(1)</script><footer>©</footer></body></html>`
	title, text := ExtractText([]byte(html), "https://reia.test/members")
	if title != "Lake County REIA" {
		t.Errorf("title = %q", title)
	}
	for _, want := range []string{"# Members", "Jane Doe, Doe Rentals LLC", "jane@doerentals.test", "- Team (https://reia.test/team)"} {
		if !strings.Contains(text, want) {
			t.Errorf("text missing %q:\n%s", want, text)
		}
	}
	for _, bad := range []string{"alert", "Home | About", ".x{}", "©"} {
		if strings.Contains(text, bad) {
			t.Errorf("text should not contain %q:\n%s", bad, text)
		}
	}
}

func TestFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			fmt.Fprint(w, "User-agent: *\nDisallow: /secret\n")
		case "/page":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, "<title>T</title><p>"+strings.Repeat("word ", 100)+"</p>")
		case "/to-blocked":
			http.Redirect(w, r, "https://www.linkedin.com/in/x", http.StatusFound)
		case "/bin":
			w.Header().Set("Content-Type", "application/pdf")
			fmt.Fprint(w, "%PDF")
		}
	}))
	defer srv.Close()

	block := NewBlocklist([]string{"linkedin.com"})
	f := NewFetcher(func() *Blocklist { return block })
	ctx := context.Background()

	if _, err := f.Fetch(ctx, srv.URL+"/page", 0); !errors.Is(err, ErrPrivate) {
		t.Fatalf("default fetcher must refuse loopback, got %v", err)
	}

	f.AllowPrivate = true
	res, err := f.Fetch(ctx, srv.URL+"/page", 50)
	if err != nil {
		t.Fatal(err)
	}
	if res.Title != "T" || !res.Truncated || len([]rune(res.Text)) != 50 || res.Status != 200 {
		t.Errorf("result = %+v", res)
	}
	if _, err := f.Fetch(ctx, srv.URL+"/secret/x", 0); !errors.Is(err, ErrRobots) {
		t.Errorf("robots: got %v", err)
	}
	if _, err := f.Fetch(ctx, "https://linkedin.com/in/x", 0); !errors.Is(err, ErrBlocked) {
		t.Errorf("blocked: got %v", err)
	}
	if _, err := f.Fetch(ctx, srv.URL+"/to-blocked", 0); !errors.Is(err, ErrBlocked) {
		t.Errorf("redirect to blocked: got %v", err)
	}
	if _, err := f.Fetch(ctx, srv.URL+"/bin", 0); !errors.Is(err, ErrUnsupportedCT) {
		t.Errorf("pdf: got %v", err)
	}
	if _, err := f.Fetch(ctx, "file:///etc/passwd", 0); err == nil {
		t.Error("non-http scheme must be refused")
	}
}
