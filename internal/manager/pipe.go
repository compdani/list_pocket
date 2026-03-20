package manager

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/compdani/list_pocket/models"
	"github.com/paulbellamy/ratecounter"
)

type pipe struct {
	camp         *models.Campaign
	rate         *ratecounter.RateCounter
	wg           *sync.WaitGroup
	sent         atomic.Int64
	lastID       atomic.Uint64
	errors       atomic.Uint64
	stopped      atomic.Bool
	withErrors   atomic.Bool
	batchHasMore bool

	m *Manager
}

// newPipe adds a campaign to the process queue.
func (m *Manager) newPipe(c *models.Campaign) (*pipe, error) {
	// Validate messenger.
	if _, ok := m.messengers[c.Messenger]; !ok {
		m.store.UpdateCampaignStatus(c.ID, models.CampaignStatusCancelled)
		return nil, fmt.Errorf("unknown messenger %s on campaign %s", c.Messenger, c.Name)
	}

	// Load the template.
	if err := c.CompileTemplate(m.TemplateFuncs(c)); err != nil {
		return nil, err
	}

	// Load any media/attachments.
	if err := m.attachMedia(c); err != nil {
		return nil, err
	}

	// Add the campaign to the active map.
	p := &pipe{
		camp: c,
		rate: ratecounter.NewRateCounter(time.Minute),
		wg:   &sync.WaitGroup{},
		m:    m,
	}

	// Increment the waitgroup so that Wait() blocks immediately. This is necessary
	// as a campaign pipe is created first and subscribers/messages under it are
	// fetched asynchronolusly later. The messages each add to the wg and that
	// count is used to determine the exhaustion/completion of all messages.
	p.wg.Add(1)

	go func() {
		// Wait for all the messages in the campaign to be processed
		// (successfully or skipped after errors or cancellation).
		p.wg.Wait()

		p.cleanup()
	}()

	m.pipesMut.Lock()
	m.pipes[c.ID] = p
	m.pipesMut.Unlock()
	return p, nil
}

// NextSubscribers processes the next batch of subscribers in a given campaign.
// It returns a bool indicating whether any subscribers were processed
// in the current batch or not. A false indicates that all subscribers
// have been processed, or that a campaign has been paused or cancelled.
func (p *pipe) NextSubscribers() (bool, error) {
	batching := p.camp.Batching()
	limit := p.m.cfg.BatchSize
	if batching.Enabled && batching.BatchSize > 0 {
		limit = batching.BatchSize
	}

	// Fetch the next batch of subscribers from a 'running' campaign.
	subs, hasMore, err := p.m.store.NextSubscribers(p.camp.ID, limit)
	if err != nil {
		return false, fmt.Errorf("error fetching campaign subscribers (%s): %v", p.camp.Name, err)
	}

	// There are no subscribers from the query. Either all subscribers on the campaign
	// have been processed, or the campaign has changed from 'running' to 'paused' or 'cancelled'.
	if len(subs) == 0 {
		return false, nil
	}
	p.batchHasMore = batching.Enabled && hasMore

	// Is there a sliding window limit configured?
	hasSliding := p.m.cfg.SlidingWindow &&
		p.m.cfg.SlidingWindowRate > 0 &&
		p.m.cfg.SlidingWindowDuration.Seconds() > 1

	// Push messages.
	for _, s := range subs {
		msg, err := p.newMessage(s)
		if err != nil {
			p.m.log.Printf("error rendering message (%s) (%s): %v", p.camp.Name, s.Email, err)
			continue
		}

		// Push the message to the queue while blocking and waiting until
		// the queue is drained.
		p.m.campMsgQ <- msg

		// Check if the sliding window is active.
		if hasSliding {
			diff := time.Since(p.m.slidingStart)

			// Window has expired. Reset the clock.
			if diff >= p.m.cfg.SlidingWindowDuration {
				p.m.slidingStart = time.Now()
				p.m.slidingCount = 0
			}

			// Have the messages exceeded the limit?
			p.m.slidingCount++
			if p.m.slidingCount >= p.m.cfg.SlidingWindowRate {
				wait := p.m.cfg.SlidingWindowDuration - diff

				p.m.log.Printf("messages exceeded (%d) for the window (%v since %s). Sleeping for %s.",
					p.m.slidingCount,
					p.m.cfg.SlidingWindowDuration,
					p.m.slidingStart.Format(time.RFC822Z),
					wait.Round(time.Second)*1)

				p.m.slidingCount = 0
				time.Sleep(wait)
			}
		}
	}

	if batching.Enabled {
		return false, nil
	}

	return hasMore, nil
}

// OnError keeps track of the number of errors that occur while sending messages
// and pauses the campaign if the error threshold is met.
func (p *pipe) OnError() {
	if p.m.cfg.MaxSendErrors < 1 {
		return
	}

	// If the error threshold is met, pause the campaign.
	count := p.errors.Add(1)
	if int(count) < p.m.cfg.MaxSendErrors {
		return
	}

	p.Stop(true)
	p.m.log.Printf("error count exceeded %d. pausing campaign %s", p.m.cfg.MaxSendErrors, p.camp.Name)
}

// Stop "marks" a campaign as stopped. It doesn't actually stop the processing
// of messages. That happens when every queued message in the campaign is processed,
// marking .wg, the waitgroup counter as done. That triggers cleanup().
func (p *pipe) Stop(withErrors bool) {
	// Already stopped.
	if p.stopped.Load() {
		return
	}

	if withErrors {
		p.withErrors.Store(true)
	}

	p.stopped.Store(true)
}

// newMessage returns a campaign message while internally incrementing the
// number of messages in the pipe wait group so that the status of every
// message can be atomically tracked.
func (p *pipe) newMessage(s models.Subscriber) (CampaignMessage, error) {
	msg, err := p.m.NewCampaignMessage(p.camp, s)
	if err != nil {
		return msg, err
	}

	msg.pipe = p
	p.wg.Add(1)

	return msg, nil
}

// cleanup finishes the campaign and updates the campaign status in the DB
// and also triggers a notification to the admin. This only triggers once
// a pipe's wg counter is fully exhausted, draining all messages in its queue.
func (p *pipe) cleanup() {
	defer func() {
		p.m.pipesMut.Lock()
		delete(p.m.pipes, p.camp.ID)
		p.m.pipesMut.Unlock()
	}()

	// Update campaign's 'sent count.
	if err := p.m.store.UpdateCampaignCounts(p.camp.ID, 0, int(p.sent.Load()), int(p.lastID.Load())); err != nil {
		p.m.log.Printf("error updating campaign counts (%s): %v", p.camp.Name, err)
	}

	// The campaign was auto-paused due to errors.
	if p.withErrors.Load() {
		if err := p.m.store.UpdateCampaignStatus(p.camp.ID, models.CampaignStatusPaused); err != nil {
			p.m.log.Printf("error updating campaign (%s) status to %s: %v", p.camp.Name, models.CampaignStatusPaused, err)
		} else {
			p.m.log.Printf("set campaign (%s) to %s", p.camp.Name, models.CampaignStatusPaused)
		}

		_ = p.m.sendNotif(p.camp, models.CampaignStatusPaused, "Too many errors")
		return
	}

	// The campaign was manually stopped (pause, cancel).
	if p.stopped.Load() {
		p.m.log.Printf("stop processing campaign (%s)", p.camp.Name)
		return
	}

	if p.batchHasMore {
		nextAt := nextBatchScheduleTime(p.camp.SendAt.Time, time.Now(), p.camp.Batching())
		if err := p.m.store.ScheduleCampaignBatch(p.camp.ID, nextAt); err != nil {
			p.m.log.Printf("error scheduling next campaign batch (%s): %v", p.camp.Name, err)
			return
		}
		p.m.log.Printf("scheduled next campaign batch (%s) at %s", p.camp.Name, nextAt.Format(time.RFC3339))
		return
	}

	// Campaign wasn't manually stopped and subscribers were naturally exhausted.
	// Fetch the up-to-date campaign status from the DB.
	c, err := p.m.store.GetCampaign(p.camp.ID)
	if err != nil {
		p.m.log.Printf("error fetching campaign (%s) for ending: %v", p.camp.Name, err)
		return
	}

	// If a running campaign has exhausted subscribers, it's finished.
	if c.Status == models.CampaignStatusRunning || c.Status == models.CampaignStatusScheduled {
		c.Status = models.CampaignStatusFinished
		if err := p.m.store.UpdateCampaignStatus(p.camp.ID, models.CampaignStatusFinished); err != nil {
			p.m.log.Printf("error finishing campaign (%s): %v", p.camp.Name, err)
		} else {
			p.m.log.Printf("campaign (%s) finished", p.camp.Name)
		}
	} else {
		p.m.log.Printf("finish processing campaign (%s)", p.camp.Name)
	}

	// Notify admin.
	_ = p.m.sendNotif(c, c.Status, "")
}

func nextBatchScheduleTime(anchor time.Time, now time.Time, cfg models.CampaignBatching) time.Time {
	if !cfg.Enabled || cfg.RepeatValue < 1 {
		return now
	}

	next := anchor
	if next.IsZero() {
		next = now
	}

	for i := 0; i < 4096; i++ {
		if !batchDayAllowed(next, cfg.Days) {
			next = startOfNextDay(next, cfg.StartTime)
			continue
		}

		if adjusted, ok := batchWindowAdjusted(next, cfg.StartTime, cfg.EndTime); ok {
			if adjusted.After(now) {
				return adjusted
			}
			next = addBatchInterval(adjusted, cfg)
			continue
		}
		next = startOfNextDay(next, cfg.StartTime)
	}

	return next
}

func addBatchInterval(t time.Time, cfg models.CampaignBatching) time.Time {
	switch cfg.RepeatUnit {
	case "days":
		return t.AddDate(0, 0, cfg.RepeatValue)
	case "quarter_hours":
		return t.Add(time.Duration(cfg.RepeatValue*15) * time.Minute)
	default:
		return t.Add(time.Duration(cfg.RepeatValue) * time.Hour)
	}
}

func batchDayAllowed(t time.Time, days []string) bool {
	if len(days) == 0 {
		return true
	}
	day := strings.ToLower(t.Weekday().String()[:3])
	for _, allowed := range days {
		if strings.ToLower(allowed) == day {
			return true
		}
	}
	return false
}

func batchWindowAdjusted(t time.Time, startTime string, endTime string) (time.Time, bool) {
	startHour, startMinute, hasStart := parseClock(startTime)
	endHour, endMinute, hasEnd := parseClock(endTime)

	if hasStart {
		start := time.Date(t.Year(), t.Month(), t.Day(), startHour, startMinute, 0, 0, t.Location())
		if t.Before(start) {
			t = start
		}
	}

	if hasEnd {
		end := time.Date(t.Year(), t.Month(), t.Day(), endHour, endMinute, 0, 0, t.Location())
		if t.After(end) {
			return time.Time{}, false
		}
	}

	return t, true
}

func startOfNextDay(t time.Time, startTime string) time.Time {
	next := t.AddDate(0, 0, 1)
	hour, minute, ok := parseClock(startTime)
	if !ok {
		return time.Date(next.Year(), next.Month(), next.Day(), next.Hour(), next.Minute(), 0, 0, next.Location())
	}
	return time.Date(next.Year(), next.Month(), next.Day(), hour, minute, 0, 0, next.Location())
}

func parseClock(value string) (int, int, bool) {
	if strings.TrimSpace(value) == "" {
		return 0, 0, false
	}
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, 0, false
	}
	return parsed.Hour(), parsed.Minute(), true
}
