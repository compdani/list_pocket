package manager

import (
	"errors"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/compdani/list_pocket/models"
)

type testStore struct{}

func (testStore) NextCampaigns([]int64, []int64) ([]*models.Campaign, error)  { return nil, nil }
func (testStore) NextSubscribers(int, int) ([]models.Subscriber, bool, error) { return nil, false, nil }
func (testStore) GetCampaignLedgerMessageID(int, string) (string, error)      { return "", nil }
func (testStore) GetCampaign(int) (*models.Campaign, error)                   { return nil, nil }
func (testStore) GetAttachment(int) (models.Attachment, error)                { return models.Attachment{}, nil }
func (testStore) UpdateCampaignStatus(int, string) error                      { return nil }
func (testStore) ScheduleCampaignBatch(int, time.Time) error                  { return nil }
func (testStore) UpdateCampaignCounts(int, int, int, int) error               { return nil }
func (testStore) MarkCampaignLedgerSent(int, string) error                    { return nil }
func (testStore) MarkCampaignLedgerSkipped(int, string) error                 { return nil }
func (testStore) RollbackCampaignLedgerInflight(int, string) error            { return nil }
func (testStore) ResetCampaignLedgerInflight(int) (int64, error)              { return 0, nil }
func (testStore) FinalizeCampaignLedgerStats(int) error                       { return nil }
func (testStore) MarkSMSUnsendable(string, string) (int64, error)             { return 0, nil }
func (testStore) CreateLink(url string) (string, error)                       { return "linkrid01234501", nil }
func (testStore) CreateTransactionalMessage(msg models.TransactionalMessage) (models.TransactionalMessage, error) {
	return msg, nil
}
func (testStore) UpdateTransactionalMessageStatus(string, string, string, bool) error {
	return nil
}
func (testStore) BlocklistSubscriber(int64) error { return nil }
func (testStore) DeleteSubscriber(int64) error    { return nil }

func TestGenericTemplateFuncsAllowCampaignPlaceholdersInTransactionalTemplates(t *testing.T) {
	m := New(Config{}, nil, nil, log.New(io.Discard, "", 0))
	tpl := models.Template{
		Type: models.TemplateTypeTx,
		Body: `<a href="{{ UnsubscribeURL }}">unsubscribe</a>{{ TrackView }}`,
	}

	if err := tpl.Compile(m.GenericTemplateFuncs()); err != nil {
		t.Fatalf("expected transactional template to compile with generic func map: %v", err)
	}
}

func TestTemplateFuncsPreferCampaignOverrides(t *testing.T) {
	m := New(Config{
		UnsubURL:           "https://example.com/s/%s/%s",
		LinkTrackURL:       "https://example.com/l/%s/%s/%s",
		ViewTrackURL:       "https://example.com/v/%s/%s/px.png",
		IndividualTracking: true,
	}, testStore{}, nil, log.New(io.Discard, "", 0))

	camp := models.Campaign{
		Base:        models.Base{RecordID: "camprid01234501"},
		UUID:        "camp-uuid",
		Subject:     "Test",
		FromEmail:   "team@example.com",
		ContentType: models.CampaignContentTypeVisual,
		Body:        `<a href="{{ TrackLink "https://example.com" . }}">tracked</a><a href="{{ UnsubscribeURL }}">unsubscribe</a>{{ TrackView }}`,
	}

	if err := camp.CompileTemplate(m.TemplateFuncs(&camp)); err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	msg, err := m.NewCampaignMessage(&camp, models.Subscriber{
		Base:  models.Base{RecordID: "subrid012345678"},
		UUID:  "sub-uuid",
		Email: "person@example.com",
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	body := string(msg.Body())
	if !strings.Contains(body, "https://example.com/s/camprid01234501/subrid012345678") {
		t.Fatalf("expected unsubscribe URL in body, got %q", body)
	}
	if !strings.Contains(body, "https://example.com/v/camprid01234501/subrid012345678/px.png") {
		t.Fatalf("expected tracking pixel in body, got %q", body)
	}
	if !strings.Contains(body, "https://example.com/l/linkrid01234501/camprid01234501/subrid012345678") {
		t.Fatalf("expected tracked link in body, got %q", body)
	}
}

func TestNextBatchScheduleTimeAnchorsToOriginalSchedule(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*60*60)
	anchor := time.Date(2026, time.March, 20, 15, 0, 0, 0, loc)
	now := time.Date(2026, time.March, 20, 16, 1, 0, 0, loc)

	next := nextBatchScheduleTime(anchor, now, models.CampaignBatching{
		Enabled:     true,
		RepeatValue: 1,
		RepeatUnit:  "hours",
	})

	want := time.Date(2026, time.March, 20, 17, 0, 0, 0, loc)
	if !next.Equal(want) {
		t.Fatalf("expected next batch at %s, got %s", want, next)
	}
}

func TestNextBatchScheduleTimeSupportsQuarterHourIntervals(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*60*60)
	anchor := time.Date(2026, time.March, 20, 15, 0, 0, 0, loc)
	now := time.Date(2026, time.March, 20, 15, 16, 0, 0, loc)

	next := nextBatchScheduleTime(anchor, now, models.CampaignBatching{
		Enabled:     true,
		RepeatValue: 2,
		RepeatUnit:  "quarter_hours",
	})

	want := time.Date(2026, time.March, 20, 15, 30, 0, 0, loc)
	if !next.Equal(want) {
		t.Fatalf("expected next batch at %s, got %s", want, next)
	}
}

func TestNextBatchScheduleTimeRespectsDailyWindow(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*60*60)
	anchor := time.Date(2026, time.March, 20, 21, 0, 0, 0, loc)
	now := time.Date(2026, time.March, 20, 22, 1, 0, 0, loc)

	next := nextBatchScheduleTime(anchor, now, models.CampaignBatching{
		Enabled:     true,
		RepeatValue: 1,
		RepeatUnit:  "hours",
		StartTime:   "07:00",
		EndTime:     "22:00",
	})

	want := time.Date(2026, time.March, 21, 7, 0, 0, 0, loc)
	if !next.Equal(want) {
		t.Fatalf("expected next batch at %s, got %s", want, next)
	}
}

func TestNextBatchScheduleTimeAppliesBatchWindowInConfiguredTimezone(t *testing.T) {
	anchor := time.Date(2026, time.March, 21, 7, 0, 0, 0, time.UTC)
	now := time.Date(2026, time.March, 21, 7, 1, 0, 0, time.UTC)

	next := nextBatchScheduleTime(anchor, now, models.CampaignBatching{
		Enabled:     true,
		RepeatValue: 1,
		RepeatUnit:  "hours",
		StartTime:   "07:00",
		EndTime:     "22:00",
		Timezone:    "America/Chicago",
	})

	want := time.Date(2026, time.March, 21, 12, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("expected next batch at %s, got %s", want, next)
	}
}

func TestNextBatchScheduleTimeRespectsAllowedWeekdays(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*60*60)
	anchor := time.Date(2026, time.March, 20, 15, 0, 0, 0, loc) // Friday
	now := time.Date(2026, time.March, 20, 15, 1, 0, 0, loc)

	next := nextBatchScheduleTime(anchor, now, models.CampaignBatching{
		Enabled:     true,
		RepeatValue: 1,
		RepeatUnit:  "hours",
		Days:        []string{"mon"},
		StartTime:   "09:00",
	})

	want := time.Date(2026, time.March, 23, 9, 0, 0, 0, loc) // Monday
	if !next.Equal(want) {
		t.Fatalf("expected next batch at %s, got %s", want, next)
	}
}

func TestNextBatchScheduleTimeUsesNowWhenAnchorIsZero(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*60*60)
	now := time.Date(2026, time.March, 20, 10, 1, 0, 0, loc)

	next := nextBatchScheduleTime(time.Time{}, now, models.CampaignBatching{
		Enabled:     true,
		RepeatValue: 1,
		RepeatUnit:  "hours",
	})

	want := time.Date(2026, time.March, 20, 11, 1, 0, 0, loc)
	if !next.Equal(want) {
		t.Fatalf("expected next batch at %s, got %s", want, next)
	}
}

func TestNextBatchScheduleTimeFallsBackOnInvalidTimezone(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*60*60)
	anchor := time.Date(2026, time.March, 20, 15, 0, 0, 0, loc)
	now := time.Date(2026, time.March, 20, 16, 1, 0, 0, loc)

	next := nextBatchScheduleTime(anchor, now, models.CampaignBatching{
		Enabled:     true,
		RepeatValue: 1,
		RepeatUnit:  "hours",
		Timezone:    "Mars/Phobos",
	})

	want := time.Date(2026, time.March, 20, 17, 0, 0, 0, loc)
	if !next.Equal(want) {
		t.Fatalf("expected next batch at %s, got %s", want, next)
	}
}

func TestNextBatchScheduleTimeReturnsNowWhenBatchingDisabled(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*60*60)
	anchor := time.Date(2026, time.March, 20, 15, 0, 0, 0, loc)
	now := time.Date(2026, time.March, 20, 16, 1, 0, 0, loc)

	next := nextBatchScheduleTime(anchor, now, models.CampaignBatching{
		Enabled:     false,
		RepeatValue: 1,
		RepeatUnit:  "hours",
	})

	if !next.Equal(now) {
		t.Fatalf("expected next batch at %s, got %s", now, next)
	}
}

func TestNextBatchScheduleTimeWithInvalidStartTimeStillSchedules(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*60*60)
	anchor := time.Date(2026, time.March, 20, 21, 0, 0, 0, loc)
	now := time.Date(2026, time.March, 20, 22, 1, 0, 0, loc)

	next := nextBatchScheduleTime(anchor, now, models.CampaignBatching{
		Enabled:     true,
		RepeatValue: 1,
		RepeatUnit:  "hours",
		StartTime:   "not-a-time",
		EndTime:     "22:00",
	})

	want := time.Date(2026, time.March, 21, 0, 0, 0, 0, loc)
	if !next.Equal(want) {
		t.Fatalf("expected next batch at %s, got %s", want, next)
	}
}

func TestNewCampaignMessageAddressesByChannel(t *testing.T) {
	m := New(Config{UnsubURL: "https://example.com/s/%s/%s"}, testStore{}, nil, log.New(io.Discard, "", 0))
	sub := models.Subscriber{
		Base:  models.Base{RecordID: "subrid012345678"},
		Email: "person@example.com",
		Phone: " +15550100 ",
	}

	quoCamp := models.Campaign{
		Base:      models.Base{RecordID: "camprid01234501"},
		Messenger: models.CampaignMessengerQuo,
		FromEmail: "legacy@example.com",
		Body:      "hello",
	}
	if err := quoCamp.CompileTemplate(m.TemplateFuncs(&quoCamp)); err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	msg, err := m.NewCampaignMessage(&quoCamp, sub)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if msg.to != "+15550100" {
		t.Fatalf("quo should send to the trimmed phone, got %q", msg.to)
	}
	if msg.from != "" {
		t.Fatalf("quo should drop an email-shaped from, got %q", msg.from)
	}

	mailCamp := models.Campaign{
		Base:        models.Base{RecordID: "camprid01234502"},
		Messenger:   "email",
		FromEmail:   "team@example.com",
		ContentType: models.CampaignContentTypeVisual,
		Body:        "hello",
	}
	if err := mailCamp.CompileTemplate(m.TemplateFuncs(&mailCamp)); err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	msg, err = m.NewCampaignMessage(&mailCamp, sub)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if msg.to != sub.Email || msg.from != "team@example.com" {
		t.Fatalf("email should keep address and from, got to=%q from=%q", msg.to, msg.from)
	}
}

type statusRecordingStore struct {
	testStore
	status string
}

func (s *statusRecordingStore) UpdateCampaignStatus(_ int, status string) error {
	s.status = status
	return nil
}

func TestNewPipePausesCampaignOnMissingPluginChannel(t *testing.T) {
	store := &statusRecordingStore{}
	m := New(Config{}, store, nil, log.New(io.Discard, "", 0))
	if _, err := m.newPipe(&models.Campaign{Messenger: "ch:gone"}); err == nil {
		t.Fatal("expected an error for a missing plugin channel")
	}
	if store.status != models.CampaignStatusPaused {
		t.Fatalf("status = %q, want paused so the campaign can resume", store.status)
	}

	store.status = ""
	if _, err := m.newPipe(&models.Campaign{Messenger: "no-such-postback"}); err == nil {
		t.Fatal("expected an error for an unknown messenger")
	}
	if store.status != models.CampaignStatusCancelled {
		t.Fatalf("status = %q, want the existing cancel behaviour", store.status)
	}
}

func TestCloseJoinsRunAndRejectsProducers(t *testing.T) {
	m := New(Config{ScanCampaigns: true, ScanInterval: time.Millisecond}, testStore{}, nil, log.New(io.Discard, "", 0))
	done := make(chan struct{})
	go func() { m.Run(); close(done) }()
	time.Sleep(5 * time.Millisecond)
	m.Close()
	m.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop")
	}
	if err := m.PushMessage(models.Message{}); err == nil {
		t.Fatal("accepted message after close")
	}
	if err := m.PushCampaignMessage(CampaignMessage{}); err == nil {
		t.Fatal("accepted campaign after close")
	}
	m.ScanCampaignsOnce()
}

func TestCloseBeforeRun(t *testing.T) {
	m := New(Config{}, testStore{}, nil, log.New(io.Discard, "", 0))
	m.Close()
	m.Run()
}

func TestShutdownInterruptsLongRateLimitWait(t *testing.T) {
	m := New(Config{}, testStore{}, nil, log.New(io.Discard, "", 0))
	waiting := make(chan bool, 1)
	go func() { waiting <- m.waitOrStop(time.Hour) }()
	m.Close()
	select {
	case elapsed := <-waiting:
		if elapsed {
			t.Fatal("wait elapsed instead of observing shutdown")
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not interrupt wait")
	}
}

type blockingDeliveryMessenger struct {
	entered chan struct{}
	release chan struct{}
}

func (*blockingDeliveryMessenger) Name() string { return "email" }
func (m *blockingDeliveryMessenger) Push(models.Message) error {
	close(m.entered)
	<-m.release
	return nil
}
func (*blockingDeliveryMessenger) Flush() error { return nil }
func (*blockingDeliveryMessenger) Close() error { return nil }

func TestCloseJoinsDeliveryWithoutHoldingAdmissionLock(t *testing.T) {
	m := New(Config{}, testStore{}, nil, log.New(io.Discard, "", 0))
	messenger := &blockingDeliveryMessenger{entered: make(chan struct{}), release: make(chan struct{})}
	if err := m.AddMessenger(messenger); err != nil {
		t.Fatal(err)
	}
	go m.Run()
	for {
		m.lifecycle.RLock()
		started := m.started
		m.lifecycle.RUnlock()
		if started {
			break
		}
		time.Sleep(time.Millisecond)
	}
	sent := make(chan error, 1)
	go func() { sent <- m.DeliverMessage(models.Message{Messenger: "email"}) }()
	<-messenger.entered
	closed := make(chan struct{})
	go func() { m.Close(); close(closed) }()
	select {
	case <-m.stop:
	case <-time.After(time.Second):
		t.Fatal("provider call held admission lock")
	}
	select {
	case <-closed:
		t.Fatal("closed before delivery completed")
	default:
	}
	close(messenger.release)
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("close failed to join delivery")
	}
}

func TestSaturatedAdmissionCannotBlockStopSignal(t *testing.T) {
	m := New(Config{Concurrency: 1}, testStore{}, nil, log.New(io.Discard, "", 0))
	provider := &blockingDeliveryMessenger{entered: make(chan struct{}), release: make(chan struct{})}
	if err := m.AddMessenger(provider); err != nil {
		t.Fatal(err)
	}
	go m.Run()
	for {
		m.lifecycle.RLock()
		started := m.started
		m.lifecycle.RUnlock()
		if started {
			break
		}
		time.Sleep(time.Millisecond)
	}
	first := make(chan error, 1)
	second := make(chan error, 1)
	third := make(chan error, 1)
	go func() { first <- m.DeliverMessage(models.Message{Messenger: "email"}) }()
	<-provider.entered
	go func() { second <- m.DeliverMessage(models.Message{Messenger: "email"}) }()
	deadline := time.Now().Add(time.Second)
	for len(m.deliveryQ) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("second delivery not queued")
		}
		time.Sleep(time.Millisecond)
	}
	go func() { third <- m.DeliverMessage(models.Message{Messenger: "email"}) }()
	// The only admission reader left is the submitter blocked on the full queue.
	for m.lifecycle.TryLock() {
		m.lifecycle.Unlock()
		if time.Now().After(deadline) {
			t.Fatal("third delivery not waiting in admission")
		}
		time.Sleep(time.Millisecond)
	}
	closed := make(chan struct{})
	go func() { m.Close(); close(closed) }()
	select {
	case <-m.stop:
	case <-time.After(time.Second):
		t.Fatal("saturated admission blocked stop")
	}
	select {
	case err := <-third:
		if !errors.Is(err, models.ErrNotAttempted) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("submitter did not unblock")
	}
	close(provider.release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; !errors.Is(err, models.ErrNotAttempted) {
		t.Fatalf("queued delivery attempted: %v", err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not finish")
	}
}

func TestDeliveryCadencePreservesConcurrencyBudget(t *testing.T) {
	m := New(Config{Concurrency: 10, MessageRate: 10, SlidingWindow: true, SlidingWindowDuration: time.Minute, SlidingWindowRate: 1000}, testStore{}, nil, log.New(io.Discard, "", 0))
	rate, window, count := m.deliveryLimits("email")
	if rate != 100 || time.Second/time.Duration(rate) != 10*time.Millisecond {
		t.Fatalf("expected100/s, got%d", rate)
	}
	if window != time.Minute || count != 1000 {
		t.Fatalf("sliding window lost: %s/%d", window, count)
	}
	// A channel's explicit cap overrides the generic per-worker budget globally.
	models.RegisterChannel(models.ChannelSpec{ID: "ch:cadence", Label: "Cadence", MaxPerSecond: 7})
	rate, _, _ = m.deliveryLimits("ch:cadence")
	if rate != 7 {
		t.Fatalf("channel cap multiplied by concurrency: %d", rate)
	}
}

func TestRateReservationsPreserveThroughputAndChannelIndependence(t *testing.T) {
	m := New(Config{Concurrency: 10, MessageRate: 10}, testStore{}, nil, log.New(io.Discard, "", 0))
	now := time.Now()
	for i := 0; i < 100; i++ {
		if wait := m.reserveDeliverySlot("email", now); wait != time.Duration(i)*10*time.Millisecond {
			t.Fatalf("slot%d=%s", i, wait)
		}
	}
	models.RegisterChannel(models.ChannelSpec{ID: "ch:slow", Label: "Slow", MaxPerSecond: 1})
	if wait := m.reserveDeliverySlot("ch:slow", now); wait != 0 {
		t.Fatal(wait)
	}
	if wait := m.reserveDeliverySlot("ch:slow", now); wait != time.Second {
		t.Fatal(wait)
	}
	// The slow channel's future reservation does not shift email's own timeline.
	if wait := m.reserveDeliverySlot("email", now); wait != time.Second {
		t.Fatalf("email blocked by slow channel: %s", wait)
	}
}
func TestSlidingReservationsAllowBurstBeforeWindowHold(t *testing.T) {
	m := New(Config{Concurrency: 1, MessageRate: 100, SlidingWindow: true, SlidingWindowDuration: time.Minute, SlidingWindowRate: 2}, testStore{}, nil, log.New(io.Discard, "", 0))
	now := time.Now()
	for i, want := range []time.Duration{0, 10 * time.Millisecond, time.Minute, time.Minute + 10*time.Millisecond, 2 * time.Minute} {
		if got := m.reserveDeliverySlot("email", now); got != want {
			t.Fatalf("slot%d=%s want%s", i, got, want)
		}
	}
}
