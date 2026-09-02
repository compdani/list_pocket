package models

import (
	null "gopkg.in/volatiletech/null.v6"
)

const (
	ListTypePrivate    = "private"
	ListTypePublic     = "public"
	ListOptinSingle    = "single"
	ListOptinDouble    = "double"
	ListStatusActive   = "active"
	ListStatusArchived = "archived"
)

// List represents a mailing list.
type List struct {
	Base

	UUID             string       `db:"uuid" json:"uuid"`
	Name             string       `db:"name" json:"name"`
	Type             string       `db:"type" json:"type"`
	Optin            string       `db:"optin" json:"optin"`
	Status           string       `db:"status" json:"status"`
	Tags             []string     `db:"tags" json:"tags"`
	Description      string       `db:"description" json:"description"`
	SubscriberCount  int          `db:"subscriber_count" json:"subscriber_count"`
	SubscriberCounts StringIntMap `db:"subscriber_statuses" json:"subscriber_statuses"`
	SubscriberID     int          `db:"subscriber_id" json:"-"`

	// This is only relevant when querying the lists of a subscriber.
	SubscriptionStatus string `db:"subscription_status" json:"subscription_status,omitempty"`
	// SMS marketing consent for this list membership (mirrors subscription_status when sms_status is unset).
	SubscriptionSmsStatus string    `db:"subscription_sms_status" json:"subscription_sms_status,omitempty"`
	SubscriptionCreatedAt null.Time `db:"subscription_created_at" json:"subscription_created_at,omitempty"`
	SubscriptionUpdatedAt null.Time `db:"subscription_updated_at" json:"subscription_updated_at,omitempty"`

	// Pseudofield for getting the total number of subscribers
	// in searches and queries.
	Total int `db:"total" json:"-"`
}
