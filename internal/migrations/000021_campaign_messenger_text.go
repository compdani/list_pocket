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

		// Campaign messenger names are dynamic (email + named SMTPs + postback),
		// so this cannot be a fixed select list.
		collection.Fields.RemoveByName("messenger")
		collection.Fields.Add(&core.TextField{
			Name:     "messenger",
			Required: true,
		})

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("campaigns")
		if err != nil {
			return err
		}

		collection.Fields.RemoveByName("messenger")
		collection.Fields.Add(&core.SelectField{
			Name:     "messenger",
			Required: true,
			Values:   []string{"email"},
		})

		return app.Save(collection)
	})
}
