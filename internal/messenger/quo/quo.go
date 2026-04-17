package quo

import (
	"context"
	"errors"
	"fmt"
	"html"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/compdani/list_pocket/models"
)

// Quo/OpenPhone rejects content > 1600 chars (`/content: Expected string length
// less or equal to 1600`). Quo handles GSM/UCS-2 segmentation internally under
// this cap; anything longer we split into multiple API calls.
const maxQuoContentLen = 1600

// Cap the number of parts a single campaign body can be split into so a
// pathological 50KB HTML body can't fan out into 30+ SMS per recipient.
const maxQuoParts = 6

// continuationPrefix is prepended to parts 2..N so the recipient can see the
// ordering. `%d` is the 1-based part index, `%d` is the total.
const continuationPrefix = "(%d/%d) "

var (
	reHTMLTag     = regexp.MustCompile(`(?s)<[^>]+>`)
	reHTMLComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	reBlockBreak  = regexp.MustCompile(`(?i)</?(?:p|div|br|li|tr|h[1-6])\b[^>]*>`)
	// RE2 has no backrefs, so match each blocky element separately.
	reHTMLScript = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</\s*script\s*>`)
	reHTMLStyle  = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</\s*style\s*>`)
	reHTMLHead   = regexp.MustCompile(`(?is)<head\b[^>]*>.*?</\s*head\s*>`)
	reWS         = regexp.MustCompile(`[ \t]+`)
	reBlankLines = regexp.MustCompile(`\n{3,}`)
)

// sanitizeSMSBody strips any HTML and unescapes entities so only plain text is
// sent to Quo. Defends against stray templates or pasted HTML reaching the
// SMS path. Does NOT truncate — use splitSMSBody for that.
func sanitizeSMSBody(b []byte) string {
	s := string(b)
	if strings.ContainsAny(s, "<&") {
		s = reHTMLScript.ReplaceAllString(s, "")
		s = reHTMLStyle.ReplaceAllString(s, "")
		s = reHTMLHead.ReplaceAllString(s, "")
		s = reHTMLComment.ReplaceAllString(s, "")
		s = reBlockBreak.ReplaceAllString(s, "\n")
		s = reHTMLTag.ReplaceAllString(s, "")
		s = html.UnescapeString(s)
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = reWS.ReplaceAllString(s, " ")
	s = reBlankLines.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// splitSMSBody chunks a sanitized body into parts of ≤ maxQuoContentLen runes,
// prefixing parts 2..N with "(i/N) " to preserve reading order. It prefers
// breaking at whitespace within the last ~10% of each chunk so words aren't cut
// in the middle. Capped at maxQuoParts; any remainder is dropped.
func splitSMSBody(s string) []string {
	if s == "" {
		return nil
	}
	runes := []rune(s)
	if len(runes) <= maxQuoContentLen {
		return []string{s}
	}

	// First pass: cut into raw chunks ≤ maxQuoContentLen.
	var chunks []string
	i := 0
	for i < len(runes) && len(chunks) < maxQuoParts {
		end := i + maxQuoContentLen
		if end >= len(runes) {
			chunks = append(chunks, strings.TrimSpace(string(runes[i:])))
			i = len(runes)
			break
		}
		// Look for a whitespace break within the last 10% of the window.
		breakAt := end
		minBreak := i + (maxQuoContentLen * 9 / 10)
		for j := end; j > minBreak; j-- {
			if unicodeIsSpace(runes[j]) {
				breakAt = j
				break
			}
		}
		chunks = append(chunks, strings.TrimSpace(string(runes[i:breakAt])))
		i = breakAt
		for i < len(runes) && unicodeIsSpace(runes[i]) {
			i++
		}
	}

	if len(chunks) <= 1 {
		return chunks
	}

	// Second pass: prefix "(n/N) " so the prefix counts toward the chunk cap.
	// We re-split with a reduced window equal to maxQuoContentLen - len(prefix).
	total := len(chunks)
	prefixLen := len([]rune(fmt.Sprintf(continuationPrefix, total, total)))
	window := maxQuoContentLen - prefixLen
	if window < 100 {
		// Shouldn't happen for sane maxQuoContentLen, but guard anyway.
		window = maxQuoContentLen
	}

	out := make([]string, 0, total)
	i = 0
	for i < len(runes) && len(out) < maxQuoParts {
		end := i + window
		if end >= len(runes) {
			out = append(out, strings.TrimSpace(string(runes[i:])))
			i = len(runes)
			break
		}
		breakAt := end
		minBreak := i + (window * 9 / 10)
		for j := end; j > minBreak; j-- {
			if unicodeIsSpace(runes[j]) {
				breakAt = j
				break
			}
		}
		out = append(out, strings.TrimSpace(string(runes[i:breakAt])))
		i = breakAt
		for i < len(runes) && unicodeIsSpace(runes[i]) {
			i++
		}
	}

	if len(out) <= 1 {
		return out
	}
	for idx := range out {
		out[idx] = fmt.Sprintf(continuationPrefix, idx+1, len(out)) + out[idx]
	}
	return out
}

func unicodeIsSpace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	}
	return false
}

const MessengerName = models.CampaignMessengerQuo

// Messenger implements manager.Messenger for Quo SMS.
type Messenger struct {
	loadSettings func() models.TextMessagingSettings
	throttleMu   sync.Mutex
	lastSend     time.Time
}

func NewMessenger(loadSettings func() models.TextMessagingSettings) *Messenger {
	if loadSettings == nil {
		loadSettings = func() models.TextMessagingSettings { return models.DefaultTextMessagingSettings() }
	}
	return &Messenger{loadSettings: loadSettings}
}

func (m *Messenger) Name() string {
	return MessengerName
}

func (m *Messenger) client() (*Client, error) {
	s := m.loadSettings()
	p := s.QuoProvider()
	if p == nil || !p.Enabled {
		return nil, errors.New("quo messaging is not enabled")
	}
	return NewClientFromSettings(s)
}

func (m *Messenger) Push(msg models.Message) error {
	c, err := m.client()
	if err != nil {
		return err
	}
	if len(msg.To) == 0 {
		return errors.New("missing recipient phone")
	}
	to := strings.TrimSpace(msg.To[0])
	body := sanitizeSMSBody(msg.Body)
	if body == "" {
		return errors.New("empty message body")
	}

	parts := splitSMSBody(body)
	if len(parts) == 0 {
		return errors.New("empty message body")
	}

	for _, part := range parts {
		m.throttleSend()
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		sendErr := c.SendText(ctx, to, []byte(part))
		cancel()
		if sendErr != nil {
			return sendErr
		}
	}
	return nil
}

func (m *Messenger) throttleSend() {
	lim := m.loadSettings().SendLimits
	m.throttleMu.Lock()
	now := time.Now()
	if lim.MinDelayMS > 0 && !m.lastSend.IsZero() {
		d := time.Duration(lim.MinDelayMS) * time.Millisecond
		if since := now.Sub(m.lastSend); since < d {
			wait := d - since
			m.throttleMu.Unlock()
			time.Sleep(wait)
			m.throttleMu.Lock()
			now = time.Now()
		}
	}
	if lim.MaxMessagesPerSecond > 0 {
		minGap := time.Second / time.Duration(lim.MaxMessagesPerSecond)
		if minGap > 0 && !m.lastSend.IsZero() {
			if since := now.Sub(m.lastSend); since < minGap {
				m.throttleMu.Unlock()
				time.Sleep(minGap - since)
				m.throttleMu.Lock()
			}
		}
	}
	m.lastSend = time.Now()
	m.throttleMu.Unlock()
}

func (m *Messenger) Flush() error { return nil }
func (m *Messenger) Close() error { return nil }
