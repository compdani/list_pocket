package core

import (
	"encoding/json"
	"testing"

	"github.com/compdani/list_pocket/models"
)

func TestPatchSubscriberProfilePreservesOtherFieldsAndSubscriptions(t *testing.T) {
	c, _ := channelTestCore(t)
	list, err := c.CreateList(models.List{Name: "Members", Type: "private", Optin: "double"})
	if err != nil {
		t.Fatal(err)
	}
	sub, _, err := c.InsertSubscriber(models.Subscriber{Email: "ada@example.com", Phone: "+15551234567", FirstName: "Ada", LastName: "Lovelace", Status: "enabled", Attribs: models.JSON{"company": "Acme"}}, []int{list.ID}, nil, true, false)
	if err != nil {
		t.Fatal(err)
	}
	before, err := c.GetSubscriberLists(sub.ID, "", nil, nil, "", "")
	if err != nil || len(before) != 1 {
		t.Fatalf("memberships: %+v %v", before, err)
	}
	got, err := c.PatchSubscriberProfile(sub.RecordID, map[string]any{"first_name": "Augusta"})
	if err != nil {
		t.Fatal(err)
	}
	if got.FirstName != "Augusta" || got.LastName != "Lovelace" || got.Name != "Augusta Lovelace" || got.Phone != sub.Phone || got.Email != sub.Email || got.Status != sub.Status || got.Attribs["company"] != "Acme" {
		t.Fatalf("unexpected contact: %+v", got)
	}
	after, err := c.GetSubscriberLists(sub.ID, "", nil, nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(before)
	a, _ := json.Marshal(after)
	if string(a) != string(b) {
		t.Fatalf("subscriptions changed: %s -> %s", b, a)
	}
	got, err = c.PatchSubscriberProfile(sub.RecordID, map[string]any{"phone": "", "attribs": map[string]any{}})
	if err != nil || got.Phone != "" || len(got.Attribs) != 0 || got.Name != "Augusta Lovelace" {
		t.Fatalf("clear: %+v %v", got, err)
	}
	if _, err := c.PatchSubscriberProfile(sub.RecordID, map[string]any{"status": "blocklisted"}); err == nil {
		t.Fatal("accepted unsupported field")
	}
}
