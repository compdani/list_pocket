package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/compdani/list_pocket/internal/workflow/domain"
	"github.com/compdani/list_pocket/internal/workflow/executor"
	"github.com/pocketbase/pocketbase/core"
)

var runWorkerOnce sync.Once
var transactionalEmailSender executor.TransactionalEmailSendFunc

func newRunEngineForApp(app core.App) *executor.Engine {
	return executor.NewEngine(
		app,
		executor.TriggerExecutor{},
		executor.ScriptExecutor{},
		executor.ConditionExecutor{},
		executor.EventStartExecutor{},
		executor.HTTPExecutor{},
		executor.PocketBaseExecutor{},
		executor.TransactionalEmailExecutor{Send: transactionalEmailSender},
		executor.CampaignLaunchExecutor{},
		executor.WaitExecutor{},
	)
}

// StartRunWorker starts the background workflow executor loop once for the process.
func StartRunWorker(app core.App) {
	runWorkerOnce.Do(func() {
		engine := newRunEngineForApp(app)
		go func() {
			ticker := time.NewTicker(1500 * time.Millisecond)
			defer ticker.Stop()
			for {
				processQueuedRuns(app, engine)
				processWaitingRuns(app, engine)
				<-ticker.C
			}
		}()
	})
}

func processQueuedRuns(app core.App, engine *executor.Engine) {
	runs, err := app.FindRecordsByFilter("workflow_runs", `status="queued"`, "created", 10, 0)
	if err != nil {
		return
	}
	for _, runRecord := range runs {
		_ = executeQueuedRun(context.Background(), app, engine, runRecord)
	}
}

func processWaitingRuns(app core.App, engine *executor.Engine) {
	now := time.Now().UTC().Format(time.RFC3339)
	runs, err := app.FindRecordsByFilter("workflow_runs", fmt.Sprintf(`status="waiting" && wake_at!="" && wake_at<="%s"`, now), "wake_at", 10, 0)
	if err != nil {
		return
	}
	for _, runRecord := range runs {
		_ = executeQueuedRun(context.Background(), app, engine, runRecord)
	}
}

func executeQueuedRun(ctx context.Context, app core.App, engine *executor.Engine, runRecord *core.Record) error {
	runRecord.Set("status", "running")
	runRecord.Set("summary", "Workflow run is executing.")
	runRecord.Set("ended_at", nil)
	runRecord.Set("wake_at", nil)
	if err := app.Save(runRecord); err != nil {
		return err
	}

	run, compiled, contact, company, err := materializeRunExecution(app, runRecord)
	if err != nil {
		runRecord.Set("status", "failed")
		runRecord.Set("summary", err.Error())
		runRecord.Set("ended_at", time.Now().UTC())
		_ = app.Save(runRecord)
		return err
	}

	result, execErr := engine.ExecuteFrom(ctx, compiled, run, run.ResumeFromNode, run.CurrentPayload, run.RuntimeContext, contact, company)
	if persistErr := persistNodeRuns(app, runRecord, result.NodeRuns); persistErr != nil && execErr == nil {
		execErr = persistErr
	}

	endedAt := time.Now().UTC()
	runRecord.Set("current_payload", result.CurrentPayload)
	runRecord.Set("runtime_context", result.RunContext)
	runRecord.Set("resume_from_node", result.NextNodeID)
	if execErr != nil {
		runRecord.Set("ended_at", endedAt)
		runRecord.Set("status", "failed")
		runRecord.Set("summary", fmt.Sprintf("Workflow run failed: %s", execErr.Error()))
		return app.Save(runRecord)
	}

	finalStatus := "success"
	summary := fmt.Sprintf("Workflow completed successfully through %d nodes.", len(result.NodeRuns))
	for _, nodeRun := range result.NodeRuns {
		switch nodeRun.Status {
		case domain.RunStatusWaiting:
			finalStatus = "waiting"
			if result.WakeAt != nil {
				runRecord.Set("wake_at", *result.WakeAt)
				summary = fmt.Sprintf("Workflow paused until %s.", result.WakeAt.UTC().Format(time.RFC3339))
			} else {
				summary = "Workflow paused in a waiting step."
			}
		case domain.RunStatusFailed:
			finalStatus = "failed"
			summary = fmt.Sprintf("Workflow failed at node %s.", nodeRun.NodeID)
		}
	}

	runRecord.Set("status", finalStatus)
	runRecord.Set("summary", summary)
	if finalStatus != "waiting" {
		runRecord.Set("ended_at", endedAt)
		runRecord.Set("resume_from_node", "")
		runRecord.Set("wake_at", nil)
	}
	return app.Save(runRecord)
}

func materializeRunExecution(app core.App, runRecord *core.Record) (domain.WorkflowRun, *executor.CompiledWorkflow, *domain.Contact, *domain.Company, error) {
	run := domain.WorkflowRun{
		ID: runRecord.Id, WorkflowID: runRecord.GetString("workflow"), WorkflowVersion: runRecord.GetInt("workflow_version"), Status: domain.RunStatus(runRecord.GetString("status")), TriggerPayload: toStringAnyMap(runRecord.Get("trigger_payload")), RuntimeContext: toStringAnyMap(runRecord.Get("runtime_context")), CurrentPayload: toStringAnyMap(runRecord.Get("current_payload")), ResumeFromNode: runRecord.GetString("resume_from_node"), ContactID: runRecord.GetString("contact"), CompanyID: runRecord.GetString("company"), WakeAt: datePtr(runRecord.GetDateTime("wake_at").Time()),
	}

	compiled, err := compiledWorkflowFromSnapshot(runRecord.Get("compiled_snapshot"))
	if err != nil {
		workflowRecord, workflowErr := app.FindRecordById("workflows", run.WorkflowID)
		if workflowErr != nil {
			return domain.WorkflowRun{}, nil, nil, nil, err
		}
		nodes, edges, graphErr := loadWorkflowGraph(app, workflowRecord.Id)
		if graphErr != nil {
			return domain.WorkflowRun{}, nil, nil, nil, err
		}
		snapshot, snapshotErr := buildCompiledWorkflowSnapshot(workflowRecord, nodes, edges)
		if snapshotErr != nil {
			return domain.WorkflowRun{}, nil, nil, nil, err
		}
		compiled, err = compiledWorkflowFromSnapshot(snapshot)
		if err != nil {
			return domain.WorkflowRun{}, nil, nil, nil, err
		}
	}

	var contact *domain.Contact
	if run.ContactID != "" {
		if contactRecord, err := app.FindRecordById("subscribers", run.ContactID); err == nil {
			attribs := toStringAnyMap(contactRecord.Get("attribs"))
			firstName, lastName := splitSubscriberName(contactRecord.GetString("name"), contactRecord.GetString("email"))
			contact = &domain.Contact{
				ID: contactRecord.Id, FirstName: firstName, LastName: lastName, Email: contactRecord.GetString("email"), Phone: asString(attribs["phone"], ""), Stage: contactRecord.GetString("status"), OwnerID: asString(attribs["owner_id"], ""), CompanyID: asString(attribs["company"], ""), Tags: toStringSlice(attribs["tags"]), Attributes: attribs,
			}
		}
	}

	return run, compiled, contact, nil, nil
}

func persistNodeRuns(app core.App, runRecord *core.Record, nodeRuns []domain.NodeRun) error {
	workflowID := runRecord.GetString("workflow")
	workflowNodes, err := app.FindRecordsByFilter("workflow_nodes", fmt.Sprintf(`workflow="%s" && is_current=true`, workflowID), "", 500, 0)
	if err != nil {
		return err
	}

	nodeRecordIDByStableKey := map[string]string{}
	nodeStableKeyByRecordID := map[string]string{}
	for _, node := range workflowNodes {
		stableKey := stableNodeKey(node)
		nodeRecordIDByStableKey[stableKey] = node.Id
		nodeStableKeyByRecordID[node.Id] = stableKey
	}

	existing, err := app.FindRecordsByFilter("node_runs", fmt.Sprintf(`run="%s"`, runRecord.Id), "", 200, 0)
	if err != nil {
		return err
	}

	existingByNode := map[string]*core.Record{}
	for _, record := range existing {
		nodeID := record.GetString("node")
		stableKey := nodeStableKeyByRecordID[nodeID]
		if stableKey == "" {
			stableKey = nodeID
		}
		existingByNode[stableKey] = record
	}

	for _, nodeRun := range nodeRuns {
		nodeRecordID := nodeRecordIDByStableKey[nodeRun.NodeID]
		if nodeRecordID == "" {
			return fmt.Errorf("node run references unknown node %q", nodeRun.NodeID)
		}

		record := existingByNode[nodeRun.NodeID]
		if record == nil {
			record = core.NewRecord(mustCollection(app, "node_runs"))
		}
		record.Set("run", runRecord.Id)
		record.Set("node", nodeRecordID)
		record.Set("status", string(nodeRun.Status))
		record.Set("input", nodeRun.Input)
		record.Set("output", nodeRun.Output)
		record.Set("logs", nodeRun.Logs)
		record.Set("error", nodeRun.Error)
		if nodeRun.StartedAt != nil {
			record.Set("started_at", *nodeRun.StartedAt)
		}
		if nodeRun.EndedAt != nil {
			record.Set("ended_at", *nodeRun.EndedAt)
		}
		if err := app.Save(record); err != nil {
			return err
		}
	}
	return nil
}

func buildCompiledWorkflowSnapshot(workflowRecord *core.Record, nodes []*core.Record, edges []*core.Record) (map[string]any, error) {
	compiled, err := executor.Compile(domain.Workflow{
		ID: workflowRecord.Id, Name: workflowRecord.GetString("name"), Description: workflowRecord.GetString("description"), Version: workflowRecord.GetInt("version"), Status: domain.WorkflowStatus(workflowRecord.GetString("status")), TriggerType: workflowRecord.GetString("trigger_type"), IsPublished: workflowRecord.GetBool("is_published"),
	}, toDomainNodes(nodes), toDomainEdges(nodes, edges))
	if err != nil {
		return nil, err
	}

	b, err := json.Marshal(compiled)
	if err != nil {
		return nil, err
	}

	out := map[string]any{}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func compiledWorkflowFromSnapshot(snapshot any) (*executor.CompiledWorkflow, error) {
	b, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	var compiled executor.CompiledWorkflow
	if err := json.Unmarshal(b, &compiled); err != nil {
		return nil, err
	}
	return &compiled, nil
}

func toDomainNodes(nodes []*core.Record) []domain.WorkflowNode {
	out := make([]domain.WorkflowNode, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, domain.WorkflowNode{
			ID: stableNodeKey(node), WorkflowID: node.GetString("workflow"), Type: node.GetString("type"), Label: node.GetString("label"), Config: toStringAnyMap(node.Get("config")), Schema: map[string]any{"fields": toSchemaSlice(node.Get("schema"))}, PositionX: node.GetFloat("position_x"), PositionY: node.GetFloat("position_y"), ContactMode: node.GetString("contact_mode"),
		})
	}
	return out
}

func toDomainEdges(nodes []*core.Record, edges []*core.Record) []domain.WorkflowEdge {
	nodeKeysByRecordID := map[string]string{}
	for _, node := range nodes {
		nodeKeysByRecordID[node.Id] = stableNodeKey(node)
	}

	out := make([]domain.WorkflowEdge, 0, len(edges))
	for _, edge := range edges {
		out = append(out, domain.WorkflowEdge{
			ID: stableEdgeKey(edge), WorkflowID: edge.GetString("workflow"), SourceNodeID: nodeKeysByRecordID[edge.GetString("source_node")], TargetNodeID: nodeKeysByRecordID[edge.GetString("target_node")], SourceHandle: edge.GetString("source_handle"), TargetHandle: edge.GetString("target_handle"), Condition: toStringAnyMap(edge.Get("condition")),
		})
	}
	return out
}

func datePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func mustCollection(app core.App, name string) *core.Collection {
	collection, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		panic(err)
	}
	return collection
}

func splitSubscriberName(name string, fallback string) (string, string) {
	name = strings.TrimSpace(name)
	if name == "" {
		if fallback == "" {
			return "Subscriber", ""
		}
		return fallback, ""
	}
	parts := strings.Fields(name)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], strings.Join(parts[1:], " ")
}
