package core

import (
	"context"
	"database/sql"
	"testing"

	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestSMSOptOutSubscriberByRecordID(t *testing.T) {
	c := newSMSOptOutTestCore(t)

	mustSMSExec(t, c, `INSERT INTO subscribers (id, uuid, email, phone, status) VALUES ('sub_a', 'u-a', 'a@b.c', '+15551234567', 'enabled')`)
	mustSMSExec(t, c, `INSERT INTO subscribers (id, uuid, email, phone, status) VALUES ('sub_b', 'u-b', 'b@b.c', '+15557654321', 'enabled')`)
	mustSMSExec(t, c, `INSERT INTO lists (id) VALUES ('list_1')`)
	mustSMSExec(t, c, `INSERT INTO lists (id) VALUES ('list_2')`)
	mustSMSExec(t, c, `INSERT INTO subscriber_lists (subscriber_id, list_id, status, sms_status) VALUES ('sub_a', 'list_1', 'confirmed', 'confirmed')`)
	mustSMSExec(t, c, `INSERT INTO subscriber_lists (subscriber_id, list_id, status, sms_status) VALUES ('sub_a', 'list_2', 'confirmed', 'confirmed')`)
	mustSMSExec(t, c, `INSERT INTO subscriber_lists (subscriber_id, list_id, status, sms_status) VALUES ('sub_b', 'list_1', 'confirmed', 'confirmed')`)

	n, err := c.SMSOptOutSubscriberByRecordID("sub_a")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("updated %d rows, want 2", n)
	}

	var smsA1, emailA1, smsB string
	mustSMSGet(t, c, &smsA1, `SELECT sms_status FROM subscriber_lists WHERE subscriber_id = 'sub_a' AND list_id = 'list_1'`)
	mustSMSGet(t, c, &emailA1, `SELECT status FROM subscriber_lists WHERE subscriber_id = 'sub_a' AND list_id = 'list_1'`)
	mustSMSGet(t, c, &smsB, `SELECT sms_status FROM subscriber_lists WHERE subscriber_id = 'sub_b' AND list_id = 'list_1'`)
	if smsA1 != "unsubscribed" {
		t.Fatalf("sub_a sms_status = %q, want unsubscribed", smsA1)
	}
	if emailA1 != "confirmed" {
		t.Fatalf("sub_a email status = %q, want confirmed (must not change)", emailA1)
	}
	if smsB != "confirmed" {
		t.Fatalf("sub_b sms_status = %q, want confirmed (other subscriber untouched)", smsB)
	}

	n, err = c.SMSOptOutSubscriberByRecordID("sub_a")
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("second opt-out updated %d rows, want 0", n)
	}

	n, err = c.SMSOptOutSubscriberByRecordID("  ")
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("empty record id updated %d rows, want 0", n)
	}
}

func TestSubscriberIDsByExactPhone(t *testing.T) {
	c := newSMSOptOutTestCore(t)

	mustSMSExec(t, c, `INSERT INTO subscribers (id, uuid, email, phone, status) VALUES ('sub_a', 'u-a', 'a@b.c', '+15551234567', 'enabled')`)
	mustSMSExec(t, c, `INSERT INTO subscribers (id, uuid, email, phone, status) VALUES ('sub_b', 'u-b', 'b@b.c', '+15551234567', 'enabled')`)
	mustSMSExec(t, c, `INSERT INTO subscribers (id, uuid, email, phone, status) VALUES ('sub_c', 'u-c', 'c@b.c', '(555) 123-4567', 'enabled')`)

	ids, err := c.SubscriberIDsByExactPhone("+15551234567")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("exact phone ids = %v, want 2 siblings", ids)
	}

	ids, err = c.SubscriberIDsByExactPhone("(555) 123-4567")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "sub_c" {
		t.Fatalf("formatted phone ids = %v, want [sub_c]", ids)
	}

	ids, err = c.SubscriberIDsByExactPhone("  ")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("empty phone ids = %v, want none", ids)
	}
}

func TestSMSOptOutSubscribersByRecordIDsOptsOutSiblings(t *testing.T) {
	c := newSMSOptOutTestCore(t)

	mustSMSExec(t, c, `INSERT INTO subscribers (id, uuid, email, phone, status) VALUES ('sub_a', 'u-a', 'a@b.c', '+15551234567', 'enabled')`)
	mustSMSExec(t, c, `INSERT INTO subscribers (id, uuid, email, phone, status) VALUES ('sub_b', 'u-b', 'b@b.c', '+15551234567', 'enabled')`)
	mustSMSExec(t, c, `INSERT INTO subscribers (id, uuid, email, phone, status) VALUES ('sub_c', 'u-c', 'c@b.c', '(555) 123-4567', 'enabled')`)
	mustSMSExec(t, c, `INSERT INTO lists (id) VALUES ('list_1')`)
	mustSMSExec(t, c, `INSERT INTO subscriber_lists (subscriber_id, list_id, status, sms_status) VALUES ('sub_a', 'list_1', 'confirmed', 'confirmed')`)
	mustSMSExec(t, c, `INSERT INTO subscriber_lists (subscriber_id, list_id, status, sms_status) VALUES ('sub_b', 'list_1', 'confirmed', 'confirmed')`)
	mustSMSExec(t, c, `INSERT INTO subscriber_lists (subscriber_id, list_id, status, sms_status) VALUES ('sub_c', 'list_1', 'confirmed', 'confirmed')`)

	ids, err := c.SubscriberIDsByExactPhone("+15551234567")
	if err != nil {
		t.Fatal(err)
	}
	n, err := c.SMSOptOutSubscribersByRecordIDs(append(ids, "sub_a"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("updated %d rows, want 2 exact-phone siblings", n)
	}

	var smsA, smsB, smsC, emailC string
	mustSMSGet(t, c, &smsA, `SELECT sms_status FROM subscriber_lists WHERE subscriber_id = 'sub_a'`)
	mustSMSGet(t, c, &smsB, `SELECT sms_status FROM subscriber_lists WHERE subscriber_id = 'sub_b'`)
	mustSMSGet(t, c, &smsC, `SELECT sms_status FROM subscriber_lists WHERE subscriber_id = 'sub_c'`)
	mustSMSGet(t, c, &emailC, `SELECT status FROM subscriber_lists WHERE subscriber_id = 'sub_c'`)
	if smsA != "unsubscribed" || smsB != "unsubscribed" {
		t.Fatalf("siblings sms_status = %q/%q, want unsubscribed", smsA, smsB)
	}
	if smsC != "confirmed" {
		t.Fatalf("last-10-only number should not be opted out by exact phone, got %q", smsC)
	}
	if emailC != "confirmed" {
		t.Fatalf("email status = %q, want confirmed", emailC)
	}
}

func TestSMSOptOutSubscriberByPhoneMatchesLast10(t *testing.T) {
	c := newSMSOptOutTestCore(t)

	mustSMSExec(t, c, `INSERT INTO subscribers (id, uuid, email, phone, status) VALUES ('sub_c', 'u-c', 'c@b.c', '(555) 123-4567', 'enabled')`)
	mustSMSExec(t, c, `INSERT INTO lists (id) VALUES ('list_1')`)
	mustSMSExec(t, c, `INSERT INTO subscriber_lists (subscriber_id, list_id, status, sms_status) VALUES ('sub_c', 'list_1', 'confirmed', 'confirmed')`)

	n, err := c.SMSOptOutSubscriberByPhone("+15551234567")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("updated %d rows, want 1 last-10 match", n)
	}

	var smsC string
	mustSMSGet(t, c, &smsC, `SELECT sms_status FROM subscriber_lists WHERE subscriber_id = 'sub_c'`)
	if smsC != "unsubscribed" {
		t.Fatalf("sms_status = %q, want unsubscribed", smsC)
	}
}

func TestSMSOptOutSubscriberByPhoneSkipsAlreadyUnsubscribed(t *testing.T) {
	c := newSMSOptOutTestCore(t)

	mustSMSExec(t, c, `INSERT INTO subscribers (id, uuid, email, phone, status) VALUES ('sub_a', 'u-a', 'a@b.c', '+15551234567', 'enabled')`)
	mustSMSExec(t, c, `INSERT INTO lists (id) VALUES ('list_1')`)
	mustSMSExec(t, c, `INSERT INTO lists (id) VALUES ('list_2')`)
	mustSMSExec(t, c, `INSERT INTO subscriber_lists (subscriber_id, list_id, status, sms_status) VALUES ('sub_a', 'list_1', 'confirmed', 'unsubscribed')`)
	mustSMSExec(t, c, `INSERT INTO subscriber_lists (subscriber_id, list_id, status, sms_status) VALUES ('sub_a', 'list_2', 'confirmed', 'confirmed')`)

	n, err := c.SMSOptOutSubscriberByPhone("+15551234567")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("updated %d rows, want 1 remaining membership", n)
	}

	n, err = c.SMSOptOutSubscriberByPhone("+15551234567")
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("second phone opt-out updated %d rows, want 0", n)
	}

	n, err = c.SMSOptOutSubscriberByPhone("(555) 123-4567")
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("last-10 match rewrote %d already-unsubscribed rows, want 0", n)
	}
}

func newSMSOptOutTestCore(t *testing.T) *Core {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	ddl := `
CREATE TABLE subscribers (
  id TEXT NOT NULL PRIMARY KEY,
  uuid TEXT,
  email TEXT,
  phone TEXT,
  status TEXT
);
CREATE TABLE lists (
  id TEXT NOT NULL PRIMARY KEY
);
CREATE TABLE subscriber_lists (
  subscriber_id TEXT NOT NULL,
  list_id TEXT NOT NULL,
  status TEXT,
  sms_status TEXT,
  updated TEXT
);
`
	if _, err := db.ExecContext(context.Background(), ddl); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return &Core{db: pbdb.NewFromSQLX(db)}
}

func mustSMSExec(t *testing.T, c *Core, query string, args ...any) {
	t.Helper()
	if _, err := c.db.Exec(query, args...); err != nil {
		t.Fatalf("exec: %v\nquery: %s", err, query)
	}
}

func mustSMSGet(t *testing.T, c *Core, dest any, query string, args ...any) {
	t.Helper()
	if err := c.db.Get(dest, query, args...); err != nil {
		if err == sql.ErrNoRows {
			t.Fatalf("get: no rows\nquery: %s", query)
		}
		t.Fatalf("get: %v\nquery: %s", err, query)
	}
}
