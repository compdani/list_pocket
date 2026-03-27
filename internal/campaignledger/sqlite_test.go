package campaignledger

import (
	"context"
	"database/sql"
	"testing"

	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestPlaceholders(t *testing.T) {
	t.Parallel()
	if got := placeholders(0); got != "" {
		t.Fatalf("placeholders(0) = %q, want empty", got)
	}
	if got := placeholders(1); got != "?" {
		t.Fatalf("placeholders(1) = %q", got)
	}
	if got := placeholders(3); got != "?,?,?" {
		t.Fatalf("placeholders(3) = %q", got)
	}
}

func TestMarkSentRollbackInflight(t *testing.T) {
	db := newTestLedgerDB(t)
	campRec := "camp_rec_1"
	subRec := "sub_rec_1"
	mustExec(t, db, `INSERT INTO campaigns (id, type, status, to_send, sent) VALUES (?, 'broadcast', 'running', 0, 0)`, campRec)
	mustExec(t, db, `INSERT INTO subscribers (id, uuid, email, status) VALUES (?, 'u1', 'a@b.c', 'enabled')`, subRec)
	mustExec(t, db, `INSERT INTO campaign_send_ledger (id, campaign_id, subscriber_id, status, created, updated) VALUES ('l1', ?, ?, 'inflight', '2026-01-01', '2026-01-01')`, campRec, subRec)

	if err := MarkSent(db, campRec, subRec); err != nil {
		t.Fatal(err)
	}
	var st string
	mustGet(t, db, &st, `SELECT status FROM campaign_send_ledger WHERE campaign_id = ? AND subscriber_id = ?`, campRec, subRec)
	if st != "sent" {
		t.Fatalf("status after MarkSent: %q", st)
	}

	mustExec(t, db, `UPDATE campaign_send_ledger SET status = 'inflight' WHERE campaign_id = ? AND subscriber_id = ?`, campRec, subRec)
	if err := RollbackInflight(db, campRec, subRec); err != nil {
		t.Fatal(err)
	}
	mustGet(t, db, &st, `SELECT status FROM campaign_send_ledger WHERE campaign_id = ? AND subscriber_id = ?`, campRec, subRec)
	if st != "pending" {
		t.Fatalf("status after RollbackInflight: %q", st)
	}
}

func TestMarkSentOnlyInflight(t *testing.T) {
	db := newTestLedgerDB(t)
	campRec := "camp_rec_1"
	subRec := "sub_rec_1"
	mustExec(t, db, `INSERT INTO campaigns (id, type, status) VALUES (?, 'broadcast', 'running')`, campRec)
	mustExec(t, db, `INSERT INTO subscribers (id, uuid, email, status) VALUES (?, 'u1', 'a@b.c', 'enabled')`, subRec)
	mustExec(t, db, `INSERT INTO campaign_send_ledger (id, campaign_id, subscriber_id, status, created, updated) VALUES ('l1', ?, ?, 'pending', '2026-01-01', '2026-01-01')`, campRec, subRec)

	if err := MarkSent(db, campRec, subRec); err != nil {
		t.Fatal(err)
	}
	var st string
	mustGet(t, db, &st, `SELECT status FROM campaign_send_ledger WHERE campaign_id = ? AND subscriber_id = ?`, campRec, subRec)
	if st != "pending" {
		t.Fatalf("MarkSent should not update non-inflight row; got status %q", st)
	}
}

func TestFinalizeCampaignStats(t *testing.T) {
	db := newTestLedgerDB(t)
	campRec := "camp_rec_1"
	var campRowID int
	mustExec(t, db, `INSERT INTO campaigns (id, type, status, to_send, sent) VALUES (?, 'broadcast', 'running', 99, 99)`, campRec)
	mustGet(t, db, &campRowID, `SELECT rowid FROM campaigns WHERE id = ?`, campRec)

	sub1 := "sub_1"
	sub2 := "sub_2"
	mustExec(t, db, `INSERT INTO subscribers (id, uuid, email, status) VALUES (?, 'u1', 'a@b.c', 'enabled')`, sub1)
	mustExec(t, db, `INSERT INTO subscribers (id, uuid, email, status) VALUES (?, 'u2', 'b@b.c', 'enabled')`, sub2)

	sub3 := "sub_3"
	mustExec(t, db, `INSERT INTO subscribers (id, uuid, email, status) VALUES (?, 'u3', 'c@b.c', 'enabled')`, sub3)

	mustExec(t, db, `INSERT INTO campaign_send_ledger (id, campaign_id, subscriber_id, status, created, updated) VALUES ('l1', ?, ?, 'sent', '2026-01-01', '2026-01-01')`, campRec, sub1)
	mustExec(t, db, `INSERT INTO campaign_send_ledger (id, campaign_id, subscriber_id, status, created, updated) VALUES ('l2', ?, ?, 'pending', '2026-01-01', '2026-01-01')`, campRec, sub2)
	mustExec(t, db, `INSERT INTO campaign_send_ledger (id, campaign_id, subscriber_id, status, created, updated) VALUES ('l3', ?, ?, 'inflight', '2026-01-01', '2026-01-01')`, campRec, sub3)

	if err := FinalizeCampaignStats(db, campRowID, campRec); err != nil {
		t.Fatal(err)
	}
	var toSend, sent int
	mustGet(t, db, &toSend, `SELECT to_send FROM campaigns WHERE rowid = ?`, campRowID)
	mustGet(t, db, &sent, `SELECT sent FROM campaigns WHERE rowid = ?`, campRowID)
	if toSend != 3 || sent != 1 {
		t.Fatalf("FinalizeCampaignStats: to_send=%d sent=%d, want to_send=3 sent=1", toSend, sent)
	}
}

func TestSyncToSendFromLedger(t *testing.T) {
	db := newTestLedgerDB(t)
	campRec := "camp_rec_1"
	var campRowID int
	mustExec(t, db, `INSERT INTO campaigns (id, type, status, to_send, sent) VALUES (?, 'broadcast', 'running', 0, 0)`, campRec)
	mustGet(t, db, &campRowID, `SELECT rowid FROM campaigns WHERE id = ?`, campRec)
	sub1 := "sub_1"
	mustExec(t, db, `INSERT INTO subscribers (id, uuid, email, status) VALUES (?, 'u1', 'a@b.c', 'enabled')`, sub1)
	mustExec(t, db, `INSERT INTO campaign_send_ledger (id, campaign_id, subscriber_id, status, created, updated) VALUES ('l1', ?, ?, 'pending', '2026-01-01', '2026-01-01')`, campRec, sub1)

	if err := SyncToSendFromLedger(db, campRowID, campRec); err != nil {
		t.Fatal(err)
	}
	var toSend int
	mustGet(t, db, &toSend, `SELECT to_send FROM campaigns WHERE rowid = ?`, campRowID)
	if toSend != 1 {
		t.Fatalf("to_send = %d, want 1", toSend)
	}
}

func TestSyncToSendFromLedgerNoLedgerRows(t *testing.T) {
	db := newTestLedgerDB(t)
	campRec := "camp_rec_1"
	var campRowID int
	mustExec(t, db, `INSERT INTO campaigns (id, type, status, to_send, sent) VALUES (?, 'broadcast', 'running', 5, 0)`, campRec)
	mustGet(t, db, &campRowID, `SELECT rowid FROM campaigns WHERE id = ?`, campRec)

	if err := SyncToSendFromLedger(db, campRowID, campRec); err != nil {
		t.Fatal(err)
	}
	var toSend int
	mustGet(t, db, &toSend, `SELECT to_send FROM campaigns WHERE rowid = ?`, campRowID)
	if toSend != 5 {
		t.Fatalf("to_send unchanged when no ledger: got %d", toSend)
	}
}

func TestNextPending(t *testing.T) {
	pbdbDB := newTestLedgerPBDB(t)
	campRec := "camp_rec_1"
	mustExec(t, pbdbDB, `INSERT INTO campaigns (id, type, status) VALUES (?, 'broadcast', 'running')`, campRec)
	ids := []string{"l1", "l2", "l3"}
	for i, sid := range []string{"s1", "s2", "s3"} {
		mustExec(t, pbdbDB, `INSERT INTO subscribers (id, uuid, email, first_name, last_name, name, attribs, status, created, updated) VALUES (?, ?, ?, '', '', '', '', 'enabled', '2026-01-01', '2026-01-01')`, sid, sid, sid+"@t.c")
		mustExec(t, pbdbDB, `INSERT INTO campaign_send_ledger (id, campaign_id, subscriber_id, status, created, updated) VALUES (?, ?, ?, 'pending', '2026-01-01', '2026-01-02')`,
			ids[i], campRec, sid)
	}

	rows, hasMore, err := NextPending(pbdbDB, campRec, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !hasMore {
		t.Fatal("expected hasMore true with 3 pending and limit 2")
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows)=%d", len(rows))
	}

	rows2, hasMore2, err := NextPending(pbdbDB, campRec, 2)
	if err != nil {
		t.Fatal(err)
	}
	if hasMore2 {
		t.Fatal("expected hasMore false for last row")
	}
	if len(rows2) != 1 {
		t.Fatalf("second batch len=%d", len(rows2))
	}

	var inflight int
	mustGet(t, pbdbDB, &inflight, `SELECT COUNT(1) FROM campaign_send_ledger WHERE campaign_id = ? AND status = 'inflight'`, campRec)
	if inflight != 3 {
		t.Fatalf("inflight count = %d", inflight)
	}
}

func TestNextPendingEmpty(t *testing.T) {
	pbdbDB := newTestLedgerPBDB(t)
	campRec := "camp_rec_1"
	mustExec(t, pbdbDB, `INSERT INTO campaigns (id, type, status) VALUES (?, 'broadcast', 'running')`, campRec)

	rows, hasMore, err := NextPending(pbdbDB, campRec, 10)
	if err != nil {
		t.Fatal(err)
	}
	if hasMore || len(rows) != 0 {
		t.Fatalf("empty: rows=%d hasMore=%v", len(rows), hasMore)
	}
}

func TestBackfillIfEmpty(t *testing.T) {
	db := newTestLedgerDB(t)
	campRec := "camp_rec_1"
	listRec := "list_rec_1"
	subRec := "sub_rec_1"
	var campRowID int

	mustExec(t, db, `INSERT INTO campaigns (id, type, status, to_send, sent) VALUES (?, 'broadcast', 'scheduled', 0, 0)`, campRec)
	mustGet(t, db, &campRowID, `SELECT rowid FROM campaigns WHERE id = ?`, campRec)
	mustExec(t, db, `INSERT INTO lists (id, optin) VALUES (?, 'single')`, listRec)
	mustExec(t, db, `INSERT INTO subscribers (id, uuid, email, status) VALUES (?, 'u1', 'a@b.c', 'enabled')`, subRec)
	mustExec(t, db, `INSERT INTO campaign_lists (campaign_id, list_id) VALUES (?, ?)`, campRec, listRec)
	mustExec(t, db, `INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES (?, ?, 'confirmed')`, subRec, listRec)

	ran, err := BackfillIfEmpty(db, campRowID, campRec, "broadcast")
	if err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("expected backfill to run")
	}
	var n int
	mustGet(t, db, &n, `SELECT COUNT(1) FROM campaign_send_ledger WHERE campaign_id = ?`, campRec)
	if n != 1 {
		t.Fatalf("ledger rows = %d", n)
	}
	mustGet(t, db, &n, `SELECT to_send FROM campaigns WHERE rowid = ?`, campRowID)
	if n != 1 {
		t.Fatalf("to_send = %d", n)
	}

	ran2, err := BackfillIfEmpty(db, campRowID, campRec, "broadcast")
	if err != nil {
		t.Fatal(err)
	}
	if ran2 {
		t.Fatal("second backfill should be skipped when ledger non-empty")
	}
}

func TestInsertPendingIfEligible(t *testing.T) {
	db := newTestLedgerDB(t)
	campRec := "camp_rec_1"
	listRec := "list_rec_1"
	subRec := "sub_rec_1"

	mustExec(t, db, `INSERT INTO campaigns (id, type, status) VALUES (?, 'broadcast', 'scheduled')`, campRec)
	mustExec(t, db, `INSERT INTO lists (id, optin) VALUES (?, 'single')`, listRec)
	mustExec(t, db, `INSERT INTO subscribers (id, uuid, email, status) VALUES (?, 'u1', 'a@b.c', 'enabled')`, subRec)
	mustExec(t, db, `INSERT INTO campaign_lists (campaign_id, list_id) VALUES (?, ?)`, campRec, listRec)
	mustExec(t, db, `INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES (?, ?, 'confirmed')`, subRec, listRec)

	if err := InsertPendingIfEligible(db, listRec, subRec); err != nil {
		t.Fatal(err)
	}
	var n int
	mustGet(t, db, &n, `SELECT COUNT(1) FROM campaign_send_ledger WHERE campaign_id = ? AND subscriber_id = ?`, campRec, subRec)
	if n != 1 {
		t.Fatalf("expected one ledger row, got %d", n)
	}

	if err := InsertPendingIfEligible(db, listRec, subRec); err != nil {
		t.Fatal(err)
	}
	mustGet(t, db, &n, `SELECT COUNT(1) FROM campaign_send_ledger WHERE campaign_id = ? AND subscriber_id = ?`, campRec, subRec)
	if n != 1 {
		t.Fatalf("duplicate hook should be ignored; count = %d", n)
	}
}

func TestInsertPendingIfEligibleNotEligibleWhenPausedCampaignExcluded(t *testing.T) {
	db := newTestLedgerDB(t)
	campRec := "camp_rec_1"
	listRec := "list_rec_1"
	subRec := "sub_rec_1"

	mustExec(t, db, `INSERT INTO campaigns (id, type, status) VALUES (?, 'broadcast', 'draft')`, campRec)
	mustExec(t, db, `INSERT INTO lists (id, optin) VALUES (?, 'single')`, listRec)
	mustExec(t, db, `INSERT INTO subscribers (id, uuid, email, status) VALUES (?, 'u1', 'a@b.c', 'enabled')`, subRec)
	mustExec(t, db, `INSERT INTO campaign_lists (campaign_id, list_id) VALUES (?, ?)`, campRec, listRec)
	mustExec(t, db, `INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES (?, ?, 'confirmed')`, subRec, listRec)

	if err := InsertPendingIfEligible(db, listRec, subRec); err != nil {
		t.Fatal(err)
	}
	var n int
	mustGet(t, db, &n, `SELECT COUNT(1) FROM campaign_send_ledger`)
	if n != 0 {
		t.Fatalf("draft campaign should not get ledger rows, count=%d", n)
	}
}

func TestNoSecondDeliveryAfterSent(t *testing.T) {
	pbdbDB := newTestLedgerPBDB(t)
	campRec := "camp_rec_1"
	listRec := "list_rec_1"
	subRec := "sub_rec_1"

	mustExec(t, pbdbDB, `INSERT INTO campaigns (id, type, status) VALUES (?, 'broadcast', 'scheduled')`, campRec)
	mustExec(t, pbdbDB, `INSERT INTO lists (id, optin) VALUES (?, 'single')`, listRec)
	mustExec(t, pbdbDB, `INSERT INTO subscribers (id, uuid, email, first_name, last_name, name, attribs, status, created, updated) VALUES (?, 'u1', 'a@b.c', '', '', '', '', 'enabled', '2026-01-01', '2026-01-01')`, subRec)
	mustExec(t, pbdbDB, `INSERT INTO campaign_lists (campaign_id, list_id) VALUES (?, ?)`, campRec, listRec)
	mustExec(t, pbdbDB, `INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES (?, ?, 'confirmed')`, subRec, listRec)
	mustExec(t, pbdbDB, `INSERT INTO campaign_send_ledger (id, campaign_id, subscriber_id, status, created, updated) VALUES ('l1', ?, ?, 'pending', '2026-01-01', '2026-01-01')`, campRec, subRec)

	rows, _, err := NextPending(pbdbDB, campRec, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("first claim: got %d rows", len(rows))
	}
	if err := MarkSent(pbdbDB, campRec, subRec); err != nil {
		t.Fatal(err)
	}

	rows2, _, err := NextPending(pbdbDB, campRec, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows2) != 0 {
		t.Fatalf("after sent, NextPending should return no rows, got %d", len(rows2))
	}

	if err := InsertPendingIfEligible(pbdbDB, listRec, subRec); err != nil {
		t.Fatal(err)
	}
	var st string
	mustGet(t, pbdbDB, &st, `SELECT status FROM campaign_send_ledger WHERE campaign_id = ? AND subscriber_id = ?`, campRec, subRec)
	if st != "sent" {
		t.Fatalf("InsertPendingIfEligible must not recreate row; status=%q", st)
	}
	var n int
	mustGet(t, pbdbDB, &n, `SELECT COUNT(1) FROM campaign_send_ledger WHERE campaign_id = ? AND subscriber_id = ?`, campRec, subRec)
	if n != 1 {
		t.Fatalf("ledger rows for pair: %d, want 1", n)
	}

	if err := MarkSent(pbdbDB, campRec, subRec); err != nil {
		t.Fatal(err)
	}
	mustGet(t, pbdbDB, &st, `SELECT status FROM campaign_send_ledger WHERE campaign_id = ? AND subscriber_id = ?`, campRec, subRec)
	if st != "sent" {
		t.Fatalf("second MarkSent should leave row sent, got %q", st)
	}
}

func TestRollbackInflightDoesNotDowngradeSent(t *testing.T) {
	db := newTestLedgerDB(t)
	campRec := "camp_rec_1"
	subRec := "sub_rec_1"
	mustExec(t, db, `INSERT INTO campaigns (id, type, status) VALUES (?, 'broadcast', 'running')`, campRec)
	mustExec(t, db, `INSERT INTO subscribers (id, uuid, email, status) VALUES (?, 'u1', 'a@b.c', 'enabled')`, subRec)
	mustExec(t, db, `INSERT INTO campaign_send_ledger (id, campaign_id, subscriber_id, status, created, updated) VALUES ('l1', ?, ?, 'sent', '2026-01-01', '2026-01-01')`, campRec, subRec)

	if err := RollbackInflight(db, campRec, subRec); err != nil {
		t.Fatal(err)
	}
	var st string
	mustGet(t, db, &st, `SELECT status FROM campaign_send_ledger WHERE campaign_id = ? AND subscriber_id = ?`, campRec, subRec)
	if st != "sent" {
		t.Fatalf("RollbackInflight must not change sent rows; got %q", st)
	}
}

// --- test helpers ---

func newTestLedgerDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db := openMemSQLite(t)
	applyLedgerSchema(t, db)
	return db
}

func newTestLedgerPBDB(t *testing.T) *pbdb.DB {
	t.Helper()
	db := openMemSQLite(t)
	applyLedgerSchema(t, db)
	return pbdb.NewFromSQLX(db)
}

func openMemSQLite(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return db
}

func applyLedgerSchema(t *testing.T, db *sqlx.DB) {
	t.Helper()
	ddl := `
CREATE TABLE campaigns (
  id TEXT NOT NULL PRIMARY KEY,
  type TEXT,
  status TEXT,
  to_send INTEGER DEFAULT 0,
  sent INTEGER DEFAULT 0,
  updated TEXT
);
CREATE TABLE lists (
  id TEXT NOT NULL PRIMARY KEY,
  optin TEXT
);
CREATE TABLE subscribers (
  id TEXT NOT NULL PRIMARY KEY,
  uuid TEXT,
  email TEXT,
  first_name TEXT,
  last_name TEXT,
  name TEXT,
  attribs TEXT,
  status TEXT,
  created TEXT,
  updated TEXT
);
CREATE TABLE campaign_lists (
  campaign_id TEXT NOT NULL,
  list_id TEXT NOT NULL
);
CREATE TABLE subscriber_lists (
  subscriber_id TEXT NOT NULL,
  list_id TEXT NOT NULL,
  status TEXT
);
CREATE TABLE campaign_send_ledger (
  id TEXT NOT NULL UNIQUE,
  campaign_id TEXT NOT NULL,
  subscriber_id TEXT NOT NULL,
  status TEXT NOT NULL,
  created TEXT,
  updated TEXT,
  UNIQUE (campaign_id, subscriber_id)
);
CREATE INDEX idx_campaign_ledger_campaign_status ON campaign_send_ledger (campaign_id, status);
`
	if _, err := db.ExecContext(context.Background(), ddl); err != nil {
		t.Fatalf("schema: %v", err)
	}
}

func mustExec(t *testing.T, db interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, query string, args ...any) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("exec: %v\nquery: %s", err, query)
	}
}

func mustGet(t *testing.T, db sqlx.QueryerContext, dest any, query string, args ...any) {
	t.Helper()
	if err := sqlx.GetContext(context.Background(), db, dest, query, args...); err != nil {
		t.Fatalf("get: %v", err)
	}
}
