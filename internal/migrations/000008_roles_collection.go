package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("roles")
		if err != nil {
			_, _ = app.DB().NewQuery("DROP TABLE IF EXISTS roles").Execute()

			col = core.NewBaseCollection("roles")
			col.Fields.Add(
				&core.NumberField{Name: "legacy_id", Required: true, OnlyInt: true},
				&core.TextField{Name: "type", Required: true},
				&core.TextField{Name: "name"},
				&core.JSONField{Name: "permissions"},
				&core.TextField{Name: "parent_id"},
				&core.TextField{Name: "list_record_id"},
				&core.AutodateField{Name: "created", OnCreate: true},
				&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
			)
			col.AddIndex("idx_pb_roles_legacy_id", true, "legacy_id", "")
			col.AddIndex("idx_pb_roles_name", true, "type, name", "name IS NOT NULL AND name != ''")
			col.AddIndex("idx_pb_roles_parent_list", true, "parent_id, list_record_id", "list_record_id IS NOT NULL AND list_record_id != ''")

			if err := app.Save(col); err != nil {
				return err
			}
		}

		existing, err := app.FindRecordsByFilter("roles", "", "", 1, 0)
		if err == nil && len(existing) > 0 {
			return nil
		}

		rec := core.NewRecord(col)
		rec.Set("legacy_id", 1)
		rec.Set("type", "user")
		rec.Set("name", "Super Admin")
		rec.Set("permissions", []string{})
		return app.Save(rec)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("roles")
		if err != nil {
			return nil
		}

		return app.Delete(collection)
	})
}
