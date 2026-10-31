package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	// Rules stay nil (superuser only) on all of these. Credentials and people's agent
	// conversations are only ever reached through permission-checked routes.
	m.Register(func(app core.App) error {
		text := func(name string, required bool) core.Field {
			return &core.TextField{Name: name, Required: required, Max: 10_000}
		}
		secret := func(name string) core.Field { return &core.TextField{Name: name, Hidden: true, Max: 20_000} }
		jsonField := func(name string, max int64) core.Field { return &core.JSONField{Name: name, MaxSize: max} }
		dates := func(c *core.Collection) {
			c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true}, &core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		}

		settings := core.NewBaseCollection("ai_settings")
		settings.Fields.Add(text("key", true), jsonField("value", 1<<16), text("description", false))
		dates(settings)
		settings.AddIndex("idx_ai_settings_key", true, "key", "")
		if err := app.Save(settings); err != nil {
			return err
		}

		creds := core.NewBaseCollection("oauth_credentials")
		creds.Fields.Add(
			text("provider", true), text("sub", false), text("email", false), text("client_id", false),
			secret("id_token"), secret("access_token"), secret("refresh_token"),
			jsonField("scopes", 1<<16), &core.DateField{Name: "expires_at"}, text("host_id", false),
		)
		dates(creds)
		creds.AddIndex("idx_oauth_credentials_provider", true, "provider", "")
		if err := app.Save(creds); err != nil {
			return err
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
	}, func(app core.App) error {
		for _, name := range []string{"agent_attachments", "agent_messages", "agent_conversations", "oauth_credentials", "ai_settings"} {
			col, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				continue
			}
			if err := app.Delete(col); err != nil {
				return err
			}
		}
		return nil
	})
}
