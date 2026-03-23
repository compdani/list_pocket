package core

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/compdani/list_pocket/models"
)

const privacyOpenWindow = 2 * time.Minute

var privacyOpenUserAgentRe = regexp.MustCompile(`(?i)(applewebkit|apple mail|iphone|ipad|mac os x)`)

func sqliteTimestampValue(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05.000Z")
}

func sqliteUniqueCampaignViewsKey(alias string) string {
	return "COALESCE(CAST(" + alias + ".subscriber_id AS TEXT), 'anon:' || " + alias + ".rowid)"
}

func sqliteUniqueCampaignViewsExpr(alias string, filter string) string {
	key := sqliteUniqueCampaignViewsKey(alias)
	if strings.TrimSpace(filter) == "" {
		return "COUNT(DISTINCT " + key + ")"
	}

	return "COUNT(DISTINCT CASE WHEN " + filter + " THEN " + key + " END)"
}

func normalizeOpenEvent(event models.OpenEvent) models.OpenEvent {
	event.IPAddress = strings.TrimSpace(event.IPAddress)
	event.UserAgent = strings.TrimSpace(event.UserAgent)
	if len(event.IPAddress) > 255 {
		event.IPAddress = event.IPAddress[:255]
	}
	if len(event.UserAgent) > 1024 {
		event.UserAgent = event.UserAgent[:1024]
	}
	if event.OpenedAt.IsZero() {
		event.OpenedAt = time.Now().UTC()
	} else {
		event.OpenedAt = event.OpenedAt.UTC()
	}
	return event
}

func classifyPrivacyOpen(event models.OpenEvent, referenceAt time.Time, referenceType string) (bool, string, error) {
	event = normalizeOpenEvent(event)
	meta := models.JSON{
		"opened_at":  event.OpenedAt.Format(time.RFC3339Nano),
		"ip_address": event.IPAddress,
		"user_agent": event.UserAgent,
	}

	looksLikeAppleUA := privacyOpenUserAgentRe.MatchString(event.UserAgent)
	meta["looks_like_apple_mail"] = looksLikeAppleUA

	veryFastOpen := false
	if !referenceAt.IsZero() {
		referenceAt = referenceAt.UTC()
		meta["reference_at"] = referenceAt.Format(time.RFC3339Nano)
		meta["reference_type"] = referenceType

		delta := event.OpenedAt.Sub(referenceAt)
		if delta >= 0 {
			meta["seconds_after_reference"] = int(delta.Seconds())
			veryFastOpen = delta <= privacyOpenWindow
		}
	}
	meta["opened_very_quickly"] = veryFastOpen

	suspected := looksLikeAppleUA && veryFastOpen
	if suspected {
		meta["suspected_reason"] = "apple_mail_fast_open"
	}

	b, err := json.Marshal(meta)
	if err != nil {
		return suspected, "", err
	}
	return suspected, string(b), nil
}
