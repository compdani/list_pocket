package plugins

import (
	"database/sql"
	"errors"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// Plugin statuses recorded in the plugins table.
const (
	StatusDisabled = "disabled"
	StatusLoaded   = "loaded"
	StatusFailed   = "failed"
	StatusSafeMode = "safe_mode"
	StatusPending  = "pending_restart"
)

type record struct {
	PluginID  string `db:"plugin_id"`
	Version   string `db:"version"`
	Enabled   bool   `db:"enabled"`
	Status    string `db:"status"`
	LastError string `db:"last_error"`
}

type store struct {
	app core.App
}

func (s *store) list() ([]record, error) {
	var rows []record
	err := s.app.DB().NewQuery(`SELECT plugin_id, version, enabled, status, last_error FROM plugins ORDER BY plugin_id`).All(&rows)
	return rows, err
}

func (s *store) get(id string) (record, bool, error) {
	var row record
	err := s.app.DB().NewQuery(`SELECT plugin_id, version, enabled, status, last_error FROM plugins WHERE plugin_id = {:id}`).
		Bind(dbx.Params{"id": id}).One(&row)
	if errors.Is(err, sql.ErrNoRows) {
		return row, false, nil
	}
	return row, err == nil, err
}

func (s *store) upsert(r record) error {
	_, err := s.app.DB().NewQuery(`
INSERT INTO plugins (plugin_id, version, enabled, status, last_error)
VALUES ({:id}, {:version}, {:enabled}, {:status}, {:last_error})
ON CONFLICT (plugin_id) DO UPDATE SET
  version = excluded.version,
  enabled = excluded.enabled,
  status = excluded.status,
  last_error = excluded.last_error,
  updated = strftime('%Y-%m-%d %H:%M:%fZ', 'now')`).Bind(dbx.Params{
		"id": r.PluginID, "version": r.Version, "enabled": r.Enabled, "status": r.Status, "last_error": r.LastError,
	}).Execute()
	return err
}

func (s *store) setStatus(id, status, lastError string) error {
	_, err := s.app.DB().NewQuery(`
UPDATE plugins SET status = {:status}, last_error = {:last_error}, updated = strftime('%Y-%m-%d %H:%M:%fZ', 'now')
WHERE plugin_id = {:id}`).Bind(dbx.Params{"id": id, "status": status, "last_error": lastError}).Execute()
	return err
}

func (s *store) delete(id string) error {
	_, err := s.app.DB().NewQuery(`DELETE FROM plugins WHERE plugin_id = {:id}`).Bind(dbx.Params{"id": id}).Execute()
	return err
}

func (s *store) kvGet(id, key string) (string, bool, error) {
	var row struct {
		Value string `db:"value"`
	}
	err := s.app.DB().NewQuery(`
SELECT value FROM plugin_kv
WHERE plugin_id = {:id} AND key = {:key} AND (expires_at = 0 OR expires_at > {:now})`).
		Bind(dbx.Params{"id": id, "key": key, "now": time.Now().Unix()}).One(&row)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return row.Value, err == nil, err
}

func (s *store) kvSet(id, key, value string, ttlSeconds int64) error {
	_, err := s.app.DB().NewQuery(`
INSERT INTO plugin_kv (plugin_id, key, value, expires_at) VALUES ({:id}, {:key}, {:value}, {:expires})
ON CONFLICT (plugin_id, key) DO UPDATE SET value = excluded.value, expires_at = excluded.expires_at`).
		Bind(dbx.Params{"id": id, "key": key, "value": value, "expires": expiresAt(ttlSeconds)}).Execute()
	return err
}

func (s *store) kvDelete(id, key string) error {
	_, err := s.app.DB().NewQuery(`DELETE FROM plugin_kv WHERE plugin_id = {:id} AND key = {:key}`).
		Bind(dbx.Params{"id": id, "key": key}).Execute()
	return err
}

func (s *store) kvPurge(id string) error {
	_, err := s.app.DB().NewQuery(`DELETE FROM plugin_kv WHERE plugin_id = {:id}`).Bind(dbx.Params{"id": id}).Execute()
	return err
}

// kvOnce records key and reports whether this is the first time it was seen
// within its TTL. The insert is atomic, so concurrent deliveries can't both win.
func (s *store) kvOnce(id, key string, ttlSeconds int64) (bool, error) {
	if _, err := s.app.DB().NewQuery(`
DELETE FROM plugin_kv WHERE plugin_id = {:id} AND key = {:key} AND expires_at != 0 AND expires_at <= {:now}`).
		Bind(dbx.Params{"id": id, "key": key, "now": time.Now().Unix()}).Execute(); err != nil {
		return false, err
	}
	res, err := s.app.DB().NewQuery(`
INSERT OR IGNORE INTO plugin_kv (plugin_id, key, value, expires_at) VALUES ({:id}, {:key}, '1', {:expires})`).
		Bind(dbx.Params{"id": id, "key": key, "expires": expiresAt(ttlSeconds)}).Execute()
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func expiresAt(ttlSeconds int64) int64 {
	if ttlSeconds <= 0 {
		return 0
	}
	return time.Now().Unix() + ttlSeconds
}
