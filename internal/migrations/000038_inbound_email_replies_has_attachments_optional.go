package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("inbound_email_replies")
		if err != nil {
			return nil
		}

		collection.Fields.RemoveByName("has_attachments")
		collection.Fields.Add(&core.BoolField{
			Name:     "has_attachments",
			Required: false,
		})

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("inbound_email_replies")
		if err != nil {
			return nil
		}

		collection.Fields.RemoveByName("has_attachments")
		collection.Fields.Add(&core.BoolField{
			Name:     "has_attachments",
			Required: true,
		})

		return app.Save(collection)
	})
}
