package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("flow_agent_runs")
		if err != nil {
			return err
		}
		col.Fields.Add(&core.BoolField{Name: "side_effect_started"})
		if err := app.Save(col); err != nil {
			return err
		}
		// Legacy executions lack an intent journal: conservatively preserve safety.
		_, err = app.DB().NewQuery("UPDATE flow_agent_runs SET side_effect_started=1").Execute()
		return err
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("flow_agent_runs")
		if err != nil {
			return err
		}
		col.Fields.RemoveByName("side_effect_started")
		return app.Save(col)
	})
}
