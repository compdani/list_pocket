package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// Plain tables, so nothing here is reachable through the PocketBase record API.
		_, err := app.DB().NewQuery(`
CREATE TABLE IF NOT EXISTS plugins (
  plugin_id  TEXT PRIMARY KEY NOT NULL,
  version    TEXT NOT NULL,
  enabled    INTEGER NOT NULL DEFAULT 0,
  status     TEXT NOT NULL DEFAULT 'disabled',
  last_error TEXT NOT NULL DEFAULT '',
  updated    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%fZ', 'now'))
);
CREATE TABLE IF NOT EXISTS plugin_kv (
  plugin_id  TEXT NOT NULL,
  key        TEXT NOT NULL,
  value      TEXT NOT NULL,
  expires_at INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (plugin_id, key)
);`).Execute()
		return err
	}, func(app core.App) error {
		_, err := app.DB().NewQuery(`DROP TABLE IF EXISTS plugin_kv; DROP TABLE IF EXISTS plugins;`).Execute()
		return err
	})
}
