package core

import (
	"testing"

	"github.com/compdani/list_pocket/models"
)

func crmTestContact(t *testing.T, c *Core, email string) models.Subscriber {
	t.Helper()
	sub, _, err := c.InsertSubscriber(models.Subscriber{Email: email, Name: "Test " + email}, nil, nil, true, false)
	if err != nil {
		t.Fatal(err)
	}
	return sub
}

func crmSetup(t *testing.T) *Core {
	t.Helper()
	c, _ := channelTestCore(t)
	// The auth service adds these columns to users at boot, which tests don't run.
	for _, col := range []string{"username", "user_type", "status"} {
		if _, err := c.db.Exec(`ALTER TABLE users ADD COLUMN ` + col + ` TEXT DEFAULT '' NOT NULL`); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.EnsureCRMTriggers(); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestContactProfile_partialUpdateKeepsOtherFields(t *testing.T) {
	c := crmSetup(t)
	sub := crmTestContact(t, c, "a@example.com")

	company := "Acme"
	if _, err := c.UpdateContactProfile(sub.RecordID, ContactProfileUpdate{Company: &company}); err != nil {
		t.Fatal(err)
	}
	owner := ""
	got, err := c.UpdateContactProfile(sub.RecordID, ContactProfileUpdate{Owner: &owner})
	if err != nil {
		t.Fatal(err)
	}
	if got.Company != "Acme" {
		t.Fatalf("got %+v", got)
	}

	unknown := "no-such-user"
	if _, err := c.UpdateContactProfile(sub.RecordID, ContactProfileUpdate{Owner: &unknown}); err == nil {
		t.Fatal("expected an unknown owner to be refused")
	}
}

func TestContactNotes_scopedToContactAndPinnedFirst(t *testing.T) {
	c := crmSetup(t)
	a := crmTestContact(t, c, "a@example.com")
	b := crmTestContact(t, c, "b@example.com")

	first, err := c.AddContactNote(a.RecordID, "first", "u1", "Sam")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.AddContactNote(a.RecordID, "second", "u1", "Sam"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AddContactNote(a.RecordID, "   ", "u1", "Sam"); err == nil {
		t.Fatal("expected an empty note to be refused")
	}
	pinned := true
	if _, err := c.UpdateContactNote(a.RecordID, first.ID, nil, &pinned); err != nil {
		t.Fatal(err)
	}
	notes, err := c.ListContactNotes(a.RecordID)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 || notes[0].ID != first.ID || !notes[0].Pinned {
		t.Fatalf("got %+v", notes)
	}

	// Permission is checked against the contact in the URL, so a note can't be reached
	// through a different contact.
	if err := c.DeleteContactNote(b.RecordID, first.ID); err == nil {
		t.Fatal("expected deleting through another contact to fail")
	}
	body := "changed"
	if _, err := c.UpdateContactNote(b.RecordID, first.ID, &body, nil); err == nil {
		t.Fatal("expected editing through another contact to fail")
	}
}

func TestContactTasks_completionAndOrdering(t *testing.T) {
	c := crmSetup(t)
	sub := crmTestContact(t, c, "a@example.com")
	title := func(s string) *string { return &s }

	undated, err := c.AddContactTask(sub.RecordID, ContactTaskInput{Title: title("undated")}, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.AddContactTask(sub.RecordID, ContactTaskInput{Title: title("later"), DueAt: title("2030-02-01T10:00:00Z")}, "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AddContactTask(sub.RecordID, ContactTaskInput{Title: title("soon"), DueAt: title("2030-01-01T10:00:00+02:00")}, "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AddContactTask(sub.RecordID, ContactTaskInput{Title: title("bad"), DueAt: title("tomorrow")}, "u1"); err == nil {
		t.Fatal("expected an invalid due date to be refused")
	}

	tasks, err := c.ListContactTasks(ContactTaskQuery{Subscriber: sub.RecordID})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 || tasks[0].Title != "soon" || tasks[1].Title != "later" || tasks[2].Title != "undated" {
		t.Fatalf("got %+v", tasks)
	}
	if tasks[0].DueAt != "2030-01-01 08:00:00.000Z" {
		t.Fatalf("due date should be stored in UTC, got %q", tasks[0].DueAt)
	}

	done := ContactTaskDone
	got, err := c.UpdateContactTask(sub.RecordID, undated.ID, ContactTaskInput{Status: &done})
	if err != nil {
		t.Fatal(err)
	}
	if got.CompletedAt == "" {
		t.Fatal("finishing a task should stamp when")
	}
	open := ContactTaskOpen
	if got, err = c.UpdateContactTask(sub.RecordID, undated.ID, ContactTaskInput{Status: &open}); err != nil {
		t.Fatal(err)
	}
	if got.CompletedAt != "" {
		t.Fatal("reopening a task should clear the completion time")
	}
}

func TestCRMDataIsRemovedWithSubscriber(t *testing.T) {
	c := crmSetup(t)
	sub := crmTestContact(t, c, "a@example.com")
	company := "Acme"
	title := "call"
	if _, err := c.UpdateContactProfile(sub.RecordID, ContactProfileUpdate{Company: &company}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AddContactNote(sub.RecordID, "note", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AddContactTask(sub.RecordID, ContactTaskInput{Title: &title}, ""); err != nil {
		t.Fatal(err)
	}
	if err := c.MarkConversationRead(sub.RecordID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.Exec(`DELETE FROM subscribers WHERE id = ?`, sub.RecordID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"contact_profiles", "contact_notes", "contact_tasks"} {
		var n int
		if err := c.db.Get(&n, `SELECT COUNT(*) FROM `+table+` WHERE subscriber = ?`, sub.RecordID); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%s kept %d rows", table, n)
		}
	}
	var n int
	if err := c.db.Get(&n, `SELECT COUNT(*) FROM conversation_state WHERE key = ?`, sub.RecordID); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("conversation_state kept its row")
	}
}

func crmInboundEmail(t *testing.T, c *Core, id, from, subject, spam, at string) {
	t.Helper()
	if _, err := c.db.Exec(`
INSERT INTO inbound_email_replies (id, from_address, subject, body_snippet, message_id, received_at, spam_status, dedupe_key)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, from, subject, "snippet "+subject, "<"+id+"@x>", at, spam, id); err != nil {
		t.Fatal(err)
	}
}

func TestConversations_groupAcrossChannelsAndTrackState(t *testing.T) {
	c := crmSetup(t)
	sub := crmTestContact(t, c, "known@example.com")

	// An inbound email with no stored link still joins the contact by address.
	crmInboundEmail(t, c, "mail00000000001", "Known@Example.com", "hello", "", "2026-01-01 10:00:00.000Z")
	if _, err := c.RecordChannelEvent(ChannelEvent{
		Channel: testChannel, Direction: ChannelDirectionInbound, Kind: "message", ProviderMsgID: "p1",
		SubscriberID: sub.RecordID, Address: "addr", Body: "from the channel",
	}); err != nil {
		t.Fatal(err)
	}
	crmInboundEmail(t, c, "mail00000000002", "stranger@example.com", "who are you", "", "2026-01-02 10:00:00.000Z")
	crmInboundEmail(t, c, "mail00000000003", "spammer@example.com", "buy now", "spam", "2026-01-03 10:00:00.000Z")

	// A campaign send isn't a conversation, and a status stub has no address.
	stranger := crmTestContact(t, c, "campaign-only@example.com")
	if _, err := c.RecordChannelEvent(ChannelEvent{
		Channel: testChannel, Direction: ChannelDirectionOutbound, Kind: "send", ProviderMsgID: "p2",
		SubscriberID: stranger.RecordID, CampaignID: "camp1", Address: "other", Body: "blast",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyChannelStatus(testChannel, "stub", "delivered", false); err != nil {
		t.Fatal(err)
	}

	list, err := c.ListConversations(ConversationQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 2 || list.Unread != 2 {
		t.Fatalf("got %+v", list)
	}
	byKey := map[string]Conversation{}
	for _, r := range list.Results {
		byKey[r.Key] = r
	}
	known, ok := byKey[sub.RecordID]
	if !ok || known.Messages != 2 || len(known.Channels) != 2 || !known.Unread {
		t.Fatalf("known contact: %+v", known)
	}
	if _, ok := byKey["email:stranger@example.com"]; !ok {
		t.Fatalf("stranger missing: %+v", byKey)
	}

	filtered, err := c.ListConversations(ConversationQuery{Channel: testChannel})
	if err != nil {
		t.Fatal(err)
	}
	if filtered.Total != 1 || filtered.Results[0].Key != sub.RecordID {
		t.Fatalf("channel filter: %+v", filtered)
	}
	found, err := c.ListConversations(ConversationQuery{Search: "WHO ARE"})
	if err != nil {
		t.Fatal(err)
	}
	if found.Total != 1 || found.Results[0].Key != "email:stranger@example.com" {
		t.Fatalf("search: %+v", found)
	}

	if err := c.MarkConversationRead(sub.RecordID); err != nil {
		t.Fatal(err)
	}
	unread, err := c.ListConversations(ConversationQuery{View: ConversationViewUnread})
	if err != nil {
		t.Fatal(err)
	}
	if unread.Total != 1 || unread.Unread != 1 {
		t.Fatalf("after reading: %+v", unread)
	}

	if err := c.SetConversationClosed("email:stranger@example.com", true); err != nil {
		t.Fatal(err)
	}
	closed, err := c.ListConversations(ConversationQuery{View: ConversationViewClosed})
	if err != nil {
		t.Fatal(err)
	}
	if closed.Total != 1 || closed.Results[0].Key != "email:stranger@example.com" {
		t.Fatalf("closed: %+v", closed)
	}

	// A new message from them reopens it.
	crmInboundEmail(t, c, "mail00000000004", "stranger@example.com", "still there?", "", "2099-01-01 10:00:00.000Z")
	open, err := c.ListConversations(ConversationQuery{View: ConversationViewOpen})
	if err != nil {
		t.Fatal(err)
	}
	if open.Results[0].Key != "email:stranger@example.com" || !open.Results[0].Unread {
		t.Fatalf("reopened: %+v", open.Results)
	}
}

func TestConversationThread_showsCampaignSendsOnlyForContacts(t *testing.T) {
	c := crmSetup(t)
	sub := crmTestContact(t, c, "known@example.com")
	if _, err := c.RecordChannelEvent(ChannelEvent{
		Channel: testChannel, Direction: ChannelDirectionOutbound, Kind: "send", ProviderMsgID: "p1",
		SubscriberID: sub.RecordID, CampaignID: "camp1", Address: "addr", Body: "blast",
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.RecordConversationReply(models.ChannelEmail, sub.RecordID, "known@example.com", "Re: hi", "thanks", "m1"); err != nil {
		t.Fatal(err)
	}

	thread, err := c.GetConversationThread(sub.RecordID)
	if err != nil {
		t.Fatal(err)
	}
	if len(thread.Messages) != 2 {
		t.Fatalf("got %+v", thread.Messages)
	}
	var reply ConversationMessage
	for _, m := range thread.Messages {
		if m.Channel == models.ChannelEmail {
			reply = m
		}
	}
	if reply.Subject != "Re: hi" || reply.Direction != ChannelDirectionOutbound || reply.Status != "queued" {
		t.Fatalf("reply: %+v", reply)
	}

	list, err := c.ListConversations(ConversationQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 || list.Results[0].Messages != 1 {
		t.Fatalf("the list should only count the reply: %+v", list)
	}

	if _, err := c.GetConversationThread("email:nobody@example.com"); err == nil {
		t.Fatal("expected an unknown conversation to be not found")
	}
}

func TestHasSMSOptOut_anyListOrStopKeyword(t *testing.T) {
	c, app := channelTestCore(t)
	sub := crmTestContact(t, c, "a@example.com")
	got, err := c.HasSMSOptOut(sub)
	if err != nil || got {
		t.Fatalf("got %v, %v", got, err)
	}
	listID, _ := channelTestList(t, c, app)
	if _, err := c.db.Exec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status, sms_status) VALUES (?, ?, 'confirmed', ?)`,
		sub.RecordID, listID, models.SubscriptionStatusUnsubscribed); err != nil {
		t.Fatal(err)
	}
	if got, err = c.HasSMSOptOut(sub); err != nil || !got {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestRecordChannelEvent_reportsNewInboundOnce(t *testing.T) {
	c := crmSetup(t)
	var seen []ChannelEvent
	c.h.OnChannelInbound = func(e ChannelEvent) { seen = append(seen, e) }
	in := ChannelEvent{Channel: testChannel, Direction: ChannelDirectionInbound, Kind: "message", ProviderMsgID: "p1", Address: "a", Body: "hi"}
	for range 2 {
		if _, err := c.RecordChannelEvent(in); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.RecordChannelEvent(ChannelEvent{Channel: testChannel, Direction: ChannelDirectionOutbound, Kind: "reply", ProviderMsgID: "p2", Address: "a"}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 || seen[0].Body != "hi" {
		t.Fatalf("a redelivered webhook or an outbound send must not raise it: %+v", seen)
	}
}
