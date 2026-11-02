package main

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/compdani/list_pocket/internal/flow/tools"
	"github.com/compdani/list_pocket/models"
)

// contactFromTool reads a contact the way a workflow step does: through the tool, as JSON.
func contactFromTool(t *testing.T, a *App, ref string) map[string]any {
	t.Helper()
	for _, tool := range a.flowTools() {
		if tool.Name() != "contacts.get" {
			continue
		}
		out, err := tool.Invoke(context.Background(), tools.Call{Args: map[string]any{"contact": ref}})
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		var contact map[string]any
		if err := json.Unmarshal(raw, &contact); err != nil {
			t.Fatal(err)
		}
		return contact
	}
	t.Fatal("contacts.get isn't in the catalog")
	return nil
}

// addAuthUserColumn adds what auth adds to users when the app starts, which the migrations
// on their own don't.
func addAuthUserColumn(t *testing.T, a *App) {
	t.Helper()
	if _, err := a.pb.DB().NewQuery(`ALTER TABLE users ADD COLUMN username TEXT DEFAULT ''`).Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestContactsGetReturnsListMemberships(t *testing.T) {
	a, ref, _ := boardsTestApp(t)
	addAuthUserColumn(t, a)

	contact := contactFromTool(t, a, ref)
	if lists, ok := contact["lists"].([]any); !ok || len(lists) != 0 {
		t.Fatalf("a contact on no lists should have an empty list, got %#v", contact["lists"])
	}

	sub, err := a.contactFromRef(ref)
	if err != nil {
		t.Fatal(err)
	}
	subscribe := func(name, status string) models.List {
		t.Helper()
		list, err := a.core.CreateList(models.List{Name: name, Type: models.ListTypePublic, Optin: models.ListOptinSingle})
		if err != nil {
			t.Fatal(err)
		}
		if err := a.core.AddSubscriptions([]int{sub.ID}, []int{list.ID}, status); err != nil {
			t.Fatal(err)
		}
		return list
	}
	confirmed := subscribe("News", models.SubscriptionStatusConfirmed)
	unsubscribed := subscribe("Offers", models.SubscriptionStatusUnconfirmed)
	if err := a.core.UnsubscribeLists([]int{sub.ID}, []int{unsubscribed.ID}, nil); err != nil {
		t.Fatal(err)
	}

	contact = contactFromTool(t, a, ref)
	statuses := map[string]string{}
	schema := flowToolOutputs()["contacts.get"]["properties"].(map[string]any)["lists"].(map[string]any)["items"].(map[string]any)
	for _, raw := range contact["lists"].([]any) {
		list := raw.(map[string]any)
		statuses[list["id"].(string)] = list["subscription_status"].(string)
		if got, want := sortedKeys(list), propertyNames(schema); !reflect.DeepEqual(got, want) {
			t.Errorf("list has fields %v, the schema says %v", got, want)
		}
	}
	want := map[string]string{
		confirmed.RecordID:    models.SubscriptionStatusConfirmed,
		unsubscribed.RecordID: models.SubscriptionStatusUnsubscribed,
	}
	if !reflect.DeepEqual(statuses, want) {
		t.Fatalf("statuses = %v, want %v", statuses, want)
	}

	// Every field the tool returns has to be one code hints and the assistant know about.
	if got, want := sortedKeys(contact), propertyNames(flowToolOutputs()["contacts.get"]); !reflect.DeepEqual(got, want) {
		t.Errorf("contact has fields %v, the schema says %v", got, want)
	}
}

func sortedKeys(m map[string]any) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
