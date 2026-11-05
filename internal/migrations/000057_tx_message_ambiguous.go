package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("transactional_messages")
		if err != nil {
			return err
		}
		field := col.Fields.GetByName("status").(*core.SelectField)
		field.Values = []string{"queued", "sent", "failed", "ambiguous"}
		return app.Save(col)
	}, func(app core.App) error {
		if _, err := app.DB().NewQuery("UPDATE transactional_messages SET status='failed' WHERE status='ambiguous'").Execute(); err != nil {
			return err
		}
		col, err := app.FindCollectionByNameOrId("transactional_messages")
		if err != nil {
			return err
		}
		col.Fields.GetByName("status").(*core.SelectField).Values = []string{"queued", "sent", "failed"}
		return app.Save(col)
	})
}
