package schema_test

import (
	"strings"
	"testing"

	"github.com/compdani/list_pocket/internal/flow/schema"
)

func TestCheckWidgets(t *testing.T) {
	s := map[string]any{"type": "object", "properties": map[string]any{
		"body":   map[string]any{"type": "string", "x-ui": "textarea"},
		"code":   map[string]any{"type": "string", "x-ui": map[string]any{"widget": "code", "language": "html"}},
		"mail":   map[string]any{"type": "object", "x-ui": "email"},
		"tpl":    map[string]any{"type": "string", "x-ui": "template"},
		"custom": map[string]any{"type": "string", "x-ui": "mailer.editor"},
		"wrong":  map[string]any{"type": "string", "x-ui": "email"},
		"nope":   map[string]any{"x-ui": "wysiwyg"},
		"gone":   map[string]any{"x-ui": "other.editor"},
		"list":   map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"bad": map[string]any{"x-ui": 3.0}}}},
	}}
	errs := schema.CheckWidgets(s, func(w string) bool { return w == "mailer.editor" })
	joined := strings.Join(errs, "\n")
	for _, want := range []string{`$.wrong: x-ui "email" edits a object`, `$.nope: unknown x-ui widget "wysiwyg"`, `$.gone: field editor "other.editor"`, `$.list[].bad: x-ui must be`} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in:\n%s", want, joined)
		}
	}
	if len(errs) != 4 {
		t.Errorf("errs = %v", errs)
	}
}
