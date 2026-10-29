package core

import (
	"io"
	"log"
	"testing"

	"github.com/compdani/list_pocket/internal/auth"
	"github.com/compdani/list_pocket/internal/i18n"
	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/tools/types"
	null "gopkg.in/volatiletech/null.v6"
)

func TestParsePermissionsReadsPocketBaseJSON(t *testing.T) {
	raw := types.JSONRaw(`["users:get","lists:get"]`)
	got := parsePermissions(raw)
	if !sameStringSet(got, []string{"users:get", "lists:get"}) {
		t.Fatalf("parsePermissions(JSONRaw) = %#v", got)
	}
}

func TestRolesRoundTrip(t *testing.T) {
	pb := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	if err := pb.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pb.ResetBootstrapState() })

	db, err := pbdb.NewFromPocketBase(pb)
	if err != nil {
		t.Fatal(err)
	}
	lang, err := i18n.New([]byte(`{"_.code":"en","_.name":"English"}`))
	if err != nil {
		t.Fatal(err)
	}
	c := New(&Opt{
		I18n: lang,
		DB:   db,
		Log:  log.New(io.Discard, "", 0),
	}, &Hooks{})

	created, err := c.CreateRole(auth.Role{
		Name:        null.StringFrom("Editor"),
		Permissions: []string{"users:get", "lists:get"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sameStringSet(created.Permissions, []string{"users:get", "lists:get"}) {
		t.Fatalf("create response permissions = %#v", created.Permissions)
	}

	updated, err := c.UpdateUserRole(created.RecordID, auth.Role{
		Name:        null.StringFrom("Editor"),
		Permissions: []string{"users:get", "users:manage"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sameStringSet(updated.Permissions, []string{"users:get", "users:manage"}) {
		t.Fatalf("update response permissions = %#v", updated.Permissions)
	}

	roles, err := c.GetRoles()
	if err != nil {
		t.Fatal(err)
	}
	var stored auth.Role
	for _, role := range roles {
		if role.Name.String == "Editor" {
			stored = role
			break
		}
	}
	if stored.RecordID == "" || !sameStringSet(stored.Permissions, []string{"users:get", "users:manage"}) {
		t.Fatalf("stored user role = %#v", stored)
	}

	if _, err := c.CreateListRole(auth.ListRole{
		Name: null.StringFrom("List editors"),
		Lists: []auth.ListPermission{{
			ID:          "listrecordid001",
			Permissions: []string{"list:get", "list:manage"},
		}},
	}); err != nil {
		t.Fatal(err)
	}

	listRoles, err := c.GetListRoles()
	if err != nil {
		t.Fatal(err)
	}
	var listRole auth.ListRole
	for _, role := range listRoles {
		if role.Name.String == "List editors" {
			listRole = role
			break
		}
	}
	if listRole.RecordID == "" || len(listRole.Lists) != 1 || !sameStringSet(listRole.Lists[0].Permissions, []string{"list:get", "list:manage"}) {
		t.Fatalf("stored list role = %#v", listRole)
	}
}
