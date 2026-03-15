package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		stmts := []string{
			`CREATE TABLE IF NOT EXISTS roles (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				type TEXT NOT NULL DEFAULT 'user',
				parent_id INTEGER NULL,
				list_id INTEGER NULL,
				permissions TEXT NOT NULL DEFAULT '[]',
				name TEXT NULL,
				created TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%fZ')),
				updated TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%fZ')),
				FOREIGN KEY(parent_id) REFERENCES roles(id) ON DELETE CASCADE,
				FOREIGN KEY(list_id) REFERENCES lists(id) ON DELETE CASCADE
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_roles ON roles(parent_id, list_id)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_roles_name ON roles(type, name) WHERE name IS NOT NULL`,
			`INSERT INTO roles (id, type, name, permissions)
			 VALUES (1, 'user', 'Super Admin', '[]')
			 ON CONFLICT(id) DO NOTHING`,
		}

		for _, stmt := range stmts {
			if _, err := app.DB().NewQuery(stmt).Execute(); err != nil {
				return err
			}
		}

		return nil
	}, func(app core.App) error {
		stmts := []string{
			`DROP INDEX IF EXISTS idx_roles_name`,
			`DROP INDEX IF EXISTS idx_roles`,
			`DROP TABLE IF EXISTS roles`,
		}

		for _, stmt := range stmts {
			if _, err := app.DB().NewQuery(stmt).Execute(); err != nil {
				return err
			}
		}

		return nil
	})
}
