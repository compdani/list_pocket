package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("templates")
		if err != nil {
			return err
		}

		field := collection.Fields.GetByName("type")
		if field == nil {
			return app.Save(collection)
		}

		selectField, ok := field.(*core.SelectField)
		if !ok {
			return app.Save(collection)
		}

		selectField.Values = []string{"campaign", "campaign_visual", "tx"}
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("templates")
		if err != nil {
			return err
		}

		field := collection.Fields.GetByName("type")
		if field == nil {
			return app.Save(collection)
		}

		selectField, ok := field.(*core.SelectField)
		if !ok {
			return app.Save(collection)
		}

		selectField.Values = []string{"campaign", "tx"}
		return app.Save(collection)
	})
}
