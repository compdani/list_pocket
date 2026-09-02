package core

import (
	"context"
	"io"
	"log"
	"testing"
	"time"

	"github.com/compdani/list_pocket/internal/i18n"
	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/compdani/list_pocket/models"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestGetUnifiedContactTimelineOrdersUnion(t *testing.T) {
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ddl := `
CREATE TABLE subscribers (
  id TEXT NOT NULL PRIMARY KEY,
  uuid TEXT,
  email TEXT,
  status TEXT,
  created TEXT,
  updated TEXT
);
CREATE TABLE campaigns (
  id TEXT NOT NULL PRIMARY KEY,
  uuid TEXT,
  name TEXT,
  subject TEXT,
  status TEXT
);
CREATE TABLE campaign_send_ledger (
  id TEXT NOT NULL UNIQUE,
  campaign_id TEXT NOT NULL,
  subscriber_id TEXT NOT NULL,
  status TEXT NOT NULL,
  created TEXT,
  updated TEXT
);
CREATE TABLE campaign_views (
  campaign_id TEXT,
  subscriber_id TEXT,
  is_suspected_privacy_open INTEGER DEFAULT 0,
  created TEXT
);
CREATE TABLE links (
  id TEXT NOT NULL PRIMARY KEY,
  url TEXT
);
CREATE TABLE link_clicks (
  campaign_id TEXT,
  subscriber_id TEXT,
  link_id TEXT,
  created TEXT
);
CREATE TABLE inbound_sms_events (
  id TEXT NOT NULL PRIMARY KEY,
  subscriber_id TEXT,
  received_at TEXT
);
CREATE TABLE inbound_email_replies (
  id TEXT NOT NULL PRIMARY KEY,
  subscriber_id TEXT,
  received_at TEXT
);
`
	if _, err := db.Exec(ddl); err != nil {
		t.Fatalf("schema: %v", err)
	}

	subID := "sub_1"
	campID := "camp_1"
	if _, err := db.Exec(`INSERT INTO subscribers (id, uuid, email, status, created, updated) VALUES (?, 'u1', 'a@b.c', 'enabled', '2026-01-01', '2026-01-01')`, subID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO campaigns (id, uuid, name, subject, status) VALUES (?, 'cu1', 'Spring', 'Hello', 'finished')`, campID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO campaign_send_ledger (id, campaign_id, subscriber_id, status, created, updated) VALUES ('l1', ?, ?, 'sent', '2026-04-01 10:00:00.000Z', '2026-04-01 10:00:00.000Z')`, campID, subID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO campaign_views (campaign_id, subscriber_id, is_suspected_privacy_open, created) VALUES (?, ?, 0, '2026-04-01 11:00:00.000Z')`, campID, subID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO links (id, url) VALUES ('link_1', 'https://example.com')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO link_clicks (campaign_id, subscriber_id, link_id, created) VALUES (?, ?, 'link_1', '2026-04-01 12:00:00.000Z')`, campID, subID); err != nil {
		t.Fatal(err)
	}

	var rowID int
	if err := db.Get(&rowID, `SELECT rowid FROM subscribers WHERE id = ?`, subID); err != nil {
		t.Fatal(err)
	}

	lang, err := i18n.New([]byte(`{"_.code":"en","_.name":"English"}`))
	if err != nil {
		t.Fatal(err)
	}
	c := New(&Opt{
		DB:   pbdb.NewFromSQLX(db),
		I18n: lang,
		Log:  log.New(io.Discard, "", 0),
	}, &Hooks{})

	out, err := c.GetUnifiedContactTimeline(context.Background(), TimelineQueryParams{
		SubscriberID: rowID,
		Limit:        10,
	})
	if err != nil {
		t.Fatalf("GetUnifiedContactTimeline: %v", err)
	}
	if out.Total != 3 {
		t.Fatalf("total=%d, want 3", out.Total)
	}
	if len(out.Events) != 3 {
		t.Fatalf("events=%d, want 3", len(out.Events))
	}

	want := []string{
		models.TimelineEventLinkClick,
		models.TimelineEventCampaignView,
		models.TimelineEventCampaignSend,
	}
	for i, typ := range want {
		if out.Events[i].EventType != typ {
			t.Fatalf("event[%d]=%s, want %s", i, out.Events[i].EventType, typ)
		}
	}
	if !out.Events[0].OccurredAt.After(out.Events[1].OccurredAt) || !out.Events[1].OccurredAt.After(out.Events[2].OccurredAt) {
		t.Fatalf("expected newest-first timestamps, got %v %v %v",
			out.Events[0].OccurredAt, out.Events[1].OccurredAt, out.Events[2].OccurredAt)
	}
	if out.Events[0].OccurredAt.UTC().Format(time.RFC3339) != "2026-04-01T12:00:00Z" {
		t.Fatalf("first event time = %s", out.Events[0].OccurredAt.UTC().Format(time.RFC3339))
	}
}
