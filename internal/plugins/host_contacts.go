package plugins

import (
	"errors"
	"fmt"

	"github.com/dop251/goja"
)

// ContactInput is the input to host.contacts.upsertByAddress.
type ContactInput struct {
	Channel string
	Address string
	Name    string
	Lists   []string
	Consent string
	Source  string
}

// EventInput is the input to host.events.record.
type EventInput struct {
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

// BounceInput is the input to host.events.bounce.
type BounceInput struct {
	Channel      string
	SubscriberID string
	CampaignID   string
	Type         string
	Meta         string
}

// SendInput is the input to host.messages.send.
type SendInput struct {
	Channel      string
	SubscriberID string
	Subject      string
	Body         string
}

// requireOwnChannel stops one plugin from reading or changing another's data.
func (v *vm) requireOwnChannel(channel string) {
	if !v.declaresChannel(channel) {
		v.throw(fmt.Errorf("channel %q isn't declared by plugin %s", channel, v.plugin.Manifest.ID))
	}
}

func (v *vm) hostContacts() *goja.Object {
	obj := v.rt.NewObject()
	host := v.plugin.mgr.host

	_ = obj.Set("findByAddress", func(channel, address string) goja.Value {
		v.requireOwnChannel(channel)
		contact, ok, err := host.FindContact(channel, address)
		if err != nil {
			v.throw(err)
		}
		if !ok {
			return goja.Null()
		}
		return v.rt.ToValue(contact)
	})
	_ = obj.Set("get", func(id string) goja.Value {
		contact, ok, err := host.GetContact(id)
		if err != nil {
			v.throw(err)
		}
		if !ok {
			return goja.Null()
		}
		return v.rt.ToValue(contact)
	})
	_ = obj.Set("upsertByAddress", func(channel, address string, opts map[string]any) map[string]any {
		v.requireOwnChannel(channel)
		name, _ := opts["name"].(string)
		consent, _ := opts["consent"].(string)
		contact, err := host.UpsertContact(ContactInput{
			Channel: channel,
			Address: address,
			Name:    name,
			Lists:   stringList(opts["lists"]),
			Consent: consent,
			Source:  stringField(opts, "source"),
		})
		if err != nil {
			v.throw(err)
		}
		return contact
	})
	_ = obj.Set("setConsent", func(id, listID, channel, status string, opts map[string]any) {
		v.requireOwnChannel(channel)
		source := stringField(opts, "source")
		if source == "" {
			source = "plugin"
		}
		if err := host.SetConsent(id, listID, channel, status, source); err != nil {
			v.throw(err)
		}
	})
	_ = obj.Set("importSMSConsent", func(channel string, opts map[string]any) map[string]any {
		v.requireOwnChannel(channel)
		dryRun, _ := opts["dryRun"].(bool)
		inherited, _ := opts["includeInherited"].(bool)
		out, err := host.ImportSMSConsent(channel, dryRun, inherited)
		if err != nil {
			v.throw(err)
		}
		return out
	})
	return obj
}

func (v *vm) hostEvents() *goja.Object {
	obj := v.rt.NewObject()
	host := v.plugin.mgr.host

	// record returns false when the provider message ID was already recorded.
	_ = obj.Set("record", func(e map[string]any) bool {
		channel := stringField(e, "channel")
		v.requireOwnChannel(channel)
		first, err := host.RecordEvent(EventInput{
			Channel:       channel,
			Direction:     stringField(e, "direction"),
			Kind:          stringField(e, "kind"),
			Status:        stringField(e, "status"),
			ProviderMsgID: stringField(e, "providerMsgId"),
			SubscriberID:  stringField(e, "subscriberId"),
			CampaignID:    stringField(e, "campaignId"),
			Address:       stringField(e, "address"),
			Body:          stringField(e, "body"),
			Payload:       stringField(e, "payload"),
		})
		if err != nil {
			v.throw(err)
		}
		return first
	})
	_ = obj.Set("status", func(channel, providerMsgID, status string, opts map[string]any) bool {
		v.requireOwnChannel(channel)
		permanent, _ := opts["permanent"].(bool)
		found, err := host.UpdateEventStatus(channel, providerMsgID, status, permanent)
		if err != nil {
			v.throw(err)
		}
		return found
	})
	_ = obj.Set("list", func(q map[string]any) []map[string]any {
		channel := stringField(q, "channel")
		v.requireOwnChannel(channel)
		out, err := host.ListEvents(stringField(q, "subscriberId"), channel, int(intOpt(q, "limit")))
		if err != nil {
			v.throw(err)
		}
		return out
	})
	_ = obj.Set("counts", func(campaignID string) map[string]int {
		out, err := host.CountEvents(campaignID)
		if err != nil {
			v.throw(err)
		}
		return out
	})
	_ = obj.Set("bounce", func(b map[string]any) {
		channel := stringField(b, "channel")
		v.requireOwnChannel(channel)
		if err := host.RecordBounce(BounceInput{
			Channel:      channel,
			SubscriberID: stringField(b, "subscriberId"),
			CampaignID:   stringField(b, "campaignId"),
			Type:         stringField(b, "type"),
			Meta:         stringField(b, "meta"),
		}); err != nil {
			v.throw(err)
		}
	})
	_ = obj.Set("optOut", func(channel, address string) int64 {
		v.requireOwnChannel(channel)
		n, err := host.OptOut(channel, address)
		if err != nil {
			v.throw(err)
		}
		return n
	})
	return obj
}

func (v *vm) hostMessages() *goja.Object {
	obj := v.rt.NewObject()
	_ = obj.Set("send", func(m map[string]any) {
		channel := stringField(m, "channel")
		v.requireOwnChannel(channel)
		err := v.plugin.mgr.host.SendMessage(SendInput{
			Channel:      channel,
			SubscriberID: stringField(m, "subscriberId"),
			Subject:      stringField(m, "subject"),
			Body:         stringField(m, "body"),
		})
		if err != nil {
			v.throw(err)
		}
	})
	return obj
}

func stringField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

// ErrNoConsent means a message can't be sent because the contact hasn't agreed to
// that channel, or has no address on it.
var ErrNoConsent = errors.New("contact has no consent or address on this channel")
