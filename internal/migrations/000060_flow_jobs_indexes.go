package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// The queue claim query filters on target_id and locked_until; without these
// indexes every claim scans the whole table while holding the SQLite writer.
func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("flow_jobs")
		if err != nil {
			return err
		}
		col.AddIndex("idx_flow_jobs_target_locked", false, "target_id, locked_until", "")
		col.AddIndex("idx_flow_jobs_available_locked", false, "available_at, locked_until", "")
		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("flow_jobs")
		if err != nil {
			return nil
		}
		col.RemoveIndex("idx_flow_jobs_target_locked")
		col.RemoveIndex("idx_flow_jobs_available_locked")
		return app.Save(col)
	})
}
