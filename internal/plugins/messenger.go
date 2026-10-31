package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/compdani/list_pocket/models"
	"github.com/dop251/goja"
)

// CampaignFieldsAttrib is the campaign attribs key holding a channel's campaignFields.
const CampaignFieldsAttrib = "channel_fields"

// Messenger sends a plugin channel's messages through the plugin's send handler.
// It satisfies manager.Messenger.
type Messenger struct {
	mgr      *Manager
	pluginID string
	channel  string
}

// Name returns the channel ID, which is the campaign's messenger value.
func (m *Messenger) Name() string { return m.channel }

// Flush does nothing: a send finishes before its handler returns.
func (m *Messenger) Flush() error { return nil }

// Close does nothing: the plugin's VMs have no connections to close.
func (m *Messenger) Close() error { return nil }

// Push calls the plugin. Rejected addresses are retired here, so the manager only
// has to skip the ledger row when no address is left to try.
func (m *Messenger) Push(msg models.Message) error {
	p := m.mgr.Loaded(m.pluginID)
	if p == nil {
		return fmt.Errorf("plugin %s is not running", m.pluginID)
	}
	to := cleanAddresses(msg.To)
	if len(to) == 0 {
		return models.ErrUnsendableDestination
	}

	res, err := p.call(context.Background(), func(v *vm) (goja.Value, error) {
		handler := v.channels[m.channel]
		send, _ := goja.AssertFunction(handler.Get("send"))
		return send(handler, v.rt.ToValue(messageView(msg, to)))
	})
	if err != nil {
		if errors.Is(err, models.ErrUnsendableDestination) {
			m.retire(msg, to)
			return models.ErrUnsendableDestination
		}
		return fmt.Errorf("plugin %s: %s", m.pluginID, p.redact(err.Error()))
	}

	out, _ := res.(map[string]any)
	invalid := stringList(out["invalidAddresses"])
	if len(invalid) > 0 {
		m.retire(msg, invalid)
		remaining := slices.DeleteFunc(slices.Clone(to), func(a string) bool { return slices.Contains(invalid, a) })
		if len(remaining) == 0 {
			return models.ErrUnsendableDestination
		}
	}

	if id, _ := out["providerMsgId"].(string); id != "" {
		if _, err := m.mgr.host.RecordEvent(EventInput{
			Channel:       m.channel,
			Direction:     "outbound",
			Kind:          "send",
			Status:        "sent",
			ProviderMsgID: id,
			SubscriberID:  msg.Subscriber.RecordID,
			CampaignID:    campaignRecordID(msg),
			Address:       to[0],
		}); err != nil {
			m.mgr.log.Printf("plugin %s: recording send: %v", m.pluginID, err)
		}
	}
	return nil
}

func (m *Messenger) retire(msg models.Message, addresses []string) {
	for _, address := range addresses {
		if err := m.mgr.host.MarkAddressInvalid(msg.Subscriber.RecordID, m.channel, address); err != nil {
			m.mgr.log.Printf("plugin %s: retiring address: %v", m.pluginID, err)
		}
	}
}

func campaignRecordID(msg models.Message) string {
	if msg.Campaign == nil {
		return ""
	}
	return msg.Campaign.RecordID
}

// messageView is what a send handler receives. It uses only maps, slices and
// primitives, because a Go struct would expose its methods to JS.
func messageView(msg models.Message, to []string) map[string]any {
	addresses := make([]any, 0, len(to))
	for _, a := range to {
		addresses = append(addresses, a)
	}
	view := map[string]any{
		"to":          addresses,
		"from":        msg.From,
		"subject":     msg.Subject,
		"body":        string(msg.Body),
		"contentType": msg.ContentType,
		"subscriber":  contactView(msg.Subscriber),
		"fields":      map[string]any{},
	}
	if msg.TxMessage != nil && len(msg.TxMessage.Data) > 0 {
		view["fields"] = map[string]any(msg.TxMessage.Data)
	}
	if msg.Campaign != nil {
		view["campaign"] = map[string]any{"id": msg.Campaign.RecordID, "name": msg.Campaign.Name}
		if fields, ok := msg.Campaign.Attribs[CampaignFieldsAttrib].(map[string]any); ok {
			view["fields"] = fields
		}
	}
	return view
}

// contactView hides placeholder emails, which are only a storage detail.
func contactView(sub models.Subscriber) map[string]any {
	email := sub.Email
	if models.IsPlaceholderEmail(email) {
		email = ""
	}
	return map[string]any{
		"id":         sub.RecordID,
		"uuid":       sub.UUID,
		"email":      email,
		"phone":      sub.Phone,
		"name":       sub.Name,
		"first_name": sub.FirstName,
		"last_name":  sub.LastName,
		"status":     sub.Status,
		"attribs":    map[string]any(sub.Attribs),
	}
}

func cleanAddresses(in []string) []string {
	out := make([]string, 0, len(in))
	for _, a := range in {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}

func stringList(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		out = append(out, fmt.Sprint(item))
	}
	return out
}

// Messengers returns a messenger and a channel spec for every channel of every
// running plugin. Call it after Boot and before the campaign manager starts.
func (m *Manager) Messengers() ([]*Messenger, []models.ChannelSpec) {
	var (
		msgrs []*Messenger
		specs []models.ChannelSpec
	)
	for _, p := range m.LoadedPlugins() {
		for _, c := range p.Manifest.Channels {
			msgrs = append(msgrs, &Messenger{mgr: m, pluginID: p.Manifest.ID, channel: c.ID})
			specs = append(specs, channelSpec(c))
		}
	}
	return msgrs, specs
}

func channelSpec(c ChannelManifest) models.ChannelSpec {
	spec := models.ChannelSpec{
		ID:           c.ID,
		Label:        c.Label,
		Icon:         c.Icon,
		AddressLabel: c.AddressLabel,
		Address:      models.AddressSource(c.AddressSource),
		Consent:      models.ConsentSourceChannel,
		TextPipeline: true,
		TemplateType: models.TemplateTypeCampaignSMS,
		ContentTypes: c.ContentTypes,
		Unsendable:   models.UnsendableSkip,
		CampaignBody: c.CampaignBody,
	}
	if c.RateLimit != nil {
		spec.MaxPerSecond = c.RateLimit.MaxPerSecond
	}
	if len(c.CampaignFields) > 0 {
		if raw, err := json.Marshal(c.CampaignFields); err == nil {
			spec.CampaignFields = raw
		}
	}
	return spec
}
