package models

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/jmoiron/sqlx/types"
	"github.com/lib/pq"
	null "gopkg.in/volatiletech/null.v6"
)

const (
	SubscriberStatusEnabled     = "enabled"
	SubscriberStatusDisabled    = "disabled"
	SubscriberStatusBlockListed = "blocklisted"

	SubscriptionStatusUnconfirmed  = "unconfirmed"
	SubscriptionStatusConfirmed    = "confirmed"
	SubscriptionStatusUnsubscribed = "unsubscribed"
)

// Subscribers represents a slice of Subscriber.
type Subscribers []Subscriber

// Subscriber represents an e-mail subscriber.
type Subscriber struct {
	Base

	UUID      string         `db:"uuid" json:"uuid"`
	Email     string         `db:"email" json:"email" form:"email"`
	Phone     string         `db:"phone" json:"phone" form:"phone"`
	FirstName string         `db:"first_name" json:"first_name" form:"first_name"`
	LastName  string         `db:"last_name" json:"last_name" form:"last_name"`
	Name      string         `db:"name" json:"name" form:"name"`
	Attribs   JSON           `db:"attribs" json:"attribs"`
	Status    string         `db:"status" json:"status"`
	Lists     types.JSONText `db:"lists" json:"lists"`
}

type subLists struct {
	SubscriberID int            `db:"subscriber_id"`
	Lists        types.JSONText `db:"lists"`
}

// GetIDs returns the list of subscriber IDs.
func (subs Subscribers) GetIDs() []int {
	IDs := make([]int, len(subs))
	for i, c := range subs {
		IDs[i] = c.ID
	}

	return IDs
}

// LoadLists lazy loads the lists for all the subscribers
// in the Subscribers slice and attaches them to their []Lists property.
func (subs Subscribers) LoadLists(stmt *pbdb.Query) error {
	var sl []subLists
	err := stmt.Select(&sl, pq.Array(subs.GetIDs()))
	if err != nil {
		return err
	}

	if len(subs) != len(sl) {
		return errors.New("campaign stats count does not match")
	}

	for i, s := range sl {
		if s.SubscriberID == subs[i].ID {
			subs[i].Lists = s.Lists
		}
	}

	return nil
}

// JoinSubscriberName returns the display name built from the individual fields.
func JoinSubscriberName(firstName, lastName string) string {
	return strings.TrimSpace(strings.TrimSpace(firstName) + " " + strings.TrimSpace(lastName))
}

// SplitSubscriberName splits a full name into first and last parts.
func SplitSubscriberName(name string) (string, string) {
	parts := strings.Fields(strings.TrimSpace(name))
	switch len(parts) {
	case 0:
		return "", ""
	case 1:
		return parts[0], ""
	default:
		return parts[0], strings.Join(parts[1:], " ")
	}
}

// NormalizeName keeps the split fields and the full display name in sync.
func (s *Subscriber) NormalizeName() {
	s.FirstName = strings.TrimSpace(s.FirstName)
	s.LastName = strings.TrimSpace(s.LastName)
	s.Name = strings.TrimSpace(s.Name)

	if s.FirstName == "" && s.LastName == "" && s.Name != "" {
		s.FirstName, s.LastName = SplitSubscriberName(s.Name)
	}

	s.Name = JoinSubscriberName(s.FirstName, s.LastName)
}

// Subscription represents a list attached to a subscriber.
type Subscription struct {
	List
	SubscriptionStatus    null.String     `db:"subscription_status" json:"subscription_status"`
	SubscriptionCreatedAt null.String     `db:"subscription_created_at" json:"subscription_created_at"`
	Meta                  json.RawMessage `db:"meta" json:"meta"`
}

// SubscriberExport represents a subscriber record that is exported to raw data.
type SubscriberExport struct {
	Base

	UUID      string `db:"uuid" json:"uuid"`
	Email     string `db:"email" json:"email"`
	Phone     string `db:"phone" json:"phone"`
	FirstName string `db:"first_name" json:"first_name"`
	LastName  string `db:"last_name" json:"last_name"`
	Name      string `db:"name" json:"name"`
	Attribs   string `db:"attribs" json:"attribs"`
	Status    string `db:"status" json:"status"`
}

// SubscriberExportProfile represents a subscriber's collated data in JSON for export.
type SubscriberExportProfile struct {
	Email         string          `db:"email" json:"-"`
	Profile       json.RawMessage `db:"profile" json:"profile,omitempty"`
	Subscriptions json.RawMessage `db:"subscriptions" json:"subscriptions,omitempty"`
	CampaignViews json.RawMessage `db:"campaign_views" json:"campaign_views,omitempty"`
	LinkClicks    json.RawMessage `db:"link_clicks" json:"link_clicks,omitempty"`
}

// SubscriberActivity represents a subscriber's campaign views and link clicks for the Activity tab.
type SubscriberActivity struct {
	CampaignViews json.RawMessage `db:"campaign_views" json:"campaign_views"`
	LinkClicks    json.RawMessage `db:"link_clicks" json:"link_clicks"`
}
