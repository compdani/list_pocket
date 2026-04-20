package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		if existing, err := app.FindCollectionByNameOrId("inbound_email_attachments"); err == nil && existing != nil {
			return nil
		}

		replies, err := app.FindCollectionByNameOrId("inbound_email_replies")
		if err != nil {
			return err
		}

		collection := core.NewBaseCollection("inbound_email_attachments")
		authRule := "@request.auth.id != ''"
		collection.ListRule = &authRule
		collection.ViewRule = &authRule
		collection.CreateRule = &authRule
		collection.UpdateRule = &authRule
		collection.DeleteRule = &authRule

		collection.Fields.Add(
			&core.RelationField{
				Name:         "inbound_email_reply_id",
				Required:     true,
				CollectionId: replies.Id,
				MaxSelect:    1,
			},
			&core.FileField{
				Name:      "file",
				Required:  true,
				MaxSelect: 1,
				MaxSize:   20 * 1024 * 1024,
				Protected: true,
			},
			&core.TextField{
				Name:     "original_name",
				Required: false,
			},
			&core.TextField{
				Name:     "content_type",
				Required: false,
			},
			&core.TextField{
				Name:     "content_id",
				Required: false,
			},
			&core.TextField{
				Name:     "disposition",
				Required: false,
			},
			&core.NumberField{
				Name:     "size_bytes",
				Required: false,
				OnlyInt:  true,
			},
			&core.AutodateField{
				Name:     "created",
				OnCreate: true,
			},
			&core.AutodateField{
				Name:     "updated",
				OnCreate: true,
				OnUpdate: true,
			},
		)

		collection.AddIndex("idx_inbound_email_attachments_reply", false, "inbound_email_reply_id, created", "")

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("inbound_email_attachments")
		if err != nil {
			return nil
		}
		return app.Delete(collection)
	})
}
