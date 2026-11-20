package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	pbcore "ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/jmoiron/sqlx/types"
)

// /api/config tells the admin UI which chat API the server is running. Both
// values matter: "legacy" is the default, and the module's routes only exist
// when LISTPOCKET_CHAT_BACKEND=module.
func TestServerConfigReportsChatBackend(t *testing.T) {
	f := newMCPFixture(t)
	// The config endpoint lists the i18n files from the app's asset fs.
	f.app.fs = os.DirFS("..")

	// boardsTestApp builds core without a settings getter, which only /api/config needs.
	db, err := pbdb.NewFromPocketBase(f.app.pb)
	if err != nil {
		t.Fatal(err)
	}
	f.app.core = core.New(&core.Opt{
		DB:   db,
		I18n: f.app.i18n,
		Log:  f.app.log,
		GetSettings: func() (types.JSONText, error) {
			return types.JSONText(`{}`), nil
		},
	}, &core.Hooks{})

	for _, tc := range []struct {
		env  string
		want string
	}{
		{env: "", want: "legacy"},
		{env: "legacy", want: "legacy"},
		{env: "module", want: "module"},
	} {
		t.Setenv("LISTPOCKET_CHAT_BACKEND", tc.env)

		rec := httptest.NewRecorder()
		re := &pbcore.RequestEvent{Request: httptest.NewRequest(http.MethodGet, "/api/config", nil), Response: rec}
		re.Set(appContextKey, f.app)
		if err := f.app.GetServerConfig(re); err != nil {
			t.Fatalf("GetServerConfig(%q): %v", tc.env, err)
		}

		var out struct {
			Data struct {
				ChatBackend string `json:"chat_backend"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decoding /api/config: %v", err)
		}
		if out.Data.ChatBackend != tc.want {
			t.Errorf("LISTPOCKET_CHAT_BACKEND=%q: chat_backend = %q, want %q", tc.env, out.Data.ChatBackend, tc.want)
		}
	}
}
