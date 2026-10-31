package core

import (
	"context"
	"io"
	"log"
	"testing"
	"time"

	"github.com/compdani/list_pocket/internal/i18n"
	_ "github.com/compdani/list_pocket/internal/migrations"
	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/compdani/list_pocket/models"
	"github.com/pocketbase/pocketbase"
	pbcore "github.com/pocketbase/pocketbase/core"
)

const testChannel = "ch:coretest"

func channelTestCore(t *testing.T) (*Core, *pocketbase.PocketBase) {
	t.Helper()
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.ResetBootstrapState() })
	if err := app.RunAppMigrations(); err != nil {
		t.Fatal(err)
	}
	lang, err := i18n.New([]byte(`{"_.code":"en","_.name":"English"}`))
	if err != nil {
		t.Fatal(err)
	}
	db, err := pbdb.NewFromPocketBase(app)
	if err != nil {
		t.Fatal(err)
	}
	c := New(&Opt{DB: db, I18n: lang, Log: log.New(io.Discard, "", 0)}, &Hooks{})
	if err := c.EnsureChannelTriggers(); err != nil {
		t.Fatal(err)
	}
	models.RegisterChannel(models.ChannelSpec{
		ID: testChannel, Label: "Core test", Address: models.AddressSourceChannel,
		Consent: models.ConsentSourceChannel, TextPipeline: true, Unsendable: models.UnsendableSkip,
	})
	return c, app
}

func channelTestList(t *testing.T, c *Core, app *pocketbase.PocketBase) (id, uuid string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("lists")
	if err != nil {
		t.Fatal(err)
	}
	rec := pbcore.NewRecord(col)
	rec.Set("name", "Test list")
	rec.Set("uuid", "3f2b8c1e-7a4d-4e9b-9c2a-1d5e6f7a8b90")
	rec.Set("type", "public")
	rec.Set("optin", "single")
	rec.Set("status", "active")
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	return rec.Id, "3f2b8c1e-7a4d-4e9b-9c2a-1d5e6f7a8b90"
}

func TestUpsertChannelContactLifecycle(t *testing.T) {
	c, app := channelTestCore(t)
	listID, listUUID := channelTestList(t, c, app)

	sub, err := c.UpsertChannelContact(ChannelContact{
		Channel: testChannel, Address: "chat1", Name: "Ada", Lists: []string{listUUID, "no-such-list"}, Consent: "confirmed",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if !models.IsPlaceholderEmail(sub.Email) {
		t.Fatalf("expected a placeholder email, got %q", sub.Email)
	}

	again, err := c.UpsertChannelContact(ChannelContact{Channel: testChannel, Address: "chat1", Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	if again.RecordID != sub.RecordID {
		t.Fatalf("second upsert made a new contact: %s vs %s", again.RecordID, sub.RecordID)
	}

	found, ok, err := c.FindSubscriberByChannelAddress(testChannel, "chat1")
	if err != nil || !ok || found.RecordID != sub.RecordID {
		t.Fatalf("find: %v %v %+v", err, ok, found)
	}

	// Joining through a channel grants nothing on email or SMS.
	var status, smsStatus string
	row := c.db.QueryRow(`SELECT status, sms_status FROM subscriber_lists WHERE subscriber_id = ? AND list_id = ?`, sub.RecordID, listID)
	if err := row.Scan(&status, &smsStatus); err != nil {
		t.Fatal(err)
	}
	if status != "unsubscribed" || smsStatus != "unsubscribed" {
		t.Fatalf("email and SMS must stay off, got %q %q", status, smsStatus)
	}

	to, err := c.ChannelRecipients(sub, testChannel)
	if err != nil || len(to) != 1 || to[0] != "chat1" {
		t.Fatalf("recipients: %v %v", err, to)
	}

	if _, err := c.OptOutChannelSubscriber(sub.RecordID, testChannel); err != nil {
		t.Fatal(err)
	}
	if to, _ := c.ChannelRecipients(sub, testChannel); len(to) != 0 {
		t.Fatalf("opted-out contact still has recipients: %v", to)
	}

	info, err := c.GetSubscriberChannelInfo(sub.RecordID, []string{testChannel})
	if err != nil {
		t.Fatal(err)
	}
	if len(info) != 1 || len(info[0].Addresses) != 1 || len(info[0].Consent) != 1 || info[0].Consent[0].Status != "unsubscribed" {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestChannelAddressBelongsToOneSubscriber(t *testing.T) {
	c, _ := channelTestCore(t)
	a, err := c.UpsertChannelContact(ChannelContact{Channel: testChannel, Address: "chat1"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.UpsertChannelContact(ChannelContact{Channel: testChannel, Address: "chat2"})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetChannelAddress(b.RecordID, testChannel, "chat1"); err == nil {
		t.Fatal("claiming another subscriber's address must fail")
	}
	if err := c.SetChannelAddress(a.RecordID, testChannel, "chat1"); err != nil {
		t.Fatalf("re-adding your own address: %v", err)
	}
}

func TestChannelEventDedupeAndStatus(t *testing.T) {
	c, _ := channelTestCore(t)
	e := ChannelEvent{Channel: testChannel, Direction: "outbound", Kind: "send", Status: "sent", ProviderMsgID: "m1", SubscriberID: "s1"}
	if first, err := c.RecordChannelEvent(e); err != nil || !first {
		t.Fatalf("first record: %v %v", first, err)
	}
	if first, err := c.RecordChannelEvent(e); err != nil || first {
		t.Fatalf("duplicate record: %v %v", first, err)
	}
	e.ProviderMsgID = ""
	for range 2 {
		if first, err := c.RecordChannelEvent(e); err != nil || !first {
			t.Fatalf("events with no provider ID are never duplicates: %v %v", first, err)
		}
	}
	if ok, err := c.ApplyChannelStatus(testChannel, "m1", "delivered", false); err != nil || !ok {
		t.Fatalf("status: %v %v", err, ok)
	}
	if ok, err := c.ApplyChannelStatus(testChannel, "missing", "delivered", false); err != nil || ok {
		t.Fatalf("an unknown ID leaves a stub and reports no match: %v %v", err, ok)
	}
}

func TestChannelCleanupTriggersSurviveRawDeletesAndCollectionSaves(t *testing.T) {
	c, app := channelTestCore(t)
	sub, err := c.UpsertChannelContact(ChannelContact{Channel: testChannel, Address: "chat1"})
	if err != nil {
		t.Fatal(err)
	}

	// Change the collection the way a future migration might, then re-ensure as boot does.
	col, err := app.FindCollectionByNameOrId("subscribers")
	if err != nil {
		t.Fatal(err)
	}
	col.Fields.Add(&pbcore.TextField{Name: "trigger_probe"})
	if err := app.Save(col); err != nil {
		t.Fatal(err)
	}
	if err := c.EnsureChannelTriggers(); err != nil {
		t.Fatal(err)
	}

	if _, err := c.db.Exec(`DELETE FROM subscribers WHERE id = ?`, sub.RecordID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := c.db.Get(&n, `SELECT COUNT(*) FROM subscriber_addresses WHERE subscriber = ?`, sub.RecordID); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("raw delete left addresses behind")
	}
}

func TestTimelineIncludesChannelEvents(t *testing.T) {
	c, _ := channelTestCore(t)
	sub, err := c.UpsertChannelContact(ChannelContact{Channel: testChannel, Address: "chat1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.RecordChannelEvent(ChannelEvent{
		Channel: testChannel, Direction: "inbound", Kind: "message", SubscriberID: sub.RecordID, Address: "chat1", Body: "hello",
	}); err != nil {
		t.Fatal(err)
	}
	out, err := c.GetUnifiedContactTimeline(context.Background(), TimelineQueryParams{SubscriberID: sub.ID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Events) != 1 || out.Events[0].EventType != models.TimelineEventChannel || out.Events[0].Channel != testChannel {
		t.Fatalf("unexpected events: %+v", out.Events)
	}
}

func channelEventStatus(t *testing.T, c *Core, id string) string {
	t.Helper()
	var status string
	if err := c.db.Get(&status, `SELECT status FROM channel_events WHERE channel = ? AND provider_msg_id = ?`, testChannel, id); err != nil {
		t.Fatal(err)
	}
	return status
}

func TestChannelStatusNeverMovesBackwards(t *testing.T) {
	c, _ := channelTestCore(t)
	if _, err := c.RecordChannelEvent(ChannelEvent{Channel: testChannel, Direction: ChannelDirectionOutbound, Kind: "send", Status: "sent", ProviderMsgID: "m1", SubscriberID: "s1"}); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct{ apply, want string }{
		{"read", "read"},
		{"delivered", "read"},
		{"sent", "read"},
		{"failed", "failed"},
		{"read", "failed"},
	} {
		if _, err := c.ApplyChannelStatus(testChannel, "m1", step.apply, false); err != nil {
			t.Fatal(err)
		}
		if got := channelEventStatus(t, c, "m1"); got != step.want {
			t.Fatalf("after %s: status = %q, want %q", step.apply, got, step.want)
		}
	}
}

func channelTestCampaign(t *testing.T, c *Core) string {
	t.Helper()
	const id = "campaignbounce1"
	if _, err := c.db.Exec(`INSERT OR IGNORE INTO campaigns (id) VALUES (?)`, id); err != nil {
		t.Fatal(err)
	}
	return id
}

func channelBounceCount(t *testing.T, c *Core, sub string) int {
	t.Helper()
	var n int
	if err := c.db.Get(&n, `SELECT COUNT(*) FROM bounces WHERE subscriber_id = ?`, sub); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestStatusBeforeSendKeepsFailureUntilLinked(t *testing.T) {
	c, _ := channelTestCore(t)
	sub, err := c.UpsertChannelContact(ChannelContact{Channel: testChannel, Address: "chat1"})
	if err != nil {
		t.Fatal(err)
	}

	if ok, err := c.ApplyChannelStatus(testChannel, "m1", "failed", true); err != nil || ok {
		t.Fatalf("status before send: %v %v", err, ok)
	}
	first, err := c.RecordChannelEvent(ChannelEvent{
		Channel: testChannel, Direction: ChannelDirectionOutbound, Kind: "send", Status: "sent",
		ProviderMsgID: "m1", SubscriberID: sub.RecordID, CampaignID: channelTestCampaign(t, c), Address: "chat1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first {
		t.Fatal("the send should have found the stub")
	}
	if got := channelEventStatus(t, c, "m1"); got != "failed" {
		t.Fatalf("the send must not overwrite the failure, got %q", got)
	}
	if channelBounceCount(t, c, sub.RecordID) != 1 {
		t.Fatal("the held failure should record one bounce")
	}
	if to, _ := c.ChannelRecipients(sub, testChannel); len(to) != 0 {
		t.Fatalf("the failed address should be retired: %v", to)
	}
}

func channelBounceCore(t *testing.T, action string) (*Core, *pocketbase.PocketBase) {
	t.Helper()
	c, app := channelTestCore(t)
	c.consts.BounceActions = map[string]struct {
		Count  int
		Action string
	}{models.BounceTypeHard: {Count: 1, Action: action}}
	return c, app
}

func TestChannelBounceLeavesEmailDataAlone(t *testing.T) {
	for _, action := range []string{"delete", "blocklist", "unsubscribe"} {
		t.Run(action, func(t *testing.T) {
			c, app := channelBounceCore(t, action)
			_, listUUID := channelTestList(t, c, app)
			sub, err := c.InsertSubscriberForTest(t, "real@example.com")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.db.Exec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status, updated) SELECT ?, id, 'confirmed', '' FROM lists WHERE uuid = ?`, sub.RecordID, listUUID); err != nil {
				t.Fatal(err)
			}

			if err := c.RecordChannelBounce(testChannel, sub.RecordID, channelTestCampaign(t, c), models.BounceTypeHard, "131026"); err != nil {
				t.Fatal(err)
			}

			stored, ok, err := c.GetSubscriberForChannel(sub.RecordID)
			if err != nil || !ok {
				t.Fatalf("subscriber must survive: %v %v", err, ok)
			}
			if stored.Status != models.SubscriberStatusEnabled {
				t.Fatalf("status changed to %q", stored.Status)
			}
			var listStatus string
			if err := c.db.Get(&listStatus, `SELECT status FROM subscriber_lists WHERE subscriber_id = ?`, sub.RecordID); err != nil {
				t.Fatal(err)
			}
			if listStatus != "confirmed" {
				t.Fatalf("email subscription changed to %q", listStatus)
			}
			if channelBounceCount(t, c, sub.RecordID) != 1 {
				t.Fatal("the bounce should still be recorded")
			}
			if err := c.BlocklistBouncedSubscribers(); err != nil {
				t.Fatal(err)
			}
			if stored, _, _ := c.GetSubscriberForChannel(sub.RecordID); stored.Status != models.SubscriberStatusEnabled {
				t.Fatal("blocklisting bounced subscribers must skip channel bounces")
			}
		})
	}
}

func TestChannelBounceWithoutCampaignIsSkippedNotAnError(t *testing.T) {
	c, _ := channelTestCore(t)
	sub, err := c.InsertSubscriberForTest(t, "real@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.RecordChannelBounce(testChannel, sub.RecordID, "", "", "131026"); err != nil {
		t.Fatal(err)
	}
	if channelBounceCount(t, c, sub.RecordID) != 0 {
		t.Fatal("a bounce row needs a campaign")
	}
}

func phoneChannelCore(t *testing.T) (*Core, *pocketbase.PocketBase) {
	t.Helper()
	c, app := channelTestCore(t)
	models.RegisterChannel(models.ChannelSpec{
		ID: phoneChannel, Label: "Phone test", Address: models.AddressSourcePhone,
		Consent: models.ConsentSourceChannel, TextPipeline: true, Unsendable: models.UnsendableSkip,
	})
	return c, app
}

const phoneChannel = "ch:phonecore"

func TestOptOutReachesEveryoneSharingANumber(t *testing.T) {
	c, _ := phoneChannelCore(t)
	a, _ := c.InsertSubscriberForTest(t, "a@example.com", "+15550100")
	b, _ := c.InsertSubscriberForTest(t, "b@example.com", "+15550100")
	other, _ := c.InsertSubscriberForTest(t, "c@example.com", "+15550199")

	if _, err := c.OptOutChannelAddress(phoneChannel, "+15550100"); err != nil {
		t.Fatal(err)
	}
	for _, sub := range []models.Subscriber{a, b} {
		if out, err := c.ChannelHasOptOut(sub.RecordID, phoneChannel); err != nil || !out {
			t.Fatalf("%s should be opted out: %v %v", sub.Email, out, err)
		}
	}
	if out, _ := c.ChannelHasOptOut(other.RecordID, phoneChannel); out {
		t.Fatal("a different number must not be opted out")
	}
}

func TestFailedNumberIsNotAnOptOutAndInboundClearsIt(t *testing.T) {
	c, app := phoneChannelCore(t)
	listID, _ := channelTestList(t, c, app)
	sub, _ := c.InsertSubscriberForTest(t, "a@example.com", "+15550100")
	if err := c.SetChannelConsent(sub.RecordID, listID, phoneChannel, "confirmed", ConsentSourceAdmin); err != nil {
		t.Fatal(err)
	}
	if to, _ := c.ChannelRecipients(sub, phoneChannel); len(to) != 1 {
		t.Fatalf("expected the phone as recipient: %v", to)
	}

	if err := c.MarkChannelAddressInvalid(sub.RecordID, phoneChannel, "+15550100"); err != nil {
		t.Fatal(err)
	}
	if to, _ := c.ChannelRecipients(sub, phoneChannel); len(to) != 0 {
		t.Fatalf("an invalid number must not receive: %v", to)
	}
	if out, _ := c.ChannelHasOptOut(sub.RecordID, phoneChannel); out {
		t.Fatal("a failed number must not count as an opt-out")
	}

	if _, err := c.RecordChannelEvent(ChannelEvent{
		Channel: phoneChannel, Direction: ChannelDirectionInbound, Kind: "message",
		ProviderMsgID: "in1", SubscriberID: sub.RecordID, Address: "+15550100",
	}); err != nil {
		t.Fatal(err)
	}
	if to, _ := c.ChannelRecipients(sub, phoneChannel); len(to) != 1 {
		t.Fatalf("a message from the number should clear the mark: %v", to)
	}
}

func TestOptOutWithNoListsAndConsentClearsIt(t *testing.T) {
	c, app := phoneChannelCore(t)
	listID, _ := channelTestList(t, c, app)
	sub, _ := c.InsertSubscriberForTest(t, "a@example.com", "+15550100")

	if _, err := c.OptOutChannelSubscriber(sub.RecordID, phoneChannel); err != nil {
		t.Fatal(err)
	}
	if out, _ := c.ChannelHasOptOut(sub.RecordID, phoneChannel); !out {
		t.Fatal("a contact with no lists should still record an opt-out")
	}
	if err := c.SetChannelConsent(sub.RecordID, listID, phoneChannel, "confirmed", ConsentSourceAdmin); err != nil {
		t.Fatal(err)
	}
	if out, _ := c.ChannelHasOptOut(sub.RecordID, phoneChannel); out {
		t.Fatal("new consent should clear the channel-wide opt-out")
	}
}

func TestProviderAliasFindsSubscriber(t *testing.T) {
	c, _ := phoneChannelCore(t)
	sub, _ := c.InsertSubscriberForTest(t, "a@example.com", "+5511912345678")
	if err := c.SetChannelAddressAlias(sub.RecordID, phoneChannel, "551112345678"); err != nil {
		t.Fatal(err)
	}
	found, ok, err := c.FindSubscriberByChannelAddress(phoneChannel, "551112345678")
	if err != nil || !ok || found.RecordID != sub.RecordID {
		t.Fatalf("alias lookup: %v %v %+v", err, ok, found)
	}
}

func TestImportSMSConsent(t *testing.T) {
	c, app := phoneChannelCore(t)
	listID, _ := channelTestList(t, c, app)
	add := func(email, phone, status, sms string) models.Subscriber {
		sub, err := c.InsertSubscriberForTest(t, email, phone)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.db.Exec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status, sms_status, updated) VALUES (?, ?, ?, ?, '')`, sub.RecordID, listID, status, sms); err != nil {
			t.Fatal(err)
		}
		return sub
	}
	explicit := add("explicit@example.com", "+15550101", "confirmed", "confirmed")
	inherited := add("inherited@example.com", "+15550102", "confirmed", "")
	add("smsstop@example.com", "+15550103", "confirmed", "unsubscribed")
	add("nophone@example.com", "", "confirmed", "confirmed")
	stopped := add("stopped@example.com", "+15550104", "confirmed", "confirmed")
	if _, err := c.OptOutChannelSubscriber(stopped.RecordID, phoneChannel); err != nil {
		t.Fatal(err)
	}

	dry, err := c.ImportSMSConsent(phoneChannel, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if dry.Explicit != 1 || dry.Inherited != 1 || dry.Applied != 0 {
		t.Fatalf("dry run counts: %+v", dry)
	}
	var rows int
	mustSMSGet(t, c, &rows, `SELECT COUNT(*) FROM subscriber_channel_consent WHERE source = ?`, ConsentSourceSMSCopy)
	if rows != 0 {
		t.Fatal("a dry run must not change anything")
	}

	applied, err := c.ImportSMSConsent(phoneChannel, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Applied != 1 {
		t.Fatalf("applied: %+v", applied)
	}
	if to, _ := c.ChannelRecipients(explicit, phoneChannel); len(to) != 1 {
		t.Fatal("explicit SMS consent should copy")
	}
	if to, _ := c.ChannelRecipients(inherited, phoneChannel); len(to) != 0 {
		t.Fatal("inherited consent must stay out unless asked for")
	}
	if to, _ := c.ChannelRecipients(stopped, phoneChannel); len(to) != 0 {
		t.Fatal("an opt-out must never be overwritten")
	}

	if _, err := c.ImportSMSConsent(phoneChannel, false, true); err != nil {
		t.Fatal(err)
	}
	if to, _ := c.ChannelRecipients(inherited, phoneChannel); len(to) != 1 {
		t.Fatal("inherited consent should copy when asked for")
	}

	if _, err := c.SMSOptOutSubscriberByPhone("+15550101"); err != nil {
		t.Fatal(err)
	}
	if to, _ := c.ChannelRecipients(explicit, phoneChannel); len(to) != 0 {
		t.Fatal("an SMS stop should withdraw consent copied from SMS")
	}
}

func TestLastChannelInbound(t *testing.T) {
	c, _ := channelTestCore(t)
	if _, ok, err := c.LastChannelInbound("s1", testChannel); err != nil || ok {
		t.Fatalf("no events: %v %v", err, ok)
	}
	if _, err := c.RecordChannelEvent(ChannelEvent{Channel: testChannel, Direction: ChannelDirectionInbound, Kind: "message", SubscriberID: "s1"}); err != nil {
		t.Fatal(err)
	}
	at, ok, err := c.LastChannelInbound("s1", testChannel)
	if err != nil || !ok || time.Since(at) > time.Minute || time.Since(at) < -time.Minute {
		t.Fatalf("last inbound: %v %v %v", at, ok, err)
	}
}

// InsertSubscriberForTest adds a subscriber with a real email and optional phone.
func (c *Core) InsertSubscriberForTest(t *testing.T, email string, phone ...string) (models.Subscriber, error) {
	t.Helper()
	sub := models.Subscriber{Email: email, Name: email}
	if len(phone) > 0 {
		sub.Phone = phone[0]
	}
	out, _, err := c.InsertSubscriber(sub, nil, nil, true, false)
	if err != nil {
		t.Fatal(err)
	}
	return out, nil
}
