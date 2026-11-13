package mcpserver

import (
	"context"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type createListArgs struct {
	Name        string   `json:"name" jsonschema:"Mailing list name"`
	Type        string   `json:"type" jsonschema:"private or public"`
	Optin       string   `json:"optin" jsonschema:"single or double"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

type createSubscriberArgs struct {
	Email         string         `json:"email,omitempty"`
	Phone         string         `json:"phone,omitempty"`
	FirstName     string         `json:"first_name,omitempty"`
	LastName      string         `json:"last_name,omitempty"`
	Name          string         `json:"name,omitempty"`
	Status        string         `json:"status" jsonschema:"enabled, disabled, or blocklisted"`
	ListRecordIDs []string       `json:"list_record_ids,omitempty" jsonschema:"Mailing list PocketBase record IDs"`
	Attribs       map[string]any `json:"attribs,omitempty" jsonschema:"Custom contact attributes"`
}

type createTemplateArgs struct {
	Name    string `json:"name"`
	Type    string `json:"type" jsonschema:"campaign, campaign_sms, or tx"`
	Body    string `json:"body" jsonschema:"Template source; campaign email templates must contain {{ template \"content\" . }}"`
	Subject string `json:"subject,omitempty" jsonschema:"Subject for transactional templates"`
	Channel string `json:"channel,omitempty"`
}

type createNoteArgs struct {
	ID   string `json:"id" jsonschema:"Contact PocketBase record ID"`
	Body string `json:"body" jsonschema:"Note text"`
}

type updateSubscriberArgs struct {
	ID        string          `json:"id" jsonschema:"Contact PocketBase record ID"`
	Email     *string         `json:"email,omitempty"`
	Phone     *string         `json:"phone,omitempty" jsonschema:"Empty string clears the phone"`
	FirstName *string         `json:"first_name,omitempty"`
	LastName  *string         `json:"last_name,omitempty"`
	Attribs   *map[string]any `json:"attribs,omitempty" jsonschema:"Replaces the entire custom attributes object; omitted preserves it"`
}

func writeTool(name, description string) *mcp.Tool {
	no := false
	return &mcp.Tool{Name: name, Description: description + " Immediately persists the record; not a review proposal. Do not retry blindly after failure.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &no}}
}

func (b *backend) addWrites(s *mcp.Server) {
	update := writeTool("update_subscriber", "Update contact profile fields. Omitted fields are preserved. Supports email, phone, first_name, last_name, and attribs. Does not change status, list membership, or consent, or send messages.")
	yes := true
	update.Annotations.DestructiveHint = &yes
	mcp.AddTool(s, update, func(ctx context.Context, _ *mcp.CallToolRequest, args updateSubscriberArgs) (*mcp.CallToolResult, any, error) {
		path, err := recordPath("/subscribers/{id}", args.ID)
		if err != nil {
			return toolError(err), nil, nil
		}
		patch := map[string]any{}
		if args.Email != nil {
			patch["email"] = *args.Email
		}
		if args.Phone != nil {
			patch["phone"] = *args.Phone
		}
		if args.FirstName != nil {
			patch["first_name"] = *args.FirstName
		}
		if args.LastName != nil {
			patch["last_name"] = *args.LastName
		}
		if args.Attribs != nil {
			patch["attribs"] = *args.Attribs
		}
		if len(patch) == 0 {
			return toolError(fmt.Errorf("provide at least one profile field to update")), nil, nil
		}
		return b.call(ctx, http.MethodPatch, path, "", patch), nil, nil
	})
	mcp.AddTool(s, writeTool("create_list", "Create a mailing list."),
		func(ctx context.Context, _ *mcp.CallToolRequest, args createListArgs) (*mcp.CallToolResult, any, error) {
			return b.call(ctx, http.MethodPost, "/lists", "", args), nil, nil
		})
	mcp.AddTool(s, writeTool("create_subscriber", "Create a contact with email and/or phone. List memberships are not preconfirmed and no opt-in email is sent."),
		func(ctx context.Context, _ *mcp.CallToolRequest, args createSubscriberArgs) (*mcp.CallToolResult, any, error) {
			return b.call(ctx, http.MethodPost, "/subscribers", "", args), nil, nil
		})
	mcp.AddTool(s, writeTool("create_template", "Create a reusable message template."),
		func(ctx context.Context, _ *mcp.CallToolRequest, args createTemplateArgs) (*mcp.CallToolResult, any, error) {
			return b.call(ctx, http.MethodPost, "/templates", "", args), nil, nil
		})
	mcp.AddTool(s, writeTool("create_contact_note", "Add a CRM note to a contact."),
		func(ctx context.Context, _ *mcp.CallToolRequest, args createNoteArgs) (*mcp.CallToolResult, any, error) {
			path, err := recordPath("/subscribers/{id}/notes", args.ID)
			if err != nil {
				return toolError(err), nil, nil
			}
			return b.call(ctx, http.MethodPost, path, "", map[string]string{"body": args.Body}), nil, nil
		})
}
