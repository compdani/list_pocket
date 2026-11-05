package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		col := core.NewBaseCollection("flow_deliveries")
		col.Fields.Add(&core.TextField{Name: "execution_key", Required: true}, &core.JSONField{Name: "message", MaxSize: 32 << 20}, &core.TextField{Name: "status", Required: true}, &core.TextField{Name: "error"})
		col.AddIndex("idx_flow_delivery_execution", true, "execution_key", "")
		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("flow_deliveries")
		if err != nil {
			return err
		}
		return app.Delete(col)
	})
}
