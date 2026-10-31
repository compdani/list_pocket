package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// A contact can now be on many boards, so a stage is no longer one field on the
		// profile. The fixed pipeline becomes an ordinary board. Move history stores names
		// rather than links, so it still reads correctly after a board or stage is renamed
		// or deleted. Cleanup triggers are created at boot by EnsureCRMTriggers.
		_, err := app.DB().NewQuery(`
CREATE TABLE IF NOT EXISTS boards (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  key      TEXT NOT NULL UNIQUE,
  name     TEXT NOT NULL,
  position INTEGER NOT NULL DEFAULT 0,
  created  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%fZ', 'now')),
  updated  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS board_stages (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  board    INTEGER NOT NULL,
  key      TEXT NOT NULL,
  name     TEXT NOT NULL,
  color    TEXT NOT NULL DEFAULT 'default',
  position INTEGER NOT NULL DEFAULT 0,
  UNIQUE (board, key)
);

CREATE TABLE IF NOT EXISTS board_cards (
  board      INTEGER NOT NULL,
  subscriber TEXT NOT NULL,
  stage      INTEGER NOT NULL,
  position   INTEGER NOT NULL DEFAULT 0,
  moved_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%fZ', 'now')),
  PRIMARY KEY (board, subscriber)
);
CREATE INDEX IF NOT EXISTS idx_board_cards_stage ON board_cards (stage, position);
CREATE INDEX IF NOT EXISTS idx_board_cards_subscriber ON board_cards (subscriber);

CREATE TABLE IF NOT EXISTS board_card_events (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  subscriber      TEXT NOT NULL,
  board_name      TEXT NOT NULL,
  from_stage_name TEXT NOT NULL DEFAULT '',
  to_stage_name   TEXT NOT NULL DEFAULT '',
  actor_name      TEXT NOT NULL DEFAULT '',
  created         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%fZ', 'now'))
);
CREATE INDEX IF NOT EXISTS idx_board_card_events_subscriber ON board_card_events (subscriber, created);

INSERT INTO boards (key, name) VALUES ('pipeline', 'Pipeline');
INSERT INTO board_stages (board, key, name, color, position)
SELECT b.id, s.key, s.name, s.color, s.position
FROM boards b, (
  SELECT 'lead' AS key, 'Lead' AS name, 'info' AS color, 0 AS position
  UNION ALL SELECT 'prospect', 'Prospect', 'warning', 1
  UNION ALL SELECT 'customer', 'Customer', 'success', 2
  UNION ALL SELECT 'inactive', 'Inactive', 'default', 3
) s
WHERE b.key = 'pipeline';

INSERT INTO board_cards (board, subscriber, stage, position, moved_at)
SELECT s.board, p.subscriber, s.id,
  ROW_NUMBER() OVER (PARTITION BY p.stage ORDER BY datetime(p.updated) DESC) - 1, p.updated
FROM contact_profiles p
JOIN board_stages s ON s.key = p.stage
JOIN boards b ON b.id = s.board AND b.key = 'pipeline';

DROP INDEX IF EXISTS idx_contact_profiles_stage;
ALTER TABLE contact_profiles DROP COLUMN stage;

UPDATE roles SET permissions = json_insert(permissions, '$[#]', 'boards:get')
WHERE type = 'user'
  AND EXISTS (SELECT 1 FROM json_each(roles.permissions) WHERE value IN ('subscribers:get_all', 'subscribers:*'))
  AND NOT EXISTS (SELECT 1 FROM json_each(roles.permissions) WHERE value = 'boards:get');
UPDATE roles SET permissions = json_insert(permissions, '$[#]', 'boards:manage')
WHERE type = 'user'
  AND EXISTS (SELECT 1 FROM json_each(roles.permissions) WHERE value IN ('subscribers:manage', 'subscribers:*'))
  AND NOT EXISTS (SELECT 1 FROM json_each(roles.permissions) WHERE value = 'boards:manage');
`).Execute()
		return err
	}, func(app core.App) error {
		_, err := app.DB().NewQuery(`
ALTER TABLE contact_profiles ADD COLUMN stage TEXT NOT NULL DEFAULT '';
INSERT INTO contact_profiles (subscriber, stage)
SELECT c.subscriber, s.key
FROM board_cards c
JOIN board_stages s ON s.id = c.stage
JOIN boards b ON b.id = c.board AND b.key = 'pipeline'
  AND s.key IN ('lead', 'prospect', 'customer', 'inactive')
WHERE true
ON CONFLICT (subscriber) DO UPDATE SET stage = excluded.stage;
CREATE INDEX IF NOT EXISTS idx_contact_profiles_stage ON contact_profiles (stage, updated);

UPDATE roles SET permissions = (SELECT json_group_array(value) FROM json_each(roles.permissions) WHERE value NOT LIKE 'boards:%')
WHERE EXISTS (SELECT 1 FROM json_each(roles.permissions) WHERE value LIKE 'boards:%');

DROP TRIGGER IF EXISTS trg_subscribers_boards_cleanup;
DROP TABLE IF EXISTS board_card_events;
DROP TABLE IF EXISTS board_cards;
DROP TABLE IF EXISTS board_stages;
DROP TABLE IF EXISTS boards;`).Execute()
		return err
	})
}
