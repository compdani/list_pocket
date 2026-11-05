// Package delivery persists workflow message intent independently of node replay.
// Ambiguous provider outcomes are never retried automatically.
package delivery

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/compdani/list_pocket/internal/flow/queue"
	"github.com/compdani/list_pocket/models"
)

const JobKind = "workflow_delivery"

type Outbox struct {
	App   core.App
	Queue *queue.Queue
	Send  func(models.Message) error
}

func (o *Outbox) Enqueue(key string, msg models.Message) (*core.Record, error) {
	if key == "" {
		return nil, fmt.Errorf("delivery execution key is required")
	}
	var rec *core.Record
	err := o.App.RunInTransaction(func(tx core.App) error {
		existing, err := tx.FindFirstRecordByFilter("flow_deliveries", "execution_key = {:key}", dbx.Params{"key": key})
		if err == nil {
			rec = existing
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		col, err := tx.FindCachedCollectionByNameOrId("flow_deliveries")
		if err != nil {
			return err
		}
		payload, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		rec = core.NewRecord(col)
		rec.Set("execution_key", key)
		rec.Set("message", json.RawMessage(payload))
		rec.Set("status", "queued")
		if err := tx.Save(rec); err != nil {
			return err
		}
		return o.Queue.EnqueueWithApp(tx, JobKind, rec.Id, nil, time.Time{})
	})
	if err != nil {
		return nil, err
	}
	o.Queue.Notify()
	return rec, nil
}
func (o *Outbox) Register() { o.Queue.Handle(JobKind, o.dispatch) }

// Recover also requeues ambiguous interrupted attempts so they can be classified.
func (o *Outbox) Recover() error {
	records, err := o.App.FindRecordsByFilter("flow_deliveries", "status = 'queued' || status = 'sending'", "", 0, 0)
	if err != nil {
		return err
	}
	for _, rec := range records {
		if err := o.Queue.Enqueue(JobKind, rec.Id, nil, time.Time{}); err != nil {
			return err
		}
	}
	return nil
}
func (o *Outbox) dispatch(ctx context.Context, job queue.Job) error {
	rec, err := o.App.FindRecordById("flow_deliveries", job.Target)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	switch rec.GetString("status") {
	case "sent", "ambiguous", "failed":
		return nil
	case "sending":
		rec.Set("status", "ambiguous")
		rec.Set("error", "Previous delivery attempt has an unknown provider outcome; manual reconciliation required")
		return o.saveReceipt(rec)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var msg models.Message
	if err := rec.UnmarshalJSONField("message", &msg); err != nil {
		return err
	}
	rec.Set("status", "sending")
	if err := o.App.Save(rec); err != nil {
		return err
	}
	// From here, even an error may follow provider acceptance. Do not duplicate.
	if err := o.Send(msg); err != nil {
		if errors.Is(err, models.ErrNotAttempted) {
			rec.Set("status", "queued")
			rec.Set("error", err.Error())
			if saveErr := o.saveReceipt(rec); saveErr != nil {
				return saveErr
			}
			return err
		}
		rec.Set("status", "ambiguous")
		rec.Set("error", err.Error())
	} else {
		rec.Set("status", "sent")
	}
	return o.saveReceipt(rec)
}

// FinalizeDead persists a terminal receipt before the queue drops exhausted work.
func (o *Outbox) FinalizeDead(job queue.Job, cause error) error {
	rec, err := o.App.FindRecordById("flow_deliveries", job.Target)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	status := rec.GetString("status")
	if status == "sent" || status == "ambiguous" || status == "failed" {
		return nil
	}
	if status == "sending" {
		rec.Set("status", "ambiguous")
	} else {
		rec.Set("status", "failed")
	}
	rec.Set("error", cause.Error())
	return o.saveReceipt(rec)
}

// Receipt and transactional-message projection commit together on every path,
// including dead-letter finalization and recovery of an interrupted attempt.
func (o *Outbox) saveReceipt(rec *core.Record) error {
	return o.App.RunInTransaction(func(tx core.App) error {
		if err := tx.Save(rec); err != nil {
			return err
		}
		var msg models.Message
		if err := rec.UnmarshalJSONField("message", &msg); err != nil {
			if rec.GetString("status") == "failed" {
				return nil
			}
			return err
		}
		if msg.TxMessage == nil {
			return nil
		}
		record, err := tx.FindRecordById("transactional_messages", msg.TxMessage.RecordID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		record.Set("status", rec.GetString("status"))
		record.Set("error", rec.GetString("error"))
		if rec.GetString("status") == "sent" {
			record.Set("sent_at", time.Now().UTC())
		}
		return tx.Save(record)
	})
}
