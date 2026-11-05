package delivery

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tests"
	"github.com/compdani/list_pocket/internal/flow/queue"
	_ "github.com/compdani/list_pocket/internal/migrations"
	"github.com/compdani/list_pocket/models"
)

func setup(t *testing.T) *Outbox {
	t.Helper()
	app, err := tests.NewTestAppWithConfig(core.BaseAppConfig{DataDir: t.TempDir(), EncryptionEnv: "pb_test_env"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	if err := app.RunAllMigrations(); err != nil {
		t.Fatal(err)
	}
	return &Outbox{App: app, Queue: queue.New(app)}
}
func TestReplayAndRestartPreserveSingleIntent(t *testing.T) {
	o := setup(t)
	msg := models.Message{To: []string{"a@example.com"}, Body: []byte("body"), Messenger: "email"}
	rec, err := o.Enqueue("node1", msg)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := o.Enqueue("node1", msg)
	if err != nil || replay.Id != rec.Id {
		t.Fatalf("replay %v", err)
	}
	calls := 0
	// A new dispatcher process uses the persisted payload, not the old memory queue.
	restart := &Outbox{App: o.App, Queue: queue.New(o.App), Send: func(got models.Message) error {
		calls++
		if string(got.Body) != "body" {
			t.Error("payload lost")
		}
		return nil
	}}
	restart.Register()
	if err := restart.Recover(); err != nil {
		t.Fatal(err)
	}
	restart.Queue.Drain(context.Background())
	fresh, _ := o.App.FindRecordById("flow_deliveries", rec.Id)
	if calls != 1 || fresh.GetString("status") != "sent" {
		t.Fatalf("calls=%d status=%s", calls, fresh.GetString("status"))
	}
}
func TestAcceptedButTerminalSaveFailedBecomesAmbiguous(t *testing.T) {
	o := setup(t)
	rec, err := o.Enqueue("node2", models.Message{Messenger: "email"})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	o.Send = func(models.Message) error { calls++; return nil }
	o.Register()
	o.App.OnRecordUpdate("flow_deliveries").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetString("status") == "sent" {
			return fmt.Errorf("injected receipt failure")
		}
		return e.Next()
	})
	o.Queue.RunOnce(context.Background())
	// Simulate lease recovery without waiting for its normal retry backoff.
	jobs, _ := o.App.FindAllRecords("flow_jobs")
	if len(jobs) != 1 {
		t.Fatal("job lost")
	}
	jobs[0].Set("available_at", time.Now().Add(-time.Second))
	if err := o.App.Save(jobs[0]); err != nil {
		t.Fatal(err)
	}
	o.Queue.Drain(context.Background())
	fresh, _ := o.App.FindRecordById("flow_deliveries", rec.Id)
	if calls != 1 || fresh.GetString("status") != "ambiguous" {
		t.Fatalf("calls=%d status=%s", calls, fresh.GetString("status"))
	}
}
func TestEnqueueFailureRollsBackIntent(t *testing.T) {
	o := setup(t)
	o.App.OnRecordCreate("flow_jobs").BindFunc(func(*core.RecordEvent) error { return fmt.Errorf("injected job failure") })
	if _, err := o.Enqueue("node3", models.Message{}); err == nil {
		t.Fatal("expected failure")
	}
	count, err := o.App.CountRecords("flow_deliveries")
	if err != nil || count != 0 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestDeadDeliveryPersistsTerminalReceiptBeforeAcknowledgement(t *testing.T) {
	for _, status := range []string{"queued", "sending"} {
		t.Run(status, func(t *testing.T) {
			o := setup(t)
			rec, err := o.Enqueue("exhausted", models.Message{})
			if err != nil {
				t.Fatal(err)
			}
			rec.Set("status", status)
			if err := o.App.Save(rec); err != nil {
				t.Fatal(err)
			}
			if err := o.FinalizeDead(queue.Job{Target: rec.Id}, fmt.Errorf("retry exhaustion")); err != nil {
				t.Fatal(err)
			}
			fresh, _ := o.App.FindRecordById("flow_deliveries", rec.Id)
			expected := "failed"
			if status == "sending" {
				expected = "ambiguous"
			}
			if fresh.GetString("status") != expected {
				t.Fatal(fresh.GetString("status"))
			}
		})
	}
}

func TestDefiniteNonAttemptIsRetryable(t *testing.T) {
	o := setup(t)
	o.Send = func(models.Message) error { return fmt.Errorf("%w: unavailable", models.ErrNotAttempted) }
	o.Register()
	rec, err := o.Enqueue("nonattempt", models.Message{})
	if err != nil {
		t.Fatal(err)
	}
	o.Queue.RunOnce(context.Background())
	fresh, _ := o.App.FindRecordById("flow_deliveries", rec.Id)
	if fresh.GetString("status") != "queued" {
		t.Fatal(fresh.GetString("status"))
	}
	if count, _ := o.App.CountRecords("flow_jobs"); count != 1 {
		t.Fatal("retry lost")
	}
}
func TestTerminalReceiptProjectsTransactionalStatus(t *testing.T) {
	o := setup(t)
	col, _ := o.App.FindCachedCollectionByNameOrId("transactional_messages")
	tx := core.NewRecord(col)
	for key, value := range map[string]string{"uuid": "test-projection", "to_email": "a@example.com", "from_email": "b@example.com", "subject": "test", "content_type": "plain", "messenger": "email", "status": "queued"} {
		tx.Set(key, value)
	}
	if err := o.App.Save(tx); err != nil {
		t.Fatal(err)
	}
	rec, err := o.Enqueue("projection", models.Message{TxMessage: &models.TransactionalMessage{Base: models.Base{RecordID: tx.Id}}})
	if err != nil {
		t.Fatal(err)
	}
	rec.Set("status", "sending")
	if err := o.App.Save(rec); err != nil {
		t.Fatal(err)
	}
	if err := o.FinalizeDead(queue.Job{Target: rec.Id}, fmt.Errorf("interrupted")); err != nil {
		t.Fatal(err)
	}
	fresh, _ := o.App.FindRecordById("transactional_messages", tx.Id)
	if fresh.GetString("status") != "ambiguous" {
		t.Fatal(fresh.GetString("status"))
	}
	if err := o.App.Save(fresh); err != nil {
		t.Fatalf("invalid projected status: %v", err)
	}
}
