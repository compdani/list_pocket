package main

import (
	"strings"
	"testing"

	"github.com/compdani/list_pocket/internal/flow/tools"
)

func TestFlowEmailHTMLSanitizes(t *testing.T) {
	out, _, err := flowEmailHTML("html", `<h1 style="color:red;position:fixed" onclick="x()">Hi</h1>
		<script>alert(1)</script><style>a{}</style><iframe src="https://evil.example"></iframe>
		<form action="https://evil.example"><input name="password"></form>
		<a href="javascript:alert(1)">bad</a><a href="https://example.com">good</a>
		<img src="https://example.com/logo.png" alt="Logo" onerror="x()"><img src="data:text/html,x">`)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, banned := range []string{"<script", "alert(1)", "<style", "<iframe", "<form", "<input", "onclick", "onerror", "javascript:", "data:text", "position"} {
		if strings.Contains(got, banned) {
			t.Errorf("%q survived: %s", banned, got)
		}
	}
	for _, kept := range []string{"<h1", "color: red", `href="https://example.com"`, `src="https://example.com/logo.png"`} {
		if !strings.Contains(got, kept) {
			t.Errorf("%q was lost: %s", kept, got)
		}
	}
}

func TestFlowEmailHTMLMarkdownCannotCarryRawHTML(t *testing.T) {
	out, text, err := flowEmailHTML("markdown", "# Welcome\n\nSee [the guide](https://example.com/guide).\n\n<script>alert(1)</script>\n\n- one\n- two")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "<h1") || strings.Contains(string(out), "<script") {
		t.Fatalf("unexpected HTML: %s", out)
	}
	for _, want := range []string{"Welcome", "the guide (https://example.com/guide)", "- one\n- two"} {
		if !strings.Contains(text, want) {
			t.Errorf("text version is missing %q: %q", want, text)
		}
	}
	if strings.Contains(text, "<") || strings.Contains(text, "alert") {
		t.Errorf("text version has markup: %q", text)
	}
}

func TestFlowEmailHTMLRefusals(t *testing.T) {
	if _, _, err := flowEmailHTML("html", `<script>only this</script>`); err == nil {
		t.Error("a body with nothing left should be refused")
	}
	if _, _, err := flowEmailHTML("grapes", `<p>hi</p>`); err == nil {
		t.Error("an unknown content type should be refused")
	}
}

func TestFlowEmailHumanWritten(t *testing.T) {
	tool := flowTool{name: "t", humanWritten: flowEmailHumanWritten}
	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{"default is plain", map[string]any{"body": "hi"}, ""},
		{"plain", map[string]any{"body": "hi", "content_type": "plain"}, ""},
		{"html", map[string]any{"body": "<p>hi</p>", "content_type": "html"}, "body"},
		{"markdown", map[string]any{"body": "hi", "content_type": "markdown"}, "body"},
		{"visual", map[string]any{"body": "<p>hi</p>", "content_type": "visual"}, "body"},
		{"template ignores body", map[string]any{"template_id": "x", "content_type": "html"}, ""},
	}
	for _, tt := range tests {
		if got := tools.HumanWrittenField(tool, tt.args); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestFlowEmailHTMLEditorFormats(t *testing.T) {
	doc := `<!DOCTYPE html><html><head><style>p{color:red}</style></head><body style="background-color:#eee"><table width="600" cellpadding="4" style="border-collapse:collapse"><tr><td style="padding:8px;font-size:14px">Hello</td></tr></table></body></html>`
	for _, format := range []string{"richtext", "visual", "grapes_mjml"} {
		out, text, err := flowEmailHTML(format, doc)
		if err != nil {
			t.Fatalf("%s: %v", format, err)
		}
		got := string(out)
		for _, kept := range []string{`width="600"`, `cellpadding="4"`, "border-collapse", "padding: 8px", "Hello"} {
			if !strings.Contains(got, kept) {
				t.Errorf("%s lost %q: %s", format, kept, got)
			}
		}
		if strings.Contains(got, "<style") || text != "Hello" {
			t.Errorf("%s: html %s, text %q", format, got, text)
		}
	}
}
