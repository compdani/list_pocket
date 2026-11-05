package main

import (
	"github.com/compdani/list_pocket/internal/auth"
	null "gopkg.in/volatiletech/null.v6"
	"io"
	"log"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	pbcore "ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/filesystem"
)

func TestNativeUsersRecordsCannotEscalateRoles(t *testing.T) {
	a, _, _ := boardsTestApp(t)
	col, err := a.pb.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	user := pbcore.NewRecord(col)
	user.SetEmail("owner@example.com")
	user.SetPassword("test-password")
	if err := a.pb.Save(user); err != nil {
		t.Fatal(err)
	}
	token, err := user.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	routes, err := apis.NewRouter(a.pb)
	if err != nil {
		t.Fatal(err)
	}
	mux, err := routes.BuildMux()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ method, path, body, token string }{
		{"POST", "/api/collections/users/records", `{"email":"evil@example.com","password":"test-password","passwordConfirm":"test-password","role":"1"}`, ""},
		{"PATCH", "/api/collections/users/records/" + user.Id, `{"role":"1"}`, token},
		{"GET", "/api/collections/inbound_email_replies/records", "", token},
	} {
		request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", tc.token)
		out := httptest.NewRecorder()
		mux.ServeHTTP(out, request)
		if out.Code < 400 {
			t.Fatalf("native route permitted %s %s: %d %s", tc.method, tc.path, out.Code, out.Body)
		}
	}
}

func TestAuthorizedAttachmentStreamsThroughPocketBaseFilesystem(t *testing.T) {
	a, _, _ := boardsTestApp(t)
	col, err := a.pb.FindCollectionByNameOrId("inbound_email_replies")
	if err != nil {
		t.Fatal(err)
	}
	reply := pbcore.NewRecord(col)
	reply.Load(map[string]any{"from_address": "sender@example.com", "subject": "Proof", "message_id": "proof@example.com", "received_at": time.Now(), "processed_at": time.Now(), "body_snippet": "proof", "match_score": "unmatched", "dedupe_key": "proof"})
	if err := a.pb.Save(reply); err != nil {
		t.Fatal(err)
	}
	col, err = a.pb.FindCollectionByNameOrId("inbound_email_attachments")
	if err != nil {
		t.Fatal(err)
	}
	file, err := filesystem.NewFileFromBytes([]byte("attachment bytes"), "proof.txt")
	if err != nil {
		t.Fatal(err)
	}
	attachment := pbcore.NewRecord(col)
	attachment.Set("inbound_email_reply_id", reply.Id)
	attachment.Set("file", file)
	attachment.Set("original_name", "original proof.txt")
	if err := a.pb.Save(attachment); err != nil {
		t.Fatal(err)
	}
	a.cfg = &Config{}
	a.auth, err = auth.New(auth.Config{}, nil, a.pb, &auth.Callbacks{}, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	user, err := a.auth.UpsertUserAuthRecord(auth.User{Username: "reader", Email: null.StringFrom("reader@example.com"), Type: auth.UserTypeUser, Status: auth.UserStatusEnabled, UserRoleID: 1}, "")
	if err != nil {
		t.Fatal(err)
	}
	token, err := user.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	routes, err := apis.NewRouter(a.pb)
	if err != nil {
		t.Fatal(err)
	}
	routes.BindFunc(func(e *pbcore.RequestEvent) error { e.Set(appContextKey, a); return e.Next() })
	registerHandlers(routes, a)
	mux, err := routes.BuildMux()
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/mailapi/inbound-email-attachments/"+attachment.Id+"/download", nil)
	request.Header.Set("Authorization", token)
	out := httptest.NewRecorder()
	mux.ServeHTTP(out, request)
	if out.Code != 200 || out.Body.String() != "attachment bytes" || out.Header().Get("Location") != "" {
		t.Fatalf("download: %d %s", out.Code, out.Body)
	}
	if out.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("unsafe private file caching: %s", out.Header())
	}
	if !strings.Contains(out.Header().Get("Content-Disposition"), "original proof.txt") {
		t.Fatalf("filename missing: %s", out.Header())
	}
	user.Set("role", "0")
	if err := a.pb.Save(user); err != nil {
		t.Fatal(err)
	}
	out = httptest.NewRecorder()
	mux.ServeHTTP(out, request.Clone(request.Context()))
	if out.Code != 403 {
		t.Fatalf("user without permission downloaded attachment: %d %s", out.Code, out.Body)
	}

	out = httptest.NewRecorder()
	mux.ServeHTTP(out, httptest.NewRequest("GET", "/api/files/"+col.Id+"/"+attachment.Id+"/"+attachment.GetString("file"), nil))
	if out.Code != 404 {
		t.Fatalf("protected file exposed: %d", out.Code)
	}
}
