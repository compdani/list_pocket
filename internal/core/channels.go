package core

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/apperr"
	"github.com/compdani/list_pocket/internal/phoneutil"
	"github.com/compdani/list_pocket/models"
)

// Consent and address statuses for plugin channels.
const (
	ChannelAddressActive  = "active"
	ChannelAddressInvalid = "invalid"

	// channelConsentAll is the list value of a channel-wide opt-out row. A contact
	// with no lists still needs somewhere to record that they said stop.
	channelConsentAll = "*"

	// Where consent came from.
	ConsentSourceInbound = "inbound"
	ConsentSourceAdmin   = "admin"
	ConsentSourceSMSCopy = "sms_copy"
)

// EnsureChannelTriggers creates the triggers that clear channel data when a
// subscriber or list is deleted. Several deletes use raw SQL, so PocketBase
// cascades don't fire. It runs at every boot because rebuilding a collection's
// table drops its triggers.
func (c *Core) EnsureChannelTriggers() error {
	_, err := c.db.Exec(`
CREATE TRIGGER IF NOT EXISTS trg_subscribers_channel_cleanup AFTER DELETE ON subscribers
BEGIN
  DELETE FROM subscriber_addresses WHERE subscriber = OLD.id;
  DELETE FROM subscriber_channel_consent WHERE subscriber = OLD.id;
END;
CREATE TRIGGER IF NOT EXISTS trg_lists_channel_cleanup AFTER DELETE ON lists
BEGIN
  DELETE FROM subscriber_channel_consent WHERE list = OLD.id;
END;`)
	return err
}

func requirePluginChannel(channel string) (models.ChannelSpec, error) {
	spec := models.ChannelFor(channel)
	if !strings.HasPrefix(spec.ID, models.ChannelMessengerPrefix) {
		return spec, apperr.BadRequest(fmt.Sprintf("%q is not a plugin channel", channel))
	}
	return spec, nil
}

// GetChannelAddresses returns active addresses for subscribers on a channel,
// keyed by subscriber record ID.
func (c *Core) GetChannelAddresses(channel string, subRecIDs []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(subRecIDs) == 0 {
		return out, nil
	}
	args := make([]any, 0, len(subRecIDs)+2)
	args = append(args, channel, ChannelAddressActive)
	for _, id := range subRecIDs {
		args = append(args, id)
	}
	var rows []struct {
		Subscriber string `db:"subscriber"`
		Address    string `db:"address"`
	}
	if err := c.db.Select(&rows, `
SELECT subscriber, address FROM subscriber_addresses
WHERE channel = ? AND status = ? AND subscriber IN (`+sqlitePlaceholders(len(subRecIDs))+`)
ORDER BY id`, args...); err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.Subscriber] = append(out[r.Subscriber], r.Address)
	}
	return out, nil
}

// SubscriberChannelInfo is one subscriber's state on one plugin channel.
type SubscriberChannelInfo struct {
	Channel   string          `json:"channel"`
	Addresses []ChannelAddr   `json:"addresses"`
	Consent   []ChannelStatus `json:"consent"`
}

// ChannelAddr is a subscriber's address on a channel.
type ChannelAddr struct {
	Address string `db:"address" json:"address"`
	Status  string `db:"status" json:"status"`
}

// ChannelStatus is a subscriber's consent for a list on a channel.
type ChannelStatus struct {
	ListID   string `db:"list" json:"list_id"`
	ListName string `db:"list_name" json:"list_name"`
	Status   string `db:"status" json:"status"`
}

// GetSubscriberChannelInfo returns the subscriber's addresses and per-list consent
// on the given channels. Every list the subscriber belongs to is included, with
// "none" when there's no consent row, so the UI can offer to grant it.
func (c *Core) GetSubscriberChannelInfo(subRecID string, channels []string) ([]SubscriberChannelInfo, error) {
	out := make([]SubscriberChannelInfo, 0, len(channels))
	for _, channel := range channels {
		info := SubscriberChannelInfo{Channel: channel, Addresses: []ChannelAddr{}, Consent: []ChannelStatus{}}
		if err := c.db.Select(&info.Addresses, `
SELECT address, status FROM subscriber_addresses WHERE subscriber = ? AND channel = ? ORDER BY id`, subRecID, channel); err != nil {
			return nil, err
		}
		if err := c.db.Select(&info.Consent, `
SELECT sl.list_id AS list, l.name AS list_name, COALESCE(cc.status, 'none') AS status
FROM subscriber_lists sl
JOIN lists l ON l.id = sl.list_id
LEFT JOIN subscriber_channel_consent cc
  ON cc.subscriber = sl.subscriber_id AND cc.list = sl.list_id AND cc.channel = ?
WHERE sl.subscriber_id = ?
ORDER BY l.name`, channel, subRecID); err != nil {
			return nil, err
		}
		out = append(out, info)
	}
	return out, nil
}

// SetChannelAddress adds an active address. An address belongs to one subscriber
// per channel, so claiming one held by someone else is a conflict.
func (c *Core) SetChannelAddress(subRecID, channel, address string) error {
	if _, err := requirePluginChannel(channel); err != nil {
		return err
	}
	address = strings.TrimSpace(address)
	if address == "" {
		return apperr.BadRequest("address is required")
	}
	var owner string
	err := c.db.Get(&owner, `SELECT subscriber FROM subscriber_addresses WHERE channel = ? AND address = ?`, channel, address)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && owner != subRecID {
		return apperr.Conflict("that address already belongs to another subscriber")
	}
	_, err = c.db.Exec(`
INSERT INTO subscriber_addresses (subscriber, channel, address, status) VALUES (?, ?, ?, ?)
ON CONFLICT (channel, address) DO UPDATE SET status = excluded.status`, subRecID, channel, address, ChannelAddressActive)
	return err
}

// RemoveChannelAddress deletes a subscriber's address.
func (c *Core) RemoveChannelAddress(subRecID, channel, address string) error {
	_, err := c.db.Exec(`DELETE FROM subscriber_addresses WHERE subscriber = ? AND channel = ? AND address = ?`, subRecID, channel, address)
	return err
}

// SetChannelConsent records consent for one list on a channel.
func (c *Core) SetChannelConsent(subRecID, listRecID, channel, status, source string) error {
	if _, err := requirePluginChannel(channel); err != nil {
		return err
	}
	if status != models.SubscriptionStatusConfirmed && status != models.SubscriptionStatusUnsubscribed {
		return apperr.BadRequest("consent status must be confirmed or unsubscribed")
	}
	if _, err := c.db.Exec(`
INSERT INTO subscriber_channel_consent (subscriber, list, channel, status, source) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (subscriber, list, channel) DO UPDATE SET
  status = excluded.status, source = excluded.source, updated = strftime('%Y-%m-%d %H:%M:%fZ', 'now')`,
		subRecID, listRecID, channel, status, source); err != nil {
		return err
	}
	if status != models.SubscriptionStatusConfirmed {
		return nil
	}
	// New consent replaces an earlier channel-wide opt-out.
	_, err := c.db.Exec(`DELETE FROM subscriber_channel_consent WHERE subscriber = ? AND list = ? AND channel = ?`,
		subRecID, channelConsentAll, channel)
	return err
}

// ChannelHasOptOut reports whether a subscriber said stop on a channel and hasn't
// consented since.
func (c *Core) ChannelHasOptOut(subRecID, channel string) (bool, error) {
	var out bool
	err := c.db.Get(&out, `
SELECT EXISTS (SELECT 1 FROM subscriber_channel_consent WHERE subscriber = ? AND list = ? AND channel = ?)`,
		subRecID, channelConsentAll, channel)
	return out, err
}

// MarkChannelAddressInvalid stops sending to an address after the provider rejected
// it permanently. It's kept apart from consent, because "this number isn't on the
// channel" isn't the same as "this person said stop". For a phone-sourced channel
// the row excludes everyone who shares the number.
func (c *Core) MarkChannelAddressInvalid(subRecID, channel, address string) error {
	_, err := c.db.Exec(`
INSERT INTO subscriber_addresses (subscriber, channel, address, status) VALUES (?, ?, ?, ?)
ON CONFLICT (channel, address) DO UPDATE SET status = excluded.status`,
		subRecID, channel, strings.TrimSpace(address), ChannelAddressInvalid)
	return err
}

// ReactivateChannelAddress clears an invalid mark, because a message from that
// address proves it works.
func (c *Core) ReactivateChannelAddress(channel, address string) error {
	_, err := c.db.Exec(`UPDATE subscriber_addresses SET status = ? WHERE channel = ? AND address = ? AND status = ?`,
		ChannelAddressActive, channel, strings.TrimSpace(address), ChannelAddressInvalid)
	return err
}

// SetChannelAddressAlias records another form of an address for the same subscriber,
// such as the ID a provider reports back when it differs from the number we sent to.
func (c *Core) SetChannelAddressAlias(subRecID, channel, alias string) error {
	_, err := c.db.Exec(`INSERT OR IGNORE INTO subscriber_addresses (subscriber, channel, address, status) VALUES (?, ?, ?, ?)`,
		subRecID, channel, strings.TrimSpace(alias), ChannelAddressActive)
	return err
}

// OptOutChannelSubscriber withdraws consent for a subscriber on every list of a
// channel, and skips their pending sends on it. Rows are written for every list
// they belong to, plus a channel-wide row, so a later re-add can't silently restore
// consent.
func (c *Core) OptOutChannelSubscriber(subRecID, channel string) (int64, error) {
	res, err := c.db.Exec(`
INSERT INTO subscriber_channel_consent (subscriber, list, channel, status, source)
SELECT sl.subscriber_id, sl.list_id, ?, ?, ? FROM subscriber_lists sl WHERE sl.subscriber_id = ?
ON CONFLICT (subscriber, list, channel) DO UPDATE SET
  status = excluded.status, source = excluded.source, updated = strftime('%Y-%m-%d %H:%M:%fZ', 'now')`,
		channel, models.SubscriptionStatusUnsubscribed, ConsentSourceInbound, subRecID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if _, err := c.db.Exec(`
INSERT INTO subscriber_channel_consent (subscriber, list, channel, status, source) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (subscriber, list, channel) DO UPDATE SET
  status = excluded.status, source = excluded.source, updated = strftime('%Y-%m-%d %H:%M:%fZ', 'now')`,
		subRecID, channelConsentAll, channel, models.SubscriptionStatusUnsubscribed, ConsentSourceInbound); err != nil {
		return n, err
	}
	if _, err := c.db.Exec(`
UPDATE campaign_send_ledger
SET status = 'skipped', updated = strftime('%Y-%m-%d %H:%M:%fZ', 'now')
WHERE status = 'pending' AND subscriber_id = ?
  AND campaign_id IN (SELECT id FROM campaigns WHERE trim(COALESCE(messenger, '')) = ?)`, subRecID, channel); err != nil {
		return n, err
	}
	return n, nil
}

// OptOutChannelAddress opts out everyone who owns an address. Several subscribers
// can share a phone number, and a stop from that number applies to all of them.
func (c *Core) OptOutChannelAddress(channel, address string) (int64, error) {
	subs, err := c.FindSubscribersByChannelAddress(channel, address)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, sub := range subs {
		n, err := c.OptOutChannelSubscriber(sub.RecordID, channel)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// FindSubscribersByChannelAddress finds everyone who owns an address: the address
// table on any channel, which includes provider aliases, and the phone column for
// phone-sourced channels.
func (c *Core) FindSubscribersByChannelAddress(channel, address string) ([]models.Subscriber, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, nil
	}
	var (
		out  []models.Subscriber
		seen = map[string]bool{}
	)
	var recIDs []string
	if err := c.db.Select(&recIDs, `SELECT subscriber FROM subscriber_addresses WHERE channel = ? AND address = ?`, channel, address); err != nil {
		return nil, err
	}
	for _, recID := range recIDs {
		sub, ok, err := c.subscriberForChannel(recID)
		if err != nil {
			return nil, err
		}
		if ok && !seen[sub.RecordID] {
			seen[sub.RecordID] = true
			out = append(out, sub)
		}
	}
	if models.ChannelFor(channel).Address != models.AddressSourcePhone {
		return out, nil
	}
	subs, err := c.GetSubscribersByNormalizedPhones([]string{phoneutil.NormalizeDigits(address)})
	var appErr *apperr.Error
	if errors.As(err, &appErr) && appErr.Status == http.StatusBadRequest {
		// That lookup reports "no match" as a bad request.
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, sub := range subs {
		if !seen[sub.RecordID] {
			seen[sub.RecordID] = true
			out = append(out, sub)
		}
	}
	return out, nil
}

// FindSubscriberByChannelAddress returns the first subscriber who owns an address.
// A false result means no match.
func (c *Core) FindSubscriberByChannelAddress(channel, address string) (models.Subscriber, bool, error) {
	subs, err := c.FindSubscribersByChannelAddress(channel, address)
	if err != nil || len(subs) == 0 {
		return models.Subscriber{}, false, err
	}
	return subs[0], true, nil
}

// GetSubscriberForChannel returns a subscriber by record ID. A false result means no match.
func (c *Core) GetSubscriberForChannel(recID string) (models.Subscriber, bool, error) {
	return c.subscriberForChannel(recID)
}

func (c *Core) subscriberForChannel(recID string) (models.Subscriber, bool, error) {
	var row struct {
		ID        int    `db:"id"`
		RecordID  string `db:"record_id"`
		UUID      string `db:"uuid"`
		Email     string `db:"email"`
		Phone     string `db:"phone"`
		FirstName string `db:"first_name"`
		LastName  string `db:"last_name"`
		Name      string `db:"name"`
		Attribs   []byte `db:"attribs"`
		Status    string `db:"status"`
	}
	err := c.db.Get(&row, `
SELECT rowid AS id, id AS record_id, uuid, email, COALESCE(phone, '') AS phone, COALESCE(first_name, '') AS first_name,
       COALESCE(last_name, '') AS last_name, COALESCE(name, '') AS name, COALESCE(attribs, '') AS attribs, status
FROM subscribers WHERE id = ?`, recID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Subscriber{}, false, nil
	}
	if err != nil {
		return models.Subscriber{}, false, err
	}
	attribs := models.JSON{}
	if len(row.Attribs) > 0 && string(row.Attribs) != "null" {
		_ = json.Unmarshal(row.Attribs, &attribs)
	}
	return models.Subscriber{
		Base:      models.Base{ID: row.ID, RecordID: row.RecordID},
		UUID:      row.UUID,
		Email:     row.Email,
		Phone:     row.Phone,
		FirstName: row.FirstName,
		LastName:  row.LastName,
		Name:      row.Name,
		Attribs:   attribs,
		Status:    row.Status,
	}, true, nil
}

// ChannelContact is the input to UpsertChannelContact.
type ChannelContact struct {
	Channel string
	Address string
	Name    string
	// Lists holds list UUIDs or record IDs. Unknown ones are ignored.
	Lists []string
	// Consent is "confirmed" to grant consent on Lists, or empty to leave it alone.
	Consent string
	// Source says where the consent came from. It defaults to inbound.
	Source string
}

// UpsertChannelContact finds the subscriber who owns an address, or creates one
// with a placeholder email, then applies list membership and consent.
func (c *Core) UpsertChannelContact(in ChannelContact) (models.Subscriber, error) {
	spec, err := requirePluginChannel(in.Channel)
	if err != nil {
		return models.Subscriber{}, err
	}
	address := strings.TrimSpace(in.Address)
	if address == "" {
		return models.Subscriber{}, apperr.BadRequest("address is required")
	}

	sub, found, err := c.FindSubscriberByChannelAddress(in.Channel, address)
	if err != nil {
		return models.Subscriber{}, err
	}
	if !found {
		newSub := models.Subscriber{Email: models.NewPlaceholderEmail(), Name: in.Name}
		if spec.Address == models.AddressSourcePhone {
			newSub.Phone = address
		}
		// assertOptin is false: this contact has no email to send a confirmation to.
		sub, _, err = c.InsertSubscriber(newSub, nil, nil, true, false)
		if err != nil {
			return models.Subscriber{}, err
		}
	}
	if spec.Address == models.AddressSourceChannel {
		if err := c.SetChannelAddress(sub.RecordID, in.Channel, address); err != nil {
			return models.Subscriber{}, err
		}
	}

	if in.Consent != "" && in.Consent != models.SubscriptionStatusConfirmed {
		return models.Subscriber{}, apperr.BadRequest("consent must be confirmed; use optOut to withdraw it")
	}
	for _, ref := range in.Lists {
		var listID string
		err := c.db.Get(&listID, `SELECT id FROM lists WHERE uuid = ? OR id = ? LIMIT 1`, ref, ref)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return models.Subscriber{}, err
		}
		// Joining through a channel grants nothing on email or SMS, so a new row starts
		// unsubscribed there. An existing row's consent is left alone.
		if _, err := c.db.Exec(`
INSERT OR IGNORE INTO subscriber_lists (subscriber_id, list_id, status, sms_status, updated)
VALUES (?, ?, ?, ?, strftime('%Y-%m-%d %H:%M:%fZ', 'now'))`,
			sub.RecordID, listID, models.SubscriptionStatusUnsubscribed, models.SubscriptionStatusUnsubscribed); err != nil {
			return models.Subscriber{}, err
		}
		if in.Consent != "" {
			source := in.Source
			if source == "" {
				source = ConsentSourceInbound
			}
			if err := c.SetChannelConsent(sub.RecordID, listID, in.Channel, in.Consent, source); err != nil {
				return models.Subscriber{}, err
			}
		}
	}
	return sub, nil
}

// Directions of a channel event.
const (
	ChannelDirectionInbound  = "inbound"
	ChannelDirectionOutbound = "outbound"
)

// ChannelEvent is one inbound or outbound event on a plugin channel.
type ChannelEvent struct {
	Channel       string
	Direction     string
	Kind          string
	Status        string
	ProviderMsgID string
	SubscriberID  string
	CampaignID    string
	Address       string
	Body          string
	Payload       string
}

// ChannelEventExists reports whether a provider message ID is already stored.
func (c *Core) ChannelEventExists(channel, providerMsgID string) (bool, error) {
	if providerMsgID == "" {
		return false, nil
	}
	var out bool
	err := c.db.Get(&out, `SELECT EXISTS (SELECT 1 FROM channel_events WHERE channel = ? AND provider_msg_id = ?)`, channel, providerMsgID)
	return out, err
}

// RecordChannelEvent stores an event and returns false if the provider message ID
// was already recorded, which is how redelivered webhooks are dropped. An inbound
// event proves the address works. An outbound event can find a stub left by a status
// update that beat it here, and then links it and applies any failure it was holding.
func (c *Core) RecordChannelEvent(e ChannelEvent) (bool, error) {
	res, err := c.db.Exec(`
INSERT OR IGNORE INTO channel_events
  (channel, direction, kind, status, provider_msg_id, subscriber, campaign, address, body, payload)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Channel, e.Direction, e.Kind, e.Status, e.ProviderMsgID, e.SubscriberID, e.CampaignID, e.Address, e.Body, e.Payload)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 1 {
		if e.Direction != ChannelDirectionInbound {
			return true, nil
		}
		if c.h != nil && c.h.OnChannelInbound != nil {
			c.h.OnChannelInbound(e)
		}
		if e.Address != "" {
			return true, c.ReactivateChannelAddress(e.Channel, e.Address)
		}
		return true, nil
	}
	if e.Direction == ChannelDirectionOutbound {
		return false, c.linkChannelEventStub(e)
	}
	return false, nil
}

func (c *Core) linkChannelEventStub(e ChannelEvent) error {
	res, err := c.db.Exec(`
UPDATE channel_events
SET subscriber = ?, campaign = ?, address = ?, body = ?, payload = ?
WHERE channel = ? AND provider_msg_id = ? AND subscriber = ''`,
		e.SubscriberID, e.CampaignID, e.Address, e.Body, e.Payload, e.Channel, e.ProviderMsgID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return err
	}
	var pending bool
	if err := c.db.Get(&pending, `SELECT pending_failure FROM channel_events WHERE channel = ? AND provider_msg_id = ?`, e.Channel, e.ProviderMsgID); err != nil {
		return err
	}
	if !pending {
		return nil
	}
	if _, err := c.db.Exec(`UPDATE channel_events SET pending_failure = 0 WHERE channel = ? AND provider_msg_id = ?`, e.Channel, e.ProviderMsgID); err != nil {
		return err
	}
	return c.applyChannelFailure(e)
}

// channelStatusRank orders delivery statuses, because providers don't guarantee the
// order of their updates. Unknown statuses rank lowest.
func channelStatusRank(status string) int {
	switch status {
	case "sent":
		return 1
	case "delivered":
		return 2
	case "read":
		return 3
	case "failed":
		return 4
	}
	return 0
}

// ApplyChannelStatus applies a delivery update to the outbound event for a provider
// message ID. A status that would move backwards is ignored. If the send hasn't been
// recorded yet, it leaves a stub for RecordChannelEvent to complete. A permanent
// failure also records a bounce and retires the address. The bool reports whether
// the event already existed.
func (c *Core) ApplyChannelStatus(channel, providerMsgID, status string, permanent bool) (bool, error) {
	var row struct {
		Subscriber string `db:"subscriber"`
		Campaign   string `db:"campaign"`
		Address    string `db:"address"`
		Status     string `db:"status"`
	}
	err := c.db.Get(&row, `SELECT subscriber, campaign, address, status FROM channel_events WHERE channel = ? AND provider_msg_id = ?`, channel, providerMsgID)
	if errors.Is(err, sql.ErrNoRows) {
		_, err := c.db.Exec(`
INSERT OR IGNORE INTO channel_events (channel, direction, kind, status, provider_msg_id, pending_failure)
VALUES (?, ?, 'send', ?, ?, ?)`, channel, ChannelDirectionOutbound, status, providerMsgID, permanent)
		return false, err
	}
	if err != nil {
		return false, err
	}

	if channelStatusRank(status) >= channelStatusRank(row.Status) {
		if _, err := c.db.Exec(`UPDATE channel_events SET status = ? WHERE channel = ? AND provider_msg_id = ?`, status, channel, providerMsgID); err != nil {
			return true, err
		}
	}
	if !permanent {
		return true, nil
	}
	if row.Subscriber == "" {
		_, err := c.db.Exec(`UPDATE channel_events SET pending_failure = 1 WHERE channel = ? AND provider_msg_id = ?`, channel, providerMsgID)
		return true, err
	}
	return true, c.applyChannelFailure(ChannelEvent{
		Channel: channel, ProviderMsgID: providerMsgID, Status: status,
		SubscriberID: row.Subscriber, CampaignID: row.Campaign, Address: row.Address,
	})
}

func (c *Core) applyChannelFailure(e ChannelEvent) error {
	if err := c.RecordChannelBounce(e.Channel, e.SubscriberID, e.CampaignID, models.BounceTypeHard, e.Status); err != nil {
		return err
	}
	return c.MarkChannelAddressInvalid(e.SubscriberID, e.Channel, e.Address)
}

// RecordChannelBounce records a bounce for a channel send. The ch: source keeps the
// email bounce actions from running. Bounce rows need a campaign, so a failed direct
// or transactional send isn't stored as one.
func (c *Core) RecordChannelBounce(channel, subRecID, campaignRef, bounceType, detail string) error {
	sub, ok, err := c.subscriberForChannel(subRecID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("unknown subscriber %q", subRecID)
	}
	if bounceType == "" {
		bounceType = models.BounceTypeHard
	}
	meta, err := json.Marshal(map[string]string{"channel": channel, "detail": detail})
	if err != nil {
		return err
	}
	return c.RecordBounce(models.Bounce{
		Type:           bounceType,
		Source:         channel,
		Email:          sub.Email,
		SubscriberUUID: sub.UUID,
		CampaignUUID:   campaignRef,
		Meta:           meta,
		CreatedAt:      time.Now(),
	})
}

// ChannelEventRow is a stored channel event, for threads and API responses.
type ChannelEventRow struct {
	ID            int64  `db:"id" json:"id"`
	Channel       string `db:"channel" json:"channel"`
	Direction     string `db:"direction" json:"direction"`
	Kind          string `db:"kind" json:"kind"`
	Status        string `db:"status" json:"status"`
	ProviderMsgID string `db:"provider_msg_id" json:"provider_msg_id"`
	Address       string `db:"address" json:"address"`
	Body          string `db:"body" json:"body"`
	OccurredAt    string `db:"occurred_at" json:"occurred_at"`
}

// ListChannelEvents returns a subscriber's latest events on a channel, newest first.
func (c *Core) ListChannelEvents(subRecID, channel string, limit int) ([]ChannelEventRow, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	out := []ChannelEventRow{}
	err := c.db.Select(&out, `
SELECT id, channel, direction, kind, status, provider_msg_id, address, body, occurred_at
FROM channel_events WHERE subscriber = ? AND channel = ?
ORDER BY datetime(occurred_at) DESC, id DESC LIMIT ?`, subRecID, channel, limit)
	return out, err
}

// CountChannelEvents returns a campaign's outbound events by delivery status.
func (c *Core) CountChannelEvents(campaignRecID string) (map[string]int, error) {
	var rows []struct {
		Status string `db:"status"`
		Count  int    `db:"n"`
	}
	if err := c.db.Select(&rows, `
SELECT status, COUNT(*) AS n FROM channel_events
WHERE campaign = ? AND direction = ? GROUP BY status`, campaignRecID, ChannelDirectionOutbound); err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, row := range rows {
		out[row.Status] = row.Count
	}
	return out, nil
}

// LastChannelInbound returns when a subscriber last messaged on a channel. A false
// result means they never have.
func (c *Core) LastChannelInbound(subRecID, channel string) (time.Time, bool, error) {
	var at string
	err := c.db.Get(&at, `
SELECT occurred_at FROM channel_events WHERE subscriber = ? AND channel = ? AND direction = ?
ORDER BY datetime(occurred_at) DESC, id DESC LIMIT 1`, subRecID, channel, ChannelDirectionInbound)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	t, err := time.Parse("2006-01-02 15:04:05.000Z", strings.Replace(at, "T", " ", 1))
	if err != nil {
		return time.Time{}, false, fmt.Errorf("channel event time %q: %w", at, err)
	}
	return t, true, nil
}

// SMSConsentCounts reports what ImportSMSConsent would add or did add. Explicit rows
// have their own SMS status. Inherited rows have none and fall back to email status,
// so they're really email consent.
type SMSConsentCounts struct {
	Explicit  int   `json:"explicit"`
	Inherited int   `json:"inherited"`
	Applied   int64 `json:"applied"`
}

const smsConsentCandidatesSQL = `
FROM subscriber_lists sl
JOIN subscribers s ON s.id = sl.subscriber_id
JOIN lists l ON l.id = sl.list_id
WHERE trim(COALESCE(s.phone, '')) != ''
  AND s.status != 'blocklisted'
  AND NOT EXISTS (
    SELECT 1 FROM subscriber_channel_consent cc
    WHERE cc.subscriber = sl.subscriber_id AND cc.channel = ? AND cc.list IN (sl.list_id, '*')
  )
  AND (
    (l.optin = 'double' AND COALESCE(NULLIF(trim(sl.sms_status), ''), sl.status) = 'confirmed')
    OR (l.optin != 'double' AND COALESCE(NULLIF(trim(sl.sms_status), ''), sl.status) != 'unsubscribed')
  )`

// ImportSMSConsent grants channel consent to subscribers the SMS pipeline could
// send to. It never overwrites a consent row, so earlier opt-outs stand. With dryRun
// it only counts, and inherited rows are only applied when asked for.
func (c *Core) ImportSMSConsent(channel string, dryRun, includeInherited bool) (SMSConsentCounts, error) {
	var out SMSConsentCounts
	if _, err := requirePluginChannel(channel); err != nil {
		return out, err
	}
	const explicit = ` AND trim(COALESCE(sl.sms_status, '')) != ''`
	const inherited = ` AND trim(COALESCE(sl.sms_status, '')) = ''`
	if err := c.db.Get(&out.Explicit, `SELECT COUNT(*) `+smsConsentCandidatesSQL+explicit, channel); err != nil {
		return out, err
	}
	if err := c.db.Get(&out.Inherited, `SELECT COUNT(*) `+smsConsentCandidatesSQL+inherited, channel); err != nil {
		return out, err
	}
	if dryRun {
		return out, nil
	}
	filter := explicit
	if includeInherited {
		filter = ""
	}
	res, err := c.db.Exec(`
INSERT OR IGNORE INTO subscriber_channel_consent (subscriber, list, channel, status, source)
SELECT sl.subscriber_id, sl.list_id, ?, ?, ? `+smsConsentCandidatesSQL+filter,
		channel, models.SubscriptionStatusConfirmed, ConsentSourceSMSCopy, channel)
	if err != nil {
		return out, err
	}
	out.Applied, err = res.RowsAffected()
	return out, err
}

// CountCampaignsOnChannels counts running and scheduled campaigns using any of the
// channels. Turning those channels off would leave these campaigns unable to send.
func (c *Core) CountCampaignsOnChannels(channels []string) (int, error) {
	if len(channels) == 0 {
		return 0, nil
	}
	args := make([]any, 0, len(channels)+2)
	for _, ch := range channels {
		args = append(args, ch)
	}
	args = append(args, models.CampaignStatusRunning, models.CampaignStatusScheduled)
	var n int
	err := c.db.Get(&n, `SELECT COUNT(*) FROM campaigns WHERE trim(COALESCE(messenger, '')) IN (`+sqlitePlaceholders(len(channels))+`) AND status IN (?, ?)`, args...)
	return n, err
}

// ChannelRecipients returns where to send a direct message, or nothing if the
// subscriber hasn't consented on any list or has no address on the channel.
func (c *Core) ChannelRecipients(sub models.Subscriber, channel string) ([]string, error) {
	if sub.Status == models.SubscriberStatusBlockListed {
		return nil, nil
	}
	if optedOut, err := c.ChannelHasOptOut(sub.RecordID, channel); err != nil || optedOut {
		return nil, err
	}
	var consented bool
	if err := c.db.Get(&consented, `
SELECT EXISTS (
  SELECT 1 FROM subscriber_channel_consent
  WHERE subscriber = ? AND channel = ? AND status = ?
)`, sub.RecordID, channel, models.SubscriptionStatusConfirmed); err != nil {
		return nil, err
	}
	if !consented {
		return nil, nil
	}
	if models.ChannelFor(channel).Address == models.AddressSourcePhone {
		phone := strings.TrimSpace(sub.Phone)
		if phone == "" {
			return nil, nil
		}
		var invalid bool
		if err := c.db.Get(&invalid, `
SELECT EXISTS (SELECT 1 FROM subscriber_addresses WHERE channel = ? AND address = ? AND status = ?)`,
			channel, phone, ChannelAddressInvalid); err != nil || invalid {
			return nil, err
		}
		return []string{phone}, nil
	}
	addresses, err := c.GetChannelAddresses(channel, []string{sub.RecordID})
	if err != nil {
		return nil, err
	}
	return addresses[sub.RecordID], nil
}
