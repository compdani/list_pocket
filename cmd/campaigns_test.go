package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestNormalizeCampaignReqBodyLists(t *testing.T) {
	// The shape a campaign read returns, plus bare record IDs and int IDs.
	body := []byte(`{"name":"x","lists":[{"id":"rec_a","name":"A"},"rec_b",3,"4",{"id":"rec_a"}],"list_record_ids":["rec_c"]}`)
	normalized, err := normalizeCampaignReqBody(body)
	if err != nil {
		t.Fatal(err)
	}
	var out campReq
	if err := json.Unmarshal(normalized, &out); err != nil {
		t.Fatalf("normalized body doesn't bind: %v", err)
	}
	if want := []int{3, 4}; !reflect.DeepEqual(out.ListIDs, want) {
		t.Fatalf("ListIDs = %v, want %v", out.ListIDs, want)
	}
	if want := []string{"rec_c", "rec_a", "rec_b"}; !reflect.DeepEqual(out.ListRecordIDs, want) {
		t.Fatalf("ListRecordIDs = %v, want %v", out.ListRecordIDs, want)
	}
}

func TestNormalizeCampaignReqBodyRejectsNonArrayLists(t *testing.T) {
	if _, err := normalizeCampaignReqBody([]byte(`{"lists":"rec_a"}`)); err == nil {
		t.Fatal("expected an error for a non-array lists value")
	}
}
