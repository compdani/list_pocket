package flow

import "sort"

// Events List Pocket raises. Workflows start on them with an event trigger, and pause
// for them with a wait node. Each description says what event.* holds.
var knownEvents = map[string]string{
	EventContactCreated:      "A subscriber was added. event: {contact_id, email, name}",
	EventContactTagAdded:     "A tag was added to a subscriber. event: {contact_id, email, tag}",
	EventContactTagRemoved:   "A tag was removed from a subscriber. event: {contact_id, email, tag}",
	EventContactStageChanged: "A contact was added to, moved on or removed from a board. stage is empty after a removal, and previous_stage is empty after an add. Stages are keys, and stage names can repeat across boards, so filter on board too. event: {contact_id, email, name, board, board_name, stage, stage_name, previous_stage}",
	EventInboundEmail:        "An email arrived. event: {conversation, contact_id, from, subject, snippet, email_id}",
	EventInboundSMS:          "An SMS arrived. event: {conversation, contact_id, from, body, is_stop}",
	EventInboundChannel:      "A WhatsApp or plugin channel message arrived. event: {conversation, contact_id, channel, from, body}",
	EventCampaignFinished:    "A campaign finished sending. event: {campaign_id, name}",
}

// Event names.
const (
	EventContactCreated      = "contact.created"
	EventContactTagAdded     = "contact.tag_added"
	EventContactTagRemoved   = "contact.tag_removed"
	EventContactStageChanged = "contact.stage_changed"
	EventInboundEmail        = "inbound.email"
	EventInboundSMS          = "inbound.sms"
	EventInboundChannel      = "inbound.channel"
	EventCampaignFinished    = "campaign.finished"
)

// triggerCollections are the only collections a record trigger can watch. Accounts,
// roles and settings are left out, because a record's fields become run input that
// staff and models can read.
var triggerCollections = map[string]bool{
	"subscribers": true, "lists": true, "campaigns": true, "templates": true,
	"inbound_email_replies": true, "inbound_sms_events": true,
}

// IsKnownEvent reports whether List Pocket raises this event.
func IsKnownEvent(name string) bool {
	_, ok := knownEvents[name]
	return ok
}

// RegisterEvent adds a plugin's event to the catalog, and returns false if the name is
// taken. Call it at startup, before workflows run.
func RegisterEvent(name, description string) bool {
	if IsKnownEvent(name) {
		return false
	}
	knownEvents[name] = description
	return true
}

// EventInfo describes one event for pickers and the assistant.
type EventInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Events lists every event in name order.
func Events() []EventInfo {
	out := make([]EventInfo, 0, len(knownEvents))
	for name, desc := range knownEvents {
		out = append(out, EventInfo{Name: name, Description: desc})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// TriggerCollections lists the collections a record trigger can watch.
func TriggerCollections() []string {
	out := make([]string, 0, len(triggerCollections))
	for name := range triggerCollections {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
