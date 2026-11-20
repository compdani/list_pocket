package migrations

import (
	"database/sql"
	"errors"
	"testing"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tests"
)

func TestDropLegacyAgentTablesMigration(t *testing.T) {
	app, err := tests.NewTestApp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	// Start from the pre-cutover state: recreate the legacy collections via Down
	// and populate them with rows (plus ai_settings / oauth_credentials).
	if err := dropLegacyAgentTablesDown(app); err != nil {
		t.Fatalf("down (setup): %v", err)
	}
	convCol, err := app.FindCollectionByNameOrId("agent_conversations")
	if err != nil {
		t.Fatalf("agent_conversations missing after down: %v", err)
	}
	msgCol, err := app.FindCollectionByNameOrId("agent_messages")
	if err != nil {
		t.Fatalf("agent_messages missing after down: %v", err)
	}
	attCol, err := app.FindCollectionByNameOrId("agent_attachments")
	if err != nil {
		t.Fatalf("agent_attachments missing after down: %v", err)
	}

	conv := core.NewRecord(convCol)
	conv.Set("user", "u1")
	conv.Set("title", "Old chat")
	if err := app.Save(conv); err != nil {
		t.Fatal(err)
	}
	msg := core.NewRecord(msgCol)
	msg.Set("conversation", conv.Id)
	msg.Set("role", "user")
	msg.Set("content", "Hello")
	if err := app.Save(msg); err != nil {
		t.Fatal(err)
	}
	att := core.NewRecord(attCol)
	att.Set("conversation", conv.Id)
	att.Set("user", "u1")
	att.Set("name", "notes.txt")
	att.Set("content", "hello")
	if err := app.Save(att); err != nil {
		t.Fatal(err)
	}

	// Up drops all three legacy collections even when populated, and is idempotent.
	if err := dropLegacyAgentTablesUp(app); err != nil {
		t.Fatalf("up: %v", err)
	}
	for _, name := range []string{"agent_conversations", "agent_messages", "agent_attachments"} {
		if _, err := app.FindCollectionByNameOrId(name); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("%s after up: err = %v, want sql.ErrNoRows", name, err)
		}
	}
	if err := dropLegacyAgentTablesUp(app); err != nil {
		t.Fatalf("idempotent up: %v", err)
	}
	for _, kept := range []string{"ai_settings", "oauth_credentials"} {
		if _, err := app.FindCollectionByNameOrId(kept); err != nil {
			t.Fatalf("%s should remain after up: %v", kept, err)
		}
	}

	// Down recreates the three empty collections with their expected fields.
	if err := dropLegacyAgentTablesDown(app); err != nil {
		t.Fatalf("down: %v", err)
	}
	for name, fields := range map[string][]string{
		"agent_conversations": {"user", "title", "ai"},
		"agent_messages":      {"conversation", "role", "content", "parts", "context"},
		"agent_attachments":   {"conversation", "user", "name", "mime", "size", "binary", "content"},
	} {
		col, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatalf("%s after down: %v", name, err)
		}
		for _, field := range fields {
			if col.Fields.GetByName(field) == nil {
				t.Errorf("%s missing field %q after down", name, field)
			}
		}
	}
}
