package main

import (
	"net/http/httptest"
	"strings"
	"testing"

	pbcore "ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/compdani/list_pocket/internal/auth"
)

func TestPatchSubscriberValidationAndPermissions(t *testing.T) {
	a, id, _ := boardsTestApp(t)
	a.auth = &auth.Auth{}
	call := func(body string, user auth.User) error {
		t.Helper()
		e := &pbcore.RequestEvent{App: a.pb}
		e.Request = httptest.NewRequest("PATCH", "/mailapi/subscribers/"+id, strings.NewReader(body))
		e.Request.SetPathValue("id", id)
		e.Request.Header.Set("Content-Type", "application/json")
		e.Response = httptest.NewRecorder()
		e.Set(auth.UserHTTPCtxKey, user)
		return a.auth.PermRE(a.PatchSubscriber, "subscribers:manage")(e)
	}
	admin := auth.User{UserRoleID: auth.SuperAdminRoleID}
	for _, body := range []string{`{}`, `null`, `{"phone":null}`, `{"phone":5}`, `{"attribs":[]}`, `{"attribs":null}`, `{"status":"enabled"}`, `{"list_record_ids":[]}`} {
		if err := call(body, admin); err == nil {
			t.Errorf("accepted invalid patch: %s", body)
		}
	}
	if err := call(`{"phone":"+15551234567"}`, auth.User{}); err == nil {
		t.Fatal("update without permission succeeded")
	}
	if err := call(`{"phone":"+15551234567"}`, admin); err != nil {
		t.Fatal(err)
	}
	got, err := a.core.GetSubscriber(0, id, "")
	if err != nil || got.Email != "a@example.com" || got.Name != "Ada" || got.Phone != "+15551234567" {
		t.Fatalf("unexpected contact: %+v %v", got, err)
	}
}
