package quo

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/compdani/list_pocket/models"
)

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
	m.throttleSend()
	c, err := m.client()
	if err != nil {
		return err
	}
	if len(msg.To) == 0 {
		return errors.New("missing recipient phone")
	}
	to := strings.TrimSpace(msg.To[0])
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	return c.SendText(ctx, to, msg.Body)
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
