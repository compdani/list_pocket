package core

import (
	"fmt"
	pbcore "ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/compdani/list_pocket/models"
	"strings"
	"testing"
)

func TestContactAndMembershipRollbackTogether(t *testing.T) {
	c, app := channelTestCore(t)
	_, listUUID := channelTestList(t, c, app)
	app.OnRecordCreate("subscriber_lists").BindFunc(func(e *pbcore.RecordEvent) error { return fmt.Errorf("injected membership failure") })
	_, _, err := c.InsertSubscriber(models.Subscriber{Email: "atomic@example.com", Name: "Atomic", Status: "enabled"}, nil, []string{listUUID}, true, false)
	if err == nil {
		t.Fatal("expected membership failure")
	}
	count, err := app.CountRecords("subscribers")
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("partial contact persisted: %d", count)
	}
}

func TestBulkTagsRollbackTogether(t *testing.T) {
	c, app := channelTestCore(t)
	for _, email := range []string{"first@example.com", "second@example.com"} {
		if _, _, err := c.InsertSubscriber(models.Subscriber{Email: email}, nil, nil, true, false); err != nil {
			t.Fatal(err)
		}
	}
	app.OnRecordUpdate("subscribers").BindFunc(func(e *pbcore.RecordEvent) error {
		if e.Record.GetString("email") == "second@example.com" {
			return fmt.Errorf("injected second contact failure")
		}
		return e.Next()
	})
	count, err := c.bulkUpdateSubscriberTags([]string{"first@example.com", "second@example.com"}, []string{"customer"}, nil)
	if err == nil || count != 0 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	rec, err := app.FindFirstRecordByData("subscribers", "email", "first@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rec.GetString("attribs"), "customer") {
		t.Fatal("partial tag committed")
	}
}

func TestBulkListFailureReportsCommittedChunks(t *testing.T) {
	for _, remove := range []bool{false, true} {
		t.Run(fmt.Sprint(remove), func(t *testing.T) {
			c, app := channelTestCore(t)
			listID, _ := channelTestList(t, c, app)
			emails := make([]string, 401)
			for i := 1; i <= 401; i++ {
				emails[i-1] = fmt.Sprintf("chunk%d@example.com", i)
				if _, err := c.db.Exec(`INSERT INTO subscribers(id,uuid,email,status) VALUES (?,?,?,'enabled')`, fmt.Sprintf("%015d", i), fmt.Sprintf("00000000-0000-4000-8000-%012d", i), emails[i-1]); err != nil {
					t.Fatal(err)
				}
			}
			if remove {
				for i := 1; i <= 401; i++ {
					if _, err := c.db.Exec(`INSERT INTO subscriber_lists(subscriber_id,list_id,status,sms_status) VALUES (?,?,'confirmed','confirmed')`, fmt.Sprintf("%015d", i), listID); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := c.db.Exec(`CREATE TRIGGER fail_delete BEFORE DELETE ON subscriber_lists WHEN old.subscriber_id='000000000000401' BEGIN SELECT RAISE(ABORT,'injected delete failure'); END`); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := c.db.Exec(`CREATE TRIGGER fail_insert BEFORE INSERT ON subscriber_lists WHEN new.subscriber_id='000000000000301' BEGIN SELECT RAISE(ABORT,'injected insert failure'); END`); err != nil {
					t.Fatal(err)
				}
			}
			var removals, updates []string
			if remove {
				removals = []string{listID}
			} else {
				updates = []string{listID}
			}
			result, err := c.BulkUpdateSubscribersByEmail(emails, nil, nil, removals, updates, "confirmed")
			if err == nil {
				t.Fatal("expected chunk failure")
			}
			if remove && result.ListsRemoved != 400 {
				t.Fatalf("removed=%d", result.ListsRemoved)
			}
			if !remove && result.ListsUpdated != 300 {
				t.Fatalf("updated=%d", result.ListsUpdated)
			}
		})
	}
}
