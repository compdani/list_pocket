package main

import (
	"github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/internal/flow/tools"
	"github.com/compdani/list_pocket/models"
)

var (
	outString = map[string]any{"type": "string"}
	outInt    = map[string]any{"type": "integer"}
	outBool   = map[string]any{"type": "boolean"}
)

func outObject(props map[string]any) map[string]any {
	return map[string]any{"type": "object", "properties": props}
}

func outArray(items map[string]any) map[string]any {
	return map[string]any{"type": "array", "items": items}
}

// contactOutput describes contactSummary.
func contactOutput(extra map[string]any) map[string]any {
	props := map[string]any{
		"id": outString, "email": outString, "name": outString, "phone": outString,
		"status": outString, "attribs": map[string]any{"type": "object"},
	}
	for name, schema := range extra {
		props[name] = schema
	}
	return outObject(props)
}

// flowToolOutputs says what each tool in flowTools returns. Results built from a type are
// described from that type. The rest are maps, so they're spelled out here.
func flowToolOutputs() map[string]map[string]any {
	campaignRef := outObject(map[string]any{"id": outString, "name": outString, "status": outString})
	listChange := outObject(map[string]any{"contact": outString, "list": outString, "subscribed": outBool})
	tagChange := outObject(map[string]any{"contact": outString, "tags": outArray(outString)})
	boardMove := tools.SchemaFor[core.BoardMove]()
	return map[string]map[string]any{
		"contacts.search": outObject(map[string]any{"total": outInt, "results": outArray(contactOutput(nil))}),
		"contacts.get": contactOutput(map[string]any{
			"lists": outArray(outObject(map[string]any{
				"id": outString, "name": outString, "type": outString,
				"subscription_status": outString, "subscription_sms_status": outString,
			})),
			"boards": tools.SchemaFor[[]core.ContactBoard](), "owner": outString, "company": outString,
			"open_tasks": tools.SchemaFor[[]core.ContactTask](), "notes": tools.SchemaFor[[]core.ContactNote](),
		}),
		"contacts.create":           outObject(map[string]any{"created": outBool, "contact": contactOutput(nil)}),
		"contacts.add_to_list":      listChange,
		"contacts.remove_from_list": listChange,
		"contacts.add_tag":          tagChange,
		"contacts.remove_tag":       tagChange,
		"boards.list":               tools.SchemaFor[[]core.Board](),
		"boards.get_contact":        outObject(map[string]any{"boards": tools.SchemaFor[[]core.ContactBoard](), "stage": outString}),
		"boards.move_contact":       boardMove,
		"boards.remove_contact":     boardMove,
		"crm.update_contact":        tools.SchemaFor[core.ContactProfile](),
		"crm.add_note":              tools.SchemaFor[core.ContactNote](),
		"crm.add_task":              tools.SchemaFor[core.ContactTask](),
		"crm.list_tasks":            tools.SchemaFor[[]core.ContactTask](),
		"conversations.list":        tools.SchemaFor[core.ConversationList](),
		"conversations.thread":      tools.SchemaFor[core.ConversationThread](),
		"conversations.reply":       outObject(map[string]any{"sent": outBool, "reason": outString}),
		"conversations.close":       outObject(map[string]any{"closed": outBool}),
		"messages.send_email":       outObject(map[string]any{"sent": outBool, "reason": outString, "id": outString, "to": outString}),
		"lists.list":                tools.SchemaFor[[]models.List](),
		"templates.list":            tools.SchemaFor[[]models.Template](),
		"campaigns.list":            outObject(map[string]any{"total": outInt, "results": tools.SchemaFor[[]models.Campaign]()}),
		"campaigns.create_draft":    campaignRef,
		"campaigns.start":           campaignRef,
		"http.request":              outObject(map[string]any{"status": outInt, "ok": outBool, "body": map[string]any{}}),
	}
}
