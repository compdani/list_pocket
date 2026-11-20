package main

// The MCP endpoint exposes the assistant's own tools. There is one definition of
// each tool (assistantTools and friends) and one executor (executeAssistantTool),
// so an MCP call behaves exactly like a chat tool call: same permission checks,
// query allowlist, SQL refusal and result links.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	pbcore "ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/compdani/list_pocket/internal/apperr"
	"github.com/compdani/list_pocket/internal/auth"
	pbmcp "github.com/compdani/pocketbase-mcp"
	pbadapter "github.com/compdani/pocketbase-mcp/adapters/pocketbase"
)

// lpHttpsToolPath is the in-process route an MCP tool runs through. It only
// needs to be a path hydrateAuthUser-style handlers recognise; executeAssistantTool
// replaces the query and path values before calling the handler.
const lpHttpsToolPath = "/mailapi/assistant-tool"

// mcpWriteTools are the only write tools an operator can also offer over MCP.
// They are the old stdio server's write set plus subscriber updates. Anything
// that sends, publishes, runs or installs stays chat-only: on MCP a write runs
// immediately, with no proposal and no review in List Pocket.
var mcpWriteTools = map[string]bool{
	"propose_create_list":       true,
	"propose_create_contact":    true,
	"propose_create_template":   true,
	"propose_contact_note":      true,
	"propose_subscriber_update": true,
}

// mcpAutoApprovedTools write drafts rather than app data. Chat runs them without
// review today, so they are write tools with Approval "auto" on the chat channel.
var mcpAutoApprovedTools = map[string]bool{
	"plugin_draft_write": true,
	"plugin_draft_edit":  true,
}

// mcpAllowWrites reports the opt-in that moves the old stdio write set onto MCP.
func mcpAllowWrites() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LISTPOCKET_MCP_ALLOW_WRITES"))) {
	case "true", "1":
		return true
	}
	return false
}

// mcpToolSpecs maps every assistant tool to a ToolSpec. It is the single place
// where channel, effect, policy and approval are decided.
func (a *App) mcpToolSpecs() []pbmcp.ToolSpec {
	tools := a.assistantTools()
	specs := make([]pbmcp.ToolSpec, 0, len(tools))
	for _, tool := range tools {
		specs = append(specs, a.mcpToolSpec(tool))
	}
	return specs
}

func (a *App) mcpToolSpec(tool assistantTool) pbmcp.ToolSpec {
	spec := pbmcp.ToolSpec{
		Name:        tool.name,
		Title:       mcpToolTitle(tool.name),
		Description: tool.description,
		InputSchema: mcpToolInputSchema(tool),
		Effects:     pbmcp.Effects{Kind: "read"},
		Channels:    "both",
	}
	if tool.mutation {
		spec.Effects.Kind = "write"
		spec.Channels = "chat"
		spec.Summarize = mcpSummarize(tool.name)
	}
	if tool.superAdmin {
		// Plugin tools are chat-only and hidden from everyone else. "superuser"
		// would mean PocketBase _superusers, which List Pocket never accepts here.
		spec.Policy = "lp_super_admin"
		spec.Channels = "chat"
	}
	if mcpAutoApprovedTools[tool.name] {
		spec.Effects.Kind = "write"
		spec.Approval = "auto"
		spec.Channels = "chat"
	}
	if mcpWriteTools[tool.name] && mcpAllowWrites() {
		spec.Channels = "both"
	}
	if tool.name == "propose_editor_patch" {
		// An editor patch is a browser action: chat-only, no server handler, and
		// every change needs review. The schema names the editor and revision.
		spec.ExecutionLocation = "editor"
		spec.InputSchema = mcpEditorSchema(tool)
		spec.Channels = "chat"
		spec.Validate = mcpValidateEditorPatch
		return spec
	}
	spec.Handler = a.mcpToolHandler(tool)
	if tool.mutation {
		if tool.name == "propose_plugin" {
			spec.Prepare = a.mcpPreparePluginProposal
		}
		spec.Validate = a.mcpValidateProposal(tool.name)
		spec.Warn = a.mcpWarnProposal(tool.name)
	}
	return spec
}

func mcpToolInputSchema(tool assistantTool) map[string]any {
	required := tool.required
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": tool.properties, "required": required}
}

// mcpEditorSchema renames the chat proposal's revision to baseRevision and adds
// the editor the patch targets, which is what the module's editor contract needs.
func mcpEditorSchema(tool assistantTool) map[string]any {
	props := make(map[string]any, len(tool.properties)+2)
	for name, prop := range tool.properties {
		if name == "revision" {
			continue
		}
		props[name] = prop
	}
	props["editorId"] = stringProp("The open editor this patch applies to (editor.key from the current context)")
	props["baseRevision"] = stringProp("editor.revision token from the current context, copied verbatim")
	return map[string]any{
		"type":       "object",
		"properties": props,
		"required":   []string{"editorId", "baseRevision", "operations"},
	}
}

// mcpValidateEditorPatch checks the same operations the chat engine checks, with
// baseRevision mapped to the revision validateAssistantProposal expects.
func mcpValidateEditorPatch(_ context.Context, _ pbmcp.Principal, args map[string]any) error {
	patch := make(map[string]any, len(args)+1)
	for name, value := range args {
		patch[name] = value
	}
	patch["revision"], _ = args["baseRevision"].(string)
	raw, err := json.Marshal(patch)
	if err != nil {
		return pbmcp.Invalid("invalid arguments")
	}
	if err := validateAssistantProposal("propose_editor_patch", raw); err != nil {
		return pbmcp.Invalid(err.Error())
	}
	return nil
}

// mcpValidateProposal runs the checks chat runs before a write becomes a
// proposal. Both are read-only: validateFlowProposal reads flows and plugin
// sources, and never saves anything.
func (a *App) mcpValidateProposal(name string) func(context.Context, pbmcp.Principal, map[string]any) error {
	return func(_ context.Context, _ pbmcp.Principal, args map[string]any) error {
		raw, err := json.Marshal(args)
		if err != nil {
			return pbmcp.Invalid("invalid arguments")
		}
		if err := validateAssistantProposal(name, raw); err != nil {
			return pbmcp.Invalid(err.Error())
		}
		if _, err := a.validateFlowProposal(name, raw); err != nil {
			return pbmcp.Invalid(err.Error())
		}
		return nil
	}
}

// mcpWarnProposal surfaces the workflow and plugin warnings chat shows on a card.
func (a *App) mcpWarnProposal(name string) func(context.Context, pbmcp.Principal, map[string]any) []string {
	return func(_ context.Context, _ pbmcp.Principal, args map[string]any) []string {
		raw, err := json.Marshal(args)
		if err != nil {
			return nil
		}
		warnings, _ := a.validateFlowProposal(name, raw)
		return warnings
	}
}

// mcpPreparePluginProposal snapshots a plugin draft's files into the proposal, so
// a reviewer approves exactly those files and not whatever the draft holds later.
func (a *App) mcpPreparePluginProposal(_ context.Context, _ pbmcp.Principal, args map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(args)
	if err != nil {
		return nil, pbmcp.Invalid("invalid arguments")
	}
	expanded, err := a.expandAssistantProposal("propose_plugin", raw)
	if err != nil {
		return nil, pbmcp.Invalid(err.Error())
	}
	var out map[string]any
	if err := json.Unmarshal(expanded, &out); err != nil {
		return nil, pbmcp.Invalid("invalid arguments")
	}
	return out, nil
}

// mcpToolHandler runs one assistant tool as the MCP caller. The module has already
// checked the channel, the policy and the schema; the handler rebuilds the same
// signed-in request the HTTP route would see, then calls executeAssistantTool.
func (a *App) mcpToolHandler(tool assistantTool) func(context.Context, pbmcp.CallContext, map[string]any) (any, error) {
	return func(ctx context.Context, call pbmcp.CallContext, args map[string]any) (any, error) {
		caller, err := pbadapter.ResolveCaller(a.pb, call)
		if err != nil {
			return nil, err
		}
		e, err := a.callerRequestEvent(ctx, caller)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(args)
		if err != nil {
			return nil, pbmcp.Invalid("invalid arguments")
		}
		// approved=true is correct: the module already enforced review. A chat
		// write only reaches here after a person approved it, and an MCP write is
		// only offered when the operator opted into LISTPOCKET_MCP_ALLOW_WRITES.
		result, err := a.executeAssistantTool(e, tool.name, raw, true)
		if err != nil {
			return nil, mcpToolCallError(tool.mutation, err)
		}
		return result, nil
	}
}

// callerRequestEvent rebuilds the signed-in request in-process handlers expect,
// for a caller the module already validated: the same event and context values
// an HTTP route would build for that user. A chat Setup hook and a tool call
// both use it, so both run under exactly the caller's permissions.
func (a *App) callerRequestEvent(ctx context.Context, caller *pbadapter.Caller) (*pbcore.RequestEvent, error) {
	user, err := a.mcpCallerUser(caller.Record)
	if err != nil {
		return nil, err
	}
	e, _ := caller.RequestEvent(ctx, http.MethodGet, lpHttpsToolPath, nil)
	e.Set(auth.AuthRecordHTTPCtxKey, caller.Record)
	e.Set(auth.UserHTTPCtxKey, user)
	e.Set(appContextKey, a)
	return e, nil
}

// mcpCallerUser loads the List Pocket user behind the validated PocketBase record,
// exactly like hydrateAuthUser does for the HTTP routes.
func (a *App) mcpCallerUser(record *pbcore.Record) (auth.User, error) {
	if a.core == nil {
		return auth.User{}, errors.New("mcp: app not ready")
	}
	user, err := a.core.GetUser(record.Id, "", "")
	if err != nil || user.Status != auth.UserStatusEnabled {
		return auth.User{}, pbmcp.Denied("Sign in again to use this tool.")
	}
	if roleID := auth.ExtractRoleIDFromRecord(record); roleID > 0 {
		user.UserRoleID = roleID
	}
	return user, nil
}

// mcpToolCallError maps an application error onto the module's outcomes:
//   - 400 on a write was refused before writing, so a chat proposal stays pending
//     (pbmcp.Refused). On a read the model gets the reason.
//   - 403 is a permission refusal, 404 a missing record.
//   - Anything else on a write is returned as a plain error. The module's custom
//     tool layer turns that into "Tool execution failed." and records the
//     proposal as failed; only a timeout or cancellation is unknown. List Pocket
//     never retries a write that might have committed. On a read the model sees
//     the message, as it does today.
func mcpToolCallError(write bool, err error) error {
	msg := agentErrorText(err)
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		switch appErr.Status {
		case http.StatusBadRequest:
			if write {
				return pbmcp.Refused(msg)
			}
			return pbmcp.Invalid(msg)
		case http.StatusForbidden:
			return pbmcp.Denied(msg)
		case http.StatusNotFound:
			return pbmcp.NotFound(msg)
		}
	}
	if write {
		return fmt.Errorf("%s: %w", msg, err)
	}
	return pbmcp.Invalid(msg)
}

// mcpSummarize builds a proposal's one-line detail from identifiers in the
// arguments. The module writes this line for the reviewer, so model prose (the
// summary argument, a message body, a description) must never reach it. An empty
// result falls back to the module's default summary.
func mcpSummarize(name string) func(map[string]any) string {
	return func(args map[string]any) string {
		switch name {
		case "propose_contact_update", "propose_board_move", "propose_contact_note", "propose_subscriber_update", "propose_contact_task":
			if id := summaryText(args, "id"); id != "" {
				return "contact " + id
			}
		case "propose_editor_patch":
			if revision := summaryText(args, "baseRevision"); revision != "" {
				return fmt.Sprintf("%d operations on revision %s", summaryLen(args, "operations"), revision)
			}
		case "propose_workflow_patch":
			if workflow := summaryText(args, "workflow"); workflow != "" {
				return fmt.Sprintf("workflow %s, %d ops", workflow, summaryLen(args, "ops"))
			}
		case "propose_workflow":
			if definition := summaryObject(args, "definition"); definition != nil {
				if workflow := summaryText(definition, "name"); workflow != "" {
					return "workflow " + workflow
				}
			}
		case "propose_test_run", "propose_publish", "propose_workflow_run":
			if workflow := summaryText(args, "workflow"); workflow != "" {
				return "workflow " + workflow
			}
		case "propose_approval_decision":
			if approval := summaryText(args, "approval"); approval != "" {
				return "approval " + approval
			}
		case "propose_reply":
			body := summaryObject(args, "body")
			channel, key := summaryText(body, "channel"), summaryText(body, "key")
			if channel != "" && key != "" {
				return fmt.Sprintf("reply by %s in conversation %s", channel, key)
			}
		case "propose_plugin":
			id := summaryText(args, "plugin")
			if id == "" {
				id = summaryPluginManifestID(args)
			}
			if id != "" {
				return fmt.Sprintf("plugin %s, %d files", id, summaryLen(args, "files"))
			}
		case "propose_create_list", "propose_create_template":
			label := "list"
			if name == "propose_create_template" {
				label = "template"
			}
			if record := summaryObject(args, "body"); record != nil {
				if recordName := summaryText(record, "name"); recordName != "" {
					return fmt.Sprintf("%s %q", label, recordName)
				}
			}
		case "propose_create_contact":
			if record := summaryObject(args, "body"); record != nil {
				if email := summaryText(record, "email"); email != "" {
					return "contact " + email
				}
			}
		case "create_campaign_draft":
			if draft := summaryObject(args, "draft"); draft != nil {
				if campaign := summaryText(draft, "name"); campaign != "" {
					return fmt.Sprintf("campaign %q", campaign)
				}
			}
		}
		return ""
	}
}

func summaryText(args map[string]any, key string) string {
	text, _ := args[key].(string)
	return strings.TrimSpace(text)
}

func summaryObject(args map[string]any, key string) map[string]any {
	object, _ := args[key].(map[string]any)
	return object
}

func summaryLen(args map[string]any, key string) int {
	values, _ := args[key].([]any)
	return len(values)
}

// summaryPluginManifestID reads the ID from a files-only plugin proposal, which
// is what Prepare stores for a draft submission.
func summaryPluginManifestID(args map[string]any) string {
	files, _ := args["files"].([]any)
	for _, entry := range files {
		file, _ := entry.(map[string]any)
		if file["path"] != "plugin.json" {
			continue
		}
		content, _ := file["content"].(string)
		var manifest struct {
			ID string `json:"id"`
		}
		if json.Unmarshal([]byte(content), &manifest) == nil {
			return manifest.ID
		}
	}
	return ""
}

// mcpToolTitle is the same human label the assistant UI builds for a tool name.
func mcpToolTitle(name string) string {
	text := strings.ReplaceAll(strings.TrimPrefix(name, "propose_"), "_", " ")
	if text == "" {
		return name
	}
	return strings.ToUpper(text[:1]) + text[1:]
}
