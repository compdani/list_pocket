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

		field := collection.Fields.GetByName("content_type")
		if field == nil {
			return app.Save(collection)
		}

		selectField, ok := field.(*core.SelectField)
		if !ok {
			return app.Save(collection)
		}

		selectField.Values = []string{"richtext", "html", "markdown", "plain", "visual"}
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("campaigns")
		if err != nil {
			return err
		}

		field := collection.Fields.GetByName("content_type")
		if field == nil {
			return app.Save(collection)
		}

		selectField, ok := field.(*core.SelectField)
		if !ok {
			return app.Save(collection)
		}

		selectField.Values = []string{"richtext", "html", "markdown", "plain"}
		return app.Save(collection)
	})
}
