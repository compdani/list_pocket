package subimporter

import (
	"reflect"
	"testing"

	"github.com/compdani/list_pocket/models"
)

func TestNormalizeImportTags(t *testing.T) {
	in := []string{" alpha ", "BETA", "beta", "", "  ", "Alpha", "gamma"}
	got := normalizeImportTags(in)
	want := []string{"alpha", "BETA", "gamma"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeImportTags()=%v, want %v", got, want)
	}
}

func TestMergeImportTagsAddsTagsToEmptyAttribs(t *testing.T) {
	got := mergeImportTags(nil, []string{"news", "vip"})
	want := models.JSON{
		"tags": []string{"news", "vip"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeImportTags()=%v, want %v", got, want)
	}
}

func TestMergeImportTagsMergesExistingTags(t *testing.T) {
	attribs := models.JSON{
		"tags": []any{"existing", "VIP"},
	}

	got := mergeImportTags(attribs, []string{"vip", "new"})
	want := models.JSON{
		"tags": []string{"existing", "VIP", "new"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeImportTags()=%v, want %v", got, want)
	}
}
