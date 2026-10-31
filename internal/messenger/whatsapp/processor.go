package whatsapp

import (
	"fmt"
	"strings"

	"github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/models"
)

// Processor applies webhook events. Providers redeliver, so it's safe to run the same
// event twice, and a failure partway through can simply be retried.
type Processor struct {
	Core *core.Core
	// OnConnection stores a change to the phone link. It may be nil.
	OnConnection func(connected bool, note string) error
}

// Apply handles events in order, stopping at the first failure so the webhook can
// return an error and the provider retries the delivery.
func (p *Processor) Apply(events []Event) error {
	for _, e := range events {
		var err error
		switch e.Type {
		case EventMessage:
			err = p.applyMessage(e)
		case EventStatus:
			err = p.applyStatus(e)
		case EventPreference:
			err = p.applyPreference(e)
		case EventConnection:
			if p.OnConnection != nil {
				err = p.OnConnection(e.Connected, e.Note)
			}
		}
		if err != nil {
			return fmt.Errorf("whatsapp %s event: %w", e.Type, err)
		}
	}
	return nil
}

// applyMessage records a message and acts on STOP and JOIN. Side effects run before
// the event is stored, so a failure between them is retried rather than lost.
func (p *Processor) applyMessage(e Event) error {
	if e.From == "" {
		return nil
	}
	seen, err := p.Core.ChannelEventExists(MessengerName, e.MessageID)
	if err != nil || seen {
		return err
	}

	sub, found, err := p.Core.FindSubscriberByChannelAddress(MessengerName, e.From)
	if err != nil {
		return err
	}
	if !found {
		// They can be replied to, but get no campaigns without consent.
		sub, err = p.Core.UpsertChannelContact(core.ChannelContact{
			Channel: MessengerName, Address: "+" + e.From, Name: e.Name,
		})
		if err != nil {
			return err
		}
	}

	kind := "message"
	command, arg := parseCommand(e.Text)
	switch command {
	case commandStop:
		kind = "opt_out"
		if _, err := p.Core.OptOutChannelAddress(MessengerName, e.From); err != nil {
			return err
		}
	case commandJoin:
		kind = "opt_in"
		// The address is how the existing contact is found. Phone-sourced channels
		// match it against the phone column.
		if _, err := p.Core.UpsertChannelContact(core.ChannelContact{
			Channel: MessengerName, Address: "+" + e.From, Lists: []string{arg},
			Consent: models.SubscriptionStatusConfirmed, Source: core.ConsentSourceInbound,
		}); err != nil {
			return err
		}
	}

	address := strings.TrimSpace(sub.Phone)
	if address == "" {
		address = "+" + e.From
	}
	_, err = p.Core.RecordChannelEvent(core.ChannelEvent{
		Channel: MessengerName, Direction: core.ChannelDirectionInbound, Kind: kind,
		ProviderMsgID: e.MessageID, SubscriberID: sub.RecordID, Address: address, Body: e.InboundText(),
	})
	return err
}

func (p *Processor) applyStatus(e Event) error {
	if e.MessageID == "" || e.Status == "" {
		return nil
	}
	action := ActionError
	if e.Status == "failed" {
		action = Classify(e.ErrorCode)
	}
	if _, err := p.Core.ApplyChannelStatus(MessengerName, e.MessageID, e.Status, action == ActionRetire); err != nil {
		return err
	}
	if action == ActionOptOut && e.RecipientID != "" {
		_, err := p.Core.OptOutChannelAddress(MessengerName, e.RecipientID)
		return err
	}
	return nil
}

// applyPreference honours a user stopping marketing messages. Resuming doesn't restore
// consent: that takes a new opt-in, so an old list can't quietly start up again.
func (p *Processor) applyPreference(e Event) error {
	if e.Preference != "stop" {
		return nil
	}
	_, err := p.Core.OptOutChannelAddress(MessengerName, e.From)
	return err
}

const (
	commandNone = iota
	commandStop
	commandJoin
)

// parseCommand recognises a message that's only a keyword, so a sentence that happens
// to contain "stop" isn't an opt-out.
func parseCommand(text string) (int, string) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return commandNone, ""
	}
	switch word := strings.ToUpper(fields[0]); {
	case len(fields) == 1 && (word == "STOP" || word == "UNSUBSCRIBE"):
		return commandStop, ""
	case len(fields) == 2 && word == "JOIN":
		return commandJoin, fields[1]
	}
	return commandNone, ""
}
