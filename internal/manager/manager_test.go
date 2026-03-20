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

func (testStore) NextCampaigns([]int64, []int64) ([]*models.Campaign, error) { return nil, nil }
func (testStore) NextSubscribers(int, int) ([]models.Subscriber, bool, error) { return nil, false, nil }
func (testStore) GetCampaign(int) (*models.Campaign, error) { return nil, nil }
func (testStore) GetAttachment(int) (models.Attachment, error) { return models.Attachment{}, nil }
func (testStore) UpdateCampaignStatus(int, string) error { return nil }
func (testStore) ScheduleCampaignBatch(int, time.Time) error { return nil }
func (testStore) UpdateCampaignCounts(int, int, int, int) error { return nil }
func (testStore) CreateLink(url string) (string, error) { return "link-uuid", nil }
func (testStore) BlocklistSubscriber(int64) error { return nil }
func (testStore) DeleteSubscriber(int64) error { return nil }

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
		Body: `<a href="{{ TrackLink "https://example.com" . }}">tracked</a><a href="{{ UnsubscribeURL }}">unsubscribe</a>{{ TrackView }}`,
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
