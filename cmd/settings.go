package main

import (
	pbcore "github.com/pocketbase/pocketbase/core"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/compdani/list_pocket/internal/messenger/email"
	"github.com/compdani/list_pocket/internal/notifs"
	"github.com/compdani/list_pocket/models"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx/types"
	"github.com/labstack/echo/v4"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/cron"
)

const pwdMask = "•"

type aboutHost struct {
	OS       string `json:"os"`
	Machine  string `json:"arch"`
	Hostname string `json:"hostname"`
}

type aboutSystem struct {
	NumCPU  int    `json:"num_cpu"`
	AllocMB uint64 `json:"memory_alloc_mb"`
	OSMB    uint64 `json:"memory_from_os_mb"`
}

type about struct {
	Version   string         `json:"version"`
	Build     string         `json:"build"`
	GoVersion string         `json:"go_version"`
	GoArch    string         `json:"go_arch"`
	Database  types.JSONText `json:"database"`
	System    aboutSystem    `json:"system"`
	Host      aboutHost      `json:"host"`
}

var (
	reAlphaNum = regexp.MustCompile(`[^a-z0-9\-]`)
)

// GetSettings returns settings from the DB.
func (a *App) GetSettings(re *pbcore.RequestEvent) error {
	s, err := a.core.GetSettings()
	if err != nil {
		return err
	}

	// Empty out passwords.
	for i := range s.SMTP {
		s.SMTP[i].Password = strings.Repeat(pwdMask, utf8.RuneCountInString(s.SMTP[i].Password))
	}
	for i := range s.BounceBoxes {
		s.BounceBoxes[i].Password = strings.Repeat(pwdMask, utf8.RuneCountInString(s.BounceBoxes[i].Password))
	}
	for i := range s.Messengers {
		s.Messengers[i].Password = strings.Repeat(pwdMask, utf8.RuneCountInString(s.Messengers[i].Password))
	}

	s.UploadS3AwsSecretAccessKey = strings.Repeat(pwdMask, utf8.RuneCountInString(s.UploadS3AwsSecretAccessKey))
	s.SendgridKey = strings.Repeat(pwdMask, utf8.RuneCountInString(s.SendgridKey))
	s.BouncePostmark.Password = strings.Repeat(pwdMask, utf8.RuneCountInString(s.BouncePostmark.Password))
	s.BounceForwardEmail.Key = strings.Repeat(pwdMask, utf8.RuneCountInString(s.BounceForwardEmail.Key))
	s.SecurityCaptcha.HCaptcha.Secret = strings.Repeat(pwdMask, utf8.RuneCountInString(s.SecurityCaptcha.HCaptcha.Secret))
	return okJSON(re, s)
}

// UpdateSettings returns settings from the DB.
func (a *App) UpdateSettings(re *pbcore.RequestEvent) error {
	// Unmarshal and marshal the fields once to sanitize the settings blob.
	var set models.Settings
	if err := bindJSON(re, &set); err != nil {
		return err
	}

	// Get the existing settings.
	cur, err := a.core.GetSettings()
	if err != nil {
		return err
	}

	// Validate and sanitize postback Messenger names along with SMTP names
	// (where each SMTP is also considered as a standalone messenger).
	// Duplicates are disallowed and "email" is a reserved name.
	names := map[string]bool{emailMsgr: true}

	// There should be at least one SMTP block that's enabled.
	has := false
	for i, s := range set.SMTP {
		if s.Enabled {
			has = true
		}

		// Sanitize and normalize the SMTP server name.
		name := reAlphaNum.ReplaceAllString(strings.ToLower(strings.TrimSpace(s.Name)), "-")
		if name != "" {
			if !strings.HasPrefix(name, "email-") {
				name = "email-" + name
			}

			if _, ok := names[name]; ok {
				return echo.NewHTTPError(http.StatusBadRequest,
					a.i18n.Ts("settings.duplicateMessengerName", "name", name))
			}

			names[name] = true
		}
		set.SMTP[i].Name = name

		// Assign a UUID. The frontend only sends a password when the user explicitly
		// changes the password. In other cases, the existing password in the DB
		// is copied while updating the settings and the UUID is used to match
		// the incoming array of SMTP blocks with the array in the DB.
		if s.UUID == "" {
			set.SMTP[i].UUID = uuid.Must(uuid.NewV4()).String()
		}

		// Ensure the HOST is trimmed of any whitespace.
		// This is a common mistake when copy-pasting SMTP settings.
		set.SMTP[i].Host = strings.TrimSpace(s.Host)
		fromAddrs, defaultFromEmail, err := a.sanitizeSMTPFromEmails(s.FromAddresses, s.DefaultFromEmail)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		set.SMTP[i].FromAddresses = fromAddrs
		set.SMTP[i].DefaultFromEmail = defaultFromEmail

		// If there's no password coming in from the frontend, copy the existing
		// password by matching the UUID.
		if s.Password == "" {
			for _, c := range cur.SMTP {
				if s.UUID == c.UUID {
					set.SMTP[i].Password = c.Password
				}
			}
		}
	}
	if !has {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("settings.errorNoSMTP"))
	}

	// Always remove the trailing slash from the app root URL.
	set.AppRootURL = strings.TrimRight(set.AppRootURL, "/")

	// Bounce boxes.
	for i, s := range set.BounceBoxes {
		// Assign a UUID. The frontend only sends a password when the user explicitly
		// changes the password. In other cases, the existing password in the DB
		// is copied while updating the settings and the UUID is used to match
		// the incoming array of blocks with the array in the DB.
		if s.UUID == "" {
			set.BounceBoxes[i].UUID = uuid.Must(uuid.NewV4()).String()
		}

		// Ensure the HOST is trimmed of any whitespace.
		// This is a common mistake when copy-pasting SMTP settings.
		set.BounceBoxes[i].Host = strings.TrimSpace(s.Host)

		if d, _ := time.ParseDuration(s.ScanInterval); d.Minutes() < 1 {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("settings.bounces.invalidScanInterval"))
		}

		// If there's no password coming in from the frontend, copy the existing
		// password by matching the UUID.
		if s.Password == "" {
			for _, c := range cur.BounceBoxes {
				if s.UUID == c.UUID {
					set.BounceBoxes[i].Password = c.Password
				}
			}
		}
	}

	for i, m := range set.Messengers {
		// UUID to keep track of password changes similar to the SMTP logic above.
		if m.UUID == "" {
			set.Messengers[i].UUID = uuid.Must(uuid.NewV4()).String()
		}

		if m.Password == "" {
			for _, c := range cur.Messengers {
				if m.UUID == c.UUID {
					set.Messengers[i].Password = c.Password
				}
			}
		}

		name := reAlphaNum.ReplaceAllString(strings.ToLower(m.Name), "")
		if _, ok := names[name]; ok {
			return echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("settings.duplicateMessengerName", "name", name))
		}
		if len(name) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("settings.invalidMessengerName"))
		}

		set.Messengers[i].Name = name
		names[name] = true
	}

	// S3 password?
	if set.UploadS3AwsSecretAccessKey == "" {
		set.UploadS3AwsSecretAccessKey = cur.UploadS3AwsSecretAccessKey
	}
	if set.SendgridKey == "" {
		set.SendgridKey = cur.SendgridKey
	}
	if set.BouncePostmark.Password == "" {
		set.BouncePostmark.Password = cur.BouncePostmark.Password
	}
	if set.BounceForwardEmail.Key == "" {
		set.BounceForwardEmail.Key = cur.BounceForwardEmail.Key
	}
	if set.SecurityCaptcha.HCaptcha.Secret == "" {
		set.SecurityCaptcha.HCaptcha.Secret = cur.SecurityCaptcha.HCaptcha.Secret
	}
	for n, v := range set.UploadExtensions {
		set.UploadExtensions[n] = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(v), "."))
	}

	// Domain blocklist / allowlist.
	doms := make([]string, 0, len(set.DomainBlocklist))
	for _, d := range set.DomainBlocklist {
		if d = strings.TrimSpace(strings.ToLower(d)); d != "" {
			doms = append(doms, d)
		}
	}
	set.DomainBlocklist = doms

	doms = make([]string, 0, len(set.DomainAllowlist))
	for _, d := range set.DomainAllowlist {
		if d = strings.TrimSpace(strings.ToLower(d)); d != "" {
			doms = append(doms, d)
		}
	}
	set.DomainAllowlist = doms

	// Validate and clean CORS domains.
	cors := make([]string, 0, len(set.SecurityCORSOrigins))
	for _, d := range set.SecurityCORSOrigins {
		if d = strings.TrimSpace(d); d != "" {
			if d == "*" {
				cors = append(cors, d)
				continue
			}

			// Parse and validate the URL.
			u, err := url.Parse(d)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return echo.NewHTTPError(http.StatusBadRequest,
					a.i18n.Ts("globals.messages.invalidData")+": invalid CORS domain: "+d)
			}
			// Save clean scheme + host
			cors = append(cors, u.Scheme+"://"+u.Host)
		}
	}
	set.SecurityCORSOrigins = cors

	// Validate slow query caching cron.
	if set.CacheSlowQueries {
		if _, err := cron.NewSchedule(set.CacheSlowQueriesInterval); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidData")+": slow query cron: "+err.Error())
		}
	}

	// Update the settings in the DB.
	if err := a.core.UpdateSettings(set); err != nil {
		return err
	}

	return a.handleSettingsRestart(re)
}

func (a *App) sanitizeFromAddress(addr string) (string, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", nil
	}

	if reFromAddress.Match([]byte(addr)) {
		return addr, nil
	}

	out, err := a.importer.SanitizeEmail(addr)
	if err != nil {
		return "", errors.New(a.i18n.T("campaigns.fieldInvalidFromEmail"))
	}
	return out, nil
}

func (a *App) sanitizeSMTPFromEmails(fromAddresses []string, defaultFromEmail string) ([]string, string, error) {
	out := make([]string, 0, len(fromAddresses))
	seen := make(map[string]struct{}, len(fromAddresses))

	for _, addr := range fromAddresses {
		sanitized, err := a.sanitizeFromAddress(addr)
		if err != nil {
			return nil, "", err
		}
		if sanitized == "" {
			continue
		}
		if _, ok := seen[sanitized]; ok {
			continue
		}
		seen[sanitized] = struct{}{}
		out = append(out, sanitized)
	}

	sanitizedDefault, err := a.sanitizeFromAddress(defaultFromEmail)
	if err != nil {
		return nil, "", err
	}
	if sanitizedDefault != "" {
		if _, ok := seen[sanitizedDefault]; !ok {
			out = append(out, sanitizedDefault)
		}
	}

	return out, sanitizedDefault, nil
}

// UpdateSettingsByKey updates a single setting key-value in the DB.
func (a *App) UpdateSettingsByKey(re *pbcore.RequestEvent) error {
	key := pathParam(re, "key")
	if key == "" {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidData"))
	}

	// Read the raw JSON body as the value.
	var b json.RawMessage
	if err := bindJSON(re, &b); err != nil {
		return err
	}

	// Update the value in the DB.
	if err := a.core.UpdateSettingsByKey(key, b); err != nil {
		return err
	}

	return a.handleSettingsRestart(re)
}

// handleSettingsRestart checks for running campaigns and either triggers an
// immediate app restart or marks the app as needing a restart.
func (a *App) handleSettingsRestart(re *pbcore.RequestEvent) error {
	// If there are any active campaigns, don't do an auto reload and
	// warn the user on the frontend.
	if a.manager.HasRunningCampaigns() {
		a.Lock()
		a.needsRestart = true
		a.Unlock()

		return okJSON(re, struct {
			NeedsRestart bool `json:"needs_restart"`
		}{true})
	}

	// No running campaigns. Reload the app.
	go func() {
		<-time.After(time.Millisecond * 500)
		a.chReload <- syscall.SIGHUP
	}()

	return okJSON(re, true)
}

// GetLogs returns the log entries stored in the log buffer.
func (a *App) GetLogs(re *pbcore.RequestEvent) error {
	return okJSON(re, a.bufLog.Lines())
}

// TestSMTPSettings returns the log entries stored in the log buffer.
func (a *App) TestSMTPSettings(re *pbcore.RequestEvent) error {
	// Copy the raw JSON post body.
	reqBody, err := io.ReadAll(re.Request.Body)
	if err != nil {
		a.log.Printf("error reading SMTP test: %v", err)
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.internalError"))
	}

	req, to, from, err := email.ParseSMTPTestRequest(reqBody)
	if err != nil {
		a.log.Printf("error unmarshalling SMTP test request: %v", err)
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.internalError"))
	}
	if to == "" {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.missingFields", "name", "email"))
	}

	// Initialize a new SMTP pool.
	req.MaxConns = 1
	req.IdleTimeout = time.Second * 2
	req.PoolWaitTimeout = time.Second * 2
	msgr, err := email.New("", req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.errorCreating", "name", "SMTP", "error", err.Error()))
	}

	// Render the test email template body.
	var b bytes.Buffer
	if err := notifs.Tpls.ExecuteTemplate(&b, "smtp-test", nil); err != nil {
		a.log.Printf("error compiling notification template '%s': %v", "smtp-test", err)
		return err
	}

	m := models.Message{}
	if from != "" {
		m.From = from
	} else {
		m.From = a.cfg.FromEmail
	}
	m.To = []string{to}
	m.Subject = a.i18n.T("settings.smtp.testConnection")
	m.Body = b.Bytes()
	if err := msgr.Push(m); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return okJSON(re, a.bufLog.Lines())
}

func (a *App) GetAboutInfo(re *pbcore.RequestEvent) error {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	out := a.about
	out.System.AllocMB = mem.Alloc / 1024 / 1024
	out.System.OSMB = mem.Sys / 1024 / 1024

	return re.JSON(http.StatusOK, out)
}

type aiBuilderSettingsPayload struct {
	Model           string   `json:"model"`
	TimeoutSeconds  int      `json:"timeoutSeconds"`
	AvailableModels []string `json:"availableModels"`
}

func (a *App) GetAIBuilderSettings(re *pbcore.RequestEvent) error {
	model, timeout, availableModels := a.getAIBuilderSettingsValues()
	return okJSON(re, aiBuilderSettingsPayload{
		Model:           model,
		TimeoutSeconds:  timeout,
		AvailableModels: availableModels,
	})
}

func (a *App) UpdateAIBuilderSettings(re *pbcore.RequestEvent) error {
	var in aiBuilderSettingsPayload
	if err := bindJSON(re, &in); err != nil {
		return err
	}

	availableModels, err := sanitizeAIBuilderAvailableModels(in.AvailableModels)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	model := strings.TrimSpace(in.Model)
	if model == "" {
		model = availableModels[0]
	}
	if len(model) > 120 {
		return echo.NewHTTPError(http.StatusBadRequest, "model is too long")
	}
	if !containsString(availableModels, model) {
		return echo.NewHTTPError(http.StatusBadRequest, "model must be one of availableModels")
	}
	if a == nil || a.pb == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "settings backend unavailable")
	}
	timeout := in.TimeoutSeconds
	if timeout <= 0 {
		_, currentTimeout, _ := a.getAIBuilderSettingsValues()
		timeout = currentTimeout
	}
	if timeout < 30 || timeout > 900 {
		return echo.NewHTTPError(http.StatusBadRequest, "timeoutSeconds must be between 30 and 900")
	}

	savePayload := map[string]any{
		"model":            model,
		"timeout_seconds":  timeout,
		"available_models": availableModels,
	}
	payloadBytes, err := json.Marshal(savePayload)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save ai settings")
	}

	if err := a.upsertTypedSettingsRecord("ai_builder", string(payloadBytes)); err != nil {
		return err
	}

	return okJSON(re, aiBuilderSettingsPayload{
		Model:           model,
		TimeoutSeconds:  timeout,
		AvailableModels: availableModels,
	})
}

func (a *App) getAIBuilderSettingsValues() (string, int, []string) {
	model := defaultAIBuilderModel
	timeout := defaultAIBuilderTimeoutSeconds
	availableModels := defaultAIBuilderAvailableModels()

	if a == nil || a.pb == nil {
		return model, timeout, availableModels
	}

	rec, err := a.pb.FindFirstRecordByFilter("listpocket_settings", "type={:type}", dbx.Params{"type": "ai_builder"})
	if err != nil || rec == nil {
		return model, timeout, availableModels
	}

	raw := strings.TrimSpace(rec.GetString("value"))
	if raw == "" {
		return model, timeout, availableModels
	}

	var parsed struct {
		Model           string   `json:"model"`
		TimeoutSeconds  int      `json:"timeout_seconds"`
		AvailableModels []string `json:"available_models"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return model, timeout, availableModels
	}

	if cleaned, err := sanitizeAIBuilderAvailableModels(parsed.AvailableModels); err == nil {
		availableModels = cleaned
	}
	if v := strings.TrimSpace(parsed.Model); v != "" {
		model = v
	}
	if parsed.TimeoutSeconds >= 30 && parsed.TimeoutSeconds <= 900 {
		timeout = parsed.TimeoutSeconds
	}
	if !containsString(availableModels, model) {
		model = availableModels[0]
	}

	return model, timeout, availableModels
}

func sanitizeAIBuilderAvailableModels(in []string) ([]string, error) {
	if len(in) == 0 {
		return defaultAIBuilderAvailableModels(), nil
	}

	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, raw := range in {
		model := strings.TrimSpace(raw)
		if model == "" {
			continue
		}
		if len(model) > 120 {
			return nil, errors.New("available model name is too long")
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
		if len(out) >= 50 {
			break
		}
	}

	if len(out) == 0 {
		return nil, errors.New("at least one available model is required")
	}
	return out, nil
}

func containsString(items []string, value string) bool {
	for _, v := range items {
		if v == value {
			return true
		}
	}
	return false
}

func defaultAIBuilderAvailableModels() []string {
	return []string{"gpt-5.4-mini", "gpt-5.4-nano", "gpt-5.4"}
}

func (a *App) upsertTypedSettingsRecord(recordType, value string) error {
	if a == nil || a.pb == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "settings backend unavailable")
	}

	col, err := a.pb.FindCollectionByNameOrId("listpocket_settings")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "settings collection not found")
	}

	rec, err := a.pb.FindFirstRecordByFilter("listpocket_settings", "type={:type}", dbx.Params{"type": recordType})
	if err != nil || rec == nil {
		rec = core.NewRecord(col)
		rec.Set("type", recordType)
	}
	rec.Set("value", value)
	return a.pb.Save(rec)
}
