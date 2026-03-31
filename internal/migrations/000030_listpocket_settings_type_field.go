package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("listpocket_settings")
		if err != nil {
			return nil
		}

		if col.Fields.GetByName("type") == nil {
			col.Fields.Add(&core.TextField{
				Name:     "type",
				Required: false,
			})
		}

		col.AddIndex("idx_listpocket_settings_type", true, "type", "")
		if err := app.Save(col); err != nil {
			return err
		}

		records, err := app.FindRecordsByFilter("listpocket_settings", `type=""`, "", 0, 0)
		if err == nil {
			for _, rec := range records {
				rec.Set("type", "app")
				if saveErr := app.Save(rec); saveErr != nil {
					return saveErr
				}
			}
		}

		return nil
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("listpocket_settings")
		if err != nil {
			return nil
		}
		col.Fields.RemoveByName("type")
		return app.Save(col)
	})
}
