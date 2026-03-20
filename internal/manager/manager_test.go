package manager

import (
	"io"
	"log"
	"testing"

	"github.com/compdani/list_pocket/models"
)

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
