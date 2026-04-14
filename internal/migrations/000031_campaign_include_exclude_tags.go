package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("campaigns")
		if err != nil {
			return err
		}

		if col.Fields.GetByName("include_tags") == nil {
			col.Fields.Add(&core.JSONField{
				Name:     "include_tags",
				Required: false,
			})
		}
		if col.Fields.GetByName("exclude_tags") == nil {
			col.Fields.Add(&core.JSONField{
				Name:     "exclude_tags",
				Required: false,
			})
		}

		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("campaigns")
		if err != nil {
			return nil
		}

		col.Fields.RemoveByName("include_tags")
		col.Fields.RemoveByName("exclude_tags")
		return app.Save(col)
	})
}
