package core

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/compdani/list_pocket/internal/apperr"
	"github.com/compdani/list_pocket/models"
)

// Views of the conversation list. Closing is only ever temporary: a new inbound
// message puts a conversation back in the open view.
const (
	ConversationViewOpen   = "open"
	ConversationViewUnread = "unread"
	ConversationViewClosed = "closed"
	ConversationViewAll    = "all"

	maxThreadMessages = 200
)

// conversationMessagesSQL merges every person-to-person message into one shape. The
// key groups a contact's messages across channels, and senders who aren't contacts
// fall back to a channel:address key. An email from an address that later became a
// contact joins that contact's conversation, which is why email matches on address
// as well as the stored link.
//
// Campaign sends only appear when includeCampaigns is set, so a blast to thousands of
// people doesn't open thousands of conversations. Status-only stubs have no address.
func conversationMessagesSQL(includeCampaigns bool) string {
	campaignFilter := ` AND ce.campaign = ''`
	if includeCampaigns {
		campaignFilter = ``
	}
	return `
SELECT
  CASE WHEN subscriber != '' THEN subscriber ELSE channel || ':' || address END AS conv_key,
  id, subscriber, channel, direction, kind, status, address, subject, body, occurred_at, has_attachments
FROM (
  SELECT
    e.id AS id,
    COALESCE(NULLIF(e.subscriber_id, ''), m.id, '') AS subscriber,
    '` + models.ChannelEmail + `' AS channel,
    'inbound' AS direction,
    'message' AS kind,
    '' AS status,
    lower(e.from_address) AS address,
    e.subject AS subject,
    e.body_snippet AS body,
    e.received_at AS occurred_at,
    e.has_attachments AS has_attachments
  FROM inbound_email_replies e
  LEFT JOIN subscribers m ON m.email = lower(e.from_address)
  WHERE COALESCE(e.spam_status, '') = ''
  UNION ALL
  SELECT
    sms.id, COALESCE(sms.subscriber_id, ''), '` + models.ChannelSMS + `', 'inbound', 'message', '',
    sms.from_number, '', sms.message_body, sms.received_at, 0
  FROM inbound_sms_events sms
  UNION ALL
  SELECT
    CAST(ce.id AS TEXT), ce.subscriber, ce.channel, ce.direction, ce.kind, ce.status,
    ce.address, COALESCE(json_extract(CASE WHEN json_valid(ce.payload) THEN ce.payload END, '$.subject'), ''),
    ce.body, ce.occurred_at, 0
  FROM channel_events ce
  WHERE ce.address != ''` + campaignFilter + `
)`
}

// Conversation is one row of the conversation list.
type Conversation struct {
	Key            string   `db:"conv_key" json:"key"`
	Subscriber     string   `db:"subscriber" json:"subscriber"`
	Name           string   `db:"name" json:"name"`
	Email          string   `db:"email" json:"email"`
	Address        string   `db:"address" json:"address"`
	Channel        string   `db:"channel" json:"channel"`
	Direction      string   `db:"direction" json:"direction"`
	Subject        string   `db:"subject" json:"subject"`
	Preview        string   `db:"preview" json:"preview"`
	LastAt         string   `db:"last_at" json:"last_at"`
	Messages       int      `db:"messages" json:"messages"`
	Unread         bool     `db:"unread" json:"unread"`
	Closed         bool     `db:"closed" json:"closed"`
	HasAttachments bool     `db:"has_attachments" json:"has_attachments"`
	OwnerName      string   `db:"owner_name" json:"owner_name"`
	ChannelsCSV    string   `db:"channels" json:"-"`
	Channels       []string `db:"-" json:"channels"`
}

// ConversationQuery filters ListConversations.
type ConversationQuery struct {
	View    string
	Channel string
	Search  string
	Limit   int
	Offset  int
}

// ConversationList is a page of conversations. Unread counts the whole inbox, not
// just this page, so the list can show it next to any filter.
type ConversationList struct {
	Results []Conversation `json:"results"`
	Total   int            `json:"total"`
	Unread  int            `json:"unread"`
}

// ListConversations returns conversations with the latest activity first.
func (c *Core) ListConversations(q ConversationQuery) (ConversationList, error) {
	out := ConversationList{Results: []Conversation{}}
	limit := q.Limit
	if limit < 1 || limit > 100 {
		limit = 50
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}

	const unreadExpr = `(d.last_inbound != '' AND datetime(d.last_inbound) > datetime(COALESCE(NULLIF(st.last_read_at, ''), '0001-01-01')))`
	const closedExpr = `(COALESCE(st.closed_at, '') != '' AND (d.last_inbound = '' OR datetime(d.last_inbound) <= datetime(st.closed_at)))`

	base := `
WITH msgs AS (` + conversationMessagesSQL(false) + `),
ranked AS (
  SELECT *, ROW_NUMBER() OVER (PARTITION BY conv_key ORDER BY datetime(occurred_at) DESC, id DESC) AS rn
  FROM msgs
),
agg AS (
  SELECT conv_key AS agg_key,
    COUNT(*) AS messages,
    COALESCE(MAX(CASE WHEN direction = 'inbound' THEN occurred_at END), '') AS last_inbound,
    GROUP_CONCAT(DISTINCT channel) AS channels
  FROM msgs GROUP BY conv_key
),
d AS (
  SELECT r.*, a.messages, a.last_inbound, a.channels AS channels
  FROM ranked r JOIN agg a ON a.agg_key = r.conv_key
  WHERE r.rn = 1
)`
	from := `
FROM d
LEFT JOIN conversation_state st ON st.key = d.conv_key
LEFT JOIN subscribers s ON s.id = d.subscriber
LEFT JOIN contact_profiles p ON p.subscriber = d.subscriber
LEFT JOIN users u ON u.id = p.owner`

	var (
		conds []string
		args  []any
	)
	switch q.View {
	case ConversationViewAll:
	case ConversationViewUnread:
		conds = append(conds, unreadExpr, `NOT `+closedExpr)
	case ConversationViewClosed:
		conds = append(conds, closedExpr)
	default:
		conds = append(conds, `NOT `+closedExpr)
	}
	if ch := strings.TrimSpace(q.Channel); ch != "" {
		conds = append(conds, `(',' || d.channels || ',') LIKE ?`+likeEsc)
		args = append(args, likeContains(","+ch+","))
	}
	if search := strings.TrimSpace(q.Search); search != "" {
		like := likeContains(strings.ToLower(search))
		conds = append(conds, `(LOWER(COALESCE(s.name, '')) LIKE ?`+likeEsc+
			` OR LOWER(COALESCE(s.email, '')) LIKE ?`+likeEsc+
			` OR LOWER(d.address) LIKE ?`+likeEsc+
			` OR LOWER(d.subject) LIKE ?`+likeEsc+
			` OR LOWER(d.body) LIKE ?`+likeEsc+`)`)
		args = append(args, like, like, like, like, like)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	if err := c.db.Get(&out.Total, base+`SELECT COUNT(*)`+from+where, args...); err != nil {
		return out, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "conversations", "error", dbErr(err)))
	}
	if err := c.db.Get(&out.Unread, base+`SELECT COUNT(*)`+from+` WHERE `+unreadExpr+` AND NOT `+closedExpr); err != nil {
		return out, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "conversations", "error", dbErr(err)))
	}
	if out.Total == 0 {
		return out, nil
	}

	listArgs := append(append([]any{}, args...), limit, offset)
	if err := c.db.Select(&out.Results, base+`
SELECT d.conv_key, d.subscriber,
  COALESCE(s.name, '') AS name, COALESCE(s.email, '') AS email,
  d.address, d.channel, d.direction, d.subject, d.body AS preview, d.occurred_at AS last_at,
  d.messages, d.channels, d.has_attachments,
  `+unreadExpr+` AS unread, `+closedExpr+` AS closed,
  COALESCE(NULLIF(trim(u.name), ''), u.username, '') AS owner_name`+from+where+`
ORDER BY datetime(d.occurred_at) DESC, d.conv_key LIMIT ? OFFSET ?`, listArgs...); err != nil {
		return out, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "conversations", "error", dbErr(err)))
	}
	for i := range out.Results {
		r := &out.Results[i]
		r.Channels = strings.Split(r.ChannelsCSV, ",")
		if models.IsPlaceholderEmail(r.Email) {
			r.Email = ""
		}
	}
	return out, nil
}

// ConversationMessage is one message in a thread.
type ConversationMessage struct {
	ConvKey        string `db:"conv_key" json:"-"`
	ID             string `db:"id" json:"id"`
	Subscriber     string `db:"subscriber" json:"-"`
	Channel        string `db:"channel" json:"channel"`
	Direction      string `db:"direction" json:"direction"`
	Kind           string `db:"kind" json:"kind"`
	Status         string `db:"status" json:"status"`
	Address        string `db:"address" json:"address"`
	Subject        string `db:"subject" json:"subject"`
	Body           string `db:"body" json:"body"`
	OccurredAt     string `db:"occurred_at" json:"occurred_at"`
	HasAttachments bool   `db:"has_attachments" json:"has_attachments"`
}

// ConversationThread is a conversation's latest messages, oldest first.
type ConversationThread struct {
	Key        string                `json:"key"`
	Subscriber string                `json:"subscriber"`
	Address    string                `json:"address"`
	Channel    string                `json:"channel"`
	Messages   []ConversationMessage `json:"messages"`
}

// GetConversationThread returns a conversation's messages. A known contact's thread
// also shows campaign sends on text channels, because a reply is often to one of them.
func (c *Core) GetConversationThread(key string) (ConversationThread, error) {
	key = strings.TrimSpace(key)
	out := ConversationThread{Key: key, Messages: []ConversationMessage{}}
	if key == "" {
		return out, apperr.BadRequest("a conversation key is required")
	}
	isContact := !strings.Contains(key, ":")
	if err := c.db.Select(&out.Messages, `
SELECT * FROM (
  SELECT * FROM (`+conversationMessagesSQL(isContact)+`) WHERE conv_key = ?
  ORDER BY datetime(occurred_at) DESC, id DESC LIMIT ?
) ORDER BY datetime(occurred_at) ASC, id ASC`, key, maxThreadMessages); err != nil {
		return out, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "conversation", "error", dbErr(err)))
	}
	if len(out.Messages) == 0 {
		if !isContact {
			return out, apperr.NotFound("conversation not found")
		}
		out.Subscriber = key
		return out, nil
	}
	last := out.Messages[len(out.Messages)-1]
	out.Subscriber, out.Channel, out.Address = last.Subscriber, last.Channel, last.Address
	return out, nil
}

// MarkConversationRead records that the team has seen a conversation up to now.
func (c *Core) MarkConversationRead(key string) error {
	_, err := c.db.Exec(`
INSERT INTO conversation_state (key, last_read_at) VALUES (?, strftime('%Y-%m-%d %H:%M:%fZ', 'now'))
ON CONFLICT (key) DO UPDATE SET last_read_at = excluded.last_read_at`, strings.TrimSpace(key))
	return err
}

// SetConversationClosed closes or reopens a conversation.
func (c *Core) SetConversationClosed(key string, closed bool) error {
	closedAt := `strftime('%Y-%m-%d %H:%M:%fZ', 'now')`
	if !closed {
		closedAt = `''`
	}
	_, err := c.db.Exec(`
INSERT INTO conversation_state (key, closed_at) VALUES (?, `+closedAt+`)
ON CONFLICT (key) DO UPDATE SET closed_at = excluded.closed_at`, strings.TrimSpace(key))
	return err
}

// LatestConversationEmail returns the inbound email a reply on this conversation should
// answer. A false result means there isn't one.
func (c *Core) LatestConversationEmail(thread ConversationThread) (models.InboundEmailReplyEvent, bool, error) {
	var id string
	for i := len(thread.Messages) - 1; i >= 0; i-- {
		m := thread.Messages[i]
		if m.Channel == models.ChannelEmail && m.Direction == ChannelDirectionInbound {
			id = m.ID
			break
		}
	}
	if id == "" {
		return models.InboundEmailReplyEvent{}, false, nil
	}
	e, err := c.GetInboundEmailByID(context.Background(), id)
	if err != nil {
		return models.InboundEmailReplyEvent{}, false, err
	}
	return *e, true, nil
}

// HasSMSOptOut reports whether a contact said stop by SMS or is unsubscribed from SMS
// on any list. A reply is refused in either case, so one list's consent can't override
// another's opt-out.
func (c *Core) HasSMSOptOut(sub models.Subscriber) (bool, error) {
	var out bool
	err := c.db.Get(&out, `
SELECT EXISTS (SELECT 1 FROM subscriber_lists WHERE subscriber_id = ? AND sms_status = ?)
    OR EXISTS (SELECT 1 FROM inbound_sms_events WHERE subscriber_id = ? AND is_stop_keyword = 1)`,
		sub.RecordID, models.SubscriptionStatusUnsubscribed, sub.RecordID)
	return out, err
}

// RecordConversationReply stores a staff reply sent on a channel that has no event log
// of its own, so it shows in the thread and the contact's timeline.
func (c *Core) RecordConversationReply(channel, subRecID, address, subject, body, msgID string) error {
	payload, err := json.Marshal(map[string]string{"subject": subject})
	if err != nil {
		return err
	}
	_, err = c.RecordChannelEvent(ChannelEvent{
		Channel: channel, Direction: ChannelDirectionOutbound, Kind: "reply", Status: "queued",
		ProviderMsgID: msgID, SubscriberID: subRecID, Address: address, Body: body, Payload: string(payload),
	})
	return err
}
