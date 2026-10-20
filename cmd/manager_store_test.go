package main

import (
	"context"
	"io"
	"log"
	"testing"

	"github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestSQLiteAdvanceBatchCursorUsesCreatedAndRecordID(t *testing.T) {
	cursor := sqliteBatchCursor{}
	rows := []sqliteStoreSubscriberRow{
		{
			RecordID:  "sub-001",
			CreatedAt: "2026-03-26 14:00:00.000Z",
			UpdatedAt: "2026-03-26 14:01:00.000Z",
		},
		{
			RecordID:  "sub-002",
			CreatedAt: "2026-03-26 14:00:00.000Z",
			UpdatedAt: "2026-03-26 14:10:00.000Z",
		},
		{
			RecordID:  "sub-003",
			CreatedAt: "2026-03-26 14:00:01.000Z",
			UpdatedAt: "2026-03-26 14:30:00.000Z",
		},
	}

	cursor = sqliteAdvanceBatchCursor(cursor, rows, 2)
	if cursor.LastCreated != "2026-03-26 14:00:00.000Z" {
		t.Fatalf("expected last_created to track stable created_at, got %q", cursor.LastCreated)
	}
	if cursor.LastID != "sub-002" {
		t.Fatalf("expected last_id to track stable subscriber record id, got %q", cursor.LastID)
	}
}

func TestSQLiteAdvanceBatchCursorUnaffectedByUpdatedAtOnlyChanges(t *testing.T) {
	base := sqliteBatchCursor{}

	rowsBefore := []sqliteStoreSubscriberRow{
		{
			RecordID:  "sub-002",
			CreatedAt: "2026-03-26 14:00:00.000Z",
			UpdatedAt: "2026-03-26 14:10:00.000Z",
		},
	}
	rowsAfter := []sqliteStoreSubscriberRow{
		{
			RecordID:  "sub-002",
			CreatedAt: "2026-03-26 14:00:00.000Z",
			UpdatedAt: "2026-03-26 16:45:00.000Z", // eg: bounce/status side effect
		},
	}

	before := sqliteAdvanceBatchCursor(base, rowsBefore, 1)
	after := sqliteAdvanceBatchCursor(base, rowsAfter, 1)

	if before.LastCreated != after.LastCreated {
		t.Fatalf("cursor last_created changed due to updated_at-only change: before=%q after=%q", before.LastCreated, after.LastCreated)
	}
	if before.LastID != after.LastID {
		t.Fatalf("cursor last_id changed due to updated_at-only change: before=%q after=%q", before.LastID, after.LastID)
	}
}

func TestSQLiteAdvanceBatchCursorStaysMonotonicWhenLastSubscriberDropsOut(t *testing.T) {
	// Batch 1 processes sub-001 and sub-002; pointer is set to sub-002.
	cursor := sqliteAdvanceBatchCursor(sqliteBatchCursor{}, []sqliteStoreSubscriberRow{
		{
			RecordID:  "sub-001",
			CreatedAt: "2026-03-26 14:00:00.000Z",
		},
		{
			RecordID:  "sub-002",
			CreatedAt: "2026-03-26 14:00:00.000Z",
		},
	}, 2)

	// Between batches, sub-002 becomes ineligible (eg: unsubscribed/blocklisted),
	// so it is no longer present in fetched rows. The cursor must still move forward.
	next := sqliteAdvanceBatchCursor(cursor, []sqliteStoreSubscriberRow{
		{
			RecordID:  "sub-003",
			CreatedAt: "2026-03-26 14:00:01.000Z",
		},
	}, 1)

	if next.LastCreated != "2026-03-26 14:00:01.000Z" {
		t.Fatalf("expected cursor to advance to the next eligible row, got last_created=%q", next.LastCreated)
	}
	if next.LastID != "sub-003" {
		t.Fatalf("expected cursor to advance to sub-003, got last_id=%q", next.LastID)
	}
}

func TestMarkSMSUnsendableExactPhoneAndSkipPending(t *testing.T) {
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}

	ddl := `
CREATE TABLE campaigns (
  id TEXT NOT NULL PRIMARY KEY,
  type TEXT,
  status TEXT,
  messenger TEXT DEFAULT 'email',
  to_send INTEGER DEFAULT 0,
  sent INTEGER DEFAULT 0,
  updated TEXT
);
CREATE TABLE lists (id TEXT NOT NULL PRIMARY KEY);
CREATE TABLE subscribers (
  id TEXT NOT NULL PRIMARY KEY,
  uuid TEXT,
  email TEXT,
  phone TEXT,
  status TEXT
);
CREATE TABLE subscriber_lists (
  subscriber_id TEXT NOT NULL,
  list_id TEXT NOT NULL,
  status TEXT,
  sms_status TEXT,
  updated TEXT
);
CREATE TABLE campaign_send_ledger (
  id TEXT NOT NULL UNIQUE,
  campaign_id TEXT NOT NULL,
  subscriber_id TEXT NOT NULL,
  message_id TEXT,
  status TEXT NOT NULL,
  created TEXT,
  updated TEXT,
  UNIQUE (campaign_id, subscriber_id)
);
`
	if _, err := db.ExecContext(context.Background(), ddl); err != nil {
		t.Fatalf("schema: %v", err)
	}

	mustStoreExec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("exec: %v\nquery: %s", err, query)
		}
	}
	mustStoreGet := func(dest any, query string, args ...any) {
		t.Helper()
		if err := db.Get(dest, query, args...); err != nil {
			t.Fatalf("get: %v\nquery: %s", err, query)
		}
	}

	mustStoreExec(`INSERT INTO subscribers (id, uuid, email, phone, status) VALUES ('sub_a', 'u-a', 'a@b.c', '+15551234567', 'enabled')`)
	mustStoreExec(`INSERT INTO subscribers (id, uuid, email, phone, status) VALUES ('sub_b', 'u-b', 'b@b.c', '+15551234567', 'enabled')`)
	mustStoreExec(`INSERT INTO subscribers (id, uuid, email, phone, status) VALUES ('sub_c', 'u-c', 'c@b.c', '(555) 123-4567', 'enabled')`)
	mustStoreExec(`INSERT INTO lists (id) VALUES ('list_1')`)
	mustStoreExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status, sms_status) VALUES ('sub_a', 'list_1', 'confirmed', 'confirmed')`)
	mustStoreExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status, sms_status) VALUES ('sub_b', 'list_1', 'confirmed', 'confirmed')`)
	mustStoreExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status, sms_status) VALUES ('sub_c', 'list_1', 'confirmed', 'confirmed')`)
	mustStoreExec(`INSERT INTO campaigns (id, type, status, messenger) VALUES ('quo_a', 'broadcast', 'running', 'quo')`)
	mustStoreExec(`INSERT INTO campaigns (id, type, status, messenger) VALUES ('quo_b', 'broadcast', 'running', 'quo')`)
	mustStoreExec(`INSERT INTO campaigns (id, type, status, messenger) VALUES ('email_a', 'broadcast', 'running', 'email')`)
	mustStoreExec(`INSERT INTO campaign_send_ledger (id, campaign_id, subscriber_id, status, created, updated) VALUES ('l1', 'quo_a', 'sub_a', 'pending', '2026-01-01', '2026-01-01')`)
	mustStoreExec(`INSERT INTO campaign_send_ledger (id, campaign_id, subscriber_id, status, created, updated) VALUES ('l2', 'quo_b', 'sub_a', 'inflight', '2026-01-01', '2026-01-01')`)
	mustStoreExec(`INSERT INTO campaign_send_ledger (id, campaign_id, subscriber_id, status, created, updated) VALUES ('l3', 'email_a', 'sub_a', 'pending', '2026-01-01', '2026-01-01')`)
	mustStoreExec(`INSERT INTO campaign_send_ledger (id, campaign_id, subscriber_id, status, created, updated) VALUES ('l4', 'quo_a', 'sub_b', 'pending', '2026-01-01', '2026-01-01')`)
	mustStoreExec(`INSERT INTO campaign_send_ledger (id, campaign_id, subscriber_id, status, created, updated) VALUES ('l5', 'quo_a', 'sub_c', 'pending', '2026-01-01', '2026-01-01')`)

	s := &store{
		db:   pbdb.NewFromSQLX(db),
		core: core.New(&core.Opt{DB: pbdb.NewFromSQLX(db), Log: log.New(io.Discard, "", 0)}, nil),
		log:  log.New(io.Discard, "", 0),
	}

	n, err := s.MarkSMSUnsendable("sub_a", "+15551234567")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("opted out %d memberships, want 2 exact-phone siblings", n)
	}

	var smsA, smsB, smsC string
	mustStoreGet(&smsA, `SELECT sms_status FROM subscriber_lists WHERE subscriber_id = 'sub_a'`)
	mustStoreGet(&smsB, `SELECT sms_status FROM subscriber_lists WHERE subscriber_id = 'sub_b'`)
	mustStoreGet(&smsC, `SELECT sms_status FROM subscriber_lists WHERE subscriber_id = 'sub_c'`)
	if smsA != "unsubscribed" || smsB != "unsubscribed" {
		t.Fatalf("siblings sms_status = %q/%q, want unsubscribed", smsA, smsB)
	}
	if smsC != "confirmed" {
		t.Fatalf("last-10-only number should not be scanned on the worker path, got %q", smsC)
	}

	assertLedger := func(id, want string) {
		t.Helper()
		var st string
		mustStoreGet(&st, `SELECT status FROM campaign_send_ledger WHERE id = ?`, id)
		if st != want {
			t.Fatalf("ledger %s status = %q, want %q", id, st, want)
		}
	}
	assertLedger("l1", "skipped")
	assertLedger("l2", "inflight")
	assertLedger("l3", "pending")
	assertLedger("l4", "skipped")
	assertLedger("l5", "pending")
}
