package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collections := []string{"campaign_views", "tx_message_views"}
		for _, name := range collections {
			collection, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}

			if collection.Fields.GetByName("meta") == nil {
				collection.Fields.Add(&core.JSONField{Name: "meta"})
			}
			if collection.Fields.GetByName("is_suspected_privacy_open") == nil {
				collection.Fields.Add(&core.BoolField{Name: "is_suspected_privacy_open"})
			}

			if err := app.Save(collection); err != nil {
				return err
			}
		}

		return nil
	}, func(app core.App) error {
		collections := []string{"campaign_views", "tx_message_views"}
		for _, name := range collections {
			collection, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}

			collection.Fields.RemoveByName("meta")
			collection.Fields.RemoveByName("is_suspected_privacy_open")

			if err := app.Save(collection); err != nil {
				return err
			}
		}

		return nil
	})
}
