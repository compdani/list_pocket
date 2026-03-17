package workflow

import (
	"context"
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

func handleSubscriberTagWorkflowTriggers(app core.App, subscriber *core.Record) error {
	if subscriber == nil {
		return nil
	}

	beforeTags := subscriberTags(subscriber.Original())
	afterTags := subscriberTags(subscriber)
	addedTags, removedTags := diffTags(beforeTags, afterTags)
	if len(addedTags) == 0 && len(removedTags) == 0 {
		return nil
	}

	if err := dispatchContactTagRuns(app, subscriber, "tag_added", addedTags, beforeTags, afterTags); err != nil {
		return err
	}
	return dispatchContactTagRuns(app, subscriber, "tag_removed", removedTags, beforeTags, afterTags)
}

func dispatchContactTagRuns(app core.App, subscriber *core.Record, triggerMode string, changedTags []string, beforeTags []string, afterTags []string) error {
	if len(changedTags) == 0 {
		return nil
	}

	triggerNodes, err := app.FindRecordsByFilter("workflow_nodes", `is_current=true && type="trigger"`, "", 500, 0)
	if err != nil {
		return err
	}

	for _, triggerNode := range triggerNodes {
		config := toStringAnyMap(triggerNode.Get("config"))
		if asString(config["mode"], "manual") != triggerMode {
			continue
		}
		tagName := asString(config["tagName"], "")
		for _, changedTag := range changedTags {
			if tagName != "" && tagName != changedTag {
				continue
			}

			workflow, err := app.FindRecordById("workflows", triggerNode.GetString("workflow"))
			if err != nil {
				return err
			}
			nodes, edges, err := loadWorkflowGraph(app, workflow.Id)
			if err != nil {
				return err
			}
			compiledSnapshot, err := buildCompiledWorkflowSnapshot(workflow, nodes, edges)
			if err != nil {
				return err
			}

			run, err := createWorkflowRunRecord(app, workflow, compiledSnapshot, map[string]any{
				"mode":       triggerMode,
				"source":     "contact_tag_update",
				"contactId":  subscriber.Id,
				"tag":        changedTag,
				"tagsBefore": beforeTags,
				"tagsAfter":  afterTags,
			}, subscriber, "", fmt.Sprintf("Contact tag %s triggered by %q.", triggerModeLabel(triggerMode), changedTag))
			if err != nil {
				return err
			}

			_ = executeQueuedRun(context.Background(), app, newRunEngineForApp(app), run)
		}
	}

	return nil
}

func subscriberTags(record *core.Record) []string {
	if record == nil {
		return nil
	}

	attribs := toStringAnyMap(record.Get("attribs"))
	if tags, ok := attribs["tags"]; ok {
		return toStringSlice(tags)
	}
	return nil
}

func diffTags(before []string, after []string) ([]string, []string) {
	beforeSet := make(map[string]struct{}, len(before))
	afterSet := make(map[string]struct{}, len(after))
	for _, tag := range before {
		beforeSet[tag] = struct{}{}
	}
	for _, tag := range after {
		afterSet[tag] = struct{}{}
	}

	added := make([]string, 0)
	for _, tag := range after {
		if _, ok := beforeSet[tag]; !ok {
			added = append(added, tag)
		}
	}

	removed := make([]string, 0)
	for _, tag := range before {
		if _, ok := afterSet[tag]; !ok {
			removed = append(removed, tag)
		}
	}

	return added, removed
}

func triggerModeLabel(mode string) string {
	switch mode {
	case "tag_added":
		return "added"
	case "tag_removed":
		return "removed"
	default:
		return mode
	}
}
