package main

import (
	"testing"
	"time"
)

func TestReloadWaitsForCleanup(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	replaced := make(chan struct{})
	go reloadAfterCleanup(func() { close(started); <-release }, func() { close(replaced) })
	<-started
	select {
	case <-replaced:
		t.Fatal("replaced process before cleanup completed")
	default:
	}
	close(release)
	select {
	case <-replaced:
	case <-time.After(time.Second):
		t.Fatal("replacement did not follow cleanup")
	}
}
