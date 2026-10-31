package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/apperr"
	"github.com/compdani/list_pocket/internal/auth"
	"github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/internal/flow"
	"github.com/compdani/list_pocket/internal/flow/agent"
	"github.com/compdani/list_pocket/internal/flow/llm"
	"github.com/compdani/list_pocket/internal/flow/queue"
	"github.com/compdani/list_pocket/internal/flow/tools"
	"github.com/compdani/list_pocket/internal/plugins"
	"github.com/compdani/list_pocket/models"
	"github.com/pocketbase/dbx"
	pbcore "github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"
)

const (
	maxFlowWebhookBytes = 1 << 20
	flowRunListLimit    = 100
	flowEnrollMaxBatch  = 1000

	eventContactStageChanged = flow.EventContactStageChanged

	permFlowsGet     = "workflows:get"
	permFlowsManage  = "workflows:manage"
	permFlowsRun     = "workflows:run"
	permFlowsApprove = "workflows:approve"
)

// flowEngine runs workflows. It's a separate struct so the App keeps one field for it.
type flowEngine struct {
	engine *flow.Engine
	queue  *queue.Queue
	tools  *tools.Registry
	cancel context.CancelFunc
}

// campaignStatusChanged lets the send pipeline, which is built before the app, report
// that a campaign finished.
var campaignStatusChanged func(campID int, status string)

// channelInboundReceived does the same for inbound channel messages, which the core
// stores without going through record hooks.
var channelInboundReceived func(core.ChannelEvent)

// initFlows builds the workflow engine and, unless passive, starts it.
func (a *App) initFlows(passive bool) {
	registry := tools.NewRegistry()
	for _, t := range a.flowTools() {
		registry.Register(t)
	}
	var presets flow.PresetSource
	if a.plugins != nil {
		a.registerPluginAddons(registry)
		presets = a.plugins
	}
	q := queue.New(a.pb)
	resolver := a.aiResolver()
	runner := &agent.Runner{App: a.pb, LLM: llm.NewClient(), Tools: registry, Resolver: resolver}
	engine := flow.NewEngine(a.pb, runner, presets, q)
	engine.Notify = a.flowNotify
	a.flows = &flowEngine{engine: engine, queue: q, tools: registry}

	if err := engine.RefreshTriggers(); err != nil {
		a.log.Printf("error loading workflow triggers: %v", err)
	}
	a.bindFlowEvents()
	if passive {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.flows.cancel = cancel
	q.Start(ctx)
	if err := engine.Recover(); err != nil {
		a.log.Printf("error recovering workflow runs: %v", err)
	}
	if err := a.pb.Cron().Add("flow-triggers", "* * * * *", func() { engine.Tick(time.Now()) }); err != nil {
		a.log.Printf("error scheduling workflow triggers: %v", err)
	}
}

// flowNotify delivers a notify step to the addresses in notify_emails.
func (a *App) flowNotify(run *pbcore.Record, message string) {
	a.log.Printf("workflow notify run=%s: %s", run.Id, message)
	if len(a.cfg.NotifyEmails) == 0 {
		return
	}
	if err := a.manager.PushMessage(models.Message{
		From: a.cfg.FromEmail, To: a.cfg.NotifyEmails, Subject: "Workflow update", ContentType: models.CampaignContentTypePlain,
		Body: []byte(message), Messenger: emailMsgr,
	}); err != nil {
		a.log.Printf("error sending workflow notification: %v", err)
	}
}

// emitFlowEvent raises a workflow event.
func (a *App) emitFlowEvent(name string, payload map[string]any) {
	if a.flows != nil {
		a.flows.engine.Emit(name, payload)
	}
}

// bindFlowEvents turns List Pocket's own changes into workflow events.
func (a *App) bindFlowEvents() {
	a.flows.engine.RegisterHooks()

	a.pb.OnRecordAfterCreateSuccess("subscribers").BindFunc(func(e *pbcore.RecordEvent) error {
		a.emitFlowEvent(flow.EventContactCreated, map[string]any{"contact_id": e.Record.Id, "email": e.Record.GetString("email"), "name": e.Record.GetString("name")})
		return e.Next()
	})
	a.pb.OnRecordAfterUpdateSuccess("subscribers").BindFunc(func(e *pbcore.RecordEvent) error {
		before, after := recordTags(e.Record.Original()), recordTags(e.Record)
		for tag := range after {
			if !before[tag] {
				a.emitFlowEvent(flow.EventContactTagAdded, map[string]any{"contact_id": e.Record.Id, "email": e.Record.GetString("email"), "tag": tag})
			}
		}
		for tag := range before {
			if !after[tag] {
				a.emitFlowEvent(flow.EventContactTagRemoved, map[string]any{"contact_id": e.Record.Id, "email": e.Record.GetString("email"), "tag": tag})
			}
		}
		return e.Next()
	})
	a.pb.OnRecordAfterCreateSuccess("inbound_email_replies").BindFunc(func(e *pbcore.RecordEvent) error {
		// Spam has already been classified by the time it's stored. It isn't a message
		// anyone should respond to automatically.
		if e.Record.GetString("spam_status") == "" {
			from := strings.ToLower(e.Record.GetString("from_address"))
			contact := e.Record.GetString("subscriber_id")
			if contact == "" {
				if sub, err := a.core.GetSubscriber(0, "", from); err == nil {
					contact = sub.RecordID
				}
			}
			conversation := contact
			if conversation == "" {
				conversation = models.ChannelEmail + ":" + from
			}
			a.emitFlowEvent(flow.EventInboundEmail, map[string]any{
				"conversation": conversation, "contact_id": contact, "from": from, "subject": e.Record.GetString("subject"),
				"snippet": e.Record.GetString("body_snippet"), "email_id": e.Record.Id,
			})
		}
		return e.Next()
	})
	a.pb.OnRecordAfterCreateSuccess("inbound_sms_events").BindFunc(func(e *pbcore.RecordEvent) error {
		contact, from := e.Record.GetString("subscriber_id"), e.Record.GetString("from_number")
		conversation := contact
		if conversation == "" {
			conversation = models.ChannelSMS + ":" + from
		}
		a.emitFlowEvent(flow.EventInboundSMS, map[string]any{
			"conversation": conversation, "contact_id": contact, "from": from,
			"body": e.Record.GetString("message_body"), "is_stop": e.Record.GetBool("is_stop_keyword"),
		})
		return e.Next()
	})
	channelInboundReceived = func(e core.ChannelEvent) {
		conversation := e.SubscriberID
		if conversation == "" {
			conversation = e.Channel + ":" + e.Address
		}
		a.emitFlowEvent(flow.EventInboundChannel, map[string]any{
			"conversation": conversation, "contact_id": e.SubscriberID, "channel": e.Channel, "from": e.Address, "body": e.Body,
		})
	}
	campaignStatusChanged = func(campID int, status string) {
		if status != models.CampaignStatusFinished {
			return
		}
		var row struct {
			ID   string `db:"id"`
			Name string `db:"name"`
		}
		if err := a.pb.DB().NewQuery("SELECT id, name FROM campaigns WHERE rowid = {:id}").Bind(dbx.Params{"id": campID}).One(&row); err != nil {
			a.log.Printf("error reading finished campaign %d: %v", campID, err)
			return
		}
		a.emitFlowEvent(flow.EventCampaignFinished, map[string]any{"campaign_id": row.ID, "name": row.Name})
	}
}

func recordTags(rec *pbcore.Record) map[string]bool {
	out := map[string]bool{}
	if rec == nil {
		return out
	}
	attribs := map[string]any{}
	_ = rec.UnmarshalJSONField("attribs", &attribs)
	for _, t := range tagsFromAttribs(attribs) {
		out[t] = true
	}
	return out
}

func (a *App) flowFromRoute(re *pbcore.RequestEvent) (*pbcore.Record, error) {
	wf, err := a.flows.engine.FindWorkflow(pathParam(re, "id"))
	if err != nil {
		return nil, apperr.NotFound("workflow not found")
	}
	return wf, nil
}

func flowActor(re *pbcore.RequestEvent) flow.Actor {
	return flow.Actor{ID: auth.GetUserRE(re).RecordID}
}

func exportRecords(recs []*pbcore.Record) []map[string]any {
	out := make([]map[string]any, 0, len(recs))
	for _, r := range recs {
		out = append(out, r.PublicExport())
	}
	return out
}

func (a *App) flowSummary(wf *pbcore.Record) map[string]any {
	out := wf.PublicExport()
	delete(out, "draft")
	var last struct {
		ID       string `db:"id"`
		Status   string `db:"status"`
		Created  string `db:"created"`
		Finished string `db:"finished"`
	}
	if err := a.pb.DB().NewQuery("SELECT id, status, created, finished FROM flow_runs WHERE workflow = {:w} ORDER BY created DESC LIMIT 1").
		Bind(dbx.Params{"w": wf.Id}).One(&last); err == nil {
		out["last_run"] = map[string]any{"id": last.ID, "status": last.Status, "created": last.Created, "finished": last.Finished}
	}
	var pending int
	_ = a.pb.DB().NewQuery(`SELECT COUNT(*) FROM flow_approvals a JOIN flow_node_runs n ON a.node_run = n.id JOIN flow_runs r ON n.workflow_run = r.id
		WHERE r.workflow = {:w} AND a.status = 'pending'`).Bind(dbx.Params{"w": wf.Id}).Row(&pending)
	out["pending_approvals"] = pending
	return out
}

// GetFlows lists workflows.
func (a *App) GetFlows(re *pbcore.RequestEvent) error {
	recs, err := a.pb.FindRecordsByFilter("flows", "", "name", 0, 0)
	if err != nil {
		return apperr.Internal(err.Error())
	}
	out := make([]map[string]any, 0, len(recs))
	for _, r := range recs {
		out = append(out, a.flowSummary(r))
	}
	return re.JSON(http.StatusOK, okResp{Data: out})
}

// slugRe finds what can't go in a workflow name.
var slugRe = regexp.MustCompile(`[^a-z0-9_]+`)

// flowSlug makes a valid workflow name from a human title.
func flowSlug(title string) string {
	s := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(title)), "_"), "_")
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		s = "workflow_" + s
	}
	if len(s) > 60 {
		s = strings.TrimRight(s[:60], "_")
	}
	return s
}

// starterDefinition is what a new workflow opens with.
func starterDefinition(name, description string) *flow.Definition {
	return &flow.Definition{
		Name: name, Description: description, Trigger: flow.Trigger{Type: "manual"},
		Nodes: []flow.Node{{ID: "start", Type: flow.NodeStart, UI: &flow.NodeUI{X: 0, Y: 0}}},
	}
}

// CreateFlow creates a workflow from a title, or from a whole definition.
func (a *App) CreateFlow(re *pbcore.RequestEvent) error {
	var in struct {
		Title       string           `json:"title"`
		Description string           `json:"description"`
		Definition  *flow.Definition `json:"definition"`
	}
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	def := in.Definition
	if def == nil {
		if strings.TrimSpace(in.Title) == "" {
			return apperr.BadRequest("a workflow needs a name")
		}
		def = starterDefinition(flowSlug(in.Title), strings.TrimSpace(in.Description))
	}
	if _, err := a.pb.FindFirstRecordByData("flows", "name", def.Name); err == nil {
		return apperr.Conflict("a workflow with that name already exists")
	}
	wf, err := a.flows.engine.Import(def, flowActor(re))
	if err != nil {
		return apperr.BadRequest(err.Error())
	}
	return okJSON(re, a.flowDetail(wf))
}

func (a *App) flowDetail(wf *pbcore.Record) map[string]any {
	out := a.flowSummary(wf)
	engine := a.flows.engine
	draft, err := engine.Draft(wf)
	if err == nil {
		out["draft"] = draft
		out["validation"] = engine.ValidateDefinition(draft)
	}
	if def, version, err := engine.PublishedDefinition(wf); err == nil {
		out["published"] = def
		out["published_version"] = version
	}
	if versions, err := a.pb.FindRecordsByFilter("flow_versions", "workflow = {:w}", "-version", 20, 0, dbx.Params{"w": wf.Id}); err == nil {
		rows := make([]map[string]any, 0, len(versions))
		for _, v := range versions {
			rows = append(rows, map[string]any{"version": v.GetInt("version"), "note": v.GetString("note"), "created": v.GetString("created")})
		}
		out["versions"] = rows
	}
	return out
}

// GetFlow returns a workflow with its draft, validation and published version.
func (a *App) GetFlow(re *pbcore.RequestEvent) error {
	wf, err := a.flowFromRoute(re)
	if err != nil {
		return err
	}
	out := a.flowDetail(wf)
	// The secret URL can start the workflow, so only people who can edit it see it.
	user := auth.GetUserRE(re)
	if user.HasPerm(permFlowsManage) {
		out["webhook_url"] = a.flowWebhookURL(wf)
	}
	return okJSON(re, out)
}

func (a *App) flowWebhookURL(wf *pbcore.Record) string {
	return strings.TrimRight(a.urlCfg.RootURL, "/") + "/webhooks/flows/" + wf.Id + "/" + wf.GetString("webhook_token")
}

// SaveFlowDraft replaces a workflow's working draft. It's saved even if invalid, so work
// in progress isn't lost, and the response says what's wrong.
func (a *App) SaveFlowDraft(re *pbcore.RequestEvent) error {
	wf, err := a.flowFromRoute(re)
	if err != nil {
		return err
	}
	var in struct {
		Definition *flow.Definition `json:"definition"`
	}
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	if in.Definition == nil {
		return apperr.BadRequest("a definition is required")
	}
	if in.Definition.Name != wf.GetString("name") {
		return apperr.BadRequest("a workflow can't be renamed in its draft")
	}
	wf.Set("draft", in.Definition)
	wf.Set("description", in.Definition.Description)
	if err := a.pb.Save(wf); err != nil {
		return apperr.Internal(err.Error())
	}
	return okJSON(re, map[string]any{"validation": a.flows.engine.ValidateDefinition(in.Definition)})
}

// ValidateFlow checks a definition without saving it.
func (a *App) ValidateFlow(re *pbcore.RequestEvent) error {
	var in struct {
		Definition *flow.Definition `json:"definition"`
	}
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	if in.Definition == nil {
		return apperr.BadRequest("a definition is required")
	}
	return okJSON(re, a.flows.engine.ValidateDefinition(in.Definition))
}

func flowError(err error) error {
	switch {
	case errors.Is(err, flow.ErrNotPublished):
		return apperr.BadRequest("publish this workflow first")
	case errors.Is(err, flow.ErrExists):
		return apperr.Conflict(err.Error())
	}
	return apperr.BadRequest(err.Error())
}

// PublishFlow makes the draft the next published version.
func (a *App) PublishFlow(re *pbcore.RequestEvent) error {
	wf, err := a.flowFromRoute(re)
	if err != nil {
		return err
	}
	var in struct {
		Note string `json:"note"`
	}
	_ = bindJSON(re, &in)
	version, validation, err := a.flows.engine.Publish(wf, flowActor(re), in.Note)
	if errors.Is(err, flow.ErrInvalid) {
		return re.JSON(http.StatusBadRequest, map[string]any{"message": "fix the errors before publishing", "validation": validation})
	}
	if err != nil {
		return flowError(err)
	}
	return okJSON(re, map[string]any{"version": version, "validation": validation})
}

// SetFlowEnabled pauses or resumes a workflow's trigger.
func (a *App) SetFlowEnabled(re *pbcore.RequestEvent) error {
	wf, err := a.flowFromRoute(re)
	if err != nil {
		return err
	}
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	if err := a.flows.engine.SetEnabled(wf, in.Enabled); err != nil {
		return flowError(err)
	}
	return okJSON(re, a.flowSummary(wf))
}

// DuplicateFlow copies a workflow as a new draft.
func (a *App) DuplicateFlow(re *pbcore.RequestEvent) error {
	wf, err := a.flowFromRoute(re)
	if err != nil {
		return err
	}
	var in struct {
		Title string `json:"title"`
	}
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	name := flowSlug(in.Title)
	if strings.TrimSpace(in.Title) == "" {
		name = wf.GetString("name") + "_copy"
	}
	out, err := a.flows.engine.Duplicate(wf, name, "", flowActor(re))
	if err != nil {
		return flowError(err)
	}
	// A copy gets its own webhook URL, so the original's can't start it.
	out.Set("webhook_token", security.RandomString(40))
	if err := a.pb.Save(out); err != nil {
		return apperr.Internal(err.Error())
	}
	return okJSON(re, a.flowDetail(out))
}

// DeleteFlow removes a workflow and its run history, unless runs are still active.
func (a *App) DeleteFlow(re *pbcore.RequestEvent) error {
	wf, err := a.flowFromRoute(re)
	if err != nil {
		return err
	}
	active, err := a.pb.CountRecords("flow_runs", dbx.HashExp{"workflow": wf.Id}, dbx.In("status", flow.StatusQueued, flow.StatusRunning, flow.StatusWaiting))
	if err != nil {
		return apperr.Internal(err.Error())
	}
	if active > 0 {
		return apperr.Conflict("cancel this workflow's active runs first")
	}
	if err := a.pb.Delete(wf); err != nil {
		return apperr.Internal(err.Error())
	}
	if err := a.flows.engine.RefreshTriggers(); err != nil {
		a.log.Printf("error refreshing workflow triggers: %v", err)
	}
	return okJSON(re, true)
}

// RunFlow starts the published version with the given input.
func (a *App) RunFlow(re *pbcore.RequestEvent) error {
	wf, err := a.flowFromRoute(re)
	if err != nil {
		return err
	}
	var in struct {
		Input map[string]any `json:"input"`
	}
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	run, err := a.flows.engine.StartRun(wf, in.Input, flow.StartOptions{Source: "manual_ui", StartedBy: flowActor(re).ID})
	if err != nil {
		return flowError(err)
	}
	return okJSON(re, map[string]any{"run_id": run.Id})
}

// EnrollFlowSubscribers starts one run of the published version per subscriber. Blocklisted
// subscribers and anyone already in an active run of this workflow are skipped, so adding
// someone twice can't send them everything twice.
func (a *App) EnrollFlowSubscribers(re *pbcore.RequestEvent) error {
	wf, err := a.flowFromRoute(re)
	if err != nil {
		return err
	}
	var in struct {
		Subscribers []string `json:"subscribers"`
	}
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	requested := uniqueNonEmptyStrings(in.Subscribers)
	if len(requested) == 0 {
		return apperr.BadRequest("choose at least one subscriber")
	}
	if len(requested) > flowEnrollMaxBatch {
		return apperr.BadRequest("add at most " + strconv.Itoa(flowEnrollMaxBatch) + " subscribers at a time")
	}
	if wf.GetInt("published_version") == 0 {
		return flowError(flow.ErrNotPublished)
	}
	ids, err := a.core.ResolveSubscriberIDs(nil, requested)
	if err != nil {
		return apperr.Internal(err.Error())
	}
	out := flowEnrollResult{Missing: len(requested) - len(ids)}
	if len(ids) > 0 {
		user := auth.GetUserRE(re)
		if err := a.hasSubPerm(user, ids); err != nil {
			return err
		}
		active, err := a.flows.engine.ContactsInActiveRuns(wf)
		if err != nil {
			return apperr.Internal(err.Error())
		}
		a.enrollSubscribers(wf, ids, active, user.RecordID, &out)
	}
	return okJSON(re, out)
}

type flowEnrollResult struct {
	Started     int    `json:"started"`
	Active      int    `json:"active"`
	Blocklisted int    `json:"blocklisted"`
	Missing     int    `json:"missing"`
	Failed      int    `json:"failed"`
	Error       string `json:"error,omitempty"`
}

func (a *App) enrollSubscribers(wf *pbcore.Record, ids []int, active map[string]bool, startedBy string, out *flowEnrollResult) {
	fail := func(err error) {
		out.Failed++
		if out.Error == "" {
			out.Error = err.Error()
		}
	}
	for _, id := range ids {
		sub, err := a.core.GetSubscriber(id, "", "")
		if err != nil {
			fail(err)
			continue
		}
		switch {
		case sub.Status == models.SubscriberStatusBlockListed:
			out.Blocklisted++
			continue
		case active[sub.RecordID]:
			out.Active++
			continue
		}
		summary := contactSummary(sub)
		input := map[string]any{"contact": sub.RecordID, "email": summary["email"], "name": sub.Name}
		if _, err := a.flows.engine.StartRun(wf, input, flow.StartOptions{Source: "manual_ui", StartedBy: startedBy}); err != nil {
			fail(err)
			continue
		}
		out.Started++
	}
}

// TestFlow runs the current draft without publishing it. Steps do real work.
func (a *App) TestFlow(re *pbcore.RequestEvent) error {
	wf, err := a.flowFromRoute(re)
	if err != nil {
		return err
	}
	var in struct {
		Input map[string]any `json:"input"`
	}
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	run, validation, err := a.flows.engine.StartTestRun(wf, in.Input, flowActor(re).ID)
	if errors.Is(err, flow.ErrInvalid) {
		return re.JSON(http.StatusBadRequest, map[string]any{"message": "fix the errors before testing", "validation": validation})
	}
	if err != nil {
		return flowError(err)
	}
	return okJSON(re, map[string]any{"run_id": run.Id})
}

// GetFlowCatalog returns everything a workflow can reference, for the builder and AI.
func (a *App) GetFlowCatalog(re *pbcore.RequestEvent) error {
	names := a.flows.tools.Names()
	rows := make([]map[string]any, 0, len(names))
	for _, name := range names {
		t, ok := a.flows.tools.Get(name)
		if !ok {
			continue
		}
		rows = append(rows, map[string]any{
			"name": name, "description": t.Description(), "input_schema": t.InputSchema(),
			"read_only": tools.IsReadOnly(t), "requires_approval": t.RequiresApproval(),
		})
	}
	wfs, err := a.pb.FindRecordsByFilter("flows", "published_version > 0", "name", 0, 0)
	if err != nil {
		return apperr.Internal(err.Error())
	}
	workflows := make([]string, 0, len(wfs))
	for _, wf := range wfs {
		workflows = append(workflows, wf.GetString("name"))
	}
	presets := []plugins.PresetInfo{}
	if a.plugins != nil {
		presets = a.plugins.Presets()
	}
	boards, err := a.core.ListBoards()
	if err != nil {
		return apperr.Internal(err.Error())
	}
	return okJSON(re, map[string]any{"tools": rows, "events": flow.Events(), "collections": flow.TriggerCollections(), "workflows": workflows, "presets": presets, "boards": boards})
}

// GetFlowRuns lists runs, newest first.
func (a *App) GetFlowRuns(re *pbcore.RequestEvent) error {
	conds := []string{}
	params := dbx.Params{}
	if ref := strings.TrimSpace(queryParam(re, "workflow")); ref != "" {
		wf, err := a.flows.engine.FindWorkflow(ref)
		if err != nil {
			return apperr.NotFound("workflow not found")
		}
		conds = append(conds, "workflow = {:w}")
		params["w"] = wf.Id
	}
	if status := strings.TrimSpace(queryParam(re, "status")); status != "" {
		conds = append(conds, "status = {:s}")
		params["s"] = status
	}
	limit, _ := strconv.Atoi(queryParam(re, "limit"))
	if limit < 1 || limit > flowRunListLimit {
		limit = 50
	}
	recs, err := a.pb.FindRecordsByFilter("flow_runs", strings.Join(conds, " && "), "-created", limit, 0, params)
	if err != nil {
		return apperr.Internal(err.Error())
	}
	names := map[string]string{}
	out := make([]map[string]any, 0, len(recs))
	for _, r := range recs {
		row := r.PublicExport()
		delete(row, "test_definition")
		id := r.GetString("workflow")
		if _, ok := names[id]; !ok {
			if wf, err := a.pb.FindRecordById("flows", id); err == nil {
				names[id] = wf.GetString("name")
			}
		}
		row["workflow_name"] = names[id]
		out = append(out, row)
	}
	return re.JSON(http.StatusOK, okResp{Data: out})
}

// GetFlowRun returns a run with each step's input, output, and agent activity.
func (a *App) GetFlowRun(re *pbcore.RequestEvent) error {
	run, err := a.pb.FindRecordById("flow_runs", pathParam(re, "id"))
	if err != nil {
		return apperr.NotFound("run not found")
	}
	nodeRuns, err := a.pb.FindRecordsByFilter("flow_node_runs", "workflow_run = {:r}", "created,@rowid", 0, 0, dbx.Params{"r": run.Id})
	if err != nil {
		return apperr.Internal(err.Error())
	}
	nodes := exportRecords(nodeRuns)
	for i, nr := range nodeRuns {
		agents, err := a.pb.FindRecordsByFilter("flow_agent_runs", "node_run = {:n}", "created", 0, 0, dbx.Params{"n": nr.Id})
		if err != nil || len(agents) == 0 {
			continue
		}
		steps, err := a.pb.FindRecordsByFilter("flow_run_steps", "agent_run = {:r}", "seq", 200, 0, dbx.Params{"r": agents[len(agents)-1].Id})
		if err == nil {
			nodes[i]["agent_steps"] = exportRecords(steps)
		}
	}
	approvals, _ := a.pb.FindRecordsByFilter("flow_approvals",
		"node_run.workflow_run = {:r}", "created", 0, 0, dbx.Params{"r": run.Id})
	out := run.PublicExport()
	delete(out, "test_definition")
	out["nodes"] = nodes
	out["approvals"] = exportRecords(approvals)
	if wf, err := a.pb.FindRecordById("flows", run.GetString("workflow")); err == nil {
		out["workflow_name"] = wf.GetString("name")
		var def *flow.Definition
		if version := run.GetInt("version"); version == 0 {
			def, _ = flow.DecodeDefinition(run.Get("test_definition"))
		} else if vr, err := a.pb.FindFirstRecordByFilter("flow_versions", "workflow = {:w} && version = {:v}", dbx.Params{"w": wf.Id, "v": version}); err == nil {
			def, _ = flow.DecodeDefinition(vr.Get("definition"))
		}
		out["definition"] = def
	}
	return okJSON(re, out)
}

// CancelFlowRun stops a run.
func (a *App) CancelFlowRun(re *pbcore.RequestEvent) error {
	if err := a.flows.engine.Cancel(pathParam(re, "id")); err != nil {
		return apperr.BadRequest(err.Error())
	}
	return okJSON(re, true)
}

// GetFlowApprovals lists approvals, pending ones by default.
func (a *App) GetFlowApprovals(re *pbcore.RequestEvent) error {
	status := strings.TrimSpace(queryParam(re, "status"))
	if status == "" {
		status = "pending"
	}
	recs, err := a.pb.FindRecordsByFilter("flow_approvals", "status = {:s}", "-created", flowRunListLimit, 0, dbx.Params{"s": status})
	if err != nil {
		return apperr.Internal(err.Error())
	}
	out := make([]map[string]any, 0, len(recs))
	for _, r := range recs {
		row := r.PublicExport()
		if nr, err := a.pb.FindRecordById("flow_node_runs", r.GetString("node_run")); err == nil {
			row["run_id"] = nr.GetString("workflow_run")
			row["node_id"] = nr.GetString("node_id")
			if run, err := a.pb.FindRecordById("flow_runs", nr.GetString("workflow_run")); err == nil {
				if wf, err := a.pb.FindRecordById("flows", run.GetString("workflow")); err == nil {
					row["workflow_name"] = wf.GetString("name")
				}
			}
		}
		out = append(out, row)
	}
	return re.JSON(http.StatusOK, okResp{Data: out})
}

// DecideFlowApproval approves or denies, optionally with edited data.
func (a *App) DecideFlowApproval(re *pbcore.RequestEvent) error {
	var in struct {
		Approved bool            `json:"approved"`
		Edited   json.RawMessage `json:"edited"`
	}
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	var edited any
	if len(in.Edited) > 0 && string(in.Edited) != "null" {
		if err := json.Unmarshal(in.Edited, &edited); err != nil {
			return apperr.BadRequest("edited data isn't valid JSON")
		}
	}
	if err := a.flows.engine.Decide(pathParam(re, "id"), in.Approved, edited, flowActor(re)); err != nil {
		return apperr.BadRequest(err.Error())
	}
	return okJSON(re, true)
}

// FlowWebhook starts a webhook workflow. The secret is in the URL, so nothing else
// identifies the caller, and it answers the same way for anything it can't match.
func (a *App) FlowWebhook(re *pbcore.RequestEvent) error {
	notFound := func() error { return re.JSON(http.StatusNotFound, map[string]any{"ok": false}) }
	wf, err := a.pb.FindRecordById("flows", pathParam(re, "id"))
	if err != nil {
		return notFound()
	}
	token := wf.GetString("webhook_token")
	if token == "" || !security.Equal(token, pathParam(re, "token")) {
		return notFound()
	}
	raw, err := io.ReadAll(http.MaxBytesReader(re.Response, re.Request.Body, maxFlowWebhookBytes))
	if err != nil {
		return re.JSON(http.StatusRequestEntityTooLarge, map[string]any{"ok": false})
	}
	var body any = map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			return re.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "the body must be JSON"})
		}
	}
	started, err := a.flows.engine.StartFromWebhook(wf, body)
	if err != nil {
		return notFound()
	}
	return re.JSON(http.StatusOK, map[string]any{"ok": true, "started": started})
}
