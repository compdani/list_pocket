package main

import (
	"ganigeorgiev.com/pocketbase/pocketbase"
	"sync"
)

// backgroundCallbacks owns cron admission and joins callbacks already running.
// PocketBase Cron.Stop stops scheduling, but doesn't join spawned callbacks.
type backgroundCallbacks struct {
	mu      sync.Mutex
	stopped bool
	wg      sync.WaitGroup
}

func (b *backgroundCallbacks) Add(pb *pocketbase.PocketBase, id, schedule string, fn func()) error {
	return pb.Cron().Add(id, schedule, b.wrap(fn))
}
func (b *backgroundCallbacks) wrap(fn func()) func() {
	return func() {
		b.mu.Lock()
		if b.stopped {
			b.mu.Unlock()
			return
		}
		b.wg.Add(1)
		b.mu.Unlock()
		defer b.wg.Done()
		fn()
	}
}
func (b *backgroundCallbacks) StopAndWait() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.stopped = true
	b.mu.Unlock()
	b.wg.Wait()
}
