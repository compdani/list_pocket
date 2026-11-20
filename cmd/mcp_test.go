package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	pbcore "ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/compdani/list_pocket/internal/apperr"
	"github.com/compdani/list_pocket/internal/auth"
	"github.com/compdani/list_pocket/internal/i18n"
	"github.com/compdani/list_pocket/internal/paginator"
	"github.com/compdani/list_pocket/internal/plugins"
	"github.com/compdani/list_pocket/models"
	pbmcp "github.com/compdani/pocketbase-mcp"
	pbadapter "github.com/compdani/pocketbase-mcp/adapters/pocketbase"
)

// mcpFixture is a test app with real PocketBase users, roles and data, so the
// converter can be exercised against the same handlers the app serves.
type mcpFixture struct {
	app         *App
	admin       *pbcore.Record
	adminToken  string
	scoped      *pbcore.Record
	scopedToken string
	noperm      *pbcore.Record
	nopermToken string
	rootToken   string
	scopedSub   models.Subscriber
	campaign    models.Campaign
}

func newMCPFixture(t *testing.T) *mcpFixture {
	t.Helper()
	a, _, _ := boardsTestApp(t)
	a.log = log.New(io.Discard, "", 0)
	a.cfg = &Config{}
	a.urlCfg = &UrlConfig{RootURL: "https://example.com"}
	a.plugins = &plugins.Manager{}
	a.pg = paginator.New(paginator.Opt{DefaultPerPage: 20, MaxPerPage: 50, NumPageNums: 10, AllowAll: true})
	lang, err := i18n.New([]byte(`{"_.code":"en","_.name":"English"}`))
	if err != nil {
		t.Fatal(err)
	}
	a.i18n = lang
	a.auth, err = auth.New(auth.Config{}, nil, a.pb, &auth.Callbacks{}, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}

	list, err := a.core.CreateList(models.List{Name: "MCP list"})
	if err != nil {
		t.Fatal(err)
	}
	sub, _, err := a.core.InsertSubscriber(models.Subscriber{Email: "scoped@example.com", Name: "Scoped"}, []int{list.ID}, nil, true, false)
	if err != nil {
		t.Fatal(err)
	}
	campaign, err := a.core.CreateCampaign(models.Campaign{Name: "MCP campaign", Subject: "Hello", ContentType: models.CampaignContentTypeRichtext}, []int{list.ID}, nil)
	if err != nil {
		t.Fatal(err)
	}

	f := &mcpFixture{app: a, scopedSub: sub, campaign: campaign}
	mcpCreateRole(t, a, 10, "user", []string{"subscribers:get", "campaigns:get"})
	mcpCreateRole(t, a, 20, "user", []string{})
	mcpCreateListRole(t, a, 30, 31, list.RecordID)

	f.admin, f.adminToken = mcpCreateUser(t, a, "mcp-admin", "1", 0)
	f.scoped, f.scopedToken = mcpCreateUser(t, a, "mcp-scoped", "10", 30)
	f.noperm, f.nopermToken = mcpCreateUser(t, a, "mcp-noperm", "20", 0)
	f.rootToken = mcpCreateSuperuser(t, a)
	return f
}

func mcpCreateRole(t *testing.T, a *App, legacyID int, typ string, permissions any) {
	t.Helper()
	collection, err := a.pb.FindCollectionByNameOrId("roles")
	if err != nil {
		t.Fatal(err)
	}
	record := pbcore.NewRecord(collection)
	record.Set("legacy_id", legacyID)
	record.Set("type", typ)
	record.Set("name", fmt.Sprintf("%s-%d", typ, legacyID))
	record.Set("permissions", permissions)
	if err := a.pb.Save(record); err != nil {
		t.Fatal(err)
	}
}

func mcpCreateListRole(t *testing.T, a *App, parentLegacyID, childLegacyID int, listRecordID string) {
	t.Helper()
	collection, err := a.pb.FindCollectionByNameOrId("roles")
	if err != nil {
		t.Fatal(err)
	}
	parent := pbcore.NewRecord(collection)
	parent.Set("legacy_id", parentLegacyID)
	parent.Set("type", "list")
	parent.Set("name", "MCP list role")
	parent.Set("permissions", []string{})
	if err := a.pb.Save(parent); err != nil {
		t.Fatal(err)
	}
	// A list role's per-list grants are child records of the parent role.
	child := pbcore.NewRecord(collection)
	child.Set("legacy_id", childLegacyID)
	child.Set("type", "list")
	child.Set("parent_id", parent.Id)
	child.Set("list_record_id", listRecordID)
	child.Set("permissions", []string{"list:get"})
	if err := a.pb.Save(child); err != nil {
		t.Fatal(err)
	}
}

func mcpCreateUser(t *testing.T, a *App, username, roleID string, listRoleID int) (*pbcore.Record, string) {
	t.Helper()
	collection, err := a.pb.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	record := pbcore.NewRecord(collection)
	record.SetEmail(username + "@example.com")
	record.SetPassword("test-password-123")
	record.Set("username", username)
	record.Set("user_type", auth.UserTypeUser)
	record.Set("status", auth.UserStatusEnabled)
	record.Set("role", roleID)
	if listRoleID > 0 {
		record.Set("list_role_id", listRoleID)
	}
	if err := a.pb.Save(record); err != nil {
		t.Fatal(err)
	}
	token, err := record.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	return record, token
}

func mcpCreateSuperuser(t *testing.T, a *App) string {
	t.Helper()
	collection, err := a.pb.FindCollectionByNameOrId("_superusers")
	if err != nil {
		t.Fatal(err)
	}
	record := pbcore.NewRecord(collection)
	record.SetEmail("mcp-root@example.com")
	record.SetPassword("test-password-123")
	if err := a.pb.Save(record); err != nil {
		t.Fatal(err)
	}
	token, err := record.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func (f *mcpFixture) spec(t *testing.T, name string) pbmcp.ToolSpec {
	t.Helper()
	for _, spec := range f.app.mcpToolSpecs() {
		if spec.Name == name {
			return spec
		}
	}
	t.Fatalf("no tool spec %q", name)
	return pbmcp.ToolSpec{}
}

func (f *mcpFixture) callTool(t *testing.T, name string, record *pbcore.Record, token string, args map[string]any) (any, error) {
	t.Helper()
	spec := f.spec(t, name)
	if spec.Handler == nil {
		t.Fatalf("%s has no handler", name)
	}
	principal := pbmcp.Principal{ID: record.Id, CollectionName: record.Collection().Name, CollectionID: record.Collection().Id, Token: token}
	return spec.Handler(context.Background(), pbmcp.CallContext{Principal: principal, Context: context.Background()}, args)
}

// callLegacy runs the same tool through the assistant's own executor.
func (f *mcpFixture) callLegacy(t *testing.T, name string, record *pbcore.Record, args map[string]any) (any, error) {
	t.Helper()
	user, err := f.app.core.GetUser(record.Id, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if roleID := auth.ExtractRoleIDFromRecord(record); roleID > 0 {
		user.UserRoleID = roleID
	}
	event := &pbcore.RequestEvent{App: f.app.pb, Auth: record}
	event.Request = httptest.NewRequest(http.MethodGet, lpHttpsToolPath, nil)
	event.Response = httptest.NewRecorder()
	event.Set(auth.UserHTTPCtxKey, user)
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return f.app.executeAssistantTool(event, name, raw, false)
}

func mcpJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// The converter must cover every assistant tool exactly once and assign the
// channel, effect, policy and approval the design calls for.
func TestMCPToolSpecsCoverAssistantTools(t *testing.T) {
	f := newMCPFixture(t)
	t.Setenv("LISTPOCKET_MCP_ALLOW_WRITES", "")

	legacy := map[string]assistantTool{}
	for _, tool := range f.app.assistantTools() {
		if _, dup := legacy[tool.name]; dup {
			t.Fatalf("assistant tool %q is defined twice", tool.name)
		}
		legacy[tool.name] = tool
	}
	specs := f.app.mcpToolSpecs()
	if len(specs) != len(legacy) {
		t.Fatalf("got %d specs for %d assistant tools", len(specs), len(legacy))
	}
	seen := map[string]bool{}
	for _, spec := range specs {
		if seen[spec.Name] {
			t.Fatalf("tool %q appears twice", spec.Name)
		}
		seen[spec.Name] = true
		tool, ok := legacy[spec.Name]
		if !ok {
			t.Fatalf("converter invented tool %q", spec.Name)
		}
		if spec.InputSchema["type"] != "object" {
			t.Errorf("%s: schema type", spec.Name)
		}
		if _, ok := spec.InputSchema["required"].([]string); !ok {
			t.Errorf("%s: required must be a string array", spec.Name)
		}
		if tool.superAdmin {
			if spec.Channels != "chat" || spec.Policy != "lp_super_admin" {
				t.Errorf("%s: super admin tool must be chat-only with lp_super_admin, got %q/%q", spec.Name, spec.Channels, spec.Policy)
			}
		}
		if mcpAutoApprovedTools[spec.Name] {
			if spec.Effects.Kind != "write" || spec.Approval != "auto" || spec.Channels != "chat" {
				t.Errorf("%s: draft tool must be write/auto/chat, got %+v %q %q", spec.Name, spec.Effects, spec.Approval, spec.Channels)
			}
		}
		if tool.mutation && spec.Effects.Kind != "write" {
			t.Errorf("%s: mutation must be a write effect", spec.Name)
		}
		if tool.mutation && !tool.superAdmin && spec.Name != "propose_editor_patch" && spec.Channels != "chat" {
			t.Errorf("%s: write tool must be chat-only by default, got %q", spec.Name, spec.Channels)
		}
		if !tool.mutation && !tool.superAdmin && spec.Channels != "both" {
			t.Errorf("%s: read tool must be offered on both channels, got %q", spec.Name, spec.Channels)
		}
		if spec.Effects.Kind == "write" && spec.Channels == "both" {
			t.Errorf("%s: no write tool may be on MCP by default", spec.Name)
		}
	}

	editor := f.spec(t, "propose_editor_patch")
	if editor.ExecutionLocation != "editor" || editor.Handler != nil || editor.Channels != "chat" || editor.Approval != "" {
		t.Fatalf("editor tool: %+v", editor)
	}
	props, _ := editor.InputSchema["properties"].(map[string]any)
	for _, name := range []string{"editorId", "baseRevision"} {
		prop, ok := props[name].(map[string]any)
		if !ok || prop["type"] != "string" {
			t.Errorf("editor schema missing string %q", name)
		}
	}
	required, _ := editor.InputSchema["required"].([]string)
	if !slices.Contains(required, "editorId") || !slices.Contains(required, "baseRevision") || !slices.Contains(required, "operations") {
		t.Fatalf("editor schema required: %v", required)
	}
	if _, ok := props["revision"]; ok {
		t.Fatal("the editor schema must use baseRevision, not revision")
	}

	// The module validates the whole registry at construction, including the
	// editor contract and every input schema.
	if _, err := pbadapter.New(f.app.pb, mcpConfigForTest(f.app)); err != nil {
		t.Fatalf("the module refused the tool registry: %v", err)
	}

	if err := editor.Validate(context.Background(), pbmcp.Principal{}, map[string]any{
		"editorId": "campaign:one", "baseRevision": "r1abc",
		"operations": []any{map[string]any{"op": "replace_body", "after": "<p>x</p>"}},
	}); err != nil {
		t.Fatalf("a valid editor patch was refused: %v", err)
	}
	var accessErr pbmcp.AccessError
	if err := editor.Validate(context.Background(), pbmcp.Principal{}, map[string]any{
		"editorId": "campaign:one", "baseRevision": "r1abc", "operations": []any{},
	}); !errors.As(err, &accessErr) || accessErr.Kind != "invalid" {
		t.Fatalf("an empty patch should be invalid: %v", err)
	}
}

func TestMCPToolSpecsOptInWrites(t *testing.T) {
	f := newMCPFixture(t)
	t.Setenv("LISTPOCKET_MCP_ALLOW_WRITES", "true")
	onBoth := map[string]bool{}
	for _, spec := range f.app.mcpToolSpecs() {
		if spec.Channels == "both" && spec.Effects.Kind == "write" {
			onBoth[spec.Name] = true
		}
	}
	for name := range mcpWriteTools {
		if !onBoth[name] {
			t.Errorf("%s should be offered on MCP when writes are opted in", name)
		}
	}
	if len(onBoth) != len(mcpWriteTools) {
		t.Fatalf("unexpected MCP write tools: %v", onBoth)
	}
	// Sending, publishing, running and installing never leave chat.
	for _, name := range []string{"propose_reply", "propose_publish", "propose_workflow_run", "propose_plugin", "propose_test_run", "create_campaign_draft"} {
		if onBoth[name] {
			t.Errorf("%s must never be offered on MCP", name)
		}
	}
}

// The Handler must return exactly what the assistant's executor returns for the
// same user, and refuse a user without the permission.
func TestMCPToolHandlersMatchAssistantTools(t *testing.T) {
	f := newMCPFixture(t)
	t.Setenv("LISTPOCKET_MCP_ALLOW_WRITES", "")

	calls := []struct {
		name      string
		args      map[string]any
		needsPerm bool
	}{
		{"lists", map[string]any{}, false},
		{"subscribers", map[string]any{"query": "search=scoped@example.com"}, true},
		{"campaigns", map[string]any{"id": f.campaign.RecordID}, true},
		{"dashboard_counts", map[string]any{}, false},
		{"subscriber_activity", map[string]any{"id": f.scopedSub.RecordID}, true},
	}
	users := []struct {
		name    string
		record  *pbcore.Record
		token   string
		noPerms bool
	}{
		{"admin", f.admin, f.adminToken, false},
		{"list-scoped", f.scoped, f.scopedToken, false},
		{"without permission", f.noperm, f.nopermToken, true},
	}
	for _, user := range users {
		for _, call := range calls {
			t.Run(user.name+"/"+call.name, func(t *testing.T) {
				got, gotErr := f.callTool(t, call.name, user.record, user.token, call.args)
				legacy, legacyErr := f.callLegacy(t, call.name, user.record, call.args)
				if legacyErr != nil {
					// The assistant refused it, so the MCP call must refuse it the
					// same way: same status mapped to the module's access kind.
					var appErr *apperr.Error
					if !errors.As(legacyErr, &appErr) {
						t.Fatalf("legacy error is not an app error: %v", legacyErr)
					}
					var accessErr pbmcp.AccessError
					if !errors.As(gotErr, &accessErr) {
						t.Fatalf("the handler should refuse like the assistant (legacy %v), got %v", legacyErr, gotErr)
					}
					if want := mcpAccessKind(appErr.Status); accessErr.Kind != want {
						t.Fatalf("access kind %q, want %q", accessErr.Kind, want)
					}
					if call.needsPerm && user.noPerms && accessErr.Kind != "denied" {
						t.Fatalf("a user without the permission must be denied, got %q", accessErr.Kind)
					}
					return
				}
				if gotErr != nil {
					t.Fatalf("handler: %v", gotErr)
				}
				if mcpJSON(t, got) != mcpJSON(t, legacy) {
					t.Fatalf("results differ\nmcp:    %s\nlegacy: %s", mcpJSON(t, got), mcpJSON(t, legacy))
				}
			})
		}
	}
}

func mcpAccessKind(status int) string {
	switch status {
	case http.StatusForbidden:
		return "denied"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusBadRequest:
		return "invalid"
	}
	return "invalid"
}

func mcpConfigForTest(a *App) pbmcp.Config {
	return pbmcp.Config{
		Path:        "/mcp",
		Auth:        pbmcp.AuthConfig{Collections: []string{"users"}},
		AllowWrites: true,
		Manifest:    &pbmcp.Manifest{SchemaVersion: "1"},
		CustomTools: a.mcpToolSpecs(),
		Policies:    a.mcpPolicies(),
	}
}

type mcpTestServer struct {
	fixture *mcpFixture
	mux     http.Handler
}

func newMCPTestServer(t *testing.T) *mcpTestServer {
	t.Helper()
	f := newMCPFixture(t)
	router, err := apis.NewRouter(f.app.pb)
	if err != nil {
		t.Fatal(err)
	}
	f.app.mountMCP(&pbcore.ServeEvent{App: f.app.pb, Router: router})
	if f.app.mcp == nil {
		t.Fatal("the MCP endpoint was not mounted")
	}
	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}
	return &mcpTestServer{fixture: f, mux: mux}
}

func (s *mcpTestServer) rpc(t *testing.T, token, method string, params any) (int, map[string]any) {
	t.Helper()
	body := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method}
	if params != nil {
		body["params"] = params
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	s.mux.ServeHTTP(response, request)
	var out map[string]any
	_ = json.Unmarshal(response.Body.Bytes(), &out)
	return response.Code, out
}

func mcpToolNames(t *testing.T, result map[string]any) map[string]bool {
	t.Helper()
	raw, _ := result["result"].(map[string]any)
	tools, _ := raw["tools"].([]any)
	names := map[string]bool{}
	for _, entry := range tools {
		tool, _ := entry.(map[string]any)
		if name, ok := tool["name"].(string); ok {
			names[name] = true
		}
	}
	return names
}

func TestMCPEndpointHTTP(t *testing.T) {
	t.Setenv("LISTPOCKET_MCP_ALLOW_WRITES", "")
	s := newMCPTestServer(t)

	code, out := s.rpc(t, s.fixture.adminToken, "initialize", map[string]any{
		"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1"},
	})
	if code != http.StatusOK {
		t.Fatalf("initialize: %d %v", code, out)
	}
	result, _ := out["result"].(map[string]any)
	info, _ := result["serverInfo"].(map[string]any)
	if info["name"] != "listpocket" {
		t.Fatalf("serverInfo: %v", info)
	}

	code, out = s.rpc(t, s.fixture.adminToken, "tools/list", nil)
	if code != http.StatusOK {
		t.Fatalf("tools/list: %d %v", code, out)
	}
	names := mcpToolNames(t, out)
	for _, name := range []string{"lists", "subscribers", "dashboard_counts", "subscriber_activity"} {
		if !names[name] {
			t.Errorf("%s is missing from tools/list", name)
		}
	}
	if names["propose_create_list"] || names["propose_create_contact"] || names["propose_subscriber_update"] {
		t.Error("a write tool is listed without LISTPOCKET_MCP_ALLOW_WRITES")
	}
	for _, name := range []string{"plugin_reference", "plugins", "plugin_files", "plugin_draft_write", "propose_plugin"} {
		if names[name] {
			t.Errorf("%s must not be offered on MCP", name)
		}
	}

	code, out = s.rpc(t, s.fixture.adminToken, "tools/call", map[string]any{"name": "lists", "arguments": map[string]any{}})
	if code != http.StatusOK {
		t.Fatalf("tools/call: %d %v", code, out)
	}
	callResult, _ := out["result"].(map[string]any)
	if callResult == nil || callResult["isError"] == true {
		t.Fatalf("lists call failed: %v", out)
	}

	if code, _ := s.rpc(t, "", "tools/list", nil); code != http.StatusUnauthorized {
		t.Errorf("a request without a token: %d", code)
	}
	if code, _ := s.rpc(t, s.fixture.rootToken, "tools/list", nil); code != http.StatusUnauthorized {
		t.Errorf("a _superusers token: %d", code)
	}

	// Plugin tools are chat-only, so /mcp never lists them; the visibility policy
	// is what decides who sees them when chat is mounted in step 3.
	policies := s.fixture.app.mcpPolicies()
	adminPrincipal := pbmcp.Principal{ID: s.fixture.admin.Id, CollectionName: "users", CollectionID: s.fixture.admin.Collection().Id}
	staffPrincipal := pbmcp.Principal{ID: s.fixture.noperm.Id, CollectionName: "users", CollectionID: s.fixture.noperm.Collection().Id}
	if !pbmcp.EvaluatePolicy("lp_super_admin", adminPrincipal, policies) {
		t.Error("lp_super_admin refused the super admin")
	}
	if pbmcp.EvaluatePolicy("lp_super_admin", staffPrincipal, policies) {
		t.Error("lp_super_admin allowed a regular user")
	}
	chatVisible := func(p pbmcp.Principal, spec pbmcp.ToolSpec) bool {
		if spec.Channels != "chat" && spec.Channels != "both" {
			return false
		}
		return pbmcp.EvaluatePolicy(spec.Policy, p, policies)
	}
	pluginReads := map[string]bool{"plugin_reference": false, "plugins": false, "plugin_files": false}
	for _, spec := range s.fixture.app.mcpToolSpecs() {
		if spec.Policy != "lp_super_admin" {
			continue
		}
		if chatVisible(staffPrincipal, spec) {
			t.Errorf("%s is visible to a regular user", spec.Name)
		}
		if !chatVisible(adminPrincipal, spec) {
			t.Errorf("%s is hidden from the super admin", spec.Name)
		}
		if _, ok := pluginReads[spec.Name]; ok {
			pluginReads[spec.Name] = true
		}
	}
	for name, seen := range pluginReads {
		if !seen {
			t.Errorf("plugin read tool %s is missing", name)
		}
	}
}

func TestMCPEndpointOffersOptedInWrites(t *testing.T) {
	t.Setenv("LISTPOCKET_MCP_ALLOW_WRITES", "true")
	s := newMCPTestServer(t)
	code, out := s.rpc(t, s.fixture.adminToken, "tools/list", nil)
	if code != http.StatusOK {
		t.Fatalf("tools/list: %d %v", code, out)
	}
	names := mcpToolNames(t, out)
	for _, name := range []string{"propose_create_list", "propose_create_contact", "propose_create_template", "propose_contact_note", "propose_subscriber_update"} {
		if !names[name] {
			t.Errorf("%s is missing after opting writes in", name)
		}
	}
	for _, name := range []string{"propose_reply", "propose_publish", "propose_workflow_run", "propose_plugin"} {
		if names[name] {
			t.Errorf("%s must never be offered on MCP", name)
		}
	}
}

// Error mapping: a 400 on a write keeps a proposal pending (Refused), a 400 on a
// read is Invalid, 403 and 404 keep their meaning, and any other write failure is
// never an AccessError (the module records an unknown outcome and never retries).
func TestMCPToolCallErrorMapping(t *testing.T) {
	var accessErr pbmcp.AccessError
	cases := []struct {
		name   string
		write  bool
		err    error
		kind   string
		access bool
	}{
		{"write 400", true, apperr.BadRequest("bad input"), "refused", true},
		{"read 400", false, apperr.BadRequest("bad input"), "invalid", true},
		{"403", false, apperr.Forbidden("permission denied: subscribers:get"), "denied", true},
		{"404", false, apperr.NotFound("missing"), "not_found", true},
		{"write 500", true, apperr.Internal("failed"), "", false},
		{"write other", true, errors.New("boom"), "", false},
		{"read 500", false, apperr.Internal("failed"), "invalid", true},
		{"read other", false, errors.New("boom"), "invalid", true},
	}
	for _, tc := range cases {
		err := mcpToolCallError(tc.write, tc.err)
		if tc.access {
			if !errors.As(err, &accessErr) || accessErr.Kind != tc.kind {
				t.Errorf("%s: got %v, want kind %q", tc.name, err, tc.kind)
			}
			continue
		}
		if errors.As(err, &accessErr) {
			t.Errorf("%s: a failed write must not be an AccessError: %v", tc.name, err)
		}
		if !errors.Is(err, tc.err) {
			t.Errorf("%s: the write error must wrap the cause: %v", tc.name, err)
		}
	}
}
