package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("inbound_sms_events")
		if err != nil {
			return nil
		}

		collection.Fields.RemoveByName("is_stop_keyword")
		collection.Fields.Add(&core.BoolField{
			Name:     "is_stop_keyword",
			Required: false,
		})

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("inbound_sms_events")
		if err != nil {
			return nil
		}

		collection.Fields.RemoveByName("is_stop_keyword")
		collection.Fields.Add(&core.BoolField{
			Name:     "is_stop_keyword",
			Required: true,
		})

		return app.Save(collection)
	})
}
