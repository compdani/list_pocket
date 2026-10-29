package auth

import (
	"testing"

	"github.com/pocketbase/pocketbase/tools/types"
)

func TestNormalizeStringArrayReadsPocketBaseJSON(t *testing.T) {
	raw := types.JSONRaw(`["users:get","lists:manage"]`)
	got := normalizeStringArray(raw)
	if len(got) != 2 || got[0] != "users:get" || got[1] != "lists:manage" {
		t.Fatalf("normalizeStringArray(JSONRaw) = %#v", got)
	}
}
