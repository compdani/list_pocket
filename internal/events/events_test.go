package events

import (
	"testing"
	"time"
)

func TestPublishDoesNotHoldLockDuringHook(t *testing.T) {
	ev := New()
	ev.SetPublishHook(func(Event) error {
		_, err := ev.Subscribe("hook-sub")
		return err
	})

	done := make(chan error, 1)
	go func() {
		done <- ev.Publish(Event{Type: TypeCampaignStats})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Publish deadlocked holding the lock during the hook")
	}

	ch, err := ev.Subscribe("listener")
	if err != nil {
		t.Fatal(err)
	}
	if err := ev.Publish(Event{Type: TypeError, Message: "ok"}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-ch:
		if got.Message != "ok" {
			t.Fatalf("got %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive event")
	}
}
