package subimporter

import (
	"strings"

	"github.com/compdani/list_pocket/models"
)

// NormalizeImportTags deduplicates tags case-insensitively while preserving first-seen casing.
func NormalizeImportTags(tags []string) []string {
	return normalizeImportTags(tags)
}

// MergeImportTags merges tags into subscriber attribs without removing existing tags.
func MergeImportTags(attribs models.JSON, tags []string) models.JSON {
	return mergeImportTags(attribs, tags)
}

// RemoveImportTags removes the given tags from attribs (case-insensitive match).
func RemoveImportTags(attribs models.JSON, tags []string) models.JSON {
	return removeImportTags(attribs, tags)
}

// ApplyImportTagChanges merges tagsAdd and removes tagsRemove from attribs.
func ApplyImportTagChanges(attribs models.JSON, tagsAdd, tagsRemove []string) models.JSON {
	attribs = mergeImportTags(attribs, tagsAdd)
	return removeImportTags(attribs, tagsRemove)
}

func removeImportTags(attribs models.JSON, tags []string) models.JSON {
	if len(tags) == 0 {
		return attribs
	}
	if attribs == nil {
		return attribs
	}

	existing := tagsFromAny(attribs["tags"])
	if len(existing) == 0 {
		return attribs
	}

	removeSet := make(map[string]struct{}, len(tags))
	for _, tag := range normalizeImportTags(tags) {
		removeSet[strings.ToLower(tag)] = struct{}{}
	}

	out := make([]string, 0, len(existing))
	for _, tag := range existing {
		if _, ok := removeSet[strings.ToLower(tag)]; !ok {
			out = append(out, tag)
		}
	}

	if len(out) > 0 {
		attribs["tags"] = out
	} else {
		delete(attribs, "tags")
	}
	return attribs
}
