package models

import (
	"encoding/json"
	"time"
)

// Timeline Event Types
const (
	// Outbound campaign events
	TimelineEventCampaignSend = "campaign_send"
	TimelineEventCampaignView = "campaign_view"
	TimelineEventLinkClick    = "link_click"

	// Inbound SMS events
	TimelineEventInboundSMS = "inbound_sms"

	// Inbound email reply events
	TimelineEventInboundEmailReply = "inbound_email_reply"
)

// TimelineEventStatus represents the status field on events
const (
	// Campaign send statuses
	SendStatusSent      = "sent"
	SendStatusBounced   = "bounced"
	SendStatusSpamming  = "spamming"
	SendStatusUntracked = "untracked"

	// Inbound SMS statuses
	InboundSMSStatusReceived = "received"

	// Inbound email reply statuses
	InboundEmailStatusReceived = "received"
)

// Channel represents the communication channel
const (
	ChannelEmail = "email"
	ChannelSMS   = "sms"
)

// TimelineEvent is the unified event type for contact timeline (replacing disparate SubscriberActivity fields)
// All timeline events share this base contract for consistent sorting and filtering.
//
// Canonical fields:
// - event_type: One of the TimelineEvent* constants (campaign_send, inbound_sms, etc.)
// - channel: One of ChannelEmail or ChannelSMS
// - occurred_at: ISO 8601 timestamp when the event occurred (received, sent, viewed, clicked)
// - source: Where the event originated (e.g., "campaign", "que_webhook", "mailbox_webhook")
// - actor: Who or what caused the event (see TimelineActor)
// - metadata: Event-specific payload (see event-specific types below)
//
// Ordering: Newest-first by occurred_at (DESC), with rowid as stable tiebreaker (DESC).
// Pagination: Offset-based with limit parameter (e.g., first 50, next 50 after offset).
type TimelineEvent struct {
	EventType  string          `json:"event_type"`
	Channel    string          `json:"channel"`
	OccurredAt time.Time       `json:"occurred_at"`
	Source     string          `json:"source"`
	Actor      TimelineActor   `json:"actor"`
	Status     string          `json:"status"`
	Metadata   json.RawMessage `json:"metadata"`
}

// TimelineActor provides context about who/what triggered the event
type TimelineActor struct {
	Type  string `json:"type"`  // "system", "campaign", "provider", "webhook"
	ID    string `json:"id"`    // campaign_id, provider, etc.
	Label string `json:"label"` // "Campaign: Spring Sale", "Webhook: Quo", etc.
}

// TimelineEventCampaignSendMetadata is the metadata payload for campaign_send events
type TimelineEventCampaignSendMetadata struct {
	CampaignID     int        `json:"campaign_id"`
	CampaignName   string     `json:"campaign_name"`
	CampaignUUID   string     `json:"campaign_uuid"`
	Subject        string     `json:"subject"`
	MessageID      string     `json:"message_id"`
	HasOpened      bool       `json:"has_opened"`
	HasClicked     bool       `json:"has_clicked"`
	FirstOpenedAt  *time.Time `json:"first_opened_at,omitempty"`
	FirstClickedAt *time.Time `json:"first_clicked_at,omitempty"`
}

// TimelineEventCampaignViewMetadata is the metadata payload for campaign_view events
type TimelineEventCampaignViewMetadata struct {
	CampaignID   int    `json:"campaign_id"`
	CampaignName string `json:"campaign_name"`
	CampaignUUID string `json:"campaign_uuid"`
	Subject      string `json:"subject"`
	ViewCount    int    `json:"view_count"`
	UserAgent    string `json:"user_agent,omitempty"`
}

// TimelineEventLinkClickMetadata is the metadata payload for link_click events
type TimelineEventLinkClickMetadata struct {
	CampaignID   int    `json:"campaign_id"`
	CampaignName string `json:"campaign_name"`
	CampaignUUID string `json:"campaign_uuid"`
	Subject      string `json:"subject"`
	URL          string `json:"url"`
	ClickCount   int    `json:"click_count"`
	UserAgent    string `json:"user_agent,omitempty"`
}

// TimelineEventInboundSMSMetadata is the metadata payload for inbound_sms events
type TimelineEventInboundSMSMetadata struct {
	FromNumber    string          `json:"from_number"`     // Normalized phone number of sender
	MessageBody   string          `json:"message_body"`    // Full text of received SMS
	ProviderID    string          `json:"provider_id"`     // "quo", "twilio", etc.
	ProviderMsgID string          `json:"provider_msg_id"` // Provider's message ID for dedup
	Raw           json.RawMessage `json:"raw,omitempty"`   // Full raw webhook payload for audit
	IsStopKeyword bool            `json:"is_stop_keyword"` // Whether message matched CTIA STOP keywords
	MatchScore    string          `json:"match_score"`     // "exact", "fallback_10digit", "unmatched"
}

// TimelineEventInboundEmailReplyMetadata is the metadata payload for inbound_email_reply events
type TimelineEventInboundEmailReplyMetadata struct {
	InboundEmailReplyID string           `json:"inbound_email_reply_id"` // Timeline event source record id
	FromAddress         string           `json:"from_address"`           // Sender email address
	Subject             string           `json:"subject"`                // Email subject line
	BodySnippet         string           `json:"body_snippet"`           // First 200 chars of body for preview
	MessageID           string           `json:"message_id"`             // RFC 5322 Message-ID for dedup
	InReplyTo           string           `json:"in_reply_to"`            // RFC 5322 In-Reply-To header (outbound msg linkage)
	References          string           `json:"references"`             // RFC 5322 References header (thread context)
	HasAttachments      bool             `json:"has_attachments"`        // Whether email contains MIME attachments
	Raw                 json.RawMessage  `json:"raw,omitempty"`          // Full raw headers + body for audit
	MatchScore          string           `json:"match_score"`            // "exact_messageID", "exact_email", "unmatched"
	Attachments         []map[string]any `json:"attachments,omitempty"`  // List of attachment metadata
}

// InboundSMSEvent represents a persistent inbound SMS event in the database
// This model is stored in the inbound_sms_events collection and linked to subscribers by phone/list context.
//
// Idempotency: (provider_id, provider_msg_id, received_at) + sender_hash fallback.
// Contact matching: Exact digit-normalized phone + last-10-digit fallback (reuse SMS opt-out logic).
// Linkage policy: Strict subscriber linkage when determinable; preserve raw payload for audit if no match found.
type InboundSMSEvent struct {
	Base

	// Linkage to subscriber and list (nullable if match could not be determined)
	SubscriberID *string `db:"subscriber_id" json:"subscriber_id,omitempty"`
	ListID       *string `db:"list_id" json:"list_id,omitempty"`
	PhoneNumber  string  `db:"phone_number" json:"phone_number"` // Normalized digits for matching

	// Message content and metadata
	ProviderID    string    `db:"provider_id" json:"provider_id"`         // "quo", "twilio", etc.
	ProviderMsgID string    `db:"provider_msg_id" json:"provider_msg_id"` // Provider's unique msg ID
	FromNumber    string    `db:"from_number" json:"from_number"`         // Sender's phone (may differ from normalized)
	MessageBody   string    `db:"message_body" json:"message_body"`
	ReceivedAt    time.Time `db:"received_at" json:"received_at"`

	// Classification
	IsStopKeyword bool   `db:"is_stop_keyword" json:"is_stop_keyword"` // CTIA STOP keyword detected
	MatchScore    string `db:"match_score" json:"match_score"`         // "exact", "fallback_10digit", "unmatched"

	// Audit trail
	RawPayload  JSON      `db:"raw_payload" json:"raw_payload"`   // Full webhook payload for compliance
	ProcessedAt time.Time `db:"processed_at" json:"processed_at"` // When this event was ingested

	// Idempotency keys (unique constraint: provider_id + provider_msg_id + received_at, plus sender_hash fallback)
	SenderHash string `db:"sender_hash" json:"sender_hash"` // Hash of from_number for fallback dedup
}

// InboundEmailReplyEvent represents a persistent inbound email reply in the database
// This model is stored in the inbound_email_replies collection and linked to subscribers by email address.
//
// Idempotency: message_id + in_reply_to + from_address (strict).
// Contact matching: Exact email address match only; preserve raw headers/body for audit if no match found.
// Threading: In-Reply-To and References headers enable grouping with outbound campaigns.
// Linkage policy: Strict subscriber linkage when determinable; attempt linkage to outbound message via Message-ID if possible.
type InboundEmailReplyEvent struct {
	Base

	// Linkage to subscriber and outbound message (nullable if match could not be determined)
	SubscriberID    *string `db:"subscriber_id" json:"subscriber_id,omitempty"`
	LinkedMessageID *string `db:"linked_message_id" json:"linked_message_id,omitempty"` // Link to outbound campaign_send_ledger

	// Message identification
	FromAddress string    `db:"from_address" json:"from_address"` // Sender's email address
	Subject     string    `db:"subject" json:"subject"`
	MessageID   string    `db:"message_id" json:"message_id"`   // RFC 5322 Message-ID (unique identifier)
	InReplyTo   string    `db:"in_reply_to" json:"in_reply_to"` // RFC 5322 In-Reply-To (outbound msg link)
	References  string    `db:"references" json:"references"`   // RFC 5322 References (thread context)
	ReceivedAt  time.Time `db:"received_at" json:"received_at"`

	// Content (storing snippet + full for preview + compliance)
	BodySnippet    string `db:"body_snippet" json:"body_snippet"` // First 200 chars for preview
	HasAttachments bool   `db:"has_attachments" json:"has_attachments"`

	// Match classification
	MatchScore string `db:"match_score" json:"match_score"` // "exact_messageID", "exact_email", "unmatched"

	// Audit trail
	RawHeaders  JSON      `db:"raw_headers" json:"raw_headers"`   // Full MIME headers for compliance
	RawBody     JSON      `db:"raw_body" json:"raw_body"`         // Full decoded body
	ProcessedAt time.Time `db:"processed_at" json:"processed_at"` // When this event was ingested

	// Idempotency key (unique constraint: message_id + from_address + received_at)
	DedupeKey string `db:"dedupe_key" json:"dedupe_key"` // Hash of message_id+from_address for dedup
}

// UnifiedContactTimeline represents the response payload from the timeline API endpoint
// It merges outbound activity with inbound SMS and email replies, ordered newest-first.
type UnifiedContactTimeline struct {
	Events  []TimelineEvent `json:"events"`   // Sorted newest-first by occurred_at DESC, rowid DESC
	Total   int             `json:"total"`    // Total event count across all types (for pagination)
	HasMore bool            `json:"has_more"` // Whether more events exist beyond current page
	Offset  int             `json:"offset"`   // Current pagination offset
	Limit   int             `json:"limit"`    // Current page size
}
