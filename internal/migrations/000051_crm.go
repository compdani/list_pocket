package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// CRM data sits beside subscribers rather than on them, so the send path and
		// bulk subscriber queries stay as they are. Cleanup triggers are created at
		// boot by EnsureCRMTriggers.
		_, err := app.DB().NewQuery(`
CREATE TABLE IF NOT EXISTS contact_profiles (
  subscriber TEXT PRIMARY KEY,
  stage      TEXT NOT NULL DEFAULT '',
  owner      TEXT NOT NULL DEFAULT '',
  company    TEXT NOT NULL DEFAULT '',
  updated    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%fZ', 'now'))
);
CREATE INDEX IF NOT EXISTS idx_contact_profiles_stage ON contact_profiles (stage, updated);

CREATE TABLE IF NOT EXISTS contact_notes (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  subscriber  TEXT NOT NULL,
  body        TEXT NOT NULL,
  pinned      INTEGER NOT NULL DEFAULT 0,
  author      TEXT NOT NULL DEFAULT '',
  author_name TEXT NOT NULL DEFAULT '',
  created     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%fZ', 'now')),
  updated     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%fZ', 'now'))
);
CREATE INDEX IF NOT EXISTS idx_contact_notes_subscriber ON contact_notes (subscriber, created);

CREATE TABLE IF NOT EXISTS contact_tasks (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  subscriber   TEXT NOT NULL,
  title        TEXT NOT NULL,
  due_at       TEXT NOT NULL DEFAULT '',
  status       TEXT NOT NULL DEFAULT 'open',
  assignee     TEXT NOT NULL DEFAULT '',
  created_by   TEXT NOT NULL DEFAULT '',
  created      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%fZ', 'now')),
  completed_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_contact_tasks_subscriber ON contact_tasks (subscriber, status);
CREATE INDEX IF NOT EXISTS idx_contact_tasks_due ON contact_tasks (status, due_at);

CREATE TABLE IF NOT EXISTS conversation_state (
  key          TEXT PRIMARY KEY,
  last_read_at TEXT NOT NULL DEFAULT '',
  closed_at    TEXT NOT NULL DEFAULT ''
);
`).Execute()
		return err
	}, func(app core.App) error {
		_, err := app.DB().NewQuery(`
DROP TRIGGER IF EXISTS trg_subscribers_crm_cleanup;
DROP TABLE IF EXISTS conversation_state;
DROP TABLE IF EXISTS contact_tasks;
DROP TABLE IF EXISTS contact_notes;
DROP TABLE IF EXISTS contact_profiles;`).Execute()
		return err
	})
}
