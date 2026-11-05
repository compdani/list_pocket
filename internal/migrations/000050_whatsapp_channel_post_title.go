package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		_, err := app.DB().NewQuery(`ALTER TABLE whatsapp_channel_posts ADD COLUMN title TEXT NOT NULL DEFAULT '';`).Execute()
		return err
	}, func(app core.App) error {
		_, err := app.DB().NewQuery(`ALTER TABLE whatsapp_channel_posts DROP COLUMN title;`).Execute()
		return err
	})
}
