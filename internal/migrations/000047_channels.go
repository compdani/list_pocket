package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// A missing consent row means no consent, so plugin channels fail closed
		// and existing subscribers need no backfill.
		if _, err := app.DB().NewQuery(`
CREATE TABLE IF NOT EXISTS subscriber_addresses (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  subscriber TEXT NOT NULL,
  channel    TEXT NOT NULL,
  address    TEXT NOT NULL,
  status     TEXT NOT NULL DEFAULT 'active',
  created    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%fZ', 'now')),
  UNIQUE (channel, address)
);
CREATE INDEX IF NOT EXISTS idx_subscriber_addresses_subscriber ON subscriber_addresses (subscriber, channel);

CREATE TABLE IF NOT EXISTS subscriber_channel_consent (
  subscriber TEXT NOT NULL,
  list       TEXT NOT NULL,
  channel    TEXT NOT NULL,
  status     TEXT NOT NULL,
  updated    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%fZ', 'now')),
  PRIMARY KEY (subscriber, list, channel)
);
CREATE INDEX IF NOT EXISTS idx_subscriber_channel_consent_channel ON subscriber_channel_consent (channel, status);

CREATE TABLE IF NOT EXISTS channel_events (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  channel         TEXT NOT NULL,
  direction       TEXT NOT NULL,
  kind            TEXT NOT NULL,
  status          TEXT NOT NULL DEFAULT '',
  provider_msg_id TEXT NOT NULL DEFAULT '',
  subscriber      TEXT NOT NULL DEFAULT '',
  campaign        TEXT NOT NULL DEFAULT '',
  address         TEXT NOT NULL DEFAULT '',
  body            TEXT NOT NULL DEFAULT '',
  payload         TEXT NOT NULL DEFAULT '',
  occurred_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%fZ', 'now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_events_provider ON channel_events (channel, provider_msg_id) WHERE provider_msg_id != '';
CREATE INDEX IF NOT EXISTS idx_channel_events_subscriber ON channel_events (subscriber, occurred_at);
`).Execute(); err != nil {
			return err
		}

		templates, err := app.FindCollectionByNameOrId("templates")
		if err != nil {
			return err
		}
		if templates.Fields.GetByName("channel") == nil {
			templates.Fields.Add(&core.TextField{Name: "channel"})
			if err := app.Save(templates); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		if templates, err := app.FindCollectionByNameOrId("templates"); err == nil && templates.Fields.GetByName("channel") != nil {
			templates.Fields.RemoveByName("channel")
			if err := app.Save(templates); err != nil {
				return err
			}
		}
		_, err := app.DB().NewQuery(`
DROP TABLE IF EXISTS channel_events;
DROP TABLE IF EXISTS subscriber_channel_consent;
DROP TABLE IF EXISTS subscriber_addresses;`).Execute()
		return err
	})
}
