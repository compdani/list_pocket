package subimporter

import (
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"reflect"
	"testing"

	"github.com/compdani/list_pocket/models"
	_ "modernc.org/sqlite"
)

func TestNormalizeImportTags(t *testing.T) {
	in := []string{" alpha ", "BETA", "beta", "", "  ", "Alpha", "gamma"}
	got := normalizeImportTags(in)
	want := []string{"alpha", "BETA", "gamma"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeImportTags()=%v, want %v", got, want)
	}
}

func TestMergeImportTagsAddsTagsToEmptyAttribs(t *testing.T) {
	got := mergeImportTags(nil, []string{"news", "vip"})
	want := models.JSON{
		"tags": []string{"news", "vip"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeImportTags()=%v, want %v", got, want)
	}
}

func TestMergeImportTagsMergesExistingTags(t *testing.T) {
	attribs := models.JSON{
		"tags": []any{"existing", "VIP"},
	}

	got := mergeImportTags(attribs, []string{"vip", "new"})
	want := models.JSON{
		"tags": []string{"existing", "VIP", "new"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeImportTags()=%v, want %v", got, want)
	}
}

func TestApplyImportTagsAddsTagsWithoutOverwritingAttribs(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
CREATE TABLE subscribers (
	email TEXT PRIMARY KEY,
	attribs TEXT,
	updated TEXT
);
`)
	if err != nil {
		t.Fatalf("create subscribers table error = %v", err)
	}

	initial := models.JSON{
		"city": "Berlin",
		"tags": []string{"existing"},
	}
	_, err = db.Exec(`INSERT INTO subscribers (email, attribs, updated) VALUES (?, ?, '')`, "user@example.com", initial)
	if err != nil {
		t.Fatalf("insert subscriber error = %v", err)
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("db.Begin() error = %v", err)
	}

	s := &Session{
		log: log.New(io.Discard, "", 0),
		opt: SessionOpt{Tags: []string{"VIP", "new"}},
	}

	if err := s.applyImportTags(tx, "user@example.com"); err != nil {
		t.Fatalf("applyImportTags() error = %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("tx.Commit() error = %v", err)
	}

	var rawAttribs string
	if err := db.QueryRow(`SELECT attribs FROM subscribers WHERE email = ?`, "user@example.com").Scan(&rawAttribs); err != nil {
		t.Fatalf("query attribs error = %v", err)
	}

	var got models.JSON
	if err := json.Unmarshal([]byte(rawAttribs), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if got["city"] != "Berlin" {
		t.Fatalf("city attribute should remain unchanged, got %v", got["city"])
	}

	tags := tagsFromAny(got["tags"])
	wantTags := []string{"existing", "VIP", "new"}
	if !reflect.DeepEqual(tags, wantTags) {
		t.Fatalf("merged tags=%v, want %v", tags, wantTags)
	}
}
