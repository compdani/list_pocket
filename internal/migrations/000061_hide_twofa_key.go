package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		if field := col.Fields.GetByName("twofa_key"); field != nil {
			field.SetHidden(true)
			return app.Save(col)
		}
		return nil
	}, nil) // Do not re-expose credentials on rollback.
}
