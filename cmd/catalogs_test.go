package main

import "testing"

func TestCatalogCollectionsCannotBypassMailapi(t *testing.T) {
	a, _, _ := boardsTestApp(t)
	for _, name := range []string{"pictures", "subscriber_tags", "campaign_tags"} {
		col, err := a.pb.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatal(err)
		}
		if col.ListRule != nil || col.ViewRule != nil || col.CreateRule != nil || col.UpdateRule != nil || col.DeleteRule != nil {
			t.Errorf("%s permits direct record API access", name)
		}
	}
}
