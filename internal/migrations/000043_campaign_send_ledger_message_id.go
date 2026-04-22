package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("campaign_send_ledger")
		if err != nil {
			return nil
		}

		if col.Fields.GetByName("message_id") == nil {
			col.Fields.Add(&core.TextField{
				Name:     "message_id",
				Required: false,
				Max:      255,
			})
		}

		col.AddIndex("idx_campaign_ledger_message_id", true, "message_id", "message_id IS NOT NULL AND message_id != ''")

		if err := app.Save(col); err != nil {
			return err
		}

		_, err = app.DB().NewQuery(`
UPDATE campaign_send_ledger
SET message_id = id || '@listpocket.local'
WHERE message_id IS NULL OR trim(message_id) = ''
`).Execute()
		return err
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("campaign_send_ledger")
		if err != nil {
			return nil
		}

		col.Fields.RemoveByName("message_id")
		return app.Save(col)
	})
}
