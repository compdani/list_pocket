package main

import (
	"fmt"
	"strings"

	"github.com/compdani/list_pocket/internal/workflow"
	"github.com/compdani/list_pocket/models"
)

const campaignDripAttribKey = "drip_automation"

type campaignDripMetadata struct {
	Enabled    bool
	WorkflowID string
	TriggerTag string
}

func parseCampaignDripMetadata(attribs models.JSON) campaignDripMetadata {
	if attribs == nil {
		return campaignDripMetadata{}
	}

	raw, ok := attribs[campaignDripAttribKey]
	if !ok {
		return campaignDripMetadata{}
	}

	obj, ok := raw.(map[string]any)
	if !ok {
		return campaignDripMetadata{}
	}

	return campaignDripMetadata{
		Enabled:    asBool(obj["enabled"]),
		WorkflowID: strings.TrimSpace(asMapString(obj["workflow_id"])),
		TriggerTag: strings.TrimSpace(asMapString(obj["trigger_tag"])),
	}
}

func mergeCampaignDripMetadata(attribs models.JSON, meta campaignDripMetadata) models.JSON {
	out := models.JSON{}
	for key, value := range attribs {
		out[key] = value
	}

	out[campaignDripAttribKey] = map[string]any{
		"enabled":     meta.Enabled,
		"workflow_id": meta.WorkflowID,
		"trigger_tag": meta.TriggerTag,
	}

	return out
}

func asBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

func asMapString(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}

func (a *App) syncCampaignDripAutomation(camp models.Campaign, enabled bool, existing campaignDripMetadata) (models.Campaign, error) {
	result, err := workflow.EnsureCampaignDripWorkflow(a.pb, workflow.CampaignDripOptions{
		CampaignRecordID:   camp.RecordID,
		CampaignName:       camp.Name,
		Enabled:            enabled,
		ExistingWorkflowID: existing.WorkflowID,
		ExistingTriggerTag: existing.TriggerTag,
	})
	if err != nil {
		return camp, err
	}

	meta := campaignDripMetadata{
		Enabled:    enabled,
		WorkflowID: result.WorkflowID,
		TriggerTag: result.TriggerTag,
	}
	rec, err := a.pb.FindRecordById("campaigns", camp.RecordID)
	if err != nil {
		return camp, err
	}
	rec.Set("attribs", mergeCampaignDripMetadata(camp.Attribs, meta))
	if err := a.pb.Save(rec); err != nil {
		return camp, err
	}

	return a.core.GetCampaign(camp.RecordID, "", "")
}
