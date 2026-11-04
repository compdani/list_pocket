package main

import (
	"context"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/compdani/list_pocket/internal/flow/queue"
	"github.com/compdani/list_pocket/internal/manager"
	"github.com/compdani/list_pocket/models"
)

type shutdownMessenger struct {
	entered, release chan struct{}
	once             sync.Once
	calls            atomic.Int32
}

func (*shutdownMessenger) Name() string { return "email" }
func (m *shutdownMessenger) Push(models.Message) error {
	m.calls.Add(1)
	m.once.Do(func() { close(m.entered) })
	<-m.release
	return nil
}
func (*shutdownMessenger) Flush() error { return nil }
func (*shutdownMessenger) Close() error { return nil }
func TestShutdownAppStopsAdmissionBeforeJoiningWorkflowDeliveries(t *testing.T) {
	a, _, _ := boardsTestApp(t)
	m := manager.New(manager.Config{Concurrency: 1}, nil, nil, log.New(io.Discard, "", 0))
	provider := &shutdownMessenger{entered: make(chan struct{}), release: make(chan struct{})}
	if err := m.AddMessenger(provider); err != nil {
		t.Fatal(err)
	}
	go m.Run()
	// A barrier cron callback lets us observe shutdown after its early stop signal
	// while holding downstream cleanup so active delivery can still be released.
	gate := &backgroundCallbacks{}
	cronEntered, cronRelease := make(chan struct{}), make(chan struct{})
	go gate.wrap(func() { close(cronEntered); <-cronRelease })()
	<-cronEntered
	q := queue.New(a.pb)
	q.Workers = 2
	attempts := make(chan string, 10)
	q.Handle("send", func(_ context.Context, job queue.Job) error {
		attempts <- job.Target
		return m.DeliverMessage(models.Message{Messenger: "email"})
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Start m.Run before dispatch: retry the typed non-attempt until workers start.
	// Queue retry itself persists a job, so no provider attempt is lost.
	q.Start(ctx)
	if err := q.Enqueue("send", "first", nil, time.Time{}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("provider did not start")
	}
	if err := q.Enqueue("send", "second", nil, time.Time{}); err != nil {
		t.Fatal(err)
	}
	// Consume both handler admission notifications.
	for {
		select {
		case target := <-attempts:
			if target == "second" {
				goto admitted
			}
		case <-time.After(5 * time.Second):
			t.Fatal("second handler did not start")
		}
	}
admitted:
	a.manager = m
	a.cronJobs = gate
	a.flows = &flowEngine{queue: q, cancel: cancel}
	done := make(chan struct{})
	go func() { shutdownApp(a); close(done) }()
	deadline := time.Now().Add(time.Second)
	for {
		gate.mu.Lock()
		stopped := gate.stopped
		gate.mu.Unlock()
		if stopped {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("shutdown did not reach cron join")
		}
		time.Sleep(time.Millisecond)
	}
	close(provider.release)
	close(cronRelease)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown blocked")
	}
	if calls := provider.calls.Load(); calls != 1 {
		t.Fatalf("queued delivery reached provider during shutdown: %d", calls)
	}
}
