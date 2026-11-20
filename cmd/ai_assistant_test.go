package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pbcore "ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/compdani/list_pocket/internal/auth"
	"github.com/compdani/list_pocket/internal/flow/llm"
	"github.com/compdani/list_pocket/internal/plugins"
)

func TestAssistantToolsEnforcePermissionsAndApproval(t *testing.T) {
	app := &App{auth: &auth.Auth{}}
	re := &pbcore.RequestEvent{}
	re.Request = httptest.NewRequest("POST", "/", nil)
	re.Set(auth.UserHTTPCtxKey, auth.User{})
	if _, err := app.executeAssistantTool(re, "campaigns", json.RawMessage(`{}`), false); err == nil {
		t.Fatal("read without campaign permissions succeeded")
	}
	if _, err := app.executeAssistantTool(re, "create_campaign_draft", json.RawMessage(`{}`), false); err == nil {
		t.Fatal("write without approval succeeded")
	}
	if _, err := app.executeAssistantTool(re, "subscribers", json.RawMessage(`{"query":"sql_query=SELECT+1"}`), false); err == nil || !strings.Contains(err.Error(), "unsupported query field") {
		t.Fatalf("arbitrary query field accepted: %v", err)
	}
	if _, err := app.executeAssistantTool(re, "subscribers", json.RawMessage(`{"query":"query=email+%3D+'a'"}`), false); err == nil || !strings.Contains(err.Error(), "SQL") {
		t.Fatalf("SQL query argument on subscribers accepted: %v", err)
	}
	if _, err := app.executeAssistantTool(re, "analytics", json.RawMessage(`{"id":"bogus","query":"id=1&from=2026-01-01&to=2026-01-02"}`), false); err == nil || !strings.Contains(err.Error(), "analytics require") {
		t.Fatalf("invalid analytics metric accepted: %v", err)
	}
	if _, err := app.executeAssistantTool(re, "analytics", json.RawMessage(`{"id":"views","query":"id=1"}`), false); err == nil || !strings.Contains(err.Error(), "analytics require") {
		t.Fatalf("analytics without date range accepted: %v", err)
	}
	if _, err := app.executeAssistantTool(re, "publish_workflow", json.RawMessage(`{}`), true); err == nil {
		t.Fatal("unregistered side effect allowed")
	}
}

func TestValidateAssistantProposal(t *testing.T) {
	valid := `{"revision":"r1","operations":[{"op":"set","field":"subject","after":"New"},{"op":"replace_body","after":"<p>x</p>"},{"op":"replace_text","before":"a","after":"b"}]}`
	if err := validateAssistantProposal("propose_editor_patch", json.RawMessage(valid)); err != nil {
		t.Fatalf("valid proposal rejected: %v", err)
	}
	for name, raw := range map[string]string{
		"no revision":         `{"operations":[{"op":"replace_body","after":"x"}]}`,
		"no operations":       `{"revision":"r1","operations":[]}`,
		"body without after":  `{"revision":"r1","operations":[{"op":"replace_body"}]}`,
		"text without before": `{"revision":"r1","operations":[{"op":"replace_text","after":"b"}]}`,
		"unknown field":       `{"revision":"r1","operations":[{"op":"set","field":"body","after":"b"}]}`,
		"unknown op":          `{"revision":"r1","operations":[{"op":"rewrite","after":"b"}]}`,
	} {
		if err := validateAssistantProposal("propose_editor_patch", json.RawMessage(raw)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	if err := validateAssistantProposal("create_campaign_draft", json.RawMessage(`{}`)); err != nil {
		t.Errorf("other tools are not validated here: %v", err)
	}
}

// Plugins run with full access, so other roles can't see or call these tools, whatever
// the model asks for.
func TestAssistantPluginToolsAreSuperAdminOnly(t *testing.T) {
	app := &App{auth: &auth.Auth{}, plugins: &plugins.Manager{}}
	staff := &pbcore.RequestEvent{}
	staff.Request = httptest.NewRequest("POST", "/", nil)
	staff.Set(auth.UserHTTPCtxKey, auth.User{UserRoleID: auth.SuperAdminRoleID + 1})
	admin := &pbcore.RequestEvent{}
	admin.Request = httptest.NewRequest("POST", "/", nil)
	admin.Set(auth.UserHTTPCtxKey, auth.User{UserRoleID: auth.SuperAdminRoleID})

	names := func(re *pbcore.RequestEvent) map[string]bool {
		out := map[string]bool{}
		for _, tool := range app.assistantToolsFor(re) {
			out[tool.name] = true
		}
		return out
	}
	for _, name := range []string{"plugin_reference", "plugins", "plugin_files", "plugin_draft_write", "plugin_draft_edit", "propose_plugin"} {
		if names(staff)[name] {
			t.Errorf("%s is visible to staff", name)
		}
		if !names(admin)[name] {
			t.Errorf("%s is missing for a super admin", name)
		}
		if _, err := app.executeAssistantTool(staff, name, json.RawMessage(`{}`), true); err == nil || err.Error() != "unknown tool" {
			t.Errorf("%s ran for staff: %v", name, err)
		}
	}
	// A super admin still needs a person's approval to install.
	if _, err := app.executeAssistantTool(admin, "propose_plugin", json.RawMessage(`{}`), false); err == nil || err.Error() != "approval required" {
		t.Errorf("install without approval: %v", err)
	}
	if out, err := app.executeAssistantTool(admin, "plugin_reference", json.RawMessage(`{}`), false); err != nil || !strings.Contains(fmt.Sprint(out), "plugin.tool") {
		t.Errorf("the reference should hold the plugin guide: %v", err)
	}
}

func TestAssistantOutputTokens(t *testing.T) {
	t.Setenv("LISTPOCKET_AI_MAX_OUTPUT_TOKENS", "")
	if got := assistantOutputTokens(); got != assistantMaxOutputTokens {
		t.Fatalf("default: %v", got)
	}
	t.Setenv("LISTPOCKET_AI_MAX_OUTPUT_TOKENS", "500000")
	if got := assistantOutputTokens(); got != 500000 {
		t.Fatalf("configured: %v", got)
	}
	t.Setenv("LISTPOCKET_AI_MAX_OUTPUT_TOKENS", "0")
	if got := assistantOutputTokens(); got != 0 {
		t.Fatalf("0 should ask for no limit: %v", got)
	}
	t.Setenv("LISTPOCKET_AI_MAX_OUTPUT_TOKENS", "lots")
	if got := assistantOutputTokens(); got != assistantMaxOutputTokens {
		t.Fatalf("invalid: %v", got)
	}
}

func TestExpandPluginProposal(t *testing.T) {
	app := &App{plugins: nil}
	raw := json.RawMessage(`{"summary":"s"}`)
	if got, err := app.expandAssistantProposal("propose_reply", raw); err != nil || string(got) != string(raw) {
		t.Fatalf("other proposals pass through: %s, %v", got, err)
	}
	if got, err := app.expandAssistantProposal("propose_plugin", raw); err != nil || string(got) != string(raw) {
		t.Fatalf("files sent directly pass through: %s, %v", got, err)
	}
	if _, err := app.expandAssistantProposal("propose_plugin", json.RawMessage(`{"plugin":"a","files":[{"path":"x.js","content":""}]}`)); err == nil {
		t.Fatal("a draft and files together should be refused")
	}
}

func TestAssistantProviderErrorExplainsCredentials(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		err := assistantProviderError(&llm.StatusError{Status: status, Body: "secret detail from the provider"})
		if !strings.Contains(err.Error(), "credentials") || strings.Contains(err.Error(), "secret detail") {
			t.Errorf("%d: %v", status, err)
		}
	}
	if err := assistantProviderError(&llm.StatusError{Status: 500}); err.Error() != "AI provider returned HTTP 500" {
		t.Errorf("other statuses: %v", err)
	}
}
