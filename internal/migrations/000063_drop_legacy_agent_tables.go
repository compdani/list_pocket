package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

func init() {
	// Legacy Agent page collections replaced by the pocketbase-mcp chat module.
	// ai_settings and oauth_credentials stay: they still back Settings → AI.
	m.Register(func(app core.App) error {
		for _, name := range []string{"agent_attachments", "agent_messages", "agent_conversations"} {
			col, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				continue
			}
			if err := app.Delete(col); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		text := func(name string, required bool) core.Field {
			return &core.TextField{Name: name, Required: required, Max: 10_000}
		}
		jsonField := func(name string, max int64) core.Field { return &core.JSONField{Name: name, MaxSize: max} }
		dates := func(c *core.Collection) {
			c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true}, &core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		}

		conversations := core.NewBaseCollection("agent_conversations")
		conversations.Fields.Add(text("user", true), text("title", false), jsonField("ai", 1<<16))
		dates(conversations)
		conversations.AddIndex("idx_agent_conversations_user", false, "user, updated", "")
		if err := app.Save(conversations); err != nil {
			return err
		}

		messages := core.NewBaseCollection("agent_messages")
		messages.Fields.Add(
			&core.RelationField{Name: "conversation", CollectionId: conversations.Id, Required: true, MaxSelect: 1, CascadeDelete: true},
			&core.SelectField{Name: "role", Required: true, MaxSelect: 1, Values: []string{"user", "assistant"}},
			&core.TextField{Name: "content", Max: 2_000_000},
			jsonField("parts", 5<<20), jsonField("context", 1<<20),
		)
		dates(messages)
		messages.AddIndex("idx_agent_messages_conversation", false, "conversation, created", "")
		if err := app.Save(messages); err != nil {
			return err
		}

		attachments := core.NewBaseCollection("agent_attachments")
		attachments.Fields.Add(
			&core.RelationField{Name: "conversation", CollectionId: conversations.Id, MaxSelect: 1, CascadeDelete: true},
			text("user", true), text("name", true), text("mime", false), &core.NumberField{Name: "size", OnlyInt: true},
			&core.BoolField{Name: "binary"}, &core.TextField{Name: "content", Max: 12 << 20},
		)
		dates(attachments)
		return app.Save(attachments)
	})
}
