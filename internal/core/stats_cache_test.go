package core

import (
	"encoding/json"
	"io"
	"log"
	"testing"

	"github.com/compdani/list_pocket/internal/i18n"
	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/compdani/list_pocket/models"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/types"
	_ "modernc.org/sqlite"
)

func TestStatsCachePersistsOutsideSettings(t *testing.T) {
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`
CREATE TABLE listpocket_stats_cache (
  id TEXT PRIMARY KEY,
  cache_key TEXT NOT NULL UNIQUE,
  value TEXT,
  created TEXT,
  updated TEXT
);
CREATE TABLE subscribers (id TEXT PRIMARY KEY, status TEXT);
CREATE TABLE subscriber_lists (subscriber_id TEXT, list_id TEXT, status TEXT);
CREATE TABLE lists (id TEXT PRIMARY KEY, type TEXT, optin TEXT);
CREATE TABLE campaigns (id TEXT PRIMARY KEY, status TEXT, sent INTEGER);
CREATE TABLE campaign_unsubscribes (id TEXT PRIMARY KEY);
`); err != nil {
		t.Fatal(err)
	}

	lang, err := i18n.New([]byte(`{"_.code":"en","_.name":"English"}`))
	if err != nil {
		t.Fatal(err)
	}
	c := New(&Opt{
		Constants: Constants{CacheSlowQueries: true},
		DB:        pbdb.NewFromSQLX(db),
		I18n:      lang,
		Log:       log.New(io.Discard, "", 0),
	}, &Hooks{})

	if err := c.RefreshMatViews(false); err != nil {
		t.Fatalf("RefreshMatViews: %v", err)
	}
	raw, ok := c.readCachedJSON(cacheKeyDashboardCounts)
	if !ok || len(raw) == 0 {
		t.Fatal("expected dashboard counts in stats cache table")
	}
	got, err := c.GetDashboardCounts()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw) {
		t.Fatalf("memory/db mismatch: %s vs %s", got, raw)
	}
}

func TestUpdateSettingsPreservesUnknownKeys(t *testing.T) {
	existing := types.JSONText(`{"app.site_name":"Old","cache.dashboard_counts":{"total":1}}`)
	var stored types.JSONText
	c := New(&Opt{
		I18n: mustTestI18n(t),
		Log:  log.New(io.Discard, "", 0),
		GetSettings: func() (types.JSONText, error) {
			return existing, nil
		},
		SetSettings: func(v types.JSONText) error {
			stored = append(types.JSONText(nil), v...)
			return nil
		},
	}, &Hooks{})

	if err := c.UpdateSettings(models.Settings{AppSiteName: "New"}); err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(stored, &m); err != nil {
		t.Fatal(err)
	}
	if string(m["app.site_name"]) != `"New"` {
		t.Fatalf("site name = %s", m["app.site_name"])
	}
	if _, ok := m["cache.dashboard_counts"]; !ok {
		t.Fatal("expected unknown cache key to be preserved")
	}
}

func mustTestI18n(t *testing.T) *i18n.I18n {
	t.Helper()
	lang, err := i18n.New([]byte(`{"_.code":"en","_.name":"English"}`))
	if err != nil {
		t.Fatal(err)
	}
	return lang
}
