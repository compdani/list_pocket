package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

const largeBodyFieldMax = 1000000

func setTextFieldMax(collection *core.Collection, name string, max int) {
	field := collection.Fields.GetByName(name)
	if field == nil {
		return
	}

	textField, ok := field.(*core.TextField)
	if !ok {
		return
	}

	textField.Max = max
}

func init() {
	m.Register(func(app core.App) error {
		for _, collectionName := range []string{"campaigns", "templates"} {
			collection, err := app.FindCollectionByNameOrId(collectionName)
			if err != nil {
				return err
			}

			for _, fieldName := range []string{"body", "body_source", "altbody"} {
				setTextFieldMax(collection, fieldName, largeBodyFieldMax)
			}

			if err := app.Save(collection); err != nil {
				return err
			}
		}

		return nil
	}, func(app core.App) error {
		for _, collectionName := range []string{"campaigns", "templates"} {
			collection, err := app.FindCollectionByNameOrId(collectionName)
			if err != nil {
				return err
			}

			for _, fieldName := range []string{"body", "body_source", "altbody"} {
				setTextFieldMax(collection, fieldName, 0)
			}

			if err := app.Save(collection); err != nil {
				return err
			}
		}

		return nil
	})
}
