package core

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/compdani/list_pocket/internal/i18n"
	_ "github.com/compdani/list_pocket/internal/migrations"
	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/compdani/list_pocket/models"
	"github.com/pocketbase/pocketbase"
	pbcore "github.com/pocketbase/pocketbase/core"
)

func spamTestCore(t *testing.T) (*Core, *pocketbase.PocketBase, *bytes.Buffer) {
	t.Helper()
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.ResetBootstrapState() })
	if err := app.RunAppMigrations(); err != nil {
		t.Fatal(err)
	}
	lang, err := i18n.New([]byte(`{"_.code":"en","_.name":"English"}`))
	if err != nil {
		t.Fatal(err)
	}
	db, err := pbdb.NewFromPocketBase(app)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	c := New(&Opt{DB: db, I18n: lang, Log: log.New(&logs, "", 0)}, &Hooks{})
	return c, app, &logs
}

func insertSpamRule(t *testing.T, app *pocketbase.PocketBase, typ, value, level string, weight float64, clean bool) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("inbound_spam_rules")
	if err != nil {
		t.Fatal(err)
	}
	rec := pbcore.NewRecord(col)
	rec.Set("type", typ)
	rec.Set("value", value)
	rec.Set("weight", weight)
	if level != "" {
		rec.Set("spam_level", level)
	}
	rec.Set("hit_count", 1)
	rec.Set("is_active", true)
	rec.Set("clean_override", clean)
	if err := app.Save(rec); err != nil {
		t.Fatalf("insert %s %s: %v", typ, value, err)
	}
}

func spamInbound(t *testing.T, c *Core, from, subject, body string, at time.Time) string {
	t.Helper()
	id, err := c.CreateInboundEmailReplyEvent(context.Background(), &models.InboundEmailReplyEvent{
		FromAddress: from,
		Subject:     subject,
		BodyText:    body,
		BodySnippet: subject,
		MessageID:   fmt.Sprintf("<m-%d@spam.test>", at.UnixNano()),
		ReceivedAt:  at,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func assertSpamClass(t *testing.T, c *Core, from, subject, body, level string, score float64) {
	t.Helper()
	gotLevel, gotScore, err := c.CheckInboundSpamRules(context.Background(), from, subject, body)
	if err != nil {
		t.Fatal(err)
	}
	if gotLevel != level || math.Abs(gotScore-score) > 1e-9 {
		t.Fatalf("%s: got %q %.4f, want %q %.4f", from, gotLevel, gotScore, level, score)
	}
}

func countSpamRules(t *testing.T, c *Core, where string, args ...any) int {
	t.Helper()
	var n int
	q := `SELECT COUNT(*) FROM inbound_spam_rules`
	if where != "" {
		q += ` WHERE ` + where
	}
	if err := c.db.Get(&n, q, args...); err != nil {
		t.Fatal(err)
	}
	return n
}

type senderSnap struct {
	Level string `db:"spam_level"`
	Clean int    `db:"clean_override"`
	Hits  int    `db:"hit_count"`
}

func senderRule(t *testing.T, c *Core, addr string) senderSnap {
	t.Helper()
	var snap senderSnap
	err := c.db.Get(&snap, `
		SELECT
			COALESCE(spam_level, '') AS spam_level,
			COALESCE(clean_override, 0) AS clean_override,
			COALESCE(hit_count, 0) AS hit_count
		FROM inbound_spam_rules
		WHERE type = 'sender' AND value = ?
	`, addr)
	if err != nil {
		t.Fatalf("sender %s: %v", addr, err)
	}
	return snap
}

func TestExtractSpamKeywordsOrderAndLimit(t *testing.T) {
	if got := extractSpamKeywords("bbbb bbbb aaaa", "", 1); len(got) != 1 || got[0] != "bbbb" {
		t.Fatalf("frequency wins: %v", got)
	}
	for i := 0; i < 20; i++ {
		got := extractSpamKeywords("bbbb aaaa bbbb aaaa", "", 2)
		if len(got) != 2 || got[0] != "aaaa" || got[1] != "bbbb" {
			t.Fatalf("tie %d: %v", i, got)
		}
	}
	if got := extractSpamKeywords("zzzz yyyy", "", 1); len(got) != 1 || got[0] != "yyyy" {
		t.Fatalf("lexical tie break: %v", got)
	}
	if got := extractSpamKeywords("aaaa bbbb", "", 0); got != nil {
		t.Fatalf("zero max: %#v", got)
	}
	if got := extractSpamKeywords("aaaa bbbb", "", -3); got != nil {
		t.Fatalf("negative max: %#v", got)
	}
	if got := extractSpamKeywords("ab this click", "", 10); len(got) != 0 {
		t.Fatalf("short and stop words: %v", got)
	}
}

func TestDecideSpamRuleWriteAndValueBound(t *testing.T) {
	create, err := decideSpamRuleWrite(nil)
	if create || err != nil {
		t.Fatalf("found row must update: create=%v err=%v", create, err)
	}
	create, err = decideSpamRuleWrite(sql.ErrNoRows)
	if !create || err != nil {
		t.Fatalf("missing row must create: create=%v err=%v", create, err)
	}
	create, err = decideSpamRuleWrite(fmt.Errorf("db: %w", sql.ErrNoRows))
	if !create || err != nil {
		t.Fatalf("wrapped miss must create: create=%v err=%v", create, err)
	}
	create, err = decideSpamRuleWrite(errors.New("locked"))
	if create || err == nil {
		t.Fatalf("lookup failure must not create: create=%v err=%v", create, err)
	}

	if _, err := normalizeSpamRuleValue(strings.Repeat("a", maxSpamRuleValueLen+1)); err == nil {
		t.Fatal("overlong value should be rejected before the filter")
	}
	got, err := normalizeSpamRuleValue("  We\"IRD  ")
	if err != nil || got != "we\"ird" {
		t.Fatalf("normalize: %q %v", got, err)
	}
	if _, err := normalizeSpamRuleType("domain; drop"); err == nil {
		t.Fatal("rule type must be one of the known kinds")
	}
}

func TestKeywordSpamEvidence(t *testing.T) {
	tokens := map[string]struct{}{
		"alpha": {}, "bravo": {}, "charlie": {}, "delta": {},
	}
	level, score := keywordSpamEvidence(tokens, []spamRuleMatch{
		{Type: "keyword", Value: "alpha", Weight: math.NaN()},
		{Type: "keyword", Value: "bravo", Weight: math.Inf(1)},
		{Type: "keyword", Value: "charlie", Weight: math.Inf(-1)},
		{Type: "keyword", Value: "delta", Weight: -2},
		{Type: "keyword", Value: "delta", Weight: 0},
	})
	if level != "" || score != 0 {
		t.Fatalf("invalid weights counted: %q %v", level, score)
	}

	level, score = keywordSpamEvidence(tokens, []spamRuleMatch{
		{Type: "keyword", Value: "alpha", Weight: 1},
		{Type: "keyword", Value: "alpha", Weight: 1},
		{Type: "keyword", Value: "bravo", Weight: 1},
	})
	if level != "" || score != 0 {
		t.Fatalf("repeated token met distinct threshold: %q %v", level, score)
	}

	level, score = keywordSpamEvidence(tokens, []spamRuleMatch{
		{Type: "keyword", Value: "alpha", Weight: 0.5},
		{Type: "keyword", Value: "bravo", Weight: 0.5},
		{Type: "keyword", Value: "charlie", Weight: 0.5},
	})
	if level != "" || score != 0 {
		t.Fatalf("sub-threshold weight: %q %v", level, score)
	}

	unrelated := []spamRuleMatch{
		{Type: "keyword", Value: "alpha", Weight: 1},
		{Type: "keyword", Value: "bravo", Weight: 1},
		{Type: "keyword", Value: "charlie", Weight: 1},
	}
	for i := 0; i < 10; i++ {
		unrelated = append(unrelated, spamRuleMatch{
			Type: "keyword", Value: fmt.Sprintf("other%02d", i), Weight: 100,
		})
	}
	level, score = keywordSpamEvidence(tokens, unrelated)
	if level != "suspected" || math.Abs(score-1) > 1e-9 {
		t.Fatalf("unrelated rules changed evidence: %q %v", level, score)
	}

	level, score = keywordSpamEvidence(tokens, []spamRuleMatch{
		{Type: "keyword", Value: "alpha", Weight: 10},
		{Type: "keyword", Value: "bravo", Weight: 10},
		{Type: "keyword", Value: "charlie", Weight: 10},
		{Type: "keyword", Value: "delta", Weight: math.NaN()},
	})
	if level != "suspected" || math.Abs(score-1) > 1e-9 {
		t.Fatalf("score not clamped: %q %v", level, score)
	}

	// Whole token, case normalized. A shorter rule must not match inside a token.
	level, score = keywordSpamEvidence(map[string]struct{}{"rarealpha": {}}, []spamRuleMatch{
		{Type: "keyword", Value: "Rare", Weight: 5},
		{Type: "keyword", Value: "RareAlpha", Weight: 1},
	})
	if level != "" || score != 0 {
		t.Fatalf("substring or single token classified: %q %v", level, score)
	}
}

func TestInboundSpamSharedDomainAndExactSender(t *testing.T) {
	c, app, _ := spamTestCore(t)
	ctx := context.Background()
	if err := c.LearnSpamFromEmail(ctx, "Spammer@Shared.test", "offer", "limited roulette bonus casino", "spam"); err != nil {
		t.Fatal(err)
	}
	if n := countSpamRules(t, c, "type = 'domain'"); n != 0 {
		t.Fatalf("learning created %d domain rules", n)
	}
	if n := countSpamRules(t, c, "type = 'sender' AND value = 'spammer@shared.test'"); n != 1 {
		t.Fatalf("sender rules: %d", n)
	}
	assertSpamClass(t, c, "other@shared.test", "hello", "just a note", "", 0)

	insertSpamRule(t, app, "domain", "shared.test", "confirmed_spam", 1, false)
	assertSpamClass(t, c, "other@shared.test", "hello", "just a note", "suspected", domainSuspectedScore)

	level, score, err := c.CheckInboundSpamRules(ctx, "spammer@shared.test", "hello", "just a note")
	if err != nil || level != "spam" || math.Abs(score-senderSpamScore) > 1e-9 {
		t.Fatalf("exact sender: %q %v %v", level, score, err)
	}

	insertSpamRule(t, app, "sender", "maybe@shared.test", "suspected", 1, false)
	for i := 0; i < 30; i++ {
		insertSpamRule(t, app, "keyword", fmt.Sprintf("noise%02dtok", i), "spam", 50, false)
	}
	assertSpamClass(t, c, "maybe@shared.test", "hello", "noise00tok noise01tok noise02tok", "suspected", senderSuspectedScore)

	insertSpamRule(t, app, "sender", "sure@shared.test", "confirmed_spam", 1, false)
	assertSpamClass(t, c, "sure@shared.test", "x", "y", "confirmed_spam", senderSpamScore)
}

func TestInboundSpamCleanFeedbackCycle(t *testing.T) {
	c, app, _ := spamTestCore(t)
	ctx := context.Background()
	body := "alphaone alphatwo alphathree"
	at := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	id := spamInbound(t, c, "bad@shared.test", "hi", body, at)
	if err := c.UpdateInboundEmailSpamStatus(ctx, id, "spam"); err != nil {
		t.Fatal(err)
	}
	if n := countSpamRules(t, c, "type = 'domain'"); n != 0 {
		t.Fatalf("domain rules: %d", n)
	}
	marked, err := c.GetInboundEmailByID(ctx, id)
	if err != nil || marked.SpamScore != senderSpamScore {
		t.Fatalf("manual spam score: %+v, %v", marked, err)
	}
	snap := senderRule(t, c, "bad@shared.test")
	if snap.Level != "spam" || snap.Clean != 0 || snap.Hits != 1 {
		t.Fatalf("after spam: %+v", snap)
	}

	future := spamInbound(t, c, "bad@shared.test", "again", body, at.Add(time.Minute))
	got, err := c.GetInboundEmailByID(ctx, future)
	if err != nil {
		t.Fatal(err)
	}
	if got.SpamStatus != "spam" || math.Abs(got.SpamScore-senderSpamScore) > 1e-9 {
		t.Fatalf("future before clean: %q %v", got.SpamStatus, got.SpamScore)
	}

	if _, err := c.db.Exec(`UPDATE inbound_email_replies SET spam_score = 0.75 WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateInboundEmailSpamStatus(ctx, id, ""); err != nil {
		t.Fatal(err)
	}
	cleared, err := c.GetInboundEmailByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.SpamStatus != "" || cleared.SpamScore != 0 {
		t.Fatalf("cleared status=%q score=%v", cleared.SpamStatus, cleared.SpamScore)
	}
	snap = senderRule(t, c, "bad@shared.test")
	if snap.Clean != 1 || snap.Level != "spam" || snap.Hits != 1 {
		t.Fatalf("clean override: %+v", snap)
	}

	rules, _, err := c.GetInboundSpamRules(ctx, 50, 0, "sender")
	if err != nil || len(rules) != 1 || !rules[0].CleanOverride {
		t.Fatalf("rules must expose clean feedback: %+v, %v", rules, err)
	}
	insertSpamRule(t, app, "domain", "shared.test", "confirmed_spam", 1, false)
	stayed := spamInbound(t, c, "bad@shared.test", "still", body, at.Add(2*time.Minute))
	stayedRec, err := c.GetInboundEmailByID(ctx, stayed)
	if err != nil {
		t.Fatal(err)
	}
	if stayedRec.SpamStatus != "" || stayedRec.SpamScore != 0 {
		t.Fatalf("clean sender ingested as %q %v", stayedRec.SpamStatus, stayedRec.SpamScore)
	}

	other := spamInbound(t, c, "other@shared.test", "hey", body, at.Add(3*time.Minute))
	otherRec, err := c.GetInboundEmailByID(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if otherRec.SpamStatus != "suspected" || math.Abs(otherRec.SpamScore-1) > 1e-9 {
		t.Fatalf("other sender: %q %v", otherRec.SpamStatus, otherRec.SpamScore)
	}

	if err := c.UpdateInboundEmailSpamStatus(ctx, stayed, "confirmed_spam"); err != nil {
		t.Fatal(err)
	}
	snap = senderRule(t, c, "bad@shared.test")
	if snap.Clean != 0 || snap.Level != "confirmed_spam" || snap.Hits != 2 {
		t.Fatalf("spam reverses clean: %+v", snap)
	}
	again := spamInbound(t, c, "bad@shared.test", "back", body, at.Add(4*time.Minute))
	againRec, err := c.GetInboundEmailByID(ctx, again)
	if err != nil {
		t.Fatal(err)
	}
	if againRec.SpamStatus != "confirmed_spam" || math.Abs(againRec.SpamScore-senderSpamScore) > 1e-9 {
		t.Fatalf("after reverse: %q %v", againRec.SpamStatus, againRec.SpamScore)
	}

	// Clearing a domain-suspected mailbox creates clean feedback even when no
	// sender rule existed yet. A different mailbox on that domain stays capped.
	fresh := spamInbound(t, c, "fresh@shared.test", "hey", "plain note", at.Add(5*time.Minute))
	freshRec, err := c.GetInboundEmailByID(ctx, fresh)
	if err != nil {
		t.Fatal(err)
	}
	if freshRec.SpamStatus != "suspected" {
		t.Fatalf("fresh domain match: %q", freshRec.SpamStatus)
	}
	if err := c.UpdateInboundEmailSpamStatus(ctx, fresh, ""); err != nil {
		t.Fatal(err)
	}
	freshSnap := senderRule(t, c, "fresh@shared.test")
	if freshSnap.Clean != 1 || freshSnap.Hits != 0 {
		t.Fatalf("clean create: %+v", freshSnap)
	}
	clearedFresh, err := c.GetInboundEmailByID(ctx, fresh)
	if err != nil {
		t.Fatal(err)
	}
	if clearedFresh.SpamStatus != "" || clearedFresh.SpamScore != 0 {
		t.Fatalf("fresh score clear: %q %v", clearedFresh.SpamStatus, clearedFresh.SpamScore)
	}
	next := spamInbound(t, c, "fresh@shared.test", "later", body, at.Add(6*time.Minute))
	nextRec, err := c.GetInboundEmailByID(ctx, next)
	if err != nil {
		t.Fatal(err)
	}
	if nextRec.SpamStatus != "" {
		t.Fatalf("fresh sender should stay clean, got %q", nextRec.SpamStatus)
	}
	neighbor := spamInbound(t, c, "neighbor@shared.test", "later", body, at.Add(7*time.Minute))
	neighborRec, err := c.GetInboundEmailByID(ctx, neighbor)
	if err != nil {
		t.Fatal(err)
	}
	if neighborRec.SpamStatus != "suspected" {
		t.Fatalf("neighbor: %q", neighborRec.SpamStatus)
	}
}

func TestInboundSpamKeywordDetection(t *testing.T) {
	c, app, _ := spamTestCore(t)
	ctx := context.Background()

	var body strings.Builder
	for i := 0; i < 20; i++ {
		word := fmt.Sprintf("freq%02dtok", i)
		for n := 0; n < 4; n++ {
			body.WriteString(word)
			body.WriteByte(' ')
		}
	}
	body.WriteString("rarealpha rarebravo raregamma")
	// Prefixes of those tokens are not whole-token matches.
	for _, kw := range []string{"rare", "brav", "gamm"} {
		insertSpamRule(t, app, "keyword", kw, "spam", 5, false)
	}
	assertSpamClass(t, c, "reader@shared.test", "subj", body.String(), "", 0)
	// The rare tokens sit outside the old frequency top 20. Mixed case still matches.
	for _, kw := range []string{"RareAlpha", "rarebravo", "raregamma"} {
		insertSpamRule(t, app, "keyword", kw, "spam", 1, false)
	}
	assertSpamClass(t, c, "reader@shared.test", "subj", body.String(), "suspected", 1)

	before := 1.0
	for i := 0; i < 15; i++ {
		insertSpamRule(t, app, "keyword", fmt.Sprintf("unrelated%02dtok", i), "confirmed_spam", 40, false)
	}
	assertSpamClass(t, c, "reader@shared.test", "subj", body.String(), "suspected", before)

	insertSpamRule(t, app, "keyword", "alpha", "spam", 9, false)
	assertSpamClass(t, c, "reader@shared.test", "subj", "alpha alpha alpha alpha", "", 0)

	insertSpamRule(t, app, "keyword", "negativetok", "spam", -4, false)
	insertSpamRule(t, app, "keyword", "zerotoken", "spam", 0, false)
	insertSpamRule(t, app, "keyword", "onlyone", "spam", 1, false)
	insertSpamRule(t, app, "keyword", "secondok", "spam", 1, false)
	assertSpamClass(t, c, "reader@shared.test", "subj", "onlyone secondok zerotoken negativetok", "", 0)

	insertSpamRule(t, app, "keyword", "thirdok", "spam", 1, false)
	assertSpamClass(t, c, "reader@shared.test", "subj", "onlyone secondok thirdok negativetok zerotoken", "suspected", 1)

	// Direct check also sees tokens that are not in the frequency top 20.
	level, score, err := c.CheckInboundSpamRules(ctx, "reader@shared.test", "", body.String())
	if err != nil || level != "suspected" || math.Abs(score-before) > 1e-9 {
		t.Fatalf("outside cutoff: %q %v %v", level, score, err)
	}
}

func TestInboundSpamFeedbackAndClassificationErrors(t *testing.T) {
	c, app, logs := spamTestCore(t)
	ctx := context.Background()
	at := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	id := spamInbound(t, c, "ok@shared.test", "hi", "hello there", at)

	if err := c.UpdateInboundEmailSpamStatus(ctx, id, "not-a-level"); err == nil {
		t.Fatal("invalid status accepted")
	}
	if err := c.UpdateInboundEmailSpamStatus(ctx, "missing-id", "spam"); err == nil {
		t.Fatal("missing id accepted")
	}

	bare := &Core{log: log.New(io.Discard, "", 0)}
	if err := bare.LearnSpamFromEmail(ctx, "a@b.co", "s", "body text", "spam"); err == nil {
		t.Fatal("nil pocketbase should fail")
	}

	longFrom := strings.Repeat("a", maxSpamRuleValueLen) + "@shared.test"
	if err := c.LearnSpamFromEmail(ctx, longFrom, "subject", "hello there", "spam"); err == nil {
		t.Fatal("overlong sender should fail before the filter")
	}
	if n := countSpamRules(t, c, "type = 'sender'"); n != 0 {
		t.Fatalf("overlong learn created %d sender rules", n)
	}

	// A valid mailbox that cannot be stored as a rule forces the feedback
	// transaction to roll the status write back.
	col, err := app.FindCollectionByNameOrId("inbound_spam_rules")
	if err != nil {
		t.Fatal(err)
	}
	valueField, ok := col.Fields.GetByName("value").(*pbcore.TextField)
	if !ok {
		t.Fatal("value field missing")
	}
	valueField.Max = 3
	if err := app.Save(col); err != nil {
		t.Fatal(err)
	}
	logs.Reset()
	if err := c.UpdateInboundEmailSpamStatus(ctx, id, "spam"); err == nil {
		t.Fatal("rule save should fail and roll back")
	}
	if !strings.Contains(logs.String(), "spam learning warning") {
		t.Fatalf("learning error was not logged: %s", logs.String())
	}
	rec, err := c.GetInboundEmailByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if rec.SpamStatus != "" {
		t.Fatalf("status committed despite learning error: %q", rec.SpamStatus)
	}
	if n := countSpamRules(t, c, "type = 'sender'"); n != 0 {
		t.Fatalf("sender rule survived rollback: %d", n)
	}
	valueField.Max = 0
	if err := app.Save(col); err != nil {
		t.Fatal(err)
	}

	quoted := "we\"ird&&or@shared.test"
	if err := c.LearnSpamFromEmail(ctx, quoted, "subject", "hello there", "spam"); err != nil {
		t.Fatal(err)
	}
	if err := c.LearnSpamFromEmail(ctx, quoted, "subject", "hello there", "confirmed_spam"); err != nil {
		t.Fatal(err)
	}
	if n := countSpamRules(t, c, "type = 'sender' AND value = ?", strings.ToLower(quoted)); n != 1 {
		t.Fatalf("quoted sender rows: %d", n)
	}
	snap := senderRule(t, c, strings.ToLower(quoted))
	if snap.Hits != 2 || snap.Level != "confirmed_spam" {
		t.Fatalf("quoted sender: %+v", snap)
	}

	if _, err := c.db.Exec(`DROP TABLE inbound_spam_rules`); err != nil {
		t.Fatal(err)
	}
	logs.Reset()
	ingested := spamInbound(t, c, "after-drop@shared.test", "hi", "hello there", at.Add(3*time.Second))
	if !strings.Contains(logs.String(), "inbound spam classification failed") {
		t.Fatalf("classification log: %s", logs.String())
	}
	ing, err := c.GetInboundEmailByID(ctx, ingested)
	if err != nil {
		t.Fatal(err)
	}
	if ing.SpamStatus != "" {
		t.Fatalf("fail-open stored %q", ing.SpamStatus)
	}
}
