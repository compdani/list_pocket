package campaignledger

import (
	"context"
	"database/sql"
	"testing"
	"time"

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

func TestCleanupSentOlderThan(t *testing.T) {
	pbdbDB := newTestLedgerPBDB(t)
	campRec := "camp_finished"
	var campRowID int
	mustExec(t, pbdbDB, `INSERT INTO campaigns (id, type, status, to_send, sent) VALUES (?, 'broadcast', 'finished', 0, 0)`, campRec)
	mustGet(t, pbdbDB, &campRowID, `SELECT rowid FROM campaigns WHERE id = ?`, campRec)

	mustExec(t, pbdbDB, `INSERT INTO subscribers (id, uuid, email, status) VALUES ('s1', 'u1', 'a@b.c', 'enabled')`)
	mustExec(t, pbdbDB, `INSERT INTO subscribers (id, uuid, email, status) VALUES ('s2', 'u2', 'b@b.c', 'enabled')`)
	mustExec(t, pbdbDB, `INSERT INTO campaign_send_ledger (id, campaign_id, subscriber_id, status, created, updated) VALUES ('l1', ?, 's1', 'sent', '2026-01-01', '2026-01-01')`, campRec)
	mustExec(t, pbdbDB, `INSERT INTO campaign_send_ledger (id, campaign_id, subscriber_id, status, created, updated) VALUES ('l2', ?, 's2', 'sent', '2026-01-02', '2026-01-02')`, campRec)

	deleted, reconciled, err := CleanupSentOlderThan(pbdbDB, time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 2 {
		t.Fatalf("deleted=%d, want 2", deleted)
	}
	if reconciled != 1 {
		t.Fatalf("reconciled=%d, want 1", reconciled)
	}

	var toSend, sent, remaining int
	mustGet(t, pbdbDB, &toSend, `SELECT to_send FROM campaigns WHERE rowid = ?`, campRowID)
	mustGet(t, pbdbDB, &sent, `SELECT sent FROM campaigns WHERE rowid = ?`, campRowID)
	mustGet(t, pbdbDB, &remaining, `SELECT COUNT(1) FROM campaign_send_ledger WHERE campaign_id = ?`, campRec)
	if toSend != 2 || sent != 2 {
		t.Fatalf("campaign stats after cleanup: to_send=%d sent=%d, want 2/2", toSend, sent)
	}
	if remaining != 0 {
		t.Fatalf("remaining ledger rows=%d, want 0", remaining)
	}
}

func TestCleanupSentOlderThanSkipsRunningAndPending(t *testing.T) {
	pbdbDB := newTestLedgerPBDB(t)
	mustExec(t, pbdbDB, `INSERT INTO campaigns (id, type, status, to_send, sent) VALUES ('camp_running', 'broadcast', 'running', 0, 0)`)
	mustExec(t, pbdbDB, `INSERT INTO campaigns (id, type, status, to_send, sent) VALUES ('camp_finished', 'broadcast', 'finished', 0, 0)`)
	mustExec(t, pbdbDB, `INSERT INTO subscribers (id, uuid, email, status) VALUES ('s1', 'u1', 'a@b.c', 'enabled')`)
	mustExec(t, pbdbDB, `INSERT INTO subscribers (id, uuid, email, status) VALUES ('s2', 'u2', 'b@b.c', 'enabled')`)
	mustExec(t, pbdbDB, `INSERT INTO campaign_send_ledger (id, campaign_id, subscriber_id, status, created, updated) VALUES ('l1', 'camp_running', 's1', 'sent', '2026-01-01', '2026-01-01')`)
	mustExec(t, pbdbDB, `INSERT INTO campaign_send_ledger (id, campaign_id, subscriber_id, status, created, updated) VALUES ('l2', 'camp_finished', 's2', 'pending', '2026-01-01', '2026-01-01')`)

	deleted, reconciled, err := CleanupSentOlderThan(pbdbDB, time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 0 {
		t.Fatalf("deleted=%d, want 0", deleted)
	}
	if reconciled != 0 {
		t.Fatalf("reconciled=%d, want 0", reconciled)
	}

	var n int
	mustGet(t, pbdbDB, &n, `SELECT COUNT(1) FROM campaign_send_ledger`)
	if n != 2 {
		t.Fatalf("ledger row count=%d, want 2", n)
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

	mustExec(t, db, `INSERT INTO campaigns (id, type, status, messenger, to_send, sent) VALUES (?, 'broadcast', 'scheduled', 'email', 0, 0)`, campRec)
	mustGet(t, db, &campRowID, `SELECT rowid FROM campaigns WHERE id = ?`, campRec)
	mustExec(t, db, `INSERT INTO lists (id, optin) VALUES (?, 'single')`, listRec)
	mustExec(t, db, `INSERT INTO subscribers (id, uuid, email, status) VALUES (?, 'u1', 'a@b.c', 'enabled')`, subRec)
	mustExec(t, db, `INSERT INTO campaign_lists (campaign_id, list_id) VALUES (?, ?)`, campRec, listRec)
	mustExec(t, db, `INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES (?, ?, 'confirmed')`, subRec, listRec)

	ran, err := BackfillIfEmpty(db, campRowID, campRec)
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

	ran2, err := BackfillIfEmpty(db, campRowID, campRec)
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

func TestResetInflight(t *testing.T) {
	db := newTestLedgerDB(t)
	campRec := "camp_rec_1"
	otherCamp := "camp_rec_2"
	mustExec(t, db, `INSERT INTO campaigns (id, type, status) VALUES (?, 'broadcast', 'running')`, campRec)
	mustExec(t, db, `INSERT INTO campaigns (id, type, status) VALUES (?, 'broadcast', 'running')`, otherCamp)
	for i, sid := range []string{"s1", "s2", "s3", "s4"} {
		mustExec(t, db, `INSERT INTO subscribers (id, uuid, email, status) VALUES (?, ?, ?, 'enabled')`, sid, sid, sid+"@t.c")
		statuses := []string{"inflight", "inflight", "sent", "pending"}
		mustExec(t, db,
			`INSERT INTO campaign_send_ledger (id, campaign_id, subscriber_id, status, created, updated) VALUES (?, ?, ?, ?, '2026-01-01', '2026-01-01')`,
			"l"+sid, campRec, sid, statuses[i])
	}
	mustExec(t, db,
		`INSERT INTO subscribers (id, uuid, email, status) VALUES ('s5', 's5', 's5@t.c', 'enabled')`)
	mustExec(t, db,
		`INSERT INTO campaign_send_ledger (id, campaign_id, subscriber_id, status, created, updated) VALUES ('l5', ?, 's5', 'inflight', '2026-01-01', '2026-01-01')`, otherCamp)

	n, err := ResetInflight(db, campRec)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("ResetInflight rows reset = %d, want 2", n)
	}

	var inflight, pending, sent int
	mustGet(t, db, &inflight, `SELECT COUNT(1) FROM campaign_send_ledger WHERE campaign_id = ? AND status = 'inflight'`, campRec)
	mustGet(t, db, &pending, `SELECT COUNT(1) FROM campaign_send_ledger WHERE campaign_id = ? AND status = 'pending'`, campRec)
	mustGet(t, db, &sent, `SELECT COUNT(1) FROM campaign_send_ledger WHERE campaign_id = ? AND status = 'sent'`, campRec)
	if inflight != 0 {
		t.Fatalf("inflight count after reset = %d, want 0", inflight)
	}
	if pending != 3 {
		t.Fatalf("pending count after reset = %d, want 3", pending)
	}
	if sent != 1 {
		t.Fatalf("sent count after reset = %d, want 1 (sent rows must not be downgraded)", sent)
	}

	// Other campaign's inflight row must not be touched.
	var otherInflight int
	mustGet(t, db, &otherInflight, `SELECT COUNT(1) FROM campaign_send_ledger WHERE campaign_id = ? AND status = 'inflight'`, otherCamp)
	if otherInflight != 1 {
		t.Fatalf("other campaign inflight count = %d, want 1 (reset must be scoped to the target campaign)", otherInflight)
	}
}

func TestMarkInflightSent(t *testing.T) {
	db := newTestLedgerDB(t)
	campRec := "camp_rec_1"
	otherCamp := "camp_rec_2"
	mustExec(t, db, `INSERT INTO campaigns (id, type, status) VALUES (?, 'broadcast', 'finished')`, campRec)
	mustExec(t, db, `INSERT INTO campaigns (id, type, status) VALUES (?, 'broadcast', 'finished')`, otherCamp)
	for i, sid := range []string{"s1", "s2", "s3", "s4"} {
		mustExec(t, db, `INSERT INTO subscribers (id, uuid, email, status) VALUES (?, ?, ?, 'enabled')`, sid, sid, sid+"@t.c")
		statuses := []string{"inflight", "inflight", "pending", "sent"}
		mustExec(t, db,
			`INSERT INTO campaign_send_ledger (id, campaign_id, subscriber_id, status, created, updated) VALUES (?, ?, ?, ?, '2026-01-01', '2026-01-01')`,
			"l"+sid, campRec, sid, statuses[i])
	}
	mustExec(t, db,
		`INSERT INTO subscribers (id, uuid, email, status) VALUES ('s5', 's5', 's5@t.c', 'enabled')`)
	mustExec(t, db,
		`INSERT INTO campaign_send_ledger (id, campaign_id, subscriber_id, status, created, updated) VALUES ('l5', ?, 's5', 'inflight', '2026-01-01', '2026-01-01')`, otherCamp)

	n, err := MarkInflightSent(db, campRec)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("MarkInflightSent rows updated = %d, want 2", n)
	}

	var inflight, pending, sent int
	mustGet(t, db, &inflight, `SELECT COUNT(1) FROM campaign_send_ledger WHERE campaign_id = ? AND status = 'inflight'`, campRec)
	mustGet(t, db, &pending, `SELECT COUNT(1) FROM campaign_send_ledger WHERE campaign_id = ? AND status = 'pending'`, campRec)
	mustGet(t, db, &sent, `SELECT COUNT(1) FROM campaign_send_ledger WHERE campaign_id = ? AND status = 'sent'`, campRec)
	if inflight != 0 {
		t.Fatalf("inflight count after MarkInflightSent = %d, want 0", inflight)
	}
	if pending != 1 {
		t.Fatalf("pending count after MarkInflightSent = %d, want 1 (pending must not be promoted to sent)", pending)
	}
	if sent != 3 {
		t.Fatalf("sent count after MarkInflightSent = %d, want 3 (both inflight rows + original sent row)", sent)
	}

	var otherInflight int
	mustGet(t, db, &otherInflight, `SELECT COUNT(1) FROM campaign_send_ledger WHERE campaign_id = ? AND status = 'inflight'`, otherCamp)
	if otherInflight != 1 {
		t.Fatalf("other campaign inflight count = %d, want 1 (MarkInflightSent must be scoped)", otherInflight)
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
  messenger TEXT DEFAULT 'email',
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
  phone TEXT,
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
  status TEXT,
  sms_status TEXT
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
