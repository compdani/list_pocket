package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/compdani/list_pocket/internal/apperr"
	"github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/internal/messenger/whatsapp"
	"github.com/compdani/list_pocket/models"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	pbcore "github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"
)

const (
	maxWhatsAppWebhookBytes = 3 << 20
	maxWhatsAppReplyChars   = 4096
	whatsAppTemplateTTL     = 5 * time.Minute
)

func loadWhatsAppSettingsFromPB(pb *pocketbase.PocketBase) models.WhatsAppSettings {
	rec, err := pb.FindFirstRecordByFilter("listpocket_settings", "type={:type}", dbx.Params{"type": models.ListPocketSettingsTypeWhatsApp})
	if err != nil || rec == nil {
		return models.WhatsAppSettings{}.Normalized()
	}
	var out models.WhatsAppSettings
	if err := json.Unmarshal([]byte(rec.GetString("value")), &out); err != nil {
		lo.Printf("error reading WhatsApp settings: %v", err)
		return models.WhatsAppSettings{}.Normalized()
	}
	return out.Normalized()
}

func (a *App) saveWhatsAppSettings(s models.WhatsAppSettings) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return a.upsertTypedSettingsRecord(models.ListPocketSettingsTypeWhatsApp, string(raw))
}

// GetWhatsAppSettings returns settings without secret values.
func (a *App) GetWhatsAppSettings(re *pbcore.RequestEvent) error {
	s := loadWhatsAppSettingsFromPB(a.pb)
	return okJSON(re, struct {
		models.WhatsAppSettingsResponse
		Capabilities whatsapp.Capabilities `json:"capabilities"`
	}{s.ToResponse(a.urlCfg.RootURL), whatsapp.NewProvider(s, nil).Capabilities()})
}

// UpdateWhatsAppSettings saves settings. Changing provider or rate changes how the
// channel behaves, which is fixed at boot, so those restart the app.
func (a *App) UpdateWhatsAppSettings(re *pbcore.RequestEvent) error {
	var in models.WhatsAppSettings
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	cur := loadWhatsAppSettingsFromPB(a.pb)
	next := models.MergeWhatsAppUpdate(cur, in)
	if err := validateWhatsAppSettings(next); err != nil {
		return apperr.BadRequest(err.Error())
	}

	needsRestart := next.Provider != cur.Provider || next.SendRate() != cur.SendRate()
	if next.Provider != cur.Provider {
		n, err := a.core.CountCampaignsOnChannels([]string{models.ChannelWhatsApp})
		if err != nil {
			return apperr.Internal(err.Error())
		}
		if n > 0 {
			return apperr.Conflict(fmt.Sprintf("%d running or scheduled WhatsApp campaigns need this provider. Pause or finish them first.", n))
		}
	}
	if next.Provider == models.WhatsAppProviderWhapi && next.WhapiWebhookToken == "" {
		next.WhapiWebhookToken = security.RandomString(40)
	}
	if err := a.saveWhatsAppSettings(next); err != nil {
		return apperr.Internal(err.Error())
	}
	a.clearWhatsAppTemplateCache()
	if needsRestart {
		return a.handleSettingsRestart(re)
	}
	return okJSON(re, true)
}

func validateWhatsAppSettings(s models.WhatsAppSettings) error {
	if !s.Enabled {
		return nil
	}
	if s.MaxPerSecond < 0 {
		return errors.New("the send rate can't be negative")
	}
	if s.Provider == models.WhatsAppProviderWhapi {
		if !s.WhapiRiskAccepted {
			return errors.New("accept the Whapi risk notice to enable it")
		}
		if s.WhapiToken == "" {
			return errors.New("a Whapi token is required")
		}
		return nil
	}
	switch {
	case s.AccessToken == "":
		return errors.New("an access token is required")
	case s.AppSecret == "":
		return errors.New("an app secret is required to verify webhooks")
	case s.VerifyToken == "":
		return errors.New("a verify token is required")
	case s.PhoneNumberID == "":
		return errors.New("a phone number ID is required")
	case s.WABAID == "":
		return errors.New("a business account ID is required")
	}
	return nil
}

// TestWhatsAppSettings checks the saved credentials against the provider.
func (a *App) TestWhatsAppSettings(re *pbcore.RequestEvent) error {
	provider, err := a.whatsapp.Provider()
	if err != nil {
		return apperr.BadRequest(err.Error())
	}
	ctx, cancel := context.WithTimeout(re.Request.Context(), 15*time.Second)
	defer cancel()
	if err := provider.CheckConnection(ctx); err != nil {
		return apperr.BadRequest(err.Error())
	}
	return okJSON(re, true)
}

// whatsAppWebhookBody reads a webhook body up to the limit.
func whatsAppWebhookBody(re *pbcore.RequestEvent) ([]byte, error) {
	return io.ReadAll(http.MaxBytesReader(re.Response, re.Request.Body, maxWhatsAppWebhookBytes))
}

func whatsAppWebhookStatus(re *pbcore.RequestEvent, status int) error {
	return re.JSON(status, map[string]any{"ok": status == http.StatusOK})
}

// WhatsAppWebhookVerify answers Meta's GET handshake.
func (a *App) WhatsAppWebhookVerify(re *pbcore.RequestEvent) error {
	s := loadWhatsAppSettingsFromPB(a.pb)
	q := re.Request.URL.Query()
	if !s.Enabled || s.Provider != models.WhatsAppProviderCloud || q.Get("hub.mode") != "subscribe" ||
		!whatsapp.TokensEqual(s.VerifyToken, q.Get("hub.verify_token")) {
		return whatsAppWebhookStatus(re, http.StatusForbidden)
	}
	re.Response.Header().Set("Content-Type", "text/plain; charset=utf-8")
	re.Response.Header().Set("X-Content-Type-Options", "nosniff")
	re.Response.WriteHeader(http.StatusOK)
	_, err := re.Response.Write([]byte(q.Get("hub.challenge")))
	return err
}

// WhatsAppWebhook receives Cloud API events. The signature is checked on the raw bytes
// before anything is parsed. Anything other than a 200 makes Meta retry for up to
// 7 days, so a 200 only goes out once everything is stored.
func (a *App) WhatsAppWebhook(re *pbcore.RequestEvent) error {
	s := loadWhatsAppSettingsFromPB(a.pb)
	if !s.Enabled || s.Provider != models.WhatsAppProviderCloud {
		return whatsAppWebhookStatus(re, http.StatusNotFound)
	}
	body, err := whatsAppWebhookBody(re)
	if err != nil {
		return whatsAppWebhookStatus(re, http.StatusRequestEntityTooLarge)
	}
	if !whatsapp.VerifySignature(body, s.AppSecret, re.Request.Header.Get("X-Hub-Signature-256")) {
		return whatsAppWebhookStatus(re, http.StatusUnauthorized)
	}
	events, err := whatsapp.ParseCloudWebhook(body, s.PhoneNumberID)
	if err != nil {
		// Signed but unreadable. Retrying can't fix it.
		a.log.Printf("whatsapp webhook: %v", err)
		return whatsAppWebhookStatus(re, http.StatusOK)
	}
	return a.applyWhatsAppEvents(re, events)
}

// WhatsAppWhapiWebhook receives Whapi events, authorized by the secret in the path.
func (a *App) WhatsAppWhapiWebhook(re *pbcore.RequestEvent) error {
	s := loadWhatsAppSettingsFromPB(a.pb)
	if !s.Enabled || s.Provider != models.WhatsAppProviderWhapi ||
		!whatsapp.TokensEqual(s.WhapiWebhookToken, pathParam(re, "token")) {
		return whatsAppWebhookStatus(re, http.StatusNotFound)
	}
	body, err := whatsAppWebhookBody(re)
	if err != nil {
		return whatsAppWebhookStatus(re, http.StatusRequestEntityTooLarge)
	}
	events, err := whatsapp.ParseWhapiWebhook(body)
	if err != nil {
		a.log.Printf("whatsapp webhook: %v", err)
		return whatsAppWebhookStatus(re, http.StatusOK)
	}
	return a.applyWhatsAppEvents(re, events)
}

func (a *App) applyWhatsAppEvents(re *pbcore.RequestEvent, events []whatsapp.Event) error {
	processor := whatsapp.Processor{Core: a.core, OnConnection: a.saveWhapiConnection}
	if err := processor.Apply(events); err != nil {
		a.log.Printf("whatsapp webhook: %v", err)
		return whatsAppWebhookStatus(re, http.StatusInternalServerError)
	}
	return whatsAppWebhookStatus(re, http.StatusOK)
}

func (a *App) saveWhapiConnection(connected bool, note string) error {
	s := loadWhatsAppSettingsFromPB(a.pb)
	s.WhapiConnected = &connected
	s.WhapiConnectionNote = note
	if !connected {
		a.log.Printf("whatsapp: %s", note)
	}
	return a.saveWhatsAppSettings(s)
}

// whatsAppTemplateCache keeps the picker from calling the API on every page load.
type whatsAppTemplateCache struct {
	sync.Mutex
	templates []whatsapp.Template
	at        time.Time
}

func (a *App) clearWhatsAppTemplateCache() {
	a.whatsappTemplates.Lock()
	defer a.whatsappTemplates.Unlock()
	a.whatsappTemplates.templates = nil
}

// GetWhatsAppTemplates lists approved templates. ?refresh=1 skips the cache.
func (a *App) GetWhatsAppTemplates(re *pbcore.RequestEvent) error {
	provider, err := a.whatsapp.Provider()
	if err != nil {
		return apperr.BadRequest(err.Error())
	}
	cache := &a.whatsappTemplates
	cache.Lock()
	defer cache.Unlock()
	if cache.templates != nil && time.Since(cache.at) < whatsAppTemplateTTL && queryParam(re, "refresh") != "1" {
		return re.JSON(http.StatusOK, okResp{Data: cache.templates})
	}
	ctx, cancel := context.WithTimeout(re.Request.Context(), 30*time.Second)
	defer cancel()
	templates, err := provider.ListTemplates(ctx)
	if errors.Is(err, whatsapp.ErrUnsupported) {
		return apperr.BadRequest("this WhatsApp provider doesn't use templates")
	}
	if err != nil {
		return apperr.BadRequest(err.Error())
	}
	cache.templates, cache.at = templates, time.Now()
	return re.JSON(http.StatusOK, okResp{Data: templates})
}

// GetWhatsAppCampaignStats returns a campaign's sends by delivery status.
func (a *App) GetWhatsAppCampaignStats(re *pbcore.RequestEvent) error {
	out, err := a.core.CountChannelEvents(pathParam(re, "id"))
	if err != nil {
		return apperr.Internal(err.Error())
	}
	return okJSON(re, out)
}

func (a *App) whatsAppSubscriber(re *pbcore.RequestEvent) (models.Subscriber, error) {
	sub, ok, err := a.core.GetSubscriberForChannel(pathParam(re, "id"))
	if err != nil {
		return sub, apperr.Internal(err.Error())
	}
	if !ok {
		return sub, apperr.NotFound("subscriber not found")
	}
	return sub, nil
}

// GetWhatsAppThread returns a subscriber's conversation and whether they can be replied to.
func (a *App) GetWhatsAppThread(re *pbcore.RequestEvent) error {
	sub, err := a.whatsAppSubscriber(re)
	if err != nil {
		return err
	}
	out, err := a.whatsapp.GetThread(sub)
	if err != nil {
		return apperr.Internal(err.Error())
	}
	return re.JSON(http.StatusOK, okResp{Data: out})
}

// SendWhatsAppReply sends staff text to a subscriber. Refusals carry a reason code,
// so the reply box can explain them.
func (a *App) SendWhatsAppReply(re *pbcore.RequestEvent) error {
	sub, err := a.whatsAppSubscriber(re)
	if err != nil {
		return err
	}
	var req struct {
		Body string `json:"body"`
	}
	if err := bindJSON(re, &req); err != nil {
		return err
	}
	if req.Body == "" || len([]rune(req.Body)) > maxWhatsAppReplyChars {
		return apperr.BadRequest(fmt.Sprintf("a reply must be 1 to %d characters", maxWhatsAppReplyChars))
	}
	if err := a.whatsapp.Reply(sub, req.Body); err != nil {
		var refused *whatsapp.ReplyError
		if errors.As(err, &refused) {
			return re.JSON(http.StatusConflict, map[string]any{"message": refused.Reason, "reason": refused.Reason})
		}
		return apperr.BadRequest(err.Error())
	}
	return okJSON(re, true)
}

// CopyWhatsAppConsent grants WhatsApp consent from SMS consent. A dry run only counts.
// Applying needs an attestation, because Meta requires that the original opt-in named
// the business, and the app can't check that.
func (a *App) CopyWhatsAppConsent(re *pbcore.RequestEvent) error {
	var req struct {
		DryRun           bool `json:"dry_run"`
		IncludeInherited bool `json:"include_inherited"`
		Attested         bool `json:"attested"`
	}
	if err := bindJSON(re, &req); err != nil {
		return err
	}
	if !req.DryRun && !req.Attested {
		return apperr.BadRequest("confirm that your opt-in wording covers WhatsApp messages from your business")
	}
	out, err := a.core.ImportSMSConsent(models.ChannelWhatsApp, req.DryRun, req.IncludeInherited)
	if err != nil {
		return apperr.Internal(err.Error())
	}
	return okJSON(re, out)
}

func (a *App) whatsAppChannelPoster() (whatsapp.Provider, error) {
	provider, err := a.whatsapp.Provider()
	if err != nil {
		return nil, apperr.BadRequest(err.Error())
	}
	if !provider.Capabilities().ChannelPosts {
		return nil, apperr.BadRequest("channel posts need the Whapi provider")
	}
	return provider, nil
}

// GetWhatsAppChannels lists the WhatsApp Channels the linked account administers.
func (a *App) GetWhatsAppChannels(re *pbcore.RequestEvent) error {
	provider, err := a.whatsAppChannelPoster()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(re.Request.Context(), 30*time.Second)
	defer cancel()
	out, err := provider.ListChannels(ctx)
	if err != nil {
		return apperr.BadRequest(err.Error())
	}
	return re.JSON(http.StatusOK, okResp{Data: out})
}

// GetWhatsAppChannelPosts lists posts, newest first.
func (a *App) GetWhatsAppChannelPosts(re *pbcore.RequestEvent) error {
	out, err := a.core.ListChannelPosts(100)
	if err != nil {
		return apperr.Internal(err.Error())
	}
	return re.JSON(http.StatusOK, okResp{Data: out})
}

type whatsAppPostReq struct {
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	Kind        string `json:"kind"`
	Body        string `json:"body"`
	MediaURL    string `json:"media_url"`
	Title       string `json:"title"`
	SendAt      string `json:"send_at"`
	Schedule    bool   `json:"schedule"`
}

func (r whatsAppPostReq) toInput(createdBy string) (core.ChannelPostInput, error) {
	input := core.ChannelPostInput{
		ChannelID: r.ChannelID, ChannelName: r.ChannelName, Kind: r.Kind, Body: r.Body, MediaURL: r.MediaURL, Title: r.Title,
		Status: core.ChannelPostDraft, CreatedBy: createdBy,
	}
	if err := whatsapp.ValidatePost(whatsapp.ChannelPost{ChannelID: r.ChannelID, Kind: r.Kind, Body: r.Body, MediaURL: r.MediaURL, Title: r.Title}); err != nil {
		return input, apperr.BadRequest(err.Error())
	}
	if !r.Schedule {
		return input, nil
	}
	input.Status = core.ChannelPostScheduled
	if r.SendAt != "" {
		at, err := time.Parse(time.RFC3339, r.SendAt)
		if err != nil {
			return input, apperr.BadRequest("invalid send time")
		}
		input.SendAt = at
	}
	return input, nil
}

// channelPostResponse adds the pacing warning, so the page can show it after saving.
type channelPostResponse struct {
	core.ChannelPost
	PostsThatDay int  `json:"posts_that_day"`
	PacingWarn   bool `json:"pacing_warning"`
}

func (a *App) channelPostResponse(post core.ChannelPost) (channelPostResponse, error) {
	out := channelPostResponse{ChannelPost: post}
	if post.Status != core.ChannelPostScheduled || len(post.SendAt) < 10 {
		return out, nil
	}
	day, err := time.Parse("2006-01-02", post.SendAt[:10])
	if err != nil {
		return out, err
	}
	out.PostsThatDay, err = a.core.CountScheduledChannelPostsOn(post.ChannelID, day)
	out.PacingWarn = out.PostsThatDay > whatsapp.PostsPerDayWarning
	return out, err
}

// CreateWhatsAppChannelPost stores a draft, or schedules a post.
func (a *App) CreateWhatsAppChannelPost(re *pbcore.RequestEvent) error {
	if _, err := a.whatsAppChannelPoster(); err != nil {
		return err
	}
	var req whatsAppPostReq
	if err := bindJSON(re, &req); err != nil {
		return err
	}
	createdBy := ""
	if re.Auth != nil {
		createdBy = re.Auth.Id
	}
	input, err := req.toInput(createdBy)
	if err != nil {
		return err
	}
	post, err := a.core.CreateChannelPost(input)
	if err != nil {
		return apperr.Internal(err.Error())
	}
	out, err := a.channelPostResponse(post)
	if err != nil {
		return apperr.Internal(err.Error())
	}
	return re.JSON(http.StatusOK, okResp{Data: out})
}

func whatsAppPostID(re *pbcore.RequestEvent) (int64, error) {
	id, err := strconv.ParseInt(pathParam(re, "id"), 10, 64)
	if err != nil {
		return 0, apperr.BadRequest("invalid post ID")
	}
	return id, nil
}

// UpdateWhatsAppChannelPost edits a draft or scheduled post.
func (a *App) UpdateWhatsAppChannelPost(re *pbcore.RequestEvent) error {
	id, err := whatsAppPostID(re)
	if err != nil {
		return err
	}
	var req whatsAppPostReq
	if err := bindJSON(re, &req); err != nil {
		return err
	}
	input, err := req.toInput("")
	if err != nil {
		return err
	}
	post, err := a.core.UpdateChannelPost(id, input)
	if err != nil {
		return err
	}
	out, err := a.channelPostResponse(post)
	if err != nil {
		return apperr.Internal(err.Error())
	}
	return re.JSON(http.StatusOK, okResp{Data: out})
}

// DeleteWhatsAppChannelPost removes a post from the list. A post that already went out
// stays on the channel.
func (a *App) DeleteWhatsAppChannelPost(re *pbcore.RequestEvent) error {
	id, err := whatsAppPostID(re)
	if err != nil {
		return err
	}
	if err := a.core.DeleteChannelPost(id); err != nil {
		return err
	}
	return okJSON(re, true)
}

// RetryWhatsAppChannelPost schedules a failed post again.
func (a *App) RetryWhatsAppChannelPost(re *pbcore.RequestEvent) error {
	id, err := whatsAppPostID(re)
	if err != nil {
		return err
	}
	post, err := a.core.RetryChannelPost(id)
	if err != nil {
		return err
	}
	return re.JSON(http.StatusOK, okResp{Data: post})
}
