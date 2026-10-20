package pbdb

import (
	"testing"

	"github.com/pocketbase/pocketbase"
)

func TestIsReadOnlySQL(t *testing.T) {
	cases := []struct {
		sql  string
		want bool
	}{
		{"SELECT * FROM subscribers", true},
		{"  select id from lists where id = ?", true},
		{"-- count\nSELECT COUNT(*) FROM subscribers", true},
		{"/* stats */ SELECT 1", true},
		{"WITH s AS (SELECT id FROM subscribers) SELECT COUNT(*) FROM s", true},
		{"INSERT INTO lists (name) VALUES (?) RETURNING id", false},
		{"UPDATE subscribers SET status = ?", false},
		{"DELETE FROM bounces", false},
		{"WITH s AS (SELECT id FROM subscribers) DELETE FROM lists WHERE id IN s", false},
		{"WITH s AS (SELECT id FROM t) UPDATE x SET y = 1", false},
		{"PRAGMA optimize", false},
		{"VACUUM", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isReadOnlySQL(c.sql); got != c.want {
			t.Errorf("isReadOnlySQL(%q) = %v, want %v", c.sql, got, c.want)
		}
	}
}

func TestNewFromPocketBaseRoutesReads(t *testing.T) {
	pb := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	if err := pb.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pb.ResetBootstrapState() })

	db, err := NewFromPocketBase(pb)
	if err != nil {
		t.Fatal(err)
	}
	if db.reader == nil || db.reader.DB == db.DB.DB {
		t.Fatal("expected a separate read pool")
	}
	if db.pick("SELECT 1") != db.reader || db.pick("DELETE FROM t") != db.DB {
		t.Fatal("queries routed to the wrong pool")
	}

	if _, err := db.Exec(`CREATE TABLE pbdb_t (v TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO pbdb_t (v) VALUES (?)`, "x"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.Get(&n, `SELECT COUNT(*) FROM pbdb_t`); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("read pool saw %d rows, want 1", n)
	}
}
