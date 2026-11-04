package core

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/compdani/list_pocket/internal/apperr"

	"github.com/compdani/list_pocket/models"
	"github.com/pocketbase/dbx"
	pbcore "github.com/pocketbase/pocketbase/core"
)

// Spam evidence is fixed and local to the message. It does not depend on how
// many unrelated rules exist.
//
// Exact sender rules classify at their own level. A suspected sender scores
// senderSuspectedScore; spam and confirmed_spam score 1. Domain rules are
// capped at suspected (domainSuspectedScore) because their provenance is
// unknown and shared domains would otherwise block unrelated senders.
// Keyword-only evidence never rises above suspected. It requires at least
// keywordMinDistinct distinct matching tokens whose positive, finite weights
// sum to at least keywordEvidenceUnit. The keyword score is
// min(1, sum/keywordEvidenceUnit). Sub-threshold keyword evidence scores 0.
const (
	senderSuspectedScore = 0.5
	senderSpamScore      = 1.0
	domainSuspectedScore = 0.5

	keywordMinDistinct  = 3
	keywordEvidenceUnit = 3.0

	// maxSpamRuleValueLen bounds values before they are passed as PocketBase
	// filter parameters or stored. It is below the text field default of 5000.
	maxSpamRuleValueLen = 1000
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

// eligibleSpamToken reports whether w is a whole token worth matching.
// w must already be lowercased and stripped to ASCII letters and digits.
func eligibleSpamToken(w string) bool {
	if len(w) < 4 {
		return false
	}
	_, stop := spamStopWords[w]
	return !stop
}

// spamWordCounts counts eligible whole tokens in subject and body.
func spamWordCounts(subject, bodyText string) map[string]int {
	text := strings.ToLower(subject + " " + bodyText)
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	seen := map[string]int{}
	for _, w := range words {
		if !eligibleSpamToken(w) {
			continue
		}
		seen[w]++
	}
	return seen
}

// extractSpamKeywords extracts up to maxN significant words from text (subject + body).
// Equal frequencies break toward ascending lexical order so the result is stable.
// A nonpositive maxN extracts nothing.
func extractSpamKeywords(subject, bodyText string, maxN int) []string {
	if maxN <= 0 {
		return nil
	}
	seen := spamWordCounts(subject, bodyText)
	type wf struct {
		word  string
		count int
	}
	ranked := make([]wf, 0, len(seen))
	for w, c := range seen {
		ranked = append(ranked, wf{w, c})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].count != ranked[j].count {
			return ranked[i].count > ranked[j].count
		}
		return ranked[i].word < ranked[j].word
	})
	if maxN > len(ranked) {
		maxN = len(ranked)
	}
	out := make([]string, 0, maxN)
	for _, w := range ranked[:maxN] {
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

type spamRuleMatch struct {
	Type          string
	Value         string
	Weight        float64
	SpamLevel     string
	CleanOverride bool
}

// positiveSpamWeight reports w when it can contribute evidence.
// Nonpositive and nonfinite weights are ignored.
func positiveSpamWeight(w float64) (float64, bool) {
	if math.IsNaN(w) || math.IsInf(w, 0) || w <= 0 {
		return 0, false
	}
	return w, true
}

func clampSpamScore(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
		return 0
	}
	if v >= 1 {
		return 1
	}
	return v
}

func senderEvidenceScore(level string) float64 {
	switch level {
	case "spam", "confirmed_spam":
		return senderSpamScore
	case "suspected":
		return senderSuspectedScore
	default:
		return 0
	}
}

// keywordSpamEvidence scores whole-token keyword matches.
// Repeated occurrences of one token count once. Unrelated rules are not part
// of the denominator. The level is suspected, or empty when the fixed bar
// (keywordMinDistinct distinct tokens and weight sum >= keywordEvidenceUnit)
// is not met.
func keywordSpamEvidence(tokens map[string]struct{}, rules []spamRuleMatch) (string, float64) {
	if len(tokens) == 0 || len(rules) == 0 {
		return "", 0
	}
	matched := make(map[string]float64, keywordMinDistinct)
	for _, rule := range rules {
		if rule.Type != "" && rule.Type != "keyword" {
			continue
		}
		weight, ok := positiveSpamWeight(rule.Weight)
		if !ok {
			continue
		}
		token := strings.ToLower(strings.TrimSpace(rule.Value))
		if _, eligible := tokens[token]; !eligible {
			continue
		}
		if prev, exists := matched[token]; !exists || weight > prev {
			matched[token] = weight
		}
	}
	if len(matched) < keywordMinDistinct {
		return "", 0
	}
	var sum float64
	for _, weight := range matched {
		sum += weight
	}
	if math.IsNaN(sum) || math.IsInf(sum, 0) || sum < keywordEvidenceUnit {
		return "", 0
	}
	return "suspected", clampSpamScore(sum / keywordEvidenceUnit)
}

// classifySpam applies the conservative evidence policy to an already loaded
// rule set. An exact-sender clean override returns ("", 0) and wins over
// domain, keyword, and older sender matches.
func classifySpam(fromAddress, subject, bodyText string, rules []spamRuleMatch) (string, float64) {
	fromAddress = strings.ToLower(strings.TrimSpace(fromAddress))
	if fromAddress == "" {
		return "", 0
	}
	domain := extractDomain(fromAddress)

	senderClean := false
	senderLevel := ""
	domainLevel := ""
	keywordRules := make([]spamRuleMatch, 0)
	for _, rule := range rules {
		switch rule.Type {
		case "sender":
			if !strings.EqualFold(rule.Value, fromAddress) {
				continue
			}
			if rule.CleanOverride {
				senderClean = true
				continue
			}
			if rule.SpamLevel != "" {
				senderLevel = maxSpamLevel(senderLevel, rule.SpamLevel)
			}
		case "domain":
			if domain != "" && rule.SpamLevel != "" && strings.EqualFold(rule.Value, domain) {
				domainLevel = maxSpamLevel(domainLevel, rule.SpamLevel)
			}
		case "keyword":
			keywordRules = append(keywordRules, rule)
		}
	}

	// Clean feedback is the newest explicit decision for this address.
	if senderClean {
		return "", 0
	}
	// Exact sender evidence stands on its own, including suspected.
	if senderLevel != "" {
		return senderLevel, senderEvidenceScore(senderLevel)
	}

	level := ""
	score := 0.0
	if domainLevel != "" {
		// Cap domain-only matches. The stored level may say spam or
		// confirmed_spam, but that provenance is not an exact-sender decision.
		level = "suspected"
		score = domainSuspectedScore
	}

	tokens := map[string]struct{}{}
	if len(keywordRules) > 0 {
		for token := range spamWordCounts(subject, bodyText) {
			tokens[token] = struct{}{}
		}
	}
	keywordLevel, keywordScore := keywordSpamEvidence(tokens, keywordRules)
	if keywordLevel != "" {
		level = maxSpamLevel(level, keywordLevel)
		if keywordScore > score {
			score = keywordScore
		}
	}
	return level, score
}

// CheckInboundSpamRules evaluates active spam rules against the incoming email fields.
// See the evidence constants for the level and score policy.
// Returns ("", 0, nil) when the message should stay clean.
func (c *Core) CheckInboundSpamRules(ctx context.Context, fromAddress, subject, bodyText string) (string, float64, error) {
	_ = ctx
	fromAddress = strings.ToLower(strings.TrimSpace(fromAddress))
	if fromAddress == "" {
		return "", 0, nil
	}

	type ruleRow struct {
		Type          string  `db:"type"`
		Value         string  `db:"value"`
		Weight        float64 `db:"weight"`
		SpamLevel     string  `db:"spam_level"`
		CleanOverride bool    `db:"clean_override"`
	}
	rows := []ruleRow{}
	if err := c.db.Select(&rows, `
		SELECT
			type,
			value,
			COALESCE(weight, 1.0) AS weight,
			COALESCE(spam_level, '') AS spam_level,
			COALESCE(clean_override, 0) AS clean_override
		FROM inbound_spam_rules
		WHERE is_active = 1
		  AND (
		    COALESCE(spam_level, '') != ''
		    OR COALESCE(clean_override, 0) = 1
		  )
		ORDER BY rowid ASC
	`); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", 0, err
	}

	rules := make([]spamRuleMatch, 0, len(rows))
	for _, row := range rows {
		rules = append(rules, spamRuleMatch{
			Type:          row.Type,
			Value:         row.Value,
			Weight:        row.Weight,
			SpamLevel:     row.SpamLevel,
			CleanOverride: row.CleanOverride,
		})
	}
	level, score := classifySpam(fromAddress, subject, bodyText, rules)
	return level, score, nil
}

// LearnSpamFromEmail upserts an exact-sender rule and keyword rules for an
// explicitly marked spam email. It does not create a domain rule. Errors are
// returned to the caller. A clean override on the same sender is cleared so
// the new spam feedback wins.
func (c *Core) LearnSpamFromEmail(ctx context.Context, fromAddress, subject, bodyText, spamStatus string) error {
	_ = ctx
	pb, err := c.pocketBaseApp()
	if err != nil {
		return err
	}
	return pb.RunInTransaction(func(tx pbcore.App) error {
		return c.learnSpamFromEmail(tx, fromAddress, subject, bodyText, spamStatus)
	})
}

func (c *Core) learnSpamFromEmail(app pbcore.App, fromAddress, subject, bodyText, spamStatus string) error {
	fromAddress = strings.ToLower(strings.TrimSpace(fromAddress))
	if fromAddress == "" {
		return nil
	}
	if err := c.upsertSpamRule(app, "sender", fromAddress, 1, spamStatus, false); err != nil {
		return err
	}
	for _, kw := range extractSpamKeywords(subject, bodyText, 10) {
		if err := c.upsertSpamRule(app, "keyword", kw, 1, spamStatus, false); err != nil {
			return err
		}
	}
	return nil
}

// recordSenderClean stores durable exact-sender clean feedback. The sender's
// historical spam level is left in place and ignored while the override is set.
func (c *Core) recordSenderClean(app pbcore.App, fromAddress string) error {
	fromAddress = strings.ToLower(strings.TrimSpace(fromAddress))
	if fromAddress == "" {
		return nil
	}
	return c.upsertSpamRule(app, "sender", fromAddress, 1, "", true)
}

func (c *Core) pocketBaseApp() (pbcore.App, error) {
	if c == nil || c.db == nil || c.db.PocketBase() == nil {
		return nil, fmt.Errorf("pocketbase is not initialized")
	}
	return c.db.PocketBase(), nil
}

// decideSpamRuleWrite reports whether a rule lookup miss should insert.
// Only sql.ErrNoRows creates a row. Any other lookup error is returned so a
// transient failure cannot insert a duplicate.
func decideSpamRuleWrite(err error) (create bool, writeErr error) {
	if err == nil {
		return false, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	return false, err
}

func normalizeSpamRuleType(ruleType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(ruleType)) {
	case "sender", "domain", "keyword":
		return strings.ToLower(strings.TrimSpace(ruleType)), nil
	default:
		return "", fmt.Errorf("invalid spam rule type %q", ruleType)
	}
}

// normalizeSpamRuleValue bounds a rule value before it is used as a PocketBase
// filter parameter. Empty input is not an error; the caller skips the write.
func normalizeSpamRuleValue(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", nil
	}
	if utf8.RuneCountInString(value) > maxSpamRuleValueLen {
		return "", fmt.Errorf("spam rule value exceeds %d characters", maxSpamRuleValueLen)
	}
	return value, nil
}

// upsertSpamRule creates or updates one spam rule using bound filter parameters.
// cleanOverride sets exact-sender clean feedback without raising hit_count or
// the stored spam level. A spam write clears that override.
func (c *Core) upsertSpamRule(app pbcore.App, ruleType, value string, weight float64, spamLevel string, cleanOverride bool) error {
	if app == nil {
		return fmt.Errorf("pocketbase is not initialized")
	}
	var err error
	ruleType, err = normalizeSpamRuleType(ruleType)
	if err != nil {
		return err
	}
	value, err = normalizeSpamRuleValue(value)
	if err != nil || value == "" {
		return err
	}

	existing, findErr := app.FindFirstRecordByFilter("inbound_spam_rules",
		"type = {:type} && value = {:value}",
		dbx.Params{"type": ruleType, "value": value})
	create, err := decideSpamRuleWrite(findErr)
	if err != nil {
		return err
	}
	if !create {
		if existing == nil {
			return fmt.Errorf("spam rule lookup returned no record")
		}
		if cleanOverride {
			existing.Set("clean_override", true)
			existing.Set("is_active", true)
			return app.Save(existing)
		}
		hitCount := int(toFloat(existing.Get("hit_count"))) + 1
		existing.Set("hit_count", hitCount)
		existing.Set("spam_level", maxSpamLevel(toString(existing.Get("spam_level")), spamLevel))
		existing.Set("clean_override", false)
		existing.Set("is_active", true)
		return app.Save(existing)
	}

	collection, err := app.FindCollectionByNameOrId("inbound_spam_rules")
	if err != nil {
		return err
	}
	rec := pbcore.NewRecord(collection)
	rec.Set("type", ruleType)
	rec.Set("value", value)
	rec.Set("weight", weight)
	rec.Set("is_active", true)
	rec.Set("clean_override", cleanOverride)
	if cleanOverride {
		rec.Set("hit_count", 0)
	} else {
		rec.Set("hit_count", 1)
		rec.Set("spam_level", spamLevel)
	}
	return app.Save(rec)
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
		RecordID      string         `db:"record_id"`
		Type          string         `db:"type"`
		Value         string         `db:"value"`
		Weight        float64        `db:"weight"`
		HitCount      int            `db:"hit_count"`
		SpamLevel     string         `db:"spam_level"`
		IsActive      bool           `db:"is_active"`
		CleanOverride bool           `db:"clean_override"`
		CreatedAt     sql.NullString `db:"created_at"`
		UpdatedAt     sql.NullString `db:"updated_at"`
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
			COALESCE(clean_override, 0) AS clean_override,
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
			RecordID:      r.RecordID,
			Type:          r.Type,
			Value:         r.Value,
			Weight:        r.Weight,
			HitCount:      r.HitCount,
			SpamLevel:     r.SpamLevel,
			IsActive:      r.IsActive,
			CleanOverride: r.CleanOverride,
			CreatedAt:     created,
			UpdatedAt:     updated,
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
		attachmentRecords, err := pb.FindAllRecords("inbound_email_attachments",
			dbx.HashExp{"inbound_email_reply_id": r.RecordID})
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
