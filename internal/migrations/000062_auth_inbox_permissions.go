package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		for _, name := range []string{"users", "inbound_email_replies", "inbound_sms_events", "inbound_email_attachments", "inbound_spam_rules"} {
			col, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			// Application routes own role authorization. Lock native record APIs;
			// server-side Record Save and the auth response pipeline remain available.
			col.ListRule = nil
			col.ViewRule = nil
			col.CreateRule = nil
			col.UpdateRule = nil
			col.DeleteRule = nil
			if err := app.Save(col); err != nil {
				return err
			}
		}
		return nil
	}, nil)
}
