package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("campaigns")
		if err != nil {
			return err
		}

		// listmonk stores full RFC 5322 mailbox strings like:
		// "Team DREIA <team@dreia.info>", not bare email addresses.
		collection.Fields.RemoveByName("from_email")
		collection.Fields.Add(&core.TextField{
			Name:     "from_email",
			Required: false,
		})

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("campaigns")
		if err != nil {
			return err
		}

		collection.Fields.RemoveByName("from_email")
		collection.Fields.Add(&core.EmailField{
			Name:     "from_email",
			Required: false,
		})

		return app.Save(collection)
	})
}
