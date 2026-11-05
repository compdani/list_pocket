package auth

import (
	"io"
	"log"
	"testing"

	"ganigeorgiev.com/pocketbase/pocketbase"
	pbcore "ganigeorgiev.com/pocketbase/pocketbase/core"
	null "gopkg.in/volatiletech/null.v6"
)

func TestSyncUserSkipsUnchangedWrites(t *testing.T) {
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.ResetBootstrapState() })
	if err := app.RunAppMigrations(); err != nil {
		t.Fatal(err)
	}
	a, err := New(Config{AuthCollection: "users"}, nil, app, &Callbacks{}, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}

	updates := 0
	app.OnRecordUpdate("users").BindFunc(func(e *pbcore.RecordEvent) error {
		updates++
		return e.Next()
	})

	u := User{
		Username:      "admin",
		Email:         null.StringFrom("admin@example.com"),
		Type:          UserTypeUser,
		Status:        UserStatusEnabled,
		PasswordLogin: true,
	}
	if err := a.SyncUser(u); err != nil {
		t.Fatal(err)
	}
	if err := a.SyncUser(u); err != nil {
		t.Fatal(err)
	}
	if updates != 0 {
		t.Fatalf("unchanged sync wrote %d times, want 0", updates)
	}

	u.Status = UserStatusDisabled
	if err := a.SyncUser(u); err != nil {
		t.Fatal(err)
	}
	if updates != 1 {
		t.Fatalf("changed sync wrote %d times, want 1", updates)
	}
}
