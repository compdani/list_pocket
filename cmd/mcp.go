package main

// The MCP endpoint turns List Pocket's assistant tools into an HTTP MCP server at
// /mcp. See docs/mcp.md for what it offers and how clients sign in.

import (
	stdlog "log"
	"log/slog"
	"net/url"
	"strings"
	"time"

	pbcore "ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/compdani/list_pocket/internal/auth"
	pbmcp "github.com/compdani/pocketbase-mcp"
	pbadapter "github.com/compdani/pocketbase-mcp/adapters/pocketbase"
)

const mcpInstructions = `List Pocket is an email, SMS and WhatsApp marketing app with a CRM and workflow automation.
Tools read and write through List Pocket's own permission-checked handlers. A write on this
endpoint runs immediately, without a proposal: look a record up first, keep changes minimal,
and never retry a write that failed because it may already have applied.`

// assistantToolTimeout bounds one tool call and the approval that can precede it.
// The module cuts a chat tool off at MaxToolTimeout while the person reviewing it
// has DecisionTimeout to approve, so the two must stay equal: an approved change
// runs inside the decision request and would otherwise be cut off mid-write.
const assistantToolTimeout = 5 * time.Minute

// mountMCP builds the endpoint once per serve and registers its routes. A failure
// is logged and leaves the rest of the app serving: the assistant doesn't depend
// on it.
func (a *App) mountMCP(se *pbcore.ServeEvent) {
	if a.pb == nil {
		a.logf("mcp: endpoint disabled: no PocketBase app")
		return
	}
	cfg := pbmcp.Config{
		Path:         "/mcp",
		ServerName:   "listpocket",
		ServerTitle:  "List Pocket",
		Instructions: mcpInstructions,
		// users tokens only. A _superusers token gets 401.
		Auth: pbmcp.AuthConfig{Collections: []string{"users"}},
		// OAuth stays off: stock OAuth signs in with a password, which would skip
		// List Pocket's own TOTP second factor (cmd/auth.go).
		OAuth: pbmcp.OAuthConfig{Enabled: false, Mode: pbmcp.OAuthModeOff},
		// Required for chat proposals in step 3; writes on MCP are gated by the
		// tool's channel instead, so they stay off unless the operator opts in.
		AllowWrites: true,
		// No collections: every tool is a custom tool running List Pocket's own
		// handlers, so PocketBase collection rules can never widen or narrow them.
		Manifest:       &pbmcp.Manifest{SchemaVersion: "1"},
		CustomTools:    a.mcpToolSpecs(),
		Policies:       a.mcpPolicies(),
		AllowedOrigins: a.mcpAllowedOrigins(),
		MaxToolTimeout: assistantToolTimeout,
		AuditFunc:      a.mcpAudit,
		Logger:         a.moduleLogger(),
	}
	srv, err := pbadapter.New(se.App, cfg)
	if err != nil {
		a.logf("mcp: endpoint disabled: %v", err)
		return
	}
	a.mcp = srv
	// Chat is opt-in: the legacy endpoints stay the live surface until the flag
	// moves a front end over. A failure here leaves /mcp mounted as it is.
	if chatModuleEnabled() {
		if err := a.enableChat(srv); err != nil {
			a.logf("chat: API disabled: %v", err)
		}
	}
	if err := srv.RegisterRoutes(se.Router); err != nil {
		a.logf("mcp: registering routes: %v", err)
		a.mcp = nil
	}
}

// moduleLogger is the module's slog sink, so its warnings land where every
// other List Pocket line does.
func (a *App) moduleLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(mcpLogWriter{a.log}, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

// mcpPolicies holds the host policies the tool specs name. lp_super_admin is
// List Pocket's super admin role, never PocketBase's built-in "superuser".
func (a *App) mcpPolicies() map[string]pbmcp.PolicyFunc {
	return map[string]pbmcp.PolicyFunc{
		"lp_super_admin": func(p pbmcp.Principal) bool {
			if a.pb == nil || p.ID == "" || p.IsSuperuser {
				return false
			}
			collection := p.CollectionName
			if collection == "" {
				collection = "users"
			}
			record, err := a.pb.FindRecordById(collection, p.ID)
			if err != nil || record == nil {
				return false
			}
			return auth.ExtractRoleIDFromRecord(record) == auth.SuperAdminRoleID
		},
	}
}

// mcpAllowedOrigins limits browser calls to the app's own origin. Without a
// configured root URL the module's default (any origin) applies; the endpoint
// still needs a bearer token.
func (a *App) mcpAllowedOrigins() []string {
	if a.urlCfg == nil {
		return nil
	}
	u, err := url.Parse(strings.TrimSpace(a.urlCfg.RootURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil
	}
	return []string{u.Scheme + "://" + u.Host}
}

// mcpAudit writes one line per tool call. Arguments and tokens never reach it.
func (a *App) mcpAudit(event pbmcp.AuditEvent) {
	a.logf("mcp: tool=%s channel=%s outcome=%s duration=%s user=%s",
		event.Tool, event.Channel, event.Outcome, event.Duration.Round(time.Millisecond), event.PrincipalID)
}

// mcpLogWriter sends the module's slog output to the app log, so it shows up
// where every other List Pocket line does.
type mcpLogWriter struct{ log *stdlog.Logger }

func (w mcpLogWriter) Write(p []byte) (int, error) {
	if w.log != nil {
		w.log.Printf("mcp: %s", strings.TrimRight(string(p), "\n"))
	}
	return len(p), nil
}

// logf logs through the app log, tolerating an App built without one in tests.
func (a *App) logf(format string, args ...any) {
	if a.log != nil {
		a.log.Printf(format, args...)
	}
}
