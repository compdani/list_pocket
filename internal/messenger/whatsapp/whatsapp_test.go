package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/internal/i18n"
	_ "github.com/compdani/list_pocket/internal/migrations"
	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/compdani/list_pocket/models"
	"github.com/pocketbase/pocketbase"
	pbcore "github.com/pocketbase/pocketbase/core"
)

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return signaturePrefix + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	body := []byte{'{', 0xff, 0xfe, '}'} // Not valid UTF-8, so only raw bytes verify.
	cases := []struct {
		name, secret, header string
		want                 bool
	}{
		{"good", "s3cret", sign("s3cret", body), true},
		{"wrong secret", "s3cret", sign("other", body), false},
		{"blank secret", "", sign("", body), false},
		{"no prefix", "s3cret", strings.TrimPrefix(sign("s3cret", body), signaturePrefix), false},
		{"not hex", "s3cret", signaturePrefix + "zz", false},
		{"missing", "s3cret", "", false},
	}
	for _, tc := range cases {
		if got := VerifySignature(body, tc.secret, tc.header); got != tc.want {
			t.Errorf("%s: got %v", tc.name, got)
		}
	}
	if TokensEqual("", "") {
		t.Error("a blank expected token must never match")
	}
}

func TestClassify(t *testing.T) {
	for code, want := range map[int]Action{
		131026: ActionRetire, 131050: ActionOptOut,
		131049: ActionError, 131047: ActionError, 130429: ActionError, 131056: ActionError,
		190: ActionError, 132001: ActionError, 0: ActionError, 999999: ActionError,
	} {
		if got := Classify(code); got != want {
			t.Errorf("code %d: got %v, want %v", code, got, want)
		}
	}
}

const cloudPayload = `{
  "object": "whatsapp_business_account",
  "entry": [{"id": "1", "changes": [
    {"field": "messages", "value": {
      "metadata": {"phone_number_id": "PN1"},
      "contacts": [{"wa_id": "15550100", "profile": {"name": "Ada"}}],
      "messages": [
        {"id": "wamid.1", "from": "15550100", "type": "text", "text": {"body": "hello"}},
        {"id": "wamid.2", "from": "15550100", "type": "button", "button": {"text": "Stop promotions", "payload": "Stop promotions"}},
        {"id": "wamid.3", "from": "15550100", "type": "image"}
      ],
      "statuses": [
        {"id": "wamid.out", "status": "failed", "recipient_id": "15550100", "errors": [{"code": 131026, "title": "Undeliverable"}]},
        {"id": "wamid.out2", "status": "read", "recipient_id": "15550100"}
      ]
    }},
    {"field": "messages", "value": {
      "metadata": {"phone_number_id": "OTHER"},
      "messages": [{"id": "wamid.other", "from": "1", "type": "text", "text": {"body": "not ours"}}]
    }},
    {"field": "user_preferences", "value": {
      "user_preferences": [
        {"wa_id": "15550100", "category": "marketing_messages", "value": "stop"},
        {"category": "marketing_messages", "value": "stop"}
      ]
    }}
  ]}]
}`

func TestParseCloudWebhook(t *testing.T) {
	events, err := ParseCloudWebhook([]byte(cloudPayload), "PN1")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 6 {
		t.Fatalf("got %d events: %+v", len(events), events)
	}
	if e := events[0]; e.Type != EventMessage || e.Text != "hello" || e.Name != "Ada" || e.From != "15550100" {
		t.Fatalf("message: %+v", e)
	}
	if events[1].Text != "Stop promotions" {
		t.Fatalf("button: %+v", events[1])
	}
	if events[2].InboundText() != "[image]" {
		t.Fatalf("media: %q", events[2].InboundText())
	}
	if e := events[3]; e.Type != EventStatus || e.ErrorCode != 131026 || e.Status != "failed" {
		t.Fatalf("status: %+v", e)
	}
	if e := events[5]; e.Type != EventPreference || e.Preference != "stop" || e.From != "15550100" {
		t.Fatalf("preference: %+v", e)
	}
	if _, err := ParseCloudWebhook([]byte("not json"), "PN1"); err == nil {
		t.Fatal("invalid JSON should fail")
	}
}

func TestParseWhapiWebhook(t *testing.T) {
	events, err := ParseWhapiWebhook([]byte(`{
	  "messages": [
	    {"id": "in1", "from_me": false, "type": "text", "chat_id": "15550100@s.whatsapp.net", "from": "15550100", "from_name": "Ada", "text": {"body": "hi"}},
	    {"id": "own", "from_me": true, "type": "text", "chat_id": "15550100@s.whatsapp.net", "from": "1999", "text": {"body": "our send"}},
	    {"id": "grp", "from_me": false, "type": "text", "chat_id": "123@g.us", "from": "15550100", "text": {"body": "a group"}},
	    {"id": "nl", "from_me": false, "type": "text", "chat_id": "120363@newsletter", "from": "120363", "text": {"body": "a channel"}}
	  ],
	  "statuses": [{"id": "out1", "code": 3, "status": "delivered", "recipient_id": "15550100@s.whatsapp.net", "timestamp": "1712995245"}],
	  "event": {"type": "messages", "event": "post"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].MessageID != "in1" || events[0].Name != "Ada" {
		t.Fatalf("own, group and channel messages must be skipped: %+v", events)
	}
	if e := events[1]; e.Status != "delivered" || e.RecipientID != "15550100" {
		t.Fatalf("status: %+v", e)
	}

	for body, connected := range map[string]bool{
		`{"user": {"id": "1"}, "event": {"type": "users", "event": "delete"}}`:                                false,
		`{"user": {"id": "1"}, "event": {"type": "users", "event": "post"}}`:                                  true,
		`{"health": {"status": {"code": 3, "text": "QR"}}, "event": {"type": "channel", "event": "post"}}`:    false,
		`{"health": {"status": {"text": "STOP"}}, "event": {"type": "channel", "event": "post"}}`:             false,
		`{"health": {"status": {"text": "SYNC_ERROR"}}, "event": {"type": "channel", "event": "post"}}`:       false,
		`{"health": {"status": {"code": 5, "text": "ERROR"}}, "event": {"type": "channel", "event": "post"}}`: false,
		`{"health": {"status": {"text": "AUTH"}}, "event": {"type": "channel", "event": "post"}}`:             true,
	} {
		events, err := ParseWhapiWebhook([]byte(body))
		if err != nil || len(events) != 1 || events[0].Type != EventConnection || events[0].Connected != connected {
			t.Fatalf("%s: %+v %v", body, events, err)
		}
	}
	// A channel that's still starting doesn't change the state shown.
	events, err = ParseWhapiWebhook([]byte(`{"health": {"status": {"code": 2, "text": "LAUNCH"}}, "event": {"type": "channel", "event": "post"}}`))
	if err != nil || len(events) != 0 {
		t.Fatalf("starting: %+v %v", events, err)
	}
}

func TestBuildTemplate(t *testing.T) {
	sub := models.Subscriber{Name: "Ada L", FirstName: "Ada", Attribs: models.JSON{"city": "Lima", "n": 3}}
	out, err := BuildTemplate(TemplateChoice{
		Name: "promo", Language: "en_US",
		Components: []ComponentChoice{
			{Type: "header", Format: "image", Link: "https://example.com/a.png"},
			{Type: "body", Parameters: []ParamChoice{
				{Value: "{{ first_name }}"},
				{Value: "{{attribs.city}} {{attribs.n}}"},
				{Value: "{{attribs.missing}}", Fallback: "friend"},
				{ParameterName: "code", Value: "ABC"},
			}},
			{Type: "button", SubType: "url", Index: "0", Parameters: []ParamChoice{{Value: "{{name}}"}}},
			{Type: "footer"},
		},
	}, sub)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Components) != 3 {
		t.Fatalf("a component with no values should be left out: %+v", out.Components)
	}
	if out.Components[0].Parameters[0].Image.Link != "https://example.com/a.png" {
		t.Fatalf("media: %+v", out.Components[0])
	}
	params := out.Components[1].Parameters
	if params[0].Text != "Ada" || params[1].Text != "Lima 3" || params[2].Text != "friend" || params[3].ParameterName != "code" {
		t.Fatalf("params: %+v", params)
	}
	if out.Components[2].SubType != "url" || out.Components[2].Parameters[0].Text != "Ada L" {
		t.Fatalf("button: %+v", out.Components[2])
	}

	if _, err := BuildTemplate(TemplateChoice{Language: "en"}, sub); !errors.Is(err, ErrNoTemplate) {
		t.Fatalf("no name: %v", err)
	}
	if _, err := BuildTemplate(TemplateChoice{Name: "x"}, sub); err == nil {
		t.Fatal("no language should fail")
	}
	if _, err := BuildTemplate(TemplateChoice{Name: "x", Language: "en", Components: []ComponentChoice{
		{Type: "body", Parameters: []ParamChoice{{Value: "{{attribs.missing}}"}}},
	}}, sub); err == nil {
		t.Fatal("an empty value with no fallback should fail, because the API rejects it")
	}
	if _, err := BuildTemplate(TemplateChoice{Name: "x", Language: "en", Components: []ComponentChoice{
		{Type: "header", Format: "audio", Link: "https://example.com/a"},
	}}, sub); err == nil {
		t.Fatal("an unsupported media type should fail")
	}
}

func TestParseCommand(t *testing.T) {
	for text, want := range map[string]int{
		"STOP": commandStop, " stop ": commandStop, "Unsubscribe": commandStop,
		"JOIN abc": commandJoin, "join abc": commandJoin,
		"please stop": commandNone, "stop it now": commandNone, "JOIN": commandNone, "": commandNone,
	} {
		if got, _ := parseCommand(text); got != want {
			t.Errorf("%q: got %d, want %d", text, got, want)
		}
	}
}

func TestWindowOpen(t *testing.T) {
	now := time.Now()
	if !WindowOpen(now.Add(-23*time.Hour), true, now) {
		t.Error("23 hours should be open")
	}
	if WindowOpen(now.Add(-25*time.Hour), true, now) {
		t.Error("25 hours should be closed")
	}
	if WindowOpen(now, false, now) {
		t.Error("someone who never messaged has no window")
	}
}

// mockAPI records requests and answers from a queue.
type mockAPI struct {
	mu       sync.Mutex
	requests []map[string]any
	paths    []string
	auth     []string
	status   int
	response string
}

func (m *mockAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	raw, _ := io.ReadAll(r.Body)
	var parsed map[string]any
	_ = json.Unmarshal(raw, &parsed)
	m.requests = append(m.requests, parsed)
	m.paths = append(m.paths, r.URL.Path)
	m.auth = append(m.auth, r.Header.Get("Authorization"))
	if m.status != 0 {
		w.WriteHeader(m.status)
	}
	_, _ = w.Write([]byte(m.response))
}

func (m *mockAPI) last() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.requests[len(m.requests)-1]
}

func TestCloudTemplatesFollowPagingOnOwnHostOnly(t *testing.T) {
	var srv *httptest.Server
	var hits int
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Query().Get("status") != "approved" {
			t.Errorf("only approved templates should be requested: %s", r.URL)
		}
		next := ""
		switch hits {
		case 1:
			next = srv.URL + "/v23.0/W1/message_templates?status=approved&after=x"
		case 2:
			next = "https://evil.example/steal?status=approved"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":   []map[string]any{{"name": "t", "language": "en", "status": "APPROVED"}},
			"paging": map[string]any{"next": next},
		})
	}))
	defer srv.Close()
	p := NewProvider(models.WhatsAppSettings{Provider: models.WhatsAppProviderCloud, AccessToken: "tok", WABAID: "W1", APIBase: srv.URL}, srv.Client())
	out, err := p.ListTemplates(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || hits != 2 {
		t.Fatalf("templates %d, hits %d: the token must not follow a link to another host", len(out), hits)
	}
}

// messengerFixture is a Messenger on a real schema, against a mock provider.
type messengerFixture struct {
	core *core.Core
	app  *pocketbase.PocketBase
	api  *mockAPI
	msgr *Messenger
	list string
	uuid string
	proc Processor
	s    *models.WhatsAppSettings
}

func TestPublishLinkPostSendsSavedTitle(t *testing.T) {
	f := newFixture(t, models.WhatsAppProviderWhapi)
	f.api.response = `{"sent":true,"message":{"id":"wh.link"}}`
	post, err := f.core.CreateChannelPost(core.ChannelPostInput{
		ChannelID: "1203@newsletter", Kind: PostLink, Body: "See https://example.com", Title: "Read this",
		Status: core.ChannelPostScheduled, SendAt: time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	f.msgr.PublishDuePosts()

	if got := f.api.last(); f.api.paths[0] != "/messages/link_preview" || got["title"] != "Read this" || got["to"] != "1203@newsletter" {
		t.Fatalf("request: %s %+v", f.api.paths[0], got)
	}
	saved, err := f.core.GetChannelPost(post.ID)
	if err != nil || saved.Status != core.ChannelPostSent || saved.ProviderMsgID != "wh.link" || saved.Title != "Read this" {
		t.Fatalf("saved: %+v %v", saved, err)
	}

	edited, err := f.core.CreateChannelPost(core.ChannelPostInput{ChannelID: "1203@newsletter", Kind: PostLink, Body: "https://a.example", Title: "Old", Status: core.ChannelPostDraft})
	if err != nil {
		t.Fatal(err)
	}
	edited, err = f.core.UpdateChannelPost(edited.ID, core.ChannelPostInput{ChannelID: "1203@newsletter", Kind: PostLink, Body: "https://a.example", Title: "New", Status: core.ChannelPostDraft})
	if err != nil || edited.Title != "New" {
		t.Fatalf("edit: %+v %v", edited, err)
	}
}

func newFixture(t *testing.T, provider string) *messengerFixture {
	t.Helper()
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.ResetBootstrapState() })
	if err := app.RunAppMigrations(); err != nil {
		t.Fatal(err)
	}
	lang, err := i18n.New([]byte(`{"_.code":"en","_.name":"English"}`))
	if err != nil {
		t.Fatal(err)
	}
	db, err := pbdb.NewFromPocketBase(app)
	if err != nil {
		t.Fatal(err)
	}
	c := core.New(&core.Opt{DB: db, I18n: lang, Log: log.New(io.Discard, "", 0)}, &core.Hooks{})
	if err := c.EnsureChannelTriggers(); err != nil {
		t.Fatal(err)
	}

	api := &mockAPI{response: `{"messages":[{"id":"wamid.out"}],"contacts":[{"wa_id":"15550100"}]}`}
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	s := &models.WhatsAppSettings{
		Enabled: true, Provider: provider,
		AccessToken: "tok", AppSecret: "sec", VerifyToken: "ver", PhoneNumberID: "PN1", WABAID: "W1", APIBase: srv.URL,
		WhapiToken: "wtok", WhapiRiskAccepted: true, WhapiAPIBase: srv.URL,
	}
	msgr := NewMessenger(func() models.WhatsAppSettings { return *s }, c, log.New(io.Discard, "", 0))

	col, err := app.FindCollectionByNameOrId("lists")
	if err != nil {
		t.Fatal(err)
	}
	rec := pbcore.NewRecord(col)
	const uuid = "3f2b8c1e-7a4d-4e9b-9c2a-1d5e6f7a8b90"
	rec.Set("name", "L")
	rec.Set("uuid", uuid)
	rec.Set("type", "public")
	rec.Set("optin", "single")
	rec.Set("status", "active")
	rec.Set("tags", []string{})
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	return &messengerFixture{core: c, app: app, api: api, msgr: msgr, list: rec.Id, uuid: uuid, proc: Processor{Core: c}, s: s}
}

func (f *messengerFixture) subscriber(t *testing.T, email, phone string) models.Subscriber {
	t.Helper()
	sub, _, err := f.core.InsertSubscriber(models.Subscriber{Email: email, Name: "Ada", FirstName: "Ada", Phone: phone}, nil, nil, true, false)
	if err != nil {
		t.Fatal(err)
	}
	return sub
}

func templateCampaign() *models.Campaign {
	return &models.Campaign{
		Base: models.Base{RecordID: ""},
		Attribs: models.JSON{CampaignFieldsAttrib: map[string]any{"template": map[string]any{
			"name": "promo", "language": "en_US",
			"components": []any{map[string]any{"type": "body", "parameters": []any{map[string]any{"value": "{{first_name}}"}}}},
		}}},
	}
}

func TestPushSendsTemplateOnCloud(t *testing.T) {
	f := newFixture(t, models.WhatsAppProviderCloud)
	sub := f.subscriber(t, "a@example.com", "+15550100")
	err := f.msgr.Push(models.Message{To: []string{"+15550100"}, Subscriber: sub, Campaign: templateCampaign()})
	if err != nil {
		t.Fatal(err)
	}
	got := f.api.last()
	if got["messaging_product"] != "whatsapp" || got["type"] != "template" || got["to"] != "+15550100" {
		t.Fatalf("request: %+v", got)
	}
	raw, _ := json.Marshal(got["template"])
	if !strings.Contains(string(raw), `"name":"promo"`) || !strings.Contains(string(raw), `"text":"Ada"`) {
		t.Fatalf("template: %s", raw)
	}
	if f.api.auth[0] != "Bearer tok" || !strings.HasSuffix(f.api.paths[0], "/PN1/messages") {
		t.Fatalf("auth %q, path %q", f.api.auth[0], f.api.paths[0])
	}
	events, _ := f.core.ListChannelEvents(sub.RecordID, MessengerName, 10)
	if len(events) != 1 || events[0].ProviderMsgID != "wamid.out" || events[0].Direction != core.ChannelDirectionOutbound {
		t.Fatalf("send not recorded: %+v", events)
	}
}

func TestPushRefusals(t *testing.T) {
	f := newFixture(t, models.WhatsAppProviderCloud)
	sub := f.subscriber(t, "a@example.com", "+15550100")

	if err := f.msgr.Push(models.Message{To: []string{"15550100"}, Subscriber: sub, Campaign: templateCampaign()}); !errors.Is(err, models.ErrUnsendableDestination) {
		t.Fatalf("a number with no country code must be skipped: %v", err)
	}
	if err := f.msgr.Push(models.Message{Subscriber: sub, Campaign: templateCampaign()}); !errors.Is(err, models.ErrUnsendableDestination) {
		t.Fatalf("no recipient: %v", err)
	}
	if err := f.msgr.Push(models.Message{To: []string{"+15550100"}, Subscriber: sub, Campaign: &models.Campaign{}}); !errors.Is(err, ErrNoTemplate) {
		t.Fatalf("a campaign with no template is a campaign problem, not a recipient one: %v", err)
	}
	if len(f.api.requests) != 0 {
		t.Fatalf("nothing should have been sent: %+v", f.api.requests)
	}

	f.s.Enabled = false
	if err := f.msgr.Push(models.Message{To: []string{"+15550100"}, Subscriber: sub, Campaign: templateCampaign()}); err == nil || errors.Is(err, models.ErrUnsendableDestination) {
		t.Fatalf("unconfigured must be a plain error: %v", err)
	}
}

func TestPushErrorHandling(t *testing.T) {
	cases := []struct {
		name       string
		code       int
		unsendable bool
		invalid    bool
		optedOut   bool
	}{
		{"not on whatsapp", 131026, true, true, false},
		{"marketing opt-out", 131050, true, false, true},
		{"rate limit", 130429, false, false, false},
		{"paused template", 132015, false, false, false},
		{"bad token", 190, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, models.WhatsAppProviderCloud)
			sub := f.subscriber(t, "a@example.com", "+15550100")
			if err := f.core.SetChannelConsent(sub.RecordID, f.list, MessengerName, "confirmed", "admin"); err != nil {
				t.Fatal(err)
			}
			f.api.status = http.StatusBadRequest
			raw, _ := json.Marshal(map[string]any{"error": map[string]any{"code": tc.code, "message": "m"}})
			f.api.response = string(raw)

			err := f.msgr.Push(models.Message{To: []string{"+15550100"}, Subscriber: sub, Campaign: templateCampaign()})
			if err == nil || errors.Is(err, models.ErrUnsendableDestination) != tc.unsendable {
				t.Fatalf("err = %v, unsendable want %v", err, tc.unsendable)
			}
			to, _ := f.core.ChannelRecipients(sub, MessengerName)
			if got := len(to) == 0; got != (tc.invalid || tc.optedOut) {
				t.Fatalf("recipients: %v", to)
			}
			out, _ := f.core.ChannelHasOptOut(sub.RecordID, MessengerName)
			if out != tc.optedOut {
				t.Fatalf("opted out = %v", out)
			}
		})
	}
}

func TestProcessorInboundContactAndCommands(t *testing.T) {
	f := newFixture(t, models.WhatsAppProviderCloud)

	apply := func(events ...Event) {
		t.Helper()
		if err := f.proc.Apply(events); err != nil {
			t.Fatal(err)
		}
	}
	msg := func(id, text string) Event {
		return Event{Type: EventMessage, MessageID: id, From: "15550100", Name: "Ada", Text: text, Kind: "text"}
	}

	apply(msg("m1", "hello"), msg("m1", "hello"))
	sub, ok, err := f.core.FindSubscriberByChannelAddress(MessengerName, "15550100")
	if err != nil || !ok {
		t.Fatalf("contact: %v %v", err, ok)
	}
	if !models.IsPlaceholderEmail(sub.Email) || sub.Phone == "" {
		t.Fatalf("expected a contact with a phone and no email: %+v", sub)
	}
	if to, _ := f.core.ChannelRecipients(sub, MessengerName); len(to) != 0 {
		t.Fatal("messaging us is not consent for campaigns")
	}
	events, _ := f.core.ListChannelEvents(sub.RecordID, MessengerName, 10)
	if len(events) != 1 {
		t.Fatalf("a repeated delivery must be recorded once: %+v", events)
	}

	apply(msg("m2", "JOIN "+f.uuid))
	if to, _ := f.core.ChannelRecipients(sub, MessengerName); len(to) != 1 {
		t.Fatal("JOIN should grant consent")
	}
	apply(msg("m3", "please stop by later"))
	if to, _ := f.core.ChannelRecipients(sub, MessengerName); len(to) != 1 {
		t.Fatal("a sentence containing stop is not an opt-out")
	}
	apply(msg("m4", "STOP"))
	if out, _ := f.core.ChannelHasOptOut(sub.RecordID, MessengerName); !out {
		t.Fatal("STOP should opt out")
	}
	// A retried JOIN from before the STOP must not undo it.
	apply(msg("m2", "JOIN "+f.uuid))
	if out, _ := f.core.ChannelHasOptOut(sub.RecordID, MessengerName); !out {
		t.Fatal("a redelivered JOIN must not undo a later STOP")
	}
	apply(msg("m5", "JOIN "+f.uuid))
	if to, _ := f.core.ChannelRecipients(sub, MessengerName); len(to) != 1 {
		t.Fatal("a new JOIN should restore consent")
	}
}

func TestProcessorStatusAndPreference(t *testing.T) {
	f := newFixture(t, models.WhatsAppProviderCloud)
	sub := f.subscriber(t, "a@example.com", "+15550100")
	if err := f.core.SetChannelConsent(sub.RecordID, f.list, MessengerName, "confirmed", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := f.msgr.Push(models.Message{To: []string{"+15550100"}, Subscriber: sub, Campaign: templateCampaign()}); err != nil {
		t.Fatal(err)
	}

	status := func(s string, code int) {
		t.Helper()
		if err := f.proc.Apply([]Event{{Type: EventStatus, MessageID: "wamid.out", Status: s, ErrorCode: code, RecipientID: "15550100"}}); err != nil {
			t.Fatal(err)
		}
	}
	status("read", 0)
	status("delivered", 0)
	events, _ := f.core.ListChannelEvents(sub.RecordID, MessengerName, 10)
	if events[0].Status != "read" {
		t.Fatalf("out-of-order delivered must not overwrite read: %q", events[0].Status)
	}

	status("failed", 131049)
	if to, _ := f.core.ChannelRecipients(sub, MessengerName); len(to) != 1 {
		t.Fatal("the marketing limit must not retire the number")
	}
	status("failed", 131026)
	if to, _ := f.core.ChannelRecipients(sub, MessengerName); len(to) != 0 {
		t.Fatal("131026 should retire the number")
	}
	if out, _ := f.core.ChannelHasOptOut(sub.RecordID, MessengerName); out {
		t.Fatal("a retired number isn't an opt-out")
	}

	other := f.subscriber(t, "b@example.com", "+15550111")
	if err := f.core.SetChannelConsent(other.RecordID, f.list, MessengerName, "confirmed", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := f.proc.Apply([]Event{{Type: EventPreference, From: "15550111", Preference: "resume"}}); err != nil {
		t.Fatal(err)
	}
	if out, _ := f.core.ChannelHasOptOut(other.RecordID, MessengerName); out {
		t.Fatal("resume must do nothing")
	}
	if err := f.proc.Apply([]Event{{Type: EventPreference, From: "15550111", Preference: "stop"}}); err != nil {
		t.Fatal(err)
	}
	if out, _ := f.core.ChannelHasOptOut(other.RecordID, MessengerName); !out {
		t.Fatal("a marketing stop should opt out")
	}
}

func TestAliasFromSendFindsReplyAuthor(t *testing.T) {
	f := newFixture(t, models.WhatsAppProviderCloud)
	sub := f.subscriber(t, "a@example.com", "+5511912345678")
	f.api.response = `{"messages":[{"id":"wamid.br"}],"contacts":[{"wa_id":"551112345678"}]}`
	if err := f.msgr.Push(models.Message{To: []string{"+5511912345678"}, Subscriber: sub, Campaign: templateCampaign()}); err != nil {
		t.Fatal(err)
	}
	if err := f.proc.Apply([]Event{{Type: EventMessage, MessageID: "m1", From: "551112345678", Text: "oi", Kind: "text"}}); err != nil {
		t.Fatal(err)
	}
	events, _ := f.core.ListChannelEvents(sub.RecordID, MessengerName, 10)
	if len(events) != 2 {
		t.Fatalf("the reply should land on the same subscriber: %+v", events)
	}
}

func TestReplyRules(t *testing.T) {
	f := newFixture(t, models.WhatsAppProviderCloud)
	sub := f.subscriber(t, "a@example.com", "+15550100")

	reason := func(err error) string {
		var refused *ReplyError
		if errors.As(err, &refused) {
			return refused.Reason
		}
		return ""
	}
	if got := reason(f.msgr.Reply(sub, "hi")); got != ReplyWindowClosed {
		t.Fatalf("no inbound yet: %q", got)
	}
	if err := f.proc.Apply([]Event{{Type: EventMessage, MessageID: "m1", From: "15550100", Text: "help", Kind: "text"}}); err != nil {
		t.Fatal(err)
	}
	if err := f.msgr.Reply(sub, "hi there"); err != nil {
		t.Fatalf("reply in window: %v", err)
	}
	if got := f.api.last(); got["type"] != "text" {
		t.Fatalf("request: %+v", got)
	}
	thread, err := f.msgr.GetThread(sub)
	if err != nil {
		t.Fatal(err)
	}
	if !thread.WindowOpen || !thread.RequiresWindow || thread.WindowClosesAt == nil || len(thread.Events) != 2 || thread.Events[0].Body != "hi there" {
		t.Fatalf("thread: %+v", thread)
	}

	f.api.status = http.StatusBadRequest
	f.api.response = `{"error":{"code":131047,"message":"window"}}`
	if got := reason(f.msgr.Reply(sub, "again")); got != ReplyWindowClosed {
		t.Fatalf("a provider window error should be explained: %q", got)
	}
	f.api.status, f.api.response = 0, `{"messages":[{"id":"wamid.x"}]}`

	if _, err := f.core.OptOutChannelSubscriber(sub.RecordID, MessengerName); err != nil {
		t.Fatal(err)
	}
	if got := reason(f.msgr.Reply(sub, "hi")); got != ReplyOptedOut {
		t.Fatalf("opted out: %q", got)
	}

	noPhone := f.subscriber(t, "b@example.com", "")
	if got := reason(f.msgr.Reply(noPhone, "hi")); got != ReplyNoPhone {
		t.Fatalf("no phone: %q", got)
	}
}

func TestWhapiProvider(t *testing.T) {
	f := newFixture(t, models.WhatsAppProviderWhapi)
	f.api.response = `{"sent":true,"message":{"id":"wh.1"}}`
	sub := f.subscriber(t, "a@example.com", "+15550100")

	if err := f.msgr.Reply(sub, "no window on whapi"); err != nil {
		t.Fatalf("whapi has no window: %v", err)
	}
	got := f.api.last()
	if got["to"] != "15550100" || got["body"] != "no window on whapi" || f.api.auth[0] != "Bearer wtok" || f.api.paths[0] != "/messages/text" {
		t.Fatalf("request: %+v %v %v", got, f.api.auth, f.api.paths)
	}

	if err := f.msgr.Push(models.Message{To: []string{"+15550100"}, Subscriber: sub, Body: []byte("Hello Ada"), Campaign: &models.Campaign{}}); err != nil {
		t.Fatal(err)
	}
	if got := f.api.last(); got["body"] != "Hello Ada" {
		t.Fatalf("campaign body: %+v", got)
	}
	if err := f.msgr.Push(models.Message{To: []string{"+15550100"}, Subscriber: sub, Body: []byte("  "), Campaign: &models.Campaign{}}); err == nil {
		t.Fatal("an empty body should fail")
	}

	// An unclear refusal stays retryable and doesn't retire the number.
	f.api.status = http.StatusForbidden
	f.api.response = `{"error":{"code":403,"message":"forbidden"}}`
	err := f.msgr.Push(models.Message{To: []string{"+15550100"}, Subscriber: sub, Body: []byte("x"), Campaign: &models.Campaign{}})
	if err == nil || errors.Is(err, models.ErrUnsendableDestination) {
		t.Fatalf("should stay retryable: %v", err)
	}
	f.api.status = 0

	provider, _ := f.msgr.Provider()
	if _, err := provider.ListTemplates(t.Context()); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("templates: %v", err)
	}
	f.api.response = `{"sent":true,"sent_message":{"id":"wh.2"}}`
	res, err := provider.PostToChannel(t.Context(), ChannelPost{ChannelID: "1203@newsletter", Kind: PostImage, Body: "cap", MediaURL: "https://example.com/a.png"})
	if err != nil || res.MessageID != "wh.2" {
		t.Fatalf("post: %+v %v", res, err)
	}
	if got := f.api.last(); got["media"] != "https://example.com/a.png" || got["to"] != "1203@newsletter" || got["caption"] != "cap" {
		t.Fatalf("post request: %+v", got)
	}
	if _, err := provider.PostToChannel(t.Context(), ChannelPost{Kind: "poll"}); err == nil {
		t.Fatal("an unsupported post kind should fail")
	}
	if _, err := provider.PostToChannel(t.Context(), ChannelPost{ChannelID: "1203@newsletter", Kind: PostLink, Body: "https://example.com", Title: "Read this"}); err != nil {
		t.Fatalf("link post: %v", err)
	}
	if got := f.api.last(); f.api.paths[len(f.api.paths)-1] != "/messages/link_preview" || got["title"] != "Read this" || got["body"] != "https://example.com" {
		t.Fatalf("link post request: %s %+v", f.api.paths[len(f.api.paths)-1], got)
	}
	if err := ValidatePost(ChannelPost{ChannelID: "1203@newsletter", Kind: PostLink, Body: "https://example.com"}); err == nil {
		t.Fatal("a link post with no title should fail")
	}
	if err := ValidatePost(ChannelPost{ChannelID: "1203@newsletter", Kind: PostLink, Body: "https://example.com", Title: "Read this"}); err != nil {
		t.Fatalf("link post: %v", err)
	}
	for body, ok := range map[string]bool{
		`{"status":{"code":4,"text":"AUTH"}}`:   true,
		`{"status":{"code":3,"text":"QR"}}`:     false,
		`{"status":{"text":"STOP"}}`:            false,
		`{"status":{"text":"SYNC_ERROR"}}`:      false,
		`{"status":{"code":2,"text":"LAUNCH"}}`: false,
		`{"uptime":5}`:                          false,
	} {
		f.api.response = body
		if err := provider.CheckConnection(t.Context()); (err == nil) != ok {
			t.Fatalf("%s: %v", body, err)
		}
	}
	f.api.response = `{"newsletters":[{"id":"1203@newsletter","name":"News"}]}`
	channels, err := provider.ListChannels(t.Context())
	if err != nil || len(channels) != 1 || channels[0].Name != "News" {
		t.Fatalf("channels: %+v %v", channels, err)
	}

	cloud := NewProvider(models.WhatsAppSettings{}, nil)
	if _, err := cloud.PostToChannel(t.Context(), ChannelPost{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("cloud can't post to channels: %v", err)
	}
}

func TestSettingsSafety(t *testing.T) {
	s := models.WhatsAppSettings{Enabled: true, AccessToken: "tok", AppSecret: "sec", VerifyToken: "ver", PhoneNumberID: "P", WhapiToken: "w", WhapiWebhookToken: "hook"}
	raw, _ := json.Marshal(s.ToResponse("https://x.test"))
	for _, secret := range []string{`"tok"`, `"sec"`, `"ver"`, `"w"`} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("%s leaked: %s", secret, raw)
		}
	}
	merged := models.MergeWhatsAppUpdate(s, models.WhatsAppSettings{Enabled: true, PhoneNumberID: "P2"})
	if merged.AccessToken != "tok" || merged.AppSecret != "sec" || merged.WhapiWebhookToken != "hook" || merged.PhoneNumberID != "P2" {
		t.Fatalf("merge: %+v", merged)
	}

	if (models.WhatsAppSettings{Enabled: true, Provider: models.WhatsAppProviderWhapi, WhapiToken: "t"}).Configured() {
		t.Fatal("whapi must not be usable until the risk is accepted")
	}
	if !(models.WhatsAppSettings{Enabled: true, AccessToken: "t", PhoneNumberID: "P"}).Configured() {
		t.Fatal("cloud should be configured")
	}
	cloud := models.WhatsAppChannelSpec(models.WhatsAppSettings{})
	whapi := models.WhatsAppChannelSpec(models.WhatsAppSettings{Provider: models.WhatsAppProviderWhapi})
	if cloud.CampaignBody != models.CampaignBodyNone || whapi.CampaignBody != "" || whapi.MaxPerSecond != 1 || cloud.MaxPerSecond != 40 {
		t.Fatalf("specs: %+v %+v", cloud, whapi)
	}
}
