package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		_, err := app.DB().NewQuery(`
CREATE TABLE IF NOT EXISTS whatsapp_channel_posts (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  channel_id      TEXT NOT NULL,
  channel_name    TEXT NOT NULL DEFAULT '',
  kind            TEXT NOT NULL,
  body            TEXT NOT NULL DEFAULT '',
  media_url       TEXT NOT NULL DEFAULT '',
  send_at         TEXT NOT NULL DEFAULT '',
  status          TEXT NOT NULL,
  provider_msg_id TEXT NOT NULL DEFAULT '',
  error           TEXT NOT NULL DEFAULT '',
  created_by      TEXT NOT NULL DEFAULT '',
  created         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%fZ', 'now')),
  updated         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%fZ', 'now'))
);
CREATE INDEX IF NOT EXISTS idx_whatsapp_channel_posts_due ON whatsapp_channel_posts (status, send_at);
`).Execute()
		return err
	}, func(app core.App) error {
		_, err := app.DB().NewQuery(`DROP TABLE IF EXISTS whatsapp_channel_posts;`).Execute()
		return err
	})
}
