package manager

import (
	"errors"
	"io"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/compdani/list_pocket/models"
	"github.com/paulbellamy/ratecounter"
)

type failingBatchStore struct {
	testStore
	subs      []models.Subscriber
	fetchErr  error
	fetches   int
	rollbacks int
}

func (s *failingBatchStore) NextSubscribers(int, int) ([]models.Subscriber, bool, error) {
	s.fetches++
	if s.fetchErr != nil {
		return nil, false, s.fetchErr
	}
	return s.subs, true, nil
}

func (s *failingBatchStore) RollbackCampaignLedgerInflight(int, string) error {
	s.rollbacks++
	return nil
}

func newTestPipe(m *Manager) *pipe {
	return &pipe{
		camp: &models.Campaign{Base: models.Base{RecordID: "camprid01234501"}, Messenger: "email", Body: "x"},
		rate: ratecounter.NewRateCounter(time.Minute),
		wg:   &sync.WaitGroup{},
		m:    m,
	}
}

// A batch where every message fails to render must stop the pipe instead of
// reporting more work, otherwise Run would reclaim the same rows in a tight loop.
func TestNextSubscribersStopsWhenNothingRenders(t *testing.T) {
	store := &failingBatchStore{subs: []models.Subscriber{
		{Base: models.Base{RecordID: "subrid000000001"}, Email: "a@example.com"},
		{Base: models.Base{RecordID: "subrid000000002"}, Email: "b@example.com"},
	}}
	m := New(Config{BatchSize: 10, MaxSendErrors: 1000}, store, nil, log.New(io.Discard, "", 0))
	p := newTestPipe(m)
	// The campaign template is intentionally not compiled, so rendering fails.
	p.camp.ContentType = "html"

	has, err := p.NextSubscribers()
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Fatal("reported more work after a batch that rendered nothing")
	}
	if !p.stopped.Load() || !p.withErrors.Load() {
		t.Fatalf("pipe should be stopped with errors, stopped=%v withErrors=%v", p.stopped.Load(), p.withErrors.Load())
	}
	if store.rollbacks != 2 {
		t.Fatalf("rollbacks = %d, want 2", store.rollbacks)
	}
}

// A failing batch fetch must back off and requeue the pipe, not drop it or spin.
func TestRunRequeuesPipeAfterFetchError(t *testing.T) {
	store := &failingBatchStore{fetchErr: errors.New("db busy")}
	m := New(Config{BatchSize: 10}, store, nil, log.New(io.Discard, "", 0))
	m.fetchRetryDelay = 20 * time.Millisecond
	p := newTestPipe(m)
	m.nextPipes <- p

	done := make(chan struct{})
	go func() { m.Run(); close(done) }()
	time.Sleep(110 * time.Millisecond)
	m.Close()
	<-done

	if store.fetches < 2 {
		t.Fatalf("fetches = %d, pipe was not requeued after the error", store.fetches)
	}
	if store.fetches > 12 {
		t.Fatalf("fetches = %d, expected a backoff between retries", store.fetches)
	}
}
