package manager

import (
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
func (testStore) GetCampaign(int) (*models.Campaign, error)                   { return nil, nil }
func (testStore) GetAttachment(int) (models.Attachment, error)                { return models.Attachment{}, nil }
func (testStore) UpdateCampaignStatus(int, string) error                      { return nil }
func (testStore) ScheduleCampaignBatch(int, time.Time) error                  { return nil }
func (testStore) UpdateCampaignCounts(int, int, int, int) error               { return nil }
func (testStore) CreateLink(url string) (string, error)                       { return "link-uuid", nil }
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
		UnsubURL:           "https://example.com/sub/%s/%s",
		LinkTrackURL:       "https://example.com/link/%s/%s/%s",
		ViewTrackURL:       "https://example.com/view/%s/%s/px.png",
		IndividualTracking: true,
	}, testStore{}, nil, log.New(io.Discard, "", 0))

	camp := models.Campaign{
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
		UUID:  "sub-uuid",
		Email: "person@example.com",
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	body := string(msg.Body())
	if !strings.Contains(body, "https://example.com/sub/camp-uuid/sub-uuid") {
		t.Fatalf("expected unsubscribe URL in body, got %q", body)
	}
	if !strings.Contains(body, "https://example.com/view/camp-uuid/sub-uuid/px.png") {
		t.Fatalf("expected tracking pixel in body, got %q", body)
	}
	if !strings.Contains(body, "https://example.com/link/link-uuid/camp-uuid/sub-uuid") {
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
