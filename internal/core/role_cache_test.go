package core

import (
	"testing"

	"github.com/compdani/list_pocket/internal/auth"
	null "gopkg.in/volatiletech/null.v6"
)

func TestCachedRolesInvalidatedByRoleWrites(t *testing.T) {
	c, _, _ := spamTestCore(t)

	before, _, err := c.cachedRoles()
	if err != nil {
		t.Fatal(err)
	}
	again, _, err := c.cachedRoles()
	if err != nil || len(again) != len(before) {
		t.Fatalf("cached read changed: %v %d != %d", err, len(again), len(before))
	}

	if _, err := c.CreateRole(auth.Role{Name: null.StringFrom("cache-test"), Permissions: []string{"subscribers:get"}}); err != nil {
		t.Fatal(err)
	}
	after, _, err := c.cachedRoles()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before)+1 {
		t.Fatalf("roles after create = %d, want %d (stale cache)", len(after), len(before)+1)
	}
}
