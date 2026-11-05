package manager

import (
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/textproto"
	"strings"
	"sync"
	"time"

	"maps"

	"github.com/Masterminds/sprig/v3"
	"github.com/compdani/list_pocket/internal/i18n"
	"github.com/compdani/list_pocket/internal/notifs"
	"github.com/compdani/list_pocket/models"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const (
	// BaseTPL is the name of the base template.
	BaseTPL = "base"

	// ContentTpl is the name of the compiled message.
	ContentTpl = "content"
)

// Store represents a data backend, such as a database,
// that provides subscriber and campaign records.
type Store interface {
	NextCampaigns(currentIDs []int64, sentCounts []int64) ([]*models.Campaign, error)
	NextSubscribers(campID, limit int) ([]models.Subscriber, bool, error)
	GetCampaignLedgerMessageID(campaignID int, subscriberRecordID string) (string, error)
	GetCampaign(campID int) (*models.Campaign, error)
	GetAttachment(mediaID int) (models.Attachment, error)
	UpdateCampaignStatus(campID int, status string) error
	ScheduleCampaignBatch(campID int, sendAt time.Time) error
	UpdateCampaignCounts(campID int, toSend int, sent int, lastSubID int) error
	MarkCampaignLedgerSent(campaignID int, subscriberRecordID string) error
	MarkCampaignLedgerSkipped(campaignID int, subscriberRecordID string) error
	RollbackCampaignLedgerInflight(campaignID int, subscriberRecordID string) error
	ResetCampaignLedgerInflight(campaignID int) (int64, error)
	FinalizeCampaignLedgerStats(campaignID int) error
	// MarkSMSUnsendable opts the subscriber out of all SMS list memberships.
	// Record ID is preferred; phone is a fallback for other memberships sharing
	// the number. Used for permanent provider rejections such as opted-out or
	// international messaging not allowed.
	MarkSMSUnsendable(subscriberRecordID, phone string) (int64, error)
	CreateLink(url string) (string, error)
	CreateTransactionalMessage(models.TransactionalMessage) (models.TransactionalMessage, error)
	UpdateTransactionalMessageStatus(recordID, status, errorMessage string, sent bool) error
	BlocklistSubscriber(id int64) error
	DeleteSubscriber(id int64) error
}

// Messenger is an interface for a generic messaging backend,
// for instance, e-mail, SMS etc.
type Messenger interface {
	Name() string
	Push(models.Message) error
	Flush() error
	Close() error
}

// CampStats contains campaign stats like per minute send rate.
type CampStats struct {
	SendRate int
}

// Manager handles the scheduling, processing, and queuing of campaigns
// and message pushes.
type Manager struct {
	cfg        Config
	store      Store
	i18n       *i18n.I18n
	messengers map[string]Messenger
	fnNotify   func(subject string, data any) error
	log        *log.Logger

	// Campaigns that are currently running.
	pipes    map[int]*pipe
	pipesMut sync.RWMutex

	tpls    map[int]*models.Template
	tplsMut sync.RWMutex

	// Links generated using Track() are cached here so as to not query
	// the database for the link UUID for every message sent. This has to
	// be locked as it may be used externally when previewing campaigns.
	links    map[string]string
	linksMut sync.RWMutex

	nextPipes chan *pipe
	// fetchRetryDelay is how long Run waits before retrying a failed batch fetch.
	fetchRetryDelay time.Duration
	campMsgQ        chan CampaignMessage
	msgQ            chan models.Message
	deliveryQ       chan deliveryRequest
	deliveryRateMu  sync.Mutex
	deliveryStates  map[string]*deliveryRateState

	// Sliding window keeps track of the total number of messages sent in a period
	// and on reaching the specified limit, waits until the window is over before
	// sending further messages.
	slidingCount int
	slidingStart time.Time

	// SMS (Quo) sliding window and live send_limits from typed settings.
	smsLimitsFn     func() models.TextMessagingSendLimits
	smsLimitsMu     sync.RWMutex
	smsSlidingCount int
	smsSlidingStart time.Time

	lifecycle sync.RWMutex
	closed    bool
	started   bool
	stop      chan struct{}
	closeOnce sync.Once
	stopOnce  sync.Once
	runWG     sync.WaitGroup
	workerWG  sync.WaitGroup
	pipeWG    sync.WaitGroup
	scanMu    sync.Mutex

	tplFuncs template.FuncMap
}

// CampaignMessage represents an instance of campaign message to be pushed out,
// specific to a subscriber, via the campaign's messenger.
type CampaignMessage struct {
	Campaign   *models.Campaign
	Subscriber models.Subscriber

	from      string
	to        string
	subject   string
	body      []byte
	altBody   []byte
	unsubURL  string
	messageID string

	pipe *pipe
}

// Config has parameters for configuring the manager.
type Config struct {
	// Number of subscribers to pull from the DB in a single iteration.
	BatchSize             int
	Concurrency           int
	MessageRate           int
	LogVerbose            bool
	MaxSendErrors         int
	SlidingWindow         bool
	SlidingWindowDuration time.Duration
	SlidingWindowRate     int
	RequeueOnError        bool
	FromEmail             string
	IndividualTracking    bool
	DisableTracking       bool
	LinkTrackURL          string
	TxLinkTrackURL        string
	UnsubURL              string
	OptinURL              string
	MessageURL            string
	ViewTrackURL          string
	TxViewTrackURL        string
	ArchiveURL            string
	RootURL               string
	UnsubHeader           bool

	// Interval to scan the DB for active campaign checkpoints.
	ScanInterval time.Duration

	// ScanCampaigns indicates whether this instance of manager will scan the DB
	// for active campaigns and process them.
	// This can be used to run multiple instances of listmonk
	// (exposed to the internet, private etc.) where only one does campaign
	// processing while the others handle other kinds of traffic.
	ScanCampaigns bool
}

var pushTimeout = time.Second * 3

// New returns a new instance of Mailer.
func New(cfg Config, store Store, i *i18n.I18n, l *log.Logger) *Manager {
	if cfg.BatchSize < 1 {
		cfg.BatchSize = 1000
	}
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 1
	}
	if cfg.MessageRate < 1 {
		cfg.MessageRate = 1
	}

	m := &Manager{
		cfg:   cfg,
		store: store,
		i18n:  i,
		fnNotify: func(subject string, data any) error {
			return notifs.NotifySystem(subject, notifs.TplCampaignStatus, data, nil)
		},
		log:             l,
		messengers:      make(map[string]Messenger),
		pipes:           make(map[int]*pipe),
		tpls:            make(map[int]*models.Template),
		links:           make(map[string]string),
		stop:            make(chan struct{}),
		nextPipes:       make(chan *pipe, 1000),
		fetchRetryDelay: 5 * time.Second,
		campMsgQ:        make(chan CampaignMessage, cfg.Concurrency*cfg.MessageRate*2),
		msgQ:            make(chan models.Message, cfg.Concurrency*cfg.MessageRate*2),
		deliveryQ:       make(chan deliveryRequest, cfg.Concurrency),
		slidingStart:    time.Now(),
		smsSlidingStart: time.Now(),
	}
	m.tplFuncs = m.makeGnericFuncMap()

	return m
}

// SetSMSRateLimits supplies live SMS send_limits (e.g. from listpocket_settings text_messaging).
func (m *Manager) SetSMSRateLimits(fn func() models.TextMessagingSendLimits) {
	m.smsLimitsMu.Lock()
	m.smsLimitsFn = fn
	m.smsLimitsMu.Unlock()
}

func (m *Manager) smsSendLimits() models.TextMessagingSendLimits {
	m.smsLimitsMu.RLock()
	fn := m.smsLimitsFn
	m.smsLimitsMu.RUnlock()
	if fn == nil {
		return models.DefaultTextMessagingSettings().SendLimits
	}
	return fn()
}

// AddMessenger adds a Messenger messaging backend to the manager.
func (m *Manager) AddMessenger(msg Messenger) error {
	id := msg.Name()
	if _, ok := m.messengers[id]; ok {
		return fmt.Errorf("messenger '%s' is already loaded", id)
	}
	m.messengers[id] = msg

	return nil
}

// PushMessage pushes an arbitrary non-campaign Message to be sent out by the workers.
// It times out if the queue is busy.
func (m *Manager) PushMessage(msg models.Message) error {
	m.lifecycle.RLock()
	defer m.lifecycle.RUnlock()
	if m.closed {
		return errors.New("manager is closed")
	}
	t := time.NewTicker(pushTimeout)
	defer t.Stop()

	select {
	case <-m.stop:
		return fmt.Errorf("%w: manager stopping", models.ErrNotAttempted)
	case m.msgQ <- msg:
	case <-t.C:
		m.log.Printf("message push timed out: '%s'", msg.Subject)
		return errors.New("message push timed out")
	}

	return nil
}

// PushCampaignMessage pushes a campaign messages into a queue to be sent out by the workers.
// It times out if the queue is busy.
func (m *Manager) PushCampaignMessage(msg CampaignMessage) error {
	m.lifecycle.RLock()
	defer m.lifecycle.RUnlock()
	if m.closed {
		return errors.New("manager is closed")
	}
	t := time.NewTicker(pushTimeout)
	defer t.Stop()

	// Load any media/attachments.
	if err := m.attachMedia(msg.Campaign); err != nil {
		return err
	}

	select {
	case <-m.stop:
		return fmt.Errorf("%w: manager stopping", models.ErrNotAttempted)
	case m.campMsgQ <- msg:
	case <-t.C:
		m.log.Printf("message push timed out: '%s'", msg.Subject())
		return errors.New("message push timed out")
	}

	return nil
}

// HasMessenger checks if a given messenger is registered.
func (m *Manager) HasMessenger(id string) bool {
	_, ok := m.messengers[id]

	return ok
}

// HasRunningCampaigns checks if there are any active campaigns.
func (m *Manager) HasRunningCampaigns() bool {
	m.pipesMut.Lock()
	defer m.pipesMut.Unlock()

	return len(m.pipes) > 0
}

// GetCampaignStats returns campaign statistics.
func (m *Manager) GetCampaignStats(id int) CampStats {
	n := 0

	m.pipesMut.Lock()
	if c, ok := m.pipes[id]; ok {
		n = int(c.rate.Rate())
	}
	m.pipesMut.Unlock()

	return CampStats{SendRate: n}
}

// Run is a blocking function (that should be invoked as a goroutine)
// that scans the data source at regular intervals for pending campaigns,
// and queues them for processing. The process queue fetches batches of
// subscribers and pushes messages to them for each queued campaign
// until all subscribers are exhausted, at which point, a campaign is marked
// as "finished".
func (m *Manager) Run() {
	m.lifecycle.Lock()
	if m.closed || m.started {
		m.lifecycle.Unlock()
		return
	}
	m.started = true
	m.runWG.Add(1)
	m.lifecycle.Unlock()
	defer m.runWG.Done()
	if m.cfg.ScanCampaigns {
		// Periodically scan campaigns and push running campaigns to nextPipes
		// to fetch subscribers from the campaign.
		m.runWG.Add(1)
		go func() { defer m.runWG.Done(); m.scanCampaigns(m.cfg.ScanInterval) }()
	}

	// Spawn N message workers.
	for i := 0; i < m.cfg.Concurrency; i++ {
		m.workerWG.Add(1)
		go func() { defer m.workerWG.Done(); m.worker() }()
	}

	// Indefinitely wait on the pipe queue to fetch the next set of subscribers
	// for any active campaigns.
	for {
		var p *pipe
		select {
		case <-m.stop:
			return
		case p = <-m.nextPipes:
		}
		select {
		case <-m.stop:
			return
		default:
		}
		has, err := p.NextSubscribers()
		if err != nil {
			m.log.Printf("error processing campaign batch (%s): %v", p.camp.Name, err)
			// Retry later rather than stranding the pipe or spinning on the DB.
			if !m.waitOrStop(m.fetchRetryDelay) {
				return
			}
			select {
			case m.nextPipes <- p:
			default:
			}
			continue
		}

		if has {
			// There are more subscribers to fetch. Queue again.
			select {
			case m.nextPipes <- p:
			default:
			}
		} else {
			// The pipe is created with a +1 on the waitgroup pseudo counter
			// so that it immediately waits. Subsequently, every message created
			// is incremented in the counter in pipe.newMessage(), and when it's'
			// processed (or ignored when a campaign is paused or cancelled),
			// the count is's reduced in worker().
			//
			// This marks down the original non-message +1, causing the waitgroup
			// to be released and the pipe to end, triggering the pg.Wait()
			// in newPipe() that calls pipe.cleanup().
			p.finishProducing()
		}
	}
}

// ScanCampaignsOnce scans the store for campaigns ready to run and queues them.
func (m *Manager) ScanCampaignsOnce() {
	select {
	case <-m.stop:
		return
	default:
	}
	m.lifecycle.RLock()
	defer m.lifecycle.RUnlock()
	if m.closed {
		return
	}
	m.scanMu.Lock()
	defer m.scanMu.Unlock()
	ids, counts := m.getCurrentCampaigns()
	if m.cfg.LogVerbose {
		m.log.Printf("campaign scheduler: scan current_ids=%v sent_counts=%v", ids, counts)
	}
	campaigns, err := m.store.NextCampaigns(ids, counts)
	if err != nil {
		m.log.Printf("error fetching campaigns: %v", err)
		return
	}
	if len(campaigns) > 0 || m.cfg.LogVerbose {
		m.log.Printf("campaign scheduler: found %d runnable campaigns", len(campaigns))
	}

	for _, c := range campaigns {
		p, err := m.newPipe(c)
		if err != nil {
			m.log.Printf("error processing campaign (%s): %v", c.Name, err)
			continue
		}
		m.log.Printf("start processing campaign (%s)", c.Name)

		select {
		case m.nextPipes <- p:
		default:
		}
	}
}

// CacheTpl caches a template for ad-hoc use. This is currently only used by tx templates.
func (m *Manager) CacheTpl(id int, tpl *models.Template) {
	m.tplsMut.Lock()
	m.tpls[id] = tpl
	m.tplsMut.Unlock()
}

// DeleteTpl deletes a cached template.
func (m *Manager) DeleteTpl(id int) {
	m.tplsMut.Lock()
	delete(m.tpls, id)
	m.tplsMut.Unlock()
}

// GetTpl returns a cached template.
func (m *Manager) GetTpl(id int) (*models.Template, error) {
	m.tplsMut.RLock()
	tpl, ok := m.tpls[id]
	m.tplsMut.RUnlock()

	if !ok {
		return nil, fmt.Errorf("template %d not found", id)
	}

	return tpl, nil
}

// TemplateFuncs returns the template functions to be applied into
// compiled campaign templates.
func (m *Manager) TemplateFuncs(c *models.Campaign) template.FuncMap {
	f := template.FuncMap{}
	maps.Copy(f, m.tplFuncs)

	fns := template.FuncMap{
		"TrackLink": func(url string, msg *CampaignMessage) string {
			if m.cfg.DisableTracking {
				return url
			}

			subSeg := msg.Subscriber.RecordID
			if !m.cfg.IndividualTracking {
				subSeg = models.TrackingURLNoSubscriberSegment
			}

			return m.trackLink(url, msg.Campaign.RecordID, subSeg)
		},
		"TrackView": func(msg *CampaignMessage) template.HTML {
			if m.cfg.DisableTracking {
				return template.HTML("")
			}

			subSeg := msg.Subscriber.RecordID
			if !m.cfg.IndividualTracking {
				subSeg = models.TrackingURLNoSubscriberSegment
			}

			return template.HTML(fmt.Sprintf(`<img src="%s" alt="" />`,
				fmt.Sprintf(m.cfg.ViewTrackURL, msg.Campaign.RecordID, subSeg)))
		},
		"UnsubscribeURL": func(msg *CampaignMessage) string {
			return msg.unsubURL
		},
		"ManageURL": func(msg *CampaignMessage) string {
			return msg.unsubURL + "?manage=true"
		},
		"OptinURL": func(msg *CampaignMessage) string {
			// Add list IDs.
			// TODO: Show private lists list on optin e-mail
			return fmt.Sprintf(m.cfg.OptinURL, msg.Subscriber.RecordID, "")
		},
		"MessageURL": func(msg *CampaignMessage) string {
			return fmt.Sprintf(m.cfg.MessageURL, c.RecordID, msg.Subscriber.RecordID)
		},
		"ArchiveURL": func() string {
			return m.cfg.ArchiveURL
		},
		"RootURL": func() string {
			return m.cfg.RootURL
		},
	}
	maps.Copy(f, fns)

	return f
}

func (m *Manager) TxTemplateFuncs(msg *models.TransactionalMessage) template.FuncMap {
	f := template.FuncMap{}
	maps.Copy(f, m.tplFuncs)

	fns := template.FuncMap{
		"TrackLink": func(url string) string {
			if m.cfg.DisableTracking || msg == nil {
				return url
			}
			return m.trackTxLink(url, msg.UUID)
		},
		"TrackView": func() template.HTML {
			if m.cfg.DisableTracking || msg == nil {
				return template.HTML("")
			}
			return template.HTML(fmt.Sprintf(`<img src="%s" alt="" />`,
				fmt.Sprintf(m.cfg.TxViewTrackURL, msg.UUID)))
		},
	}
	maps.Copy(f, fns)

	return f
}

func (m *Manager) GenericTemplateFuncs() template.FuncMap {
	return m.tplFuncs
}

func (m *Manager) CreateTransactionalMessage(msg models.TransactionalMessage) (models.TransactionalMessage, error) {
	return m.store.CreateTransactionalMessage(msg)
}

func (m *Manager) UpdateTransactionalMessageStatus(recordID, status, errorMessage string, sent bool) error {
	return m.store.UpdateTransactionalMessageStatus(recordID, status, errorMessage, sent)
}

// StopCampaign marks a running campaign as stopped so that all its queued messages are ignored.
func (m *Manager) StopCampaign(id int) {
	m.pipesMut.RLock()
	if p, ok := m.pipes[id]; ok {
		p.Stop(false)
	}
	m.pipesMut.RUnlock()
}

// Close closes and exits the campaign manager.
// BeginShutdown independently signals blocked admission and rate waits. It
// never waits for the lifecycle lock held by a saturated queue submitter.
func (m *Manager) BeginShutdown() { m.stopOnce.Do(func() { close(m.stop) }) }

func (m *Manager) Close() {
	m.BeginShutdown()
	m.closeOnce.Do(func() {
		m.lifecycle.Lock()
		m.closed = true
		m.lifecycle.Unlock()
		m.pipesMut.RLock()
		for _, p := range m.pipes {
			p.Stop(false)
		}
		m.pipesMut.RUnlock()
		m.runWG.Wait()
		m.pipesMut.RLock()
		for _, p := range m.pipes {
			p.Stop(false)
			p.finishProducing()
		}
		m.pipesMut.RUnlock()
		close(m.campMsgQ)
		close(m.msgQ)
		close(m.deliveryQ)
		m.workerWG.Wait()
		m.pipeWG.Wait()
	})
}

// scanCampaigns is a blocking function that periodically scans the data source
// for campaigns to process and dispatches them to the manager. It feeds campaigns
// into nextPipes.
func (m *Manager) scanCampaigns(tick time.Duration) {
	t := time.NewTicker(tick)
	defer t.Stop()

	// Periodically scan the data source for campaigns to process.
	for {
		select {
		case <-m.stop:
			return
		case <-t.C:
			m.ScanCampaignsOnce()
		}
	}
}

// worker is a blocking function that perpetually listents to events (message) on different
// queues and processes them.
func (m *Manager) worker() {
	// Counter to keep track of the message / sec rate limit.
	campQ, msgQ := m.campMsgQ, m.msgQ
	deliveryQ := m.deliveryQ
	for campQ != nil || msgQ != nil || deliveryQ != nil {
		select {
		case request, ok := <-deliveryQ:
			if !ok {
				deliveryQ = nil
				continue
			}
			if !m.deliverySlot(request.msg) {
				request.done <- fmt.Errorf("%w: manager stopping", models.ErrNotAttempted)
				continue
			}
			request.done <- m.messengers[request.msg.Messenger].Push(request.msg)
		// Campaign message.
		case msg, ok := <-campQ:
			if !ok {
				campQ = nil
				continue
			}

			// If the campaign has ended or stopped, ignore the message.
			if msg.pipe != nil && msg.pipe.stopped.Load() {
				// Roll the ledger row back to 'pending' so a future run of this
				// campaign (e.g. after resume) can retry this subscriber instead
				// of leaving them permanently stranded as 'inflight'.
				if rbErr := m.store.RollbackCampaignLedgerInflight(msg.Campaign.ID, msg.Subscriber.RecordID); rbErr != nil {
					m.log.Printf("error rolling back campaign ledger inflight on stopped pipe (campaign %d): %v", msg.Campaign.ID, rbErr)
				}
				// Reduce the message counter on the pipe.
				msg.pipe.wg.Done()
				continue
			}

			spec := models.ChannelFor(msg.Campaign.Messenger)
			if !m.deliverySlot(models.Message{Messenger: msg.Campaign.Messenger}) {
				if msg.pipe != nil {
					if err := m.store.RollbackCampaignLedgerInflight(msg.Campaign.ID, msg.Subscriber.RecordID); err != nil {
						m.log.Printf("rollback interrupted campaign delivery: %v", err)
					}
					msg.pipe.wg.Done()
				}
				continue
			}

			// Outgoing message.
			to := []string{msg.to}
			if spec.Address == models.AddressSourceChannel {
				to = msg.Subscriber.Addresses
			}
			out := models.Message{
				From:        msg.from,
				To:          to,
				Subject:     msg.subject,
				ContentType: msg.Campaign.ContentType,
				Body:        msg.body,
				AltBody:     msg.altBody,
				Subscriber:  msg.Subscriber,
				Campaign:    msg.Campaign,
				Attachments: msg.Campaign.Attachments,
				Messenger:   msg.Campaign.Messenger,
			}

			h := textproto.MIMEHeader{}
			if spec.EmailHeaders {
				if mid := strings.TrimSpace(msg.messageID); mid != "" {
					if !strings.HasPrefix(mid, "<") {
						mid = "<" + mid
					}
					if !strings.HasSuffix(mid, ">") {
						mid = mid + ">"
					}
					h.Set(models.EmailHeaderMessageId, mid)
				}
				h.Set(models.EmailHeaderCampaignUUID, msg.Campaign.RecordID)
				h.Set(models.EmailHeaderSubscriberUUID, msg.Subscriber.RecordID)

				if m.cfg.UnsubHeader {
					h.Set("List-Unsubscribe-Post", "List-Unsubscribe=One-Click")
					h.Set("List-Unsubscribe", `<`+msg.unsubURL+`>`)
				}

				if len(msg.Campaign.Headers) > 0 {
					for _, set := range msg.Campaign.Headers {
						for hdr, val := range set {
							h.Add(hdr, val)
						}
					}
				}
			}

			out.Headers = h

			// Push the message to the messenger.
			err := m.messengers[msg.Campaign.Messenger].Push(out)
			unsendable := err != nil &&
				errors.Is(err, models.ErrUnsendableDestination) &&
				spec.Unsendable != models.UnsendableNone
			if err != nil {
				m.log.Printf("error sending message in campaign %s: subscriber %d: %v", msg.Campaign.Name, msg.Subscriber.ID, err)

				// Provider rejected this specific recipient permanently
				// (e.g. Quo opted-out, invalid number, international not allowed).
				// Opt them out of SMS, skip the ledger row so they are not
				// retried, and do not count this toward MaxSendErrors.
				if unsendable {
					if spec.Unsendable == models.UnsendableSMSOptOut {
						if n, mErr := m.store.MarkSMSUnsendable(msg.Subscriber.RecordID, msg.Subscriber.Phone); mErr != nil {
							m.log.Printf("error marking subscriber %d sms-unsendable: %v", msg.Subscriber.ID, mErr)
						} else if n > 0 {
							m.log.Printf("subscriber %d (%s): auto opted-out of SMS after provider rejection (%d list memberships)",
								msg.Subscriber.ID, msg.Subscriber.Phone, n)
						}
					}
					if msg.pipe != nil {
						if skErr := m.store.MarkCampaignLedgerSkipped(msg.Campaign.ID, msg.Subscriber.RecordID); skErr != nil {
							m.log.Printf("error marking campaign ledger skipped (campaign %d, subscriber %s): %v", msg.Campaign.ID, msg.Subscriber.RecordID, skErr)
						}
					}
				} else if msg.pipe != nil {
					if rbErr := m.store.RollbackCampaignLedgerInflight(msg.Campaign.ID, msg.Subscriber.RecordID); rbErr != nil {
						m.log.Printf("error rolling back campaign ledger inflight (campaign %d): %v", msg.Campaign.ID, rbErr)
					}
				}
			}

			// Increment the send rate or the error counter if there was an error.
			if msg.pipe != nil {
				// Mark the message as done.
				msg.pipe.wg.Done()

				if err != nil {
					if !unsendable {
						// Call the error callback, which keeps track of the error count
						// and stops the campaign if the error count exceeds the threshold.
						msg.pipe.OnError()
					}
				} else {
					id := uint64(msg.Subscriber.ID)
					if id > msg.pipe.lastID.Load() {
						msg.pipe.lastID.Store(uint64(msg.Subscriber.ID))
					}
					msg.pipe.rate.Incr(1)
					msg.pipe.sent.Add(1)
					if err := m.store.MarkCampaignLedgerSent(msg.Campaign.ID, msg.Subscriber.RecordID); err != nil {
						m.log.Printf("error marking campaign ledger sent (campaign %d, subscriber %s): %v", msg.Campaign.ID, msg.Subscriber.RecordID, err)
					}
				}
			}

		// Arbitrary message.
		case msg, ok := <-msgQ:
			if !ok {
				msgQ = nil
				continue
			}

			// Push the message to the messenger. It may be unknown here: a workflow
			// node can name one, or a plugin channel can have been disabled.
			messenger, ok := m.messengers[msg.Messenger]
			if !ok {
				err := fmt.Errorf("unknown messenger %q", msg.Messenger)
				m.log.Printf("error sending message '%s': %v", msg.Subject, err)
				if msg.TxMessage != nil {
					if updateErr := m.store.UpdateTransactionalMessageStatus(msg.TxMessage.RecordID, "failed", err.Error(), false); updateErr != nil {
						m.log.Printf("error updating transactional message status '%s': %v", msg.TxMessage.RecordID, updateErr)
					}
				}
				continue
			}
			if err := messenger.Push(msg); err != nil {
				m.log.Printf("error sending message '%s': %v", msg.Subject, err)
				if msg.TxMessage != nil {
					if updateErr := m.store.UpdateTransactionalMessageStatus(msg.TxMessage.RecordID, "failed", err.Error(), false); updateErr != nil {
						m.log.Printf("error updating transactional message status '%s': %v", msg.TxMessage.RecordID, updateErr)
					}
				}
			} else if msg.TxMessage != nil {
				if updateErr := m.store.UpdateTransactionalMessageStatus(msg.TxMessage.RecordID, "sent", "", true); updateErr != nil {
					m.log.Printf("error updating transactional message status '%s': %v", msg.TxMessage.RecordID, updateErr)
				}
			}
		}
	}
}

// getCurrentCampaigns returns the IDs of campaigns currently being processed
// and their sent counts.
func (m *Manager) getCurrentCampaigns() ([]int64, []int64) {
	// Needs to return an empty slice in case there are no campaigns.
	m.pipesMut.RLock()
	defer m.pipesMut.RUnlock()

	var (
		ids    = make([]int64, 0, len(m.pipes))
		counts = make([]int64, 0, len(m.pipes))
	)
	for _, p := range m.pipes {
		ids = append(ids, int64(p.camp.ID))

		// Get the sent counts for campaigns and reset them to 0
		// as in the database, they're stored cumulatively (sent += $newSent).
		counts = append(counts, p.sent.Load())
		p.sent.Store(0)
	}

	return ids, counts
}

// trackLink register a URL and return its UUID to be used in message templates
// for tracking links.
func (m *Manager) trackLink(url, campUUID, subUUID string) string {
	if m.cfg.DisableTracking {
		return url
	}

	url = strings.ReplaceAll(url, "&amp;", "&")

	m.linksMut.RLock()
	if uu, ok := m.links[url]; ok {
		m.linksMut.RUnlock()
		return fmt.Sprintf(m.cfg.LinkTrackURL, uu, campUUID, subUUID)
	}
	m.linksMut.RUnlock()

	// Register link.
	uu, err := m.store.CreateLink(url)
	if err != nil {
		m.log.Printf("error registering tracking for link '%s': %v", url, err)

		// If the registration fails, fail over to the original URL.
		return url
	}

	m.linksMut.Lock()
	m.links[url] = uu
	m.linksMut.Unlock()

	return fmt.Sprintf(m.cfg.LinkTrackURL, uu, campUUID, subUUID)
}

func (m *Manager) trackTxLink(url, msgUUID string) string {
	if m.cfg.DisableTracking {
		return url
	}

	url = strings.ReplaceAll(url, "&amp;", "&")

	m.linksMut.RLock()
	if uu, ok := m.links[url]; ok {
		m.linksMut.RUnlock()
		return fmt.Sprintf(m.cfg.TxLinkTrackURL, uu, msgUUID)
	}
	m.linksMut.RUnlock()

	uu, err := m.store.CreateLink(url)
	if err != nil {
		m.log.Printf("error registering tracking for transactional link '%s': %v", url, err)
		return url
	}

	m.linksMut.Lock()
	m.links[url] = uu
	m.linksMut.Unlock()

	return fmt.Sprintf(m.cfg.TxLinkTrackURL, uu, msgUUID)
}

// sendNotif sends a notification to registered admin e-mails.
func (m *Manager) sendNotif(c *models.Campaign, status, reason string) error {
	var (
		subject = fmt.Sprintf("%s: %s", cases.Title(language.Und).String(status), c.Name)
		data    = map[string]any{
			"ID":     c.ID,
			"Name":   c.Name,
			"Status": status,
			"Sent":   c.Sent,
			"ToSend": c.ToSend,
			"Reason": reason,
		}
	)

	return m.fnNotify(subject, data)
}

// makeGnericFuncMap returns a generic template func map with custom template
// functions and sprig template functions.
func (m *Manager) makeGnericFuncMap() template.FuncMap {
	funcs := template.FuncMap{
		"TrackView": func(...any) template.HTML {
			return template.HTML("")
		},
		"TrackLink": func(url string, _ ...any) string {
			return url
		},
		"UnsubscribeURL": func(...any) string {
			return ""
		},
		"ManageURL": func(...any) string {
			return ""
		},
		"OptinURL": func(...any) string {
			return ""
		},
		"MessageURL": func(...any) string {
			return ""
		},
		"ArchiveURL": func(...any) string {
			return ""
		},
		"Date": func(layout string) string {
			if layout == "" {
				layout = time.ANSIC
			}
			return time.Now().Format(layout)
		},
		"L": func() *i18n.I18n {
			return m.i18n
		},
		"Safe": func(safeHTML string) template.HTML {
			return template.HTML(safeHTML)
		},
	}

	// Copy spring functions.
	sprigFuncs := sprig.GenericFuncMap()
	delete(sprigFuncs, "env")
	delete(sprigFuncs, "expandenv")
	delete(sprigFuncs, "getHostByName")

	maps.Copy(funcs, sprigFuncs)

	return funcs
}

// attachMedia loads any media/attachments from the media store and attaches
// the byte blobs to the campaign.
func (m *Manager) attachMedia(c *models.Campaign) error {
	// Load any media/attachments.
	for _, mid := range []int64(c.MediaIDs) {
		a, err := m.store.GetAttachment(int(mid))
		if err != nil {
			return fmt.Errorf("error fetching attachment %d on campaign %s: %v", mid, c.Name, err)
		}

		c.Attachments = append(c.Attachments, a)
	}

	return nil
}

// MakeAttachmentHeader is a helper function that returns a
// textproto.MIMEHeader tailored for attachments, primarily
// email. If no encoding is given, base64 is assumed.
func MakeAttachmentHeader(filename, encoding, contentType string) textproto.MIMEHeader {
	if encoding == "" {
		encoding = "base64"
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", "attachment; filename="+filename)
	h.Set("Content-Type", fmt.Sprintf("%s; name=\""+filename+"\"", contentType))
	h.Set("Content-Transfer-Encoding", encoding)
	return h
}

// waitOrStop interrupts rate-limit delays when shutting down producers.
func (m *Manager) waitOrStop(delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-m.stop:
		return false
	case <-timer.C:
		return true
	}
}

// DeliverMessage performs a durable outbox's provider attempt synchronously.
// The caller owns persistence and ambiguity handling; close waits for the call.
type deliveryRequest struct {
	msg  models.Message
	done chan error
}

// DeliverMessage uses the manager's worker pool and rate admission. The short
// admission lock protects channel ownership; it is never held over provider I/O.
func (m *Manager) DeliverMessage(msg models.Message) error {
	m.lifecycle.RLock()
	if m.closed || !m.started {
		m.lifecycle.RUnlock()
		return fmt.Errorf("%w: manager unavailable", models.ErrNotAttempted)
	}
	if _, ok := m.messengers[msg.Messenger]; !ok {
		m.lifecycle.RUnlock()
		return fmt.Errorf("%w: unknown messenger %q", models.ErrNotAttempted, msg.Messenger)
	}
	request := deliveryRequest{msg: msg, done: make(chan error, 1)}
	select {
	case m.deliveryQ <- request:
		m.lifecycle.RUnlock()
	case <-m.stop:
		m.lifecycle.RUnlock()
		return fmt.Errorf("%w: manager stopping", models.ErrNotAttempted)
	}
	return <-request.done
}

type deliveryRateState struct {
	last, windowStart time.Time
	windowCount       int
}

// deliveryLimits preserves message_rate per worker; channel and SMS overrides
// are channel-global. Sliding limits permit bursts until the window fills.
func (m *Manager) deliveryLimits(messenger string) (int, time.Duration, int) {
	rate := m.cfg.MessageRate * m.cfg.Concurrency
	sliding, duration, count := m.cfg.SlidingWindow, m.cfg.SlidingWindowDuration, m.cfg.SlidingWindowRate
	spec := models.ChannelFor(messenger)
	if spec.SMSLimits {
		limits := m.smsSendLimits()
		if limits.MaxMessagesPerSecond > 0 {
			rate = limits.MaxMessagesPerSecond
		}
		sliding = limits.SlidingEnabled
		duration = time.Duration(limits.SlidingWindowSeconds) * time.Second
		count = limits.SlidingMaxMessages
	}
	if spec.MaxPerSecond > 0 {
		rate = spec.MaxPerSecond
	}
	if rate < 1 {
		rate = 1
	}
	if !sliding || duration <= 0 || count <= 0 {
		duration = 0
		count = 0
	}
	return rate, duration, count
}

// reserveDeliverySlot reserves a channel-local timestamp under the short mutex.
// Sleeping outside the lock keeps slow-channel windows from blocking email.
func (m *Manager) reserveDeliverySlot(messenger string, now time.Time) time.Duration {
	m.deliveryRateMu.Lock()
	defer m.deliveryRateMu.Unlock()
	if m.deliveryStates == nil {
		m.deliveryStates = map[string]*deliveryRateState{}
	}
	state := m.deliveryStates[messenger]
	if state == nil {
		state = &deliveryRateState{}
		m.deliveryStates[messenger] = state
	}
	rate, duration, count := m.deliveryLimits(messenger)
	slot := now
	if next := state.last.Add(time.Second / time.Duration(rate)); next.After(slot) {
		slot = next
	}
	if count > 0 {
		end := state.windowStart.Add(duration)
		if state.windowStart.IsZero() || !slot.Before(end) {
			state.windowStart = slot
			state.windowCount = 0
		} else if state.windowCount >= count {
			slot = end
			state.windowStart = slot
			state.windowCount = 0
		}
		state.windowCount++
	}
	state.last = slot
	return slot.Sub(now)
}
func (m *Manager) deliverySlot(msg models.Message) bool {
	wait := m.reserveDeliverySlot(msg.Messenger, time.Now())
	if wait > 0 && !m.waitOrStop(wait) {
		return false
	}
	select {
	case <-m.stop:
		return false
	default:
		return true
	}
}
