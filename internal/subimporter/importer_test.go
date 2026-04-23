package subimporter

import (
	"database/sql"
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

func TestMergeImportTagsPreservesExistingAttribs(t *testing.T) {
	attribs := models.JSON{
		"name": "Jane",
		"tags": []any{"existing"},
	}

	got := mergeImportTags(attribs, []string{"vip"})
	want := models.JSON{
		"name": "Jane",
		"tags": []string{"existing", "vip"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeImportTags()=%v, want %v", got, want)
	}
}

func TestGetSubscriberAttribs(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE subscribers (email TEXT PRIMARY KEY, attribs TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO subscribers (email, attribs) VALUES (?, ?)`, "john@example.com", `{"city":"Berlin","tags":["existing"]}`); err != nil {
		t.Fatalf("insert row: %v", err)
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	got, found, err := getSubscriberAttribs(tx, "john@example.com")
	if err != nil {
		t.Fatalf("getSubscriberAttribs found row: %v", err)
	}
	if !found {
		t.Fatalf("expected row to be found")
	}
	want := models.JSON{
		"city": "Berlin",
		"tags": []any{"existing"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("getSubscriberAttribs()=%v, want %v", got, want)
	}

	missing, found, err := getSubscriberAttribs(tx, "missing@example.com")
	if err != nil {
		t.Fatalf("getSubscriberAttribs missing row: %v", err)
	}
	if found {
		t.Fatalf("expected missing row")
	}
	if missing != nil {
		t.Fatalf("expected nil attribs for missing row, got %v", missing)
	}
}
