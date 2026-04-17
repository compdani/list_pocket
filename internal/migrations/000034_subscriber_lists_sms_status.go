package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("subscriber_lists")
		if err != nil {
			return err
		}
		if col.Fields.GetByName("sms_status") != nil {
			return nil
		}
		col.Fields.Add(&core.SelectField{
			Name:     "sms_status",
			Required: false,
			Values:   []string{"unconfirmed", "confirmed", "unsubscribed"},
		})
		if err := app.Save(col); err != nil {
			return err
		}
		// Initialize SMS consent from existing email list status (one-time parity).
		if _, err := app.DB().NewQuery(`
UPDATE subscriber_lists
SET sms_status = status
WHERE sms_status IS NULL OR sms_status = ''
`).Execute(); err != nil {
			return err
		}
		return nil
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("subscriber_lists")
		if err != nil {
			return nil
		}
		if col.Fields.GetByName("sms_status") == nil {
			return nil
		}
		col.Fields.RemoveByName("sms_status")
		return app.Save(col)
	})
}
