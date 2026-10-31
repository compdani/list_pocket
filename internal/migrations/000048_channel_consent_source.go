package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// source says where consent came from: inbound, admin, sms_copy and so on.
		// pending_failure marks a permanent failure that arrived before the send
		// was recorded, so there was no subscriber to apply it to yet.
		_, err := app.DB().NewQuery(`
ALTER TABLE subscriber_channel_consent ADD COLUMN source TEXT NOT NULL DEFAULT '';
ALTER TABLE channel_events ADD COLUMN pending_failure INTEGER NOT NULL DEFAULT 0;
`).Execute()
		return err
	}, func(app core.App) error {
		_, err := app.DB().NewQuery(`
ALTER TABLE channel_events DROP COLUMN pending_failure;
ALTER TABLE subscriber_channel_consent DROP COLUMN source;
`).Execute()
		return err
	})
}
