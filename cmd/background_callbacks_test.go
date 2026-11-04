package main

import (
	"testing"
	"time"
)

func TestBackgroundCallbacksJoinAndRejectLateCalls(t *testing.T) {
	b := &backgroundCallbacks{}
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go b.wrap(func() { close(started); <-release })()
	<-started
	go func() { b.StopAndWait(); close(done) }()
	// Observe the closed gate without timing assumptions.
	for {
		b.mu.Lock()
		stopped := b.stopped
		b.mu.Unlock()
		if stopped {
			break
		}
		time.Sleep(time.Millisecond)
	}
	b.wrap(func() { t.Error("late callback admitted") })()
	select {
	case <-done:
		t.Fatal("returned while callback active")
	default:
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("failed to join callback")
	}
}
