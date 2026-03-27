package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

const settingsValueFieldMax = 1000000

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("listpocket_settings")
		if err != nil {
			return err
		}

		setTextFieldMax(collection, "value", settingsValueFieldMax)
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("listpocket_settings")
		if err != nil {
			return err
		}

		setTextFieldMax(collection, "value", 0)
		return app.Save(collection)
	})
}
