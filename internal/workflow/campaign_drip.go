package workflow

import (
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

type CampaignDripOptions struct {
	CampaignRecordID   string
	CampaignName       string
	Enabled            bool
	ExistingWorkflowID string
	ExistingTriggerTag string
}

type CampaignDripResult struct {
	WorkflowID string
	TriggerTag string
}

func EnsureCampaignDripWorkflow(app core.App, opts CampaignDripOptions) (CampaignDripResult, error) {
	result := CampaignDripResult{
		WorkflowID: strings.TrimSpace(opts.ExistingWorkflowID),
		TriggerTag: strings.TrimSpace(opts.ExistingTriggerTag),
	}
	if result.TriggerTag == "" {
		result.TriggerTag = defaultCampaignDripTag(opts.CampaignRecordID)
	}

	var workflowRecord *core.Record
	if result.WorkflowID != "" {
		record, err := app.FindRecordById("workflows", result.WorkflowID)
		if err == nil {
			workflowRecord = record
		}
	}

	if !opts.Enabled {
		if workflowRecord != nil {
			workflowRecord.Set("status", "archived")
			workflowRecord.Set("is_published", false)
			if err := app.Save(workflowRecord); err != nil {
				return result, err
			}
		}
		return result, nil
	}

	if workflowRecord == nil {
		collection, err := app.FindCollectionByNameOrId("workflows")
		if err != nil {
			return result, err
		}

		workflowRecord = core.NewRecord(collection)
		workflowRecord.Set("description", "")
		workflowRecord.Set("version", 1)
		workflowRecord.Set("status", "draft")
		workflowRecord.Set("trigger_type", "tag_added")
		workflowRecord.Set("is_published", false)
		workflowRecord.Set("compiled_snapshot", map[string]any{})
	}

	workflowRecord.Set("name", defaultCampaignDripName(opts.CampaignName))
	if err := app.Save(workflowRecord); err != nil {
		return result, err
	}
	result.WorkflowID = workflowRecord.Id

	nodes, edges, err := loadWorkflowGraph(app, workflowRecord.Id)
	if err != nil {
		return result, err
	}
	if len(nodes) == 0 && len(edges) == 0 {
		if err := applyWorkflowMutation(app, workflowRecord, workflowMutationRequest{
			Name: workflowRecord.GetString("name"),
			Nodes: []workflowNodeDTO{
				{
					ID:          "trigger",
					Type:        "trigger",
					Label:       "Tag Added Trigger",
					Description: "Starts the drip when a subscriber receives the campaign tag.",
					Config: map[string]any{
						"mode":    "tag_added",
						"tagName": result.TriggerTag,
					},
					Schema: []map[string]any{
						{"key": "mode", "label": "Trigger Mode", "kind": "select", "options": []string{"manual", "webhook", "tag_added", "tag_removed"}},
						{"key": "tagName", "label": "Contact Tag", "kind": "text"},
					},
					PositionX:   180,
					PositionY:   180,
					ContactMode: "contact-event",
				},
				{
					ID:          "launch",
					Type:        "campaign_launch",
					Label:       "Launch Campaign",
					Description: "Starts the campaign when the drip enters this step.",
					Config: map[string]any{
						"campaignId": opts.CampaignRecordID,
					},
					Schema: []map[string]any{
						{"key": "campaignId", "label": "Campaign Record ID", "kind": "text"},
						{"key": "campaignIdPath", "label": "Campaign ID Path", "kind": "text"},
					},
					PositionX:   180,
					PositionY:   360,
					ContactMode: "campaign-launch",
				},
			},
			Edges: []workflowEdgeDTO{
				{
					ID:           "trigger-launch",
					SourceNodeID: "trigger",
					TargetNodeID: "launch",
					SourceHandle: "next",
					TargetHandle: "",
					Condition:    map[string]any{"branch": "next", "expression": ""},
				},
			},
		}); err != nil {
			return result, err
		}
		nodes, edges, err = loadWorkflowGraph(app, workflowRecord.Id)
		if err != nil {
			return result, err
		}
	}

	if findings := validateWorkflowGraph(nodes, edges); len(findings) > 0 {
		return result, fmt.Errorf("workflow cannot be published: %s", strings.Join(findingMessages(findings), "; "))
	}

	workflowRecord, err = app.FindRecordById("workflows", workflowRecord.Id)
	if err != nil {
		return result, err
	}
	workflowRecord.Set("status", "published")
	workflowRecord.Set("is_published", true)
	workflowRecord.Set("name", defaultCampaignDripName(opts.CampaignName))
	if err := app.Save(workflowRecord); err != nil {
		return result, err
	}

	return result, nil
}

func defaultCampaignDripTag(campaignRecordID string) string {
	tag := strings.ToLower(strings.TrimSpace(campaignRecordID))
	if tag == "" {
		tag = "campaign"
	}
	return "drip-" + tag
}

func defaultCampaignDripName(campaignName string) string {
	name := strings.TrimSpace(campaignName)
	if name == "" {
		name = "Campaign"
	}
	return fmt.Sprintf("Drip: %s", name)
}
