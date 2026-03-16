package migrations

import (
	"database/sql"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

type legacyRoleRow struct {
	ID          int            `db:"id"`
	Type        string         `db:"type"`
	ParentID    sql.NullInt64  `db:"parent_id"`
	ListID      sql.NullInt64  `db:"list_id"`
	Permissions string         `db:"permissions"`
	Name        sql.NullString `db:"name"`
}

func init() {
	m.Register(func(app core.App) error {
		legacyTableExists := false
		if err := app.DB().NewQuery("SELECT name FROM sqlite_master WHERE type='table' AND name='roles'").One(&struct {
			Name string `db:"name"`
		}{}); err == nil {
			legacyTableExists = true
		}

		col, err := app.FindCollectionByNameOrId("roles")
		if err != nil {
			if legacyTableExists {
				if _, err := app.DB().NewQuery("ALTER TABLE roles RENAME TO roles_legacy").Execute(); err != nil {
					return err
				}
				legacyTableExists = false
			}

			col = core.NewBaseCollection("roles")
			col.Fields.Add(
				&core.NumberField{Name: "legacy_id", Required: true, OnlyInt: true},
				&core.TextField{Name: "type", Required: true},
				&core.TextField{Name: "name"},
				&core.JSONField{Name: "permissions"},
				&core.TextField{Name: "parent_id"},
				&core.NumberField{Name: "list_id", OnlyInt: true},
				&core.AutodateField{Name: "created", OnCreate: true},
				&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
			)
			col.AddIndex("idx_pb_roles_legacy_id", true, "legacy_id", "")
			col.AddIndex("idx_pb_roles_name", true, "type, name", "name IS NOT NULL AND name != ''")
			col.AddIndex("idx_pb_roles_parent_list", true, "parent_id, list_id", "list_id IS NOT NULL AND list_id != 0")

			if err := app.Save(col); err != nil {
				return err
			}
		}

		existing, err := app.FindRecordsByFilter("roles", "", "", 1, 0)
		if err == nil && len(existing) > 0 {
			return nil
		}

		rows := []legacyRoleRow{}
		if err := app.DB().NewQuery(`
			SELECT id, type, parent_id, list_id, permissions, name
			FROM roles_legacy
			ORDER BY id
		`).All(&rows); err != nil {
			// If the legacy SQL table doesn't exist or is empty, seed the primordial super admin role.
			rec := core.NewRecord(col)
			rec.Set("legacy_id", 1)
			rec.Set("type", "user")
			rec.Set("name", "Super Admin")
			rec.Set("permissions", []string{})
			return app.Save(rec)
		}

		idMap := make(map[int]string, len(rows))
		recordMap := make(map[int]*core.Record, len(rows))

		for _, row := range rows {
			rec := core.NewRecord(col)
			rec.Set("legacy_id", row.ID)
			rec.Set("type", row.Type)
			rec.Set("name", strings.TrimSpace(row.Name.String))
			rec.Set("permissions", row.Permissions)
			if row.ListID.Valid {
				rec.Set("list_id", int(row.ListID.Int64))
			}

			if err := app.Save(rec); err != nil {
				return err
			}

			idMap[row.ID] = rec.Id
			recordMap[row.ID] = rec
		}

		for _, row := range rows {
			if !row.ParentID.Valid {
				continue
			}

			rec := recordMap[row.ID]
			parentRecordID, ok := idMap[int(row.ParentID.Int64)]
			if !ok || rec == nil {
				continue
			}

			rec.Set("parent_id", parentRecordID)
			if err := app.Save(rec); err != nil {
				return err
			}
		}

		_, _ = app.DB().NewQuery("DROP TABLE IF EXISTS roles_legacy").Execute()

		return nil
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("roles")
		if err != nil {
			return nil
		}

		return app.Delete(collection)
	})
}
