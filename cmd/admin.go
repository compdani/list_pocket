package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/compdani/list_pocket/internal/captcha"
	"github.com/compdani/list_pocket/models"
	"github.com/labstack/echo/v4"
	null "gopkg.in/volatiletech/null.v6"
)

type serverConfig struct {
	RootURL            string             `json:"root_url"`
	FromEmail          string             `json:"from_email"`
	SMTPSenders        []smtpSenderConfig `json:"smtp_senders"`
	PublicSubscription struct {
		Enabled          bool        `json:"enabled"`
		CaptchaEnabled   bool        `json:"captcha_enabled"`
		CaptchaProvider  null.String `json:"captcha_provider"`
		CaptchaKey       null.String `json:"captcha_key"`
		AltchaComplexity int         `json:"altcha_complexity"`
	} `json:"public_subscription"`
	Privacy struct {
		DisableTracking    bool `json:"disable_tracking"`
		IndividualTracking bool `json:"individual_tracking"`
	} `json:"privacy"`
	MediaProvider string          `json:"media_provider"`
	Messengers    []string        `json:"messengers"`
	Langs         []i18nLang      `json:"langs"`
	Lang          string          `json:"lang"`
	Permissions   json.RawMessage `json:"permissions"`
	Update        *AppUpdate      `json:"update"`
	NeedsRestart  bool            `json:"needs_restart"`
	HasLegacyUser bool            `json:"has_legacy_user"`
	Version       string          `json:"version"`
}

type smtpSenderConfig struct {
	Messenger        string   `json:"messenger"`
	Name             string   `json:"name"`
	FromAddresses    []string `json:"from_addresses"`
	DefaultFromEmail string   `json:"default_from_email"`
}

// GetServerConfig returns general server config.
func (a *App) GetServerConfig(c echo.Context) error {
	out := serverConfig{
		RootURL:       a.urlCfg.RootURL,
		FromEmail:     a.cfg.FromEmail,
		Lang:          a.cfg.Lang,
		Permissions:   a.cfg.PermissionsRaw,
		HasLegacyUser: a.cfg.HasLegacyUser,
		Privacy: struct {
			DisableTracking    bool `json:"disable_tracking"`
			IndividualTracking bool `json:"individual_tracking"`
		}{
			DisableTracking:    a.cfg.Privacy.DisableTracking,
			IndividualTracking: a.cfg.Privacy.IndividualTracking,
		},
	}
	out.PublicSubscription.Enabled = a.cfg.EnablePublicSubPage

	// CAPTCHA.
	if a.cfg.Security.Captcha.Altcha.Enabled {
		out.PublicSubscription.CaptchaEnabled = true
		out.PublicSubscription.CaptchaProvider = null.StringFrom(captcha.ProviderAltcha)
		out.PublicSubscription.AltchaComplexity = a.cfg.Security.Captcha.Altcha.Complexity
	} else if a.cfg.Security.Captcha.HCaptcha.Enabled {
		out.PublicSubscription.CaptchaEnabled = true
		out.PublicSubscription.CaptchaProvider = null.StringFrom(captcha.ProviderHCaptcha)
		out.PublicSubscription.CaptchaKey = null.StringFrom(a.cfg.Security.Captcha.HCaptcha.Key)
	}

	out.MediaProvider = a.cfg.MediaUpload.Provider

	settings, err := a.core.GetSettings()
	if err != nil {
		return err
	}
	enabledSMTP := make([]models.SMTPSettings, 0, len(settings.SMTP))
	for _, item := range settings.SMTP {
		if !item.Enabled {
			continue
		}
		enabledSMTP = append(enabledSMTP, item)
		if item.Name != "" {
			out.SMTPSenders = append(out.SMTPSenders, smtpSenderConfig{
				Messenger:        item.Name,
				Name:             item.Name,
				FromAddresses:    item.FromAddresses,
				DefaultFromEmail: item.DefaultFromEmail,
			})
		}
	}
	if len(enabledSMTP) == 1 {
		out.SMTPSenders = append([]smtpSenderConfig{{
			Messenger:        emailMsgr,
			Name:             emailMsgr,
			FromAddresses:    enabledSMTP[0].FromAddresses,
			DefaultFromEmail: enabledSMTP[0].DefaultFromEmail,
		}}, out.SMTPSenders...)
	}

	// Language list.
	langList, err := getI18nLangList(a.fs)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("Error loading language list: %v", err))
	}
	out.Langs = langList

	out.Messengers = make([]string, 0, len(a.messengers))
	for _, m := range a.messengers {
		out.Messengers = append(out.Messengers, m.Name())
	}
	// Quo is registered at startup when configured; also expose it here when typed settings
	// enable SMS so the admin UI can offer SMS campaigns after saving without relying on a stale boot slice.
	if s := a.loadTextMessagingSettings(); s.QuoProvider() != nil && s.QuoProvider().Enabled && strings.TrimSpace(s.QuoProvider().APIKey) != "" {
		hasQuo := false
		for _, name := range out.Messengers {
			if name == models.CampaignMessengerQuo {
				hasQuo = true
				break
			}
		}
		if !hasQuo {
			out.Messengers = append(out.Messengers, models.CampaignMessengerQuo)
		}
	}

	a.Lock()
	out.NeedsRestart = a.needsRestart
	out.Update = a.update
	a.Unlock()
	out.Version = versionString

	return c.JSON(http.StatusOK, okResp{out})
}

// GetDashboardCharts returns chart data points to render ont he dashboard.
func (a *App) GetDashboardCharts(c echo.Context) error {
	tz := c.QueryParam("tz")

	// Get the chart data from the DB.
	out, err := a.core.GetDashboardCharts(tz)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// GetDashboardCounts returns stats counts to show on the dashboard.
func (a *App) GetDashboardCounts(c echo.Context) error {
	// Get the chart data from the DB.
	out, err := a.core.GetDashboardCounts()
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// ReloadApp sends a reload signal to the app, causing a full restart.
func (a *App) ReloadApp(c echo.Context) error {
	go func() {
		<-time.After(time.Millisecond * 500)

		// Send the reload signal to trigger the wait loop in main.
		a.chReload <- syscall.SIGHUP
	}()

	return c.JSON(http.StatusOK, okResp{true})
}
