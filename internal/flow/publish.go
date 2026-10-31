package flow

import (
	"errors"
	"fmt"
	"time"

	"github.com/compdani/list_pocket/internal/flow/tools"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"
	"github.com/pocketbase/pocketbase/tools/types"
)

var (
	ErrInvalid = errors.New("workflow failed validation")
	ErrExists  = errors.New("workflow already exists")
)

// Actor is who performed an action. Permissions are checked at the API routes.
type Actor struct {
	ID string
}

type catalog struct {
	e *Engine
}

func (c catalog) HasTool(name string) bool {
	if name == tools.WebSearch {
		return true
	}
	_, ok := c.e.Tools.Get(name)
	return ok
}

// ToolHasSideEffects reports whether a known tool changes or sends anything.
func (c catalog) ToolHasSideEffects(name string) bool {
	if name == tools.WebSearch {
		return false
	}
	t, ok := c.e.Tools.Get(name)
	return ok && !tools.IsReadOnly(t)
}

func (c catalog) Preset(name string) (map[string]any, bool) {
	if c.e.Presets == nil {
		return nil, false
	}
	return c.e.Presets.Preset(name)
}
func (c catalog) HasEvent(name string) bool      { return IsKnownEvent(name) }
func (c catalog) HasCollection(name string) bool { return triggerCollections[name] }
func (c catalog) HasWorkflow(name string) bool {
	wf, err := c.e.App.FindFirstRecordByData("flows", "name", name)
	return err == nil && wf.GetInt("published_version") > 0
}

func (e *Engine) catalog() catalog {
	return catalog{e: e}
}

// ValidateDefinition checks d against the current catalog.
func (e *Engine) ValidateDefinition(d *Definition) Validation {
	return Validate(d, e.catalog())
}

func (e *Engine) FindWorkflow(nameOrID string) (*core.Record, error) {
	if rec, err := e.App.FindRecordById("flows", nameOrID); err == nil {
		return rec, nil
	}
	return e.App.FindFirstRecordByData("flows", "name", nameOrID)
}

// Draft returns the workflow's working definition.
func (e *Engine) Draft(wf *core.Record) (*Definition, error) {
	return DecodeDefinition(wf.Get("draft"))
}

// Import creates a workflow or replaces its draft.
func (e *Engine) Import(d *Definition, actor Actor) (*core.Record, error) {
	if !nameRe.MatchString(d.Name) {
		return nil, fmt.Errorf("invalid workflow name %q", d.Name)
	}
	wf, err := e.App.FindFirstRecordByData("flows", "name", d.Name)
	if err != nil {
		col, err := e.App.FindCollectionByNameOrId("flows")
		if err != nil {
			return nil, err
		}
		wf = core.NewRecord(col)
		wf.Set("name", d.Name)
		wf.Set("created_by", actor.ID)
		wf.Set("webhook_token", security.RandomString(40))
	}
	wf.Set("description", d.Description)
	wf.Set("draft", d)
	if err := e.App.Save(wf); err != nil {
		return nil, err
	}
	return wf, nil
}

// Duplicate copies a workflow's draft under a new name as an unpublished draft, so
// copied triggers stay off until it's published.
func (e *Engine) Duplicate(wf *core.Record, name, description string, actor Actor) (*core.Record, error) {
	if _, err := e.App.FindFirstRecordByData("flows", "name", name); err == nil {
		return nil, fmt.Errorf("%w: %q", ErrExists, name)
	}
	d, err := e.Draft(wf)
	if err != nil {
		return nil, err
	}
	d.Name = name
	if description != "" {
		d.Description = description
	}
	return e.Import(d, actor)
}

// Publish validates the draft and makes it the next published version.
func (e *Engine) Publish(wf *core.Record, actor Actor, note string) (int, Validation, error) {
	d, err := e.Draft(wf)
	if err != nil {
		return 0, Validation{}, err
	}
	if d.Name != wf.GetString("name") {
		return 0, Validation{}, fmt.Errorf("draft name %q does not match workflow %q", d.Name, wf.GetString("name"))
	}
	v := e.ValidateDefinition(d)
	if !v.OK {
		return 0, v, ErrInvalid
	}

	var version int
	err = e.App.RunInTransaction(func(tx core.App) error {
		var max int
		_ = tx.DB().NewQuery("SELECT COALESCE(MAX(version),0) FROM flow_versions WHERE workflow = {:w}").Bind(dbx.Params{"w": wf.Id}).Row(&max)
		version = max + 1
		col, err := tx.FindCollectionByNameOrId("flow_versions")
		if err != nil {
			return err
		}
		vr := core.NewRecord(col)
		vr.Set("workflow", wf.Id)
		vr.Set("version", version)
		vr.Set("definition", d)
		vr.Set("validation", v)
		vr.Set("note", note)
		vr.Set("published_by", actor.ID)
		if err := tx.Save(vr); err != nil {
			return err
		}
		if wf.GetInt("published_version") == 0 {
			wf.Set("enabled", true)
		}
		wf.Set("published_version", version)
		wf.Set("trigger_type", d.Trigger.Type)
		if err := e.scheduleNext(wf, d, time.Now()); err != nil {
			return err
		}
		return tx.Save(wf)
	})
	if err != nil {
		return 0, v, err
	}
	if err := e.RefreshTriggers(); err != nil {
		e.App.Logger().Error("refreshing triggers failed", "error", err)
	}
	return version, v, nil
}

// SetEnabled pauses or resumes a workflow's trigger.
func (e *Engine) SetEnabled(wf *core.Record, enabled bool) error {
	wf.Set("enabled", enabled)
	if d, _, err := e.PublishedDefinition(wf); err == nil {
		if err := e.scheduleNext(wf, d, time.Now()); err != nil {
			return err
		}
	}
	if err := e.App.Save(wf); err != nil {
		return err
	}
	return e.RefreshTriggers()
}

// Decide resolves a pending approval and wakes its run.
func (e *Engine) Decide(approvalID string, approved bool, edited any, actor Actor) error {
	a, err := e.App.FindRecordById("flow_approvals", approvalID)
	if err != nil {
		return fmt.Errorf("approval not found")
	}
	if a.GetString("status") != "pending" {
		return fmt.Errorf("approval already %s", a.GetString("status"))
	}
	status := "denied"
	if approved {
		status = "approved"
	}
	a.Set("status", status)
	a.Set("decided_by", actor.ID)
	a.Set("decided_at", types.NowDateTime())
	if edited != nil && approved {
		a.Set("edited_payload", edited)
	}
	if err := e.App.Save(a); err != nil {
		return err
	}
	nr, err := e.App.FindRecordById("flow_node_runs", a.GetString("node_run"))
	if err != nil {
		return nil
	}
	return e.Queue.Enqueue(JobAdvance, nr.GetString("workflow_run"), nil, time.Time{})
}

// Catalog returns the reference catalog validation uses.
func (e *Engine) Catalog() Catalog {
	return e.catalog()
}
