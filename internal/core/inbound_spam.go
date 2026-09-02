package core

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/apperr"

	"github.com/compdani/list_pocket/models"
	pbcore "github.com/pocketbase/pocketbase/core"
)

func spamLevelOrder(level string) int {
	switch level {
	case "confirmed_spam":
		return 3
	case "spam":
		return 2
	case "suspected":
		return 1
	}
	return 0
}

// maxSpamLevel returns the higher of two spam level strings.
func maxSpamLevel(a, b string) string {
	if spamLevelOrder(a) >= spamLevelOrder(b) {
		return a
	}
	return b
}

// spamKeywords is a minimal English stop-word set used to filter low-value keywords.
var spamStopWords = map[string]struct{}{
	"that": {}, "this": {}, "with": {}, "from": {}, "have": {}, "will": {},
	"your": {}, "been": {}, "they": {}, "were": {}, "said": {}, "each": {},
	"which": {}, "their": {}, "there": {}, "when": {}, "what": {}, "some": {},
	"about": {}, "would": {}, "these": {}, "other": {}, "into": {}, "than": {},
	"then": {}, "more": {}, "also": {}, "click": {}, "here": {}, "http": {},
	"https": {}, "email": {}, "message": {}, "reply": {},
}

// extractSpamKeywords extracts up to maxN significant words from text (subject + body).
func extractSpamKeywords(subject, bodyText string, maxN int) []string {
	text := strings.ToLower(subject + " " + bodyText)
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	seen := map[string]int{}
	for _, w := range words {
		if len(w) < 4 {
			continue
		}
		if _, stop := spamStopWords[w]; stop {
			continue
		}
		seen[w]++
	}
	// Sort by frequency descending.
	type wf struct {
		word  string
		count int
	}
	ranked := make([]wf, 0, len(seen))
	for w, c := range seen {
		ranked = append(ranked, wf{w, c})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].count > ranked[j].count
	})
	out := make([]string, 0, maxN)
	for i, w := range ranked {
		if i >= maxN {
			break
		}
		out = append(out, w.word)
	}
	return out
}

// extractDomain returns the domain part of an email address.
func extractDomain(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(email[at+1:]))
}

// CheckInboundSpamRules evaluates active spam rules against the incoming email fields.
// It checks sender address, sender domain, then keyword scoring.
// Returns the highest implied spam level and composite score, or ("", 0, nil) if no match.
func (c *Core) CheckInboundSpamRules(ctx context.Context, fromAddress, subject, bodyText string) (string, float64, error) {
	_ = ctx
	fromAddress = strings.ToLower(strings.TrimSpace(fromAddress))
	if fromAddress == "" {
		return "", 0, nil
	}
	domain := extractDomain(fromAddress)

	type ruleRow struct {
		Type      string  `db:"type"`
		Value     string  `db:"value"`
		Weight    float64 `db:"weight"`
		SpamLevel string  `db:"spam_level"`
	}
	rules := []ruleRow{}
	if err := c.db.Select(&rules, `
		SELECT type, value, COALESCE(weight, 1.0) AS weight, COALESCE(spam_level, 'suspected') AS spam_level
		FROM inbound_spam_rules
		WHERE is_active = 1
		  AND spam_level != ''
		ORDER BY rowid ASC
	`); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", 0, err
	}

	bestLevel := ""
	var keywordRules []ruleRow
	for _, r := range rules {
		switch r.Type {
		case "sender":
			if strings.EqualFold(r.Value, fromAddress) {
				bestLevel = maxSpamLevel(bestLevel, r.SpamLevel)
			}
		case "domain":
			if domain != "" && strings.EqualFold(r.Value, domain) {
				bestLevel = maxSpamLevel(bestLevel, r.SpamLevel)
			}
		case "keyword":
			keywordRules = append(keywordRules, r)
		}
	}

	// If sender/domain rule already determined spam or confirmed_spam, return early.
	if spamLevelOrder(bestLevel) >= spamLevelOrder("spam") {
		return bestLevel, 1.0, nil
	}

	// Keyword scoring.
	var keywordScore float64
	if len(keywordRules) > 0 {
		keywords := extractSpamKeywords(subject, bodyText, 20)
		kwSet := make(map[string]struct{}, len(keywords))
		for _, kw := range keywords {
			kwSet[kw] = struct{}{}
		}
		var totalWeight, matchWeight float64
		for _, kr := range keywordRules {
			totalWeight += kr.Weight
			if _, ok := kwSet[strings.ToLower(kr.Value)]; ok {
				matchWeight += kr.Weight
			}
		}
		if totalWeight > 0 {
			keywordScore = matchWeight / totalWeight
		}
		// Mark as suspected if keyword score exceeds threshold.
		if keywordScore >= 0.3 {
			bestLevel = maxSpamLevel(bestLevel, "suspected")
		}
	}

	return bestLevel, keywordScore, nil
}

// InboxQueryParams contains filter options for the unified inbox listing.
func (c *Core) LearnSpamFromEmail(ctx context.Context, fromAddress, subject, bodyText, spamStatus string) error {
	_ = ctx
	fromAddress = strings.ToLower(strings.TrimSpace(fromAddress))
	if fromAddress == "" {
		return nil
	}
	domain := extractDomain(fromAddress)

	upsertRule := func(ruleType, value string) error {
		if value == "" {
			return nil
		}
		value = strings.ToLower(strings.TrimSpace(value))
		if err := c.upsertSpamRule(ruleType, value, 1.0, spamStatus); err != nil {
			c.log.Printf("upsert spam rule type=%q value=%q: %v", ruleType, value, err)
		}
		return nil
	}

	_ = upsertRule("sender", fromAddress)
	if domain != "" {
		_ = upsertRule("domain", domain)
	}

	keywords := extractSpamKeywords(subject, bodyText, 10)
	for _, kw := range keywords {
		_ = upsertRule("keyword", kw)
	}
	return nil
}

// upsertSpamRule creates or updates a spam rule, incrementing hit_count and applying the
// max spam level observed.
func (c *Core) upsertSpamRule(ruleType, value string, weight float64, spamLevel string) error {
	pb := c.db.PocketBase()
	if pb == nil {
		return fmt.Errorf("pocketbase is not initialized")
	}
	collection, err := pb.FindCollectionByNameOrId("inbound_spam_rules")
	if err != nil {
		return err
	}

	// Try to find existing rule.
	existing, err := pb.FindFirstRecordByFilter("inbound_spam_rules",
		fmt.Sprintf(`type = "%s" && value = "%s"`,
			strings.ReplaceAll(ruleType, `"`, ``),
			strings.ReplaceAll(value, `"`, ``)))
	if err == nil && existing != nil {
		// Update existing: increment hit_count, apply max spam_level.
		hitCount := int(toFloat(existing.Get("hit_count"))) + 1
		existing.Set("hit_count", hitCount)
		existing.Set("spam_level", maxSpamLevel(toString(existing.Get("spam_level")), spamLevel))
		existing.Set("is_active", true)
		return pb.Save(existing)
	}

	// Create new rule.
	rec := pbcore.NewRecord(collection)
	rec.Set("type", ruleType)
	rec.Set("value", value)
	rec.Set("weight", weight)
	rec.Set("hit_count", 1)
	rec.Set("spam_level", spamLevel)
	rec.Set("is_active", true)
	return pb.Save(rec)
}

// GetInboundSpamRules returns paginated spam rules.
func (c *Core) GetInboundSpamRules(ctx context.Context, limit, offset int, ruleType string) ([]models.InboundSpamRule, int, error) {
	_ = ctx
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	conds := []string{}
	args := []any{}
	if ruleType != "" {
		conds = append(conds, "type = ?")
		args = append(args, ruleType)
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := c.db.Get(&total, "SELECT COUNT(*) FROM inbound_spam_rules "+where, args...); err != nil {
		return nil, 0, err
	}

	type ruleRow struct {
		RecordID  string         `db:"record_id"`
		Type      string         `db:"type"`
		Value     string         `db:"value"`
		Weight    float64        `db:"weight"`
		HitCount  int            `db:"hit_count"`
		SpamLevel string         `db:"spam_level"`
		IsActive  bool           `db:"is_active"`
		CreatedAt sql.NullString `db:"created_at"`
		UpdatedAt sql.NullString `db:"updated_at"`
	}
	rows := []ruleRow{}
	listArgs := append(args, limit, offset)
	if err := c.db.Select(&rows, `
		SELECT
			id AS record_id,
			type,
			value,
			COALESCE(weight, 1.0) AS weight,
			COALESCE(hit_count, 0) AS hit_count,
			COALESCE(spam_level, '') AS spam_level,
			COALESCE(is_active, 0) AS is_active,
			created AS created_at,
			updated AS updated_at
		FROM inbound_spam_rules
		`+where+`
		ORDER BY hit_count DESC, rowid DESC
		LIMIT ? OFFSET ?
	`, listArgs...); err != nil {
		return nil, 0, err
	}

	out := make([]models.InboundSpamRule, 0, len(rows))
	for _, r := range rows {
		created, _ := parseSQLiteDateTime(r.CreatedAt.String)
		updated, _ := parseSQLiteDateTime(r.UpdatedAt.String)
		out = append(out, models.InboundSpamRule{
			RecordID:  r.RecordID,
			Type:      r.Type,
			Value:     r.Value,
			Weight:    r.Weight,
			HitCount:  r.HitCount,
			SpamLevel: r.SpamLevel,
			IsActive:  r.IsActive,
			CreatedAt: created,
			UpdatedAt: updated,
		})
	}
	return out, total, nil
}

// DeleteInboundSpamRule removes a spam rule by its PocketBase record ID.
func (c *Core) DeleteInboundSpamRule(ctx context.Context, id string) error {
	_ = ctx
	pb := c.db.PocketBase()
	if pb == nil {
		return fmt.Errorf("pocketbase is not initialized")
	}
	rec, err := pb.FindRecordById("inbound_spam_rules", id)
	if err != nil {
		return apperr.NotFound("spam rule not found")
	}
	return pb.Delete(rec)
}

// DeleteSpamInboundEmails removes inbound emails marked as spam or confirmed_spam that are
// older than 7 days, along with their associated attachment records.
// Returns the number of deleted email records.
func (c *Core) DeleteSpamInboundEmails(ctx context.Context) (int, error) {
	_ = ctx
	pb := c.db.PocketBase()
	if pb == nil {
		return 0, fmt.Errorf("pocketbase is not initialized")
	}

	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour).Format(time.RFC3339Nano)

	// Find spam email record IDs to delete.
	type idRow struct {
		RecordID string `db:"record_id"`
	}
	rows := []idRow{}
	if err := c.db.Select(&rows, `
		SELECT id AS record_id
		FROM inbound_email_replies
		WHERE spam_status IN ('spam', 'confirmed_spam')
		  AND received_at < ?
	`, cutoff); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}

	deleted := 0
	for _, r := range rows {
		// Delete attachment records first (PocketBase cascades file deletion).
		attachmentRecords, err := pb.FindRecordsByFilter("inbound_email_attachments",
			fmt.Sprintf(`inbound_email_reply_id = "%s"`, strings.ReplaceAll(r.RecordID, `"`, ``)), "", 0, 200)
		if err == nil {
			for _, a := range attachmentRecords {
				_ = pb.Delete(a)
			}
		}
		emailRec, err := pb.FindRecordById("inbound_email_replies", r.RecordID)
		if err != nil {
			continue
		}
		if err := pb.Delete(emailRec); err != nil {
			c.log.Printf("spam gc: failed to delete email record_id=%q: %v", r.RecordID, err)
			continue
		}
		deleted++
	}
	return deleted, nil
}

// toFloat converts an arbitrary value to float64, returning 0 on failure.
