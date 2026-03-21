package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		subscribers, err := app.FindCollectionByNameOrId("subscribers")
		if err != nil {
			return err
		}

		templates, err := app.FindCollectionByNameOrId("templates")
		if err != nil {
			return err
		}

		links, err := app.FindCollectionByNameOrId("links")
		if err != nil {
			return err
		}

		txMessages := core.NewBaseCollection("transactional_messages")
		txMessages.Fields.Add(
			&core.TextField{Name: "uuid", Required: true},
			&core.RelationField{Name: "subscriber_id", CollectionId: subscribers.Id},
			&core.RelationField{Name: "template_id", CollectionId: templates.Id},
			&core.TextField{Name: "to_email", Required: true},
			&core.TextField{Name: "from_email", Required: true},
			&core.TextField{Name: "subject", Required: true},
			&core.TextField{Name: "content_type", Required: true},
			&core.TextField{Name: "messenger", Required: true},
			&core.SelectField{Name: "status", Required: true, Values: []string{"queued", "sent", "failed"}},
			&core.TextField{Name: "error"},
			&core.EditorField{Name: "body"},
			&core.JSONField{Name: "data"},
			&core.JSONField{Name: "headers"},
			&core.DateField{Name: "sent_at"},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		txMessages.AddIndex("idx_tx_messages_uuid", true, "uuid", "")
		txMessages.AddIndex("idx_tx_messages_subscriber_id", false, "subscriber_id", "")
		txMessages.AddIndex("idx_tx_messages_template_id", false, "template_id", "")
		txMessages.AddIndex("idx_tx_messages_to_email", false, "to_email", "")
		txMessages.AddIndex("idx_tx_messages_status", false, "status", "")
		if err := app.Save(txMessages); err != nil {
			return err
		}

		txViews := core.NewBaseCollection("tx_message_views")
		txViews.Fields.Add(
			&core.RelationField{Name: "tx_message_id", Required: true, CollectionId: txMessages.Id},
			&core.RelationField{Name: "subscriber_id", CollectionId: subscribers.Id},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		txViews.AddIndex("idx_tx_views_message_id", false, "tx_message_id", "")
		txViews.AddIndex("idx_tx_views_subscriber_id", false, "subscriber_id", "")
		if err := app.Save(txViews); err != nil {
			return err
		}

		txClicks := core.NewBaseCollection("tx_link_clicks")
		txClicks.Fields.Add(
			&core.RelationField{Name: "tx_message_id", Required: true, CollectionId: txMessages.Id},
			&core.RelationField{Name: "link_id", Required: true, CollectionId: links.Id},
			&core.RelationField{Name: "subscriber_id", CollectionId: subscribers.Id},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		txClicks.AddIndex("idx_tx_clicks_message_id", false, "tx_message_id", "")
		txClicks.AddIndex("idx_tx_clicks_link_id", false, "link_id", "")
		txClicks.AddIndex("idx_tx_clicks_subscriber_id", false, "subscriber_id", "")
		return app.Save(txClicks)
	}, func(app core.App) error {
		collections := []string{"tx_link_clicks", "tx_message_views", "transactional_messages"}
		for _, name := range collections {
			collection, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				continue
			}
			if err := app.Delete(collection); err != nil {
				return err
			}
		}
		return nil
	})
}
