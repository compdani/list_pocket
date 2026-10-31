package queue_test

import (
	"context"
	"errors"
	"testing"
	"time"

	_ "github.com/compdani/list_pocket/internal/migrations"

	"github.com/compdani/list_pocket/internal/flow/queue"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestQueueRetryDeadAndDedupe(t *testing.T) {
	app, err := tests.NewTestAppWithConfig(core.BaseAppConfig{DataDir: t.TempDir(), EncryptionEnv: "pb_test_env"})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()
	if err := app.RunAllMigrations(); err != nil {
		t.Fatal(err)
	}

	q := queue.New(app)
	q.MaxAttempts = 2
	calls := map[string]int{}
	var dead []string
	q.OnDead = func(j queue.Job, err error) { dead = append(dead, j.Target) }
	q.Handle("ok", func(_ context.Context, j queue.Job) error { calls[j.Target]++; return nil })
	q.Handle("flaky", func(_ context.Context, j queue.Job) error { calls[j.Target]++; return errors.New("nope") })

	// Duplicate pending jobs collapse.
	for i := 0; i < 3; i++ {
		if err := q.Enqueue("ok", "a", nil, time.Time{}); err != nil {
			t.Fatal(err)
		}
	}
	if n, _ := app.CountRecords("flow_jobs"); n != 1 {
		t.Fatalf("pending duplicates not collapsed: %d jobs", n)
	}
	// Future jobs wait.
	_ = q.Enqueue("ok", "later", nil, time.Now().Add(time.Hour))
	_ = q.Enqueue("flaky", "f", nil, time.Time{})

	q.Drain(context.Background())
	if calls["a"] != 1 || calls["later"] != 0 || calls["f"] != 1 {
		t.Fatalf("calls = %v", calls)
	}
	// The flaky job was rescheduled with backoff; make it due and run again.
	_, _ = app.DB().NewQuery("UPDATE flow_jobs SET available_at = '2000-01-01 00:00:00.000Z' WHERE kind = 'flaky'").Execute()
	q.Drain(context.Background())
	if calls["f"] != 2 || len(dead) != 1 || dead[0] != "f" {
		t.Fatalf("after retry: calls=%v dead=%v", calls, dead)
	}
	if n, _ := app.CountRecords("flow_jobs"); n != 1 {
		t.Errorf("only the future job should remain, have %d", n)
	}
}
