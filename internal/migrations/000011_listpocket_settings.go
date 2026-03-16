package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("listmonk_settings")
		if err == nil {
			collection.Name = "listpocket_settings"
			return app.Save(collection)
		}

		// Fresh installs after the rename still need the collection.
		if _, err := app.FindCollectionByNameOrId("listpocket_settings"); err == nil {
			return nil
		}

		collection = core.NewBaseCollection("listpocket_settings")
		collection.Fields.Add(
			&core.TextField{
				Name:     "value",
				Required: true,
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

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("listpocket_settings")
		if err != nil {
			return err
		}

		collection.Name = "listmonk_settings"
		return app.Save(collection)
	})
}
