// Package queue is a small durable job queue on the jobs collection.
// Jobs survive restarts: a claimed job's lock expires, so a crashed worker's
// job is picked up again. At most one job per target runs at a time.
package queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

const collection = "flow_jobs"

type Job struct {
	ID       string
	Kind     string
	Target   string
	Payload  map[string]any
	Attempts int
}

type Handler func(ctx context.Context, job Job) error

type Queue struct {
	App         core.App
	Workers     int
	LockFor     time.Duration // how long a claimed job stays locked
	MaxAttempts int
	Poll        time.Duration
	// OnDead is called when a job exhausts its attempts.
	OnDead func(job Job, err error)

	id       string
	handlers map[string]Handler
	wake     chan struct{}
	wg       sync.WaitGroup
}

func New(app core.App) *Queue {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return &Queue{
		App:         app,
		Workers:     4,
		LockFor:     30 * time.Minute,
		MaxAttempts: 5,
		Poll:        time.Second,
		id:          "w-" + hex.EncodeToString(b),
		handlers:    map[string]Handler{},
		wake:        make(chan struct{}, 1),
	}
}

func (q *Queue) Handle(kind string, h Handler) {
	q.handlers[kind] = h
}

func dbTime(t time.Time) string {
	dt, _ := types.ParseDateTime(t.UTC())
	return dt.String()
}

// Enqueue adds a job available at `at` (zero = now). An identical pending
// job (same kind and target, not yet claimed, due no later) makes this a
// no-op, so repeated "advance this run" requests collapse into one.
func (q *Queue) Enqueue(kind, target string, payload map[string]any, at time.Time) error {
	if at.IsZero() {
		at = time.Now()
	}
	var existing int
	_ = q.App.DB().NewQuery(`SELECT COUNT(*) FROM flow_jobs WHERE kind = {:kind} AND target_id = {:target}
		AND (locked_until = '' OR locked_until IS NULL) AND available_at <= {:at}`).
		Bind(dbx.Params{"kind": kind, "target": target, "at": dbTime(at)}).Row(&existing)
	if existing > 0 && payload == nil {
		return nil
	}

	col, err := q.App.FindCachedCollectionByNameOrId(collection)
	if err != nil {
		return err
	}
	rec := core.NewRecord(col)
	rec.Set("kind", kind)
	rec.Set("target_id", target)
	rec.Set("payload", payload)
	rec.Set("available_at", at.UTC())
	rec.Set("max_attempts", q.MaxAttempts)
	if err := q.App.Save(rec); err != nil {
		return err
	}
	q.Notify()
	return nil
}

// Notify wakes an idle worker.
func (q *Queue) Notify() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// Start runs workers until ctx is cancelled.
func (q *Queue) Start(ctx context.Context) {
	for i := 0; i < q.Workers; i++ {
		q.wg.Add(1)
		go q.loop(ctx, fmt.Sprintf("%s-%d", q.id, i))
	}
}

// Wait blocks until workers stop (after ctx cancellation).
func (q *Queue) Wait() { q.wg.Wait() }

func (q *Queue) loop(ctx context.Context, worker string) {
	defer q.wg.Done()
	ticker := time.NewTicker(q.Poll)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		job, ok := q.claim(worker)
		if ok {
			q.run(ctx, job)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-q.wake:
		case <-ticker.C:
		}
	}
}

// RunOnce claims and runs one due job synchronously (tests, CLI).
func (q *Queue) RunOnce(ctx context.Context) bool {
	job, ok := q.claim(q.id + "-sync")
	if !ok {
		return false
	}
	q.run(ctx, job)
	return true
}

// Drain runs due jobs until none are left or ctx ends.
func (q *Queue) Drain(ctx context.Context) {
	for ctx.Err() == nil && q.RunOnce(ctx) {
	}
}

func (q *Queue) claim(worker string) (Job, bool) {
	now := dbTime(time.Now())
	until := dbTime(time.Now().Add(q.LockFor))
	var row struct {
		ID       string `db:"id"`
		Kind     string `db:"kind"`
		Target   string `db:"target_id"`
		Payload  string `db:"payload"`
		Attempts int    `db:"attempts"`
	}
	err := q.App.DB().NewQuery(`
		UPDATE flow_jobs SET locked_by = {:worker}, locked_until = {:until}
		WHERE id = (
			SELECT j.id FROM flow_jobs j
			WHERE j.available_at <= {:now}
			  AND (j.locked_until = '' OR j.locked_until IS NULL OR j.locked_until < {:now})
			  AND NOT EXISTS (
				SELECT 1 FROM flow_jobs o WHERE o.target_id = j.target_id AND o.id != j.id AND o.locked_until > {:now})
			ORDER BY j.available_at LIMIT 1)
		RETURNING id, kind, target_id, payload, attempts`).
		Bind(dbx.Params{"worker": worker, "until": until, "now": now}).One(&row)
	if err != nil {
		return Job{}, false
	}
	job := Job{ID: row.ID, Kind: row.Kind, Target: row.Target, Attempts: row.Attempts}
	_ = json.Unmarshal([]byte(row.Payload), &job.Payload)
	return job, true
}

func (q *Queue) run(ctx context.Context, job Job) {
	h, ok := q.handlers[job.Kind]
	var err error
	if !ok {
		err = fmt.Errorf("no handler for job kind %q", job.Kind)
	} else {
		err = safeCall(ctx, h, job)
	}
	if err == nil {
		_, _ = q.App.DB().Delete(collection, dbx.HashExp{"id": job.ID}).Execute()
		return
	}

	attempts := job.Attempts + 1
	if attempts >= q.MaxAttempts || !ok {
		_, _ = q.App.DB().Delete(collection, dbx.HashExp{"id": job.ID}).Execute()
		q.App.Logger().Error("job failed permanently", "kind", job.Kind, "target", job.Target, "error", err)
		if q.OnDead != nil {
			q.OnDead(job, err)
		}
		return
	}
	backoff := time.Duration(1<<attempts) * time.Second
	_, _ = q.App.DB().Update(collection, dbx.Params{
		"attempts":     attempts,
		"available_at": dbTime(time.Now().Add(backoff)),
		"locked_by":    "",
		"locked_until": "",
		"last_error":   err.Error(),
	}, dbx.HashExp{"id": job.ID}).Execute()
	q.App.Logger().Warn("job failed; will retry", "kind", job.Kind, "target", job.Target, "attempt", attempts, "error", err)
}

func safeCall(ctx context.Context, h Handler, job Job) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return h(ctx, job)
}
