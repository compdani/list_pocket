package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("campaigns")
		if err != nil {
			return err
		}

		collection.Fields.RemoveByName("max_subscriber_id")
		collection.Fields.RemoveByName("last_subscriber_id")

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("campaigns")
		if err != nil {
			return err
		}

		collection.Fields.Add(&core.NumberField{
			Name: "max_subscriber_id",
		})
		collection.Fields.Add(&core.NumberField{
			Name: "last_subscriber_id",
		})

		return app.Save(collection)
	})
}
