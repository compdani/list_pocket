package models

import (
	"fmt"
	"strings"
)

const CampaignAttribBatching = "batching"

type CampaignBatching struct {
	Enabled     bool
	BatchSize   int
	RepeatValue int
	RepeatUnit  string
	Days        []string
	StartTime   string
	EndTime     string
	Timezone    string
}

func (c Campaign) Batching() CampaignBatching {
	return ParseCampaignBatching(c.Attribs)
}

func ParseCampaignBatching(attribs JSON) CampaignBatching {
	out := CampaignBatching{}
	if attribs == nil {
		return out
	}

	raw, ok := attribs[CampaignAttribBatching]
	if !ok {
		return out
	}

	obj, ok := raw.(map[string]any)
	if !ok {
		return out
	}

	out.Enabled = asBatchBool(obj["enabled"])
	out.BatchSize = asBatchInt(obj["batch_size"])
	out.RepeatValue = asBatchInt(obj["repeat_value"])
	out.RepeatUnit = strings.ToLower(strings.TrimSpace(asBatchString(obj["repeat_unit"])))
	out.StartTime = strings.TrimSpace(asBatchString(obj["start_time"]))
	out.EndTime = strings.TrimSpace(asBatchString(obj["end_time"]))
	out.Timezone = strings.TrimSpace(asBatchString(obj["timezone"]))
	out.Days = normalizeBatchDays(asBatchStrings(obj["days"]))
	return out
}

func MergeCampaignBatching(attribs JSON, cfg CampaignBatching) JSON {
	out := JSON{}
	for key, value := range attribs {
		out[key] = value
	}

	out[CampaignAttribBatching] = map[string]any{
		"enabled":      cfg.Enabled,
		"batch_size":   cfg.BatchSize,
		"repeat_value": cfg.RepeatValue,
		"repeat_unit":  cfg.RepeatUnit,
		"days":         normalizeBatchDays(cfg.Days),
		"start_time":   strings.TrimSpace(cfg.StartTime),
		"end_time":     strings.TrimSpace(cfg.EndTime),
		"timezone":     strings.TrimSpace(cfg.Timezone),
	}

	return out
}

func normalizeBatchDays(days []string) []string {
	if len(days) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(days))
	out := make([]string, 0, len(days))
	for _, day := range days {
		normalized := strings.ToLower(strings.TrimSpace(day))
		switch normalized {
		case "mon", "tue", "wed", "thu", "fri", "sat", "sun":
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			out = append(out, normalized)
		}
	}
	return out
}

func asBatchBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

func asBatchInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		var out int
		_, _ = fmt.Sscanf(strings.TrimSpace(v), "%d", &out)
		return out
	default:
		return 0
	}
}

func asBatchString(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%v", value)
}

func asBatchStrings(value any) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, asBatchString(item))
		}
		return out
	default:
		return nil
	}
}
