package main

import (
	"html/template"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/compdani/list_pocket/internal/auth"
	"github.com/compdani/list_pocket/models"
	"github.com/labstack/echo/v4"
	"github.com/pocketbase/pocketbase/apis"
	pbcore "github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

const (
	// stdInputMaxLen is the maximum allowed length for a standard input field.
	stdInputMaxLen = 2000

	// URIs.
	uriAdmin = "/admin"
)

type okResp struct {
	Data any `json:"data"`
}

var (
	reUUID = regexp.MustCompile("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")
)

// registerHandlers registers HTTP handlers on the PocketBase router.
func registerHandlers(se *router.Router[*pbcore.RequestEvent], a *App, tpl *template.Template, cfg *Config, urlCfg *UrlConfig) {

	//Token exchange routes
	auth.RegisterExchangeRoutes(se)

	admin := se.Group("")
	admin.GET(path.Join(uriAdmin, "/setup"), wrapEcho(a, tpl, cfg, urlCfg, nil, a.LoginSetupPage))
	admin.POST(path.Join(uriAdmin, "/setup"), wrapEcho(a, tpl, cfg, urlCfg, nil, a.LoginSetupPage))
	admin.GET(path.Join(uriAdmin, "/custom.css"), wrapEcho(a, tpl, cfg, urlCfg, nil, serveCustomAppearance("admin.custom_css")))
	admin.GET(path.Join(uriAdmin, "/custom.js"), wrapEcho(a, tpl, cfg, urlCfg, nil, serveCustomAppearance("admin.custom_js")))
	admin.GET("/custom.css", wrapEcho(a, tpl, cfg, urlCfg, nil, serveCustomAppearance("admin.custom_css")))
	admin.GET("/custom.js", wrapEcho(a, tpl, cfg, urlCfg, nil, serveCustomAppearance("admin.custom_js")))

	// Admin SPA routing: static assets are served separately via apis.Static in init.go.
	// All other GET requests under /admin fall back to the Vue SPA index unless
	// initial setup is still required, in which case they redirect to /admin/setup.
	se.GET(path.Join(uriAdmin, ""), serveAdminSPAFallback(a))
	se.GET(path.Join(uriAdmin, "/{path...}"), serveAdminSPAFallback(a))

	authAPI := se.Group("/mailapi/auth")
	authAPI.POST("/login", wrapEcho(a, tpl, cfg, urlCfg, nil, a.AuthLogin))
	authAPI.POST("/twofa", wrapEcho(a, tpl, cfg, urlCfg, nil, a.AuthVerifyTwoFA))
	authAPI.POST("/forgot", wrapEcho(a, tpl, cfg, urlCfg, nil, a.AuthForgotPassword))
	authAPI.POST("/reset", wrapEcho(a, tpl, cfg, urlCfg, nil, a.AuthResetPassword))

	pm := a.auth.Perm
	api := se.Group("/mailapi").Bind(apis.RequireAuth())

	api.GET("/health", wrapEcho(a, tpl, cfg, urlCfg, nil, a.HealthCheck))
	api.GET("/config", wrapEcho(a, tpl, cfg, urlCfg, nil, a.GetServerConfig))
	api.GET("/lang/{lang}", wrapEcho(a, tpl, cfg, urlCfg, []string{"lang"}, a.GetI18nLang))
	api.GET("/dashboard/charts", wrapEcho(a, tpl, cfg, urlCfg, nil, a.GetDashboardCharts))
	api.GET("/dashboard/counts", wrapEcho(a, tpl, cfg, urlCfg, nil, a.GetDashboardCounts))

	api.GET("/settings", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.GetSettings, "settings:get")))
	api.PUT("/settings", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.UpdateSettings, "settings:manage")))
	api.GET("/settings/ai-builder", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.GetAIBuilderSettings, "settings:get")))
	api.PUT("/settings/ai-builder", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.UpdateAIBuilderSettings, "settings:manage")))
	api.PUT("/settings/{key}", wrapEcho(a, tpl, cfg, urlCfg, []string{"key"}, pm(a.UpdateSettingsByKey, "settings:manage")))
	api.POST("/settings/smtp/test", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.TestSMTPSettings, "settings:manage")))
	api.GET("/settings/text-messaging", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.GetTextMessagingSettings, "settings:get")))
	api.PUT("/settings/text-messaging", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.UpdateTextMessagingSettings, "settings:manage")))
	api.POST("/settings/text-messaging/test", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.TestTextMessagingSettings, "settings:manage")))
	api.POST("/webhooks/email-replies", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.InboundEmailReplyWebhook, "webhooks:post_bounce")))
	api.POST("/admin/reload", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.ReloadApp, "settings:manage")))
	api.GET("/logs", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.GetLogs, "settings:get")))
	api.GET("/about", wrapEcho(a, tpl, cfg, urlCfg, nil, a.GetAboutInfo))

	api.GET("/subscribers", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.QuerySubscribers, "subscribers:get_all", "subscribers:get")))
	api.GET("/subscribers/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.GetSubscriber, "subscribers:get_all", "subscribers:get")))
	api.GET("/subscribers/{id}/activity", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.GetSubscriberActivity, "subscribers:get_all", "subscribers:get")))
	api.GET("/subscribers/{id}/timeline", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.GetSubscriberTimeline, "subscribers:get_all", "subscribers:get")))
	api.GET("/subscribers/{id}/export", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.ExportSubscriberData, "subscribers:get_all", "subscribers:get")))
	api.GET("/inbound-email-replies/{replyId}/attachments", wrapEcho(a, tpl, cfg, urlCfg, []string{"replyId"}, pm(a.GetInboundEmailAttachments, "subscribers:get_all", "subscribers:get")))
	api.GET("/inbound-email-attachments/{id}/download", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.DownloadInboundEmailAttachment, "subscribers:get_all", "subscribers:get")))
	api.GET("/inbound-emails", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.GetInboundEmailInbox, "inbox:get")))
	api.GET("/inbound-emails/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.GetInboundEmailByID, "inbox:get")))
	api.PUT("/inbound-emails/{id}/spam", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.UpdateInboundEmailSpamStatus, "inbox:manage")))
	api.GET("/inbound-email-spam-rules", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.GetInboundSpamRules, "inbox:manage")))
	api.DELETE("/inbound-email-spam-rules/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.DeleteInboundSpamRule, "inbox:manage")))
	api.DELETE("/maintenance/inbound-emails/spam", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.GCSpamInboundEmails, "inbox:manage")))
	api.GET("/subscribers/{id}/bounces", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.GetSubscriberBounces, "bounces:get")))
	api.DELETE("/subscribers/{id}/bounces", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.DeleteSubscriberBounces, "bounces:manage")))
	api.POST("/subscribers", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.CreateSubscriber, "subscribers:manage")))
	api.PUT("/subscribers/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.UpdateSubscriber, "subscribers:manage")))
	api.POST("/subscribers/{id}/optin", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.SubscriberSendOptin, "subscribers:manage")))
	api.POST("/subscribers/{id}/sms-opt-out", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.SubscriberSMSOptOut, "subscribers:manage")))
	api.PUT("/subscribers/blocklist", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.BlocklistSubscribers, "subscribers:manage")))
	api.PUT("/subscribers/{first}/{second}", wrapEcho(a, tpl, cfg, urlCfg, []string{"first", "second"}, func(c echo.Context) error {
		switch {
		case c.Param("first") == "lists":
			c.SetParamNames("id")
			c.SetParamValues(c.Param("second"))
			return pm(a.ManageSubscriberLists, "subscribers:manage")(c)
		case c.Param("second") == "blocklist":
			c.SetParamNames("id")
			c.SetParamValues(c.Param("first"))
			return pm(a.BlocklistSubscriber, "subscribers:manage")(c)
		default:
			return echo.NewHTTPError(http.StatusNotFound, "404 unknown endpoint")
		}
	}))
	api.PUT("/subscribers/lists", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.ManageSubscriberLists, "subscribers:manage")))
	api.PUT("/subscribers/bulk-update", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.BulkUpdateSubscribers, "subscribers:manage")))
	api.POST("/subscribers/bulk-add", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.BulkAddSubscribers, "subscribers:import")))
	api.DELETE("/subscribers/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.DeleteSubscriber, "subscribers:manage")))
	api.DELETE("/subscribers", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.DeleteSubscribers, "subscribers:manage")))

	api.GET("/bounces", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.GetBounces, "bounces:get")))
	api.PUT("/bounces/blocklist", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.BlocklistBouncedSubscribers, "bounces:manage")))
	api.GET("/bounces/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(hasID(a.GetBounce), "bounces:get")))
	api.DELETE("/bounces", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.DeleteBounces, "bounces:manage")))
	api.DELETE("/bounces/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(hasID(a.DeleteBounce), "bounces:manage")))

	api.POST("/subscribers/query/delete", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.DeleteSubscribersByQuery, "subscribers:manage")))
	api.PUT("/subscribers/query/blocklist", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.BlocklistSubscribersByQuery, "subscribers:manage")))
	api.PUT("/subscribers/query/lists", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.ManageSubscriberListsByQuery, "subscribers:manage")))
	api.GET("/subscribers/export", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.ExportSubscribers, "subscribers:get_all", "subscribers:get")))

	api.GET("/import/subscribers", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.GetImportSubscribers, "subscribers:import")))
	api.GET("/import/subscribers/logs", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.GetImportSubscriberStats, "subscribers:import")))
	api.POST("/import/subscribers", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.ImportSubscribers, "subscribers:import")))
	api.DELETE("/import/subscribers", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.StopImportSubscribers, "subscribers:import")))

	api.GET("/lists", wrapEcho(a, tpl, cfg, urlCfg, nil, a.GetLists))
	api.GET("/lists/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, a.GetList))
	api.POST("/lists", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.CreateList, "lists:manage_all")))
	api.PUT("/lists/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, a.UpdateList))
	api.DELETE("/lists", wrapEcho(a, tpl, cfg, urlCfg, nil, a.DeleteLists))
	api.DELETE("/lists/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, a.DeleteList))

	api.GET("/campaigns", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.GetCampaigns, "campaigns:get_all", "campaigns:get")))
	api.GET("/campaigns/running/stats", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.GetRunningCampaignStats, "campaigns:get_all", "campaigns:get")))
	api.GET("/campaigns/{id}/recover", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.GetCampaignRecover, "campaigns:manage_all", "campaigns:manage")))
	api.GET("/campaigns/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.GetCampaign, "campaigns:get_all", "campaigns:get")))
	api.GET("/campaigns/{first}/{second}", wrapEcho(a, tpl, cfg, urlCfg, []string{"first", "second"}, func(c echo.Context) error {
		switch {
		case c.Param("first") == "analytics":
			c.SetParamNames("type")
			c.SetParamValues(c.Param("second"))
			return pm(a.GetCampaignViewAnalytics, "campaigns:get_analytics")(c)
		case c.Param("second") == "preview":
			c.SetParamNames("id")
			c.SetParamValues(c.Param("first"))
			return pm(a.PreviewCampaign, "campaigns:get_all", "campaigns:get")(c)
		default:
			return echo.NewHTTPError(http.StatusNotFound, "404 unknown endpoint")
		}
	}))
	api.POST("/campaigns/{id}/preview/archive", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.PreviewCampaignArchive, "campaigns:get_all", "campaigns:get")))
	api.POST("/campaigns/{id}/preview", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.PreviewCampaign, "campaigns:get_all", "campaigns:get")))
	api.POST("/campaigns/{id}/content", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.CampaignContent, "campaigns:manage_all", "campaigns:manage")))
	api.POST("/campaigns/{id}/text", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.PreviewCampaign, "campaigns:get")))
	api.POST("/campaigns/{id}/test", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.TestCampaign, "campaigns:manage_all", "campaigns:manage")))
	api.POST("/campaigns", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.CreateCampaign, "campaigns:manage_all", "campaigns:manage")))
	api.PUT("/campaigns/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.UpdateCampaign, "campaigns:manage_all", "campaigns:manage")))
	api.PUT("/campaigns/{id}/status", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.UpdateCampaignStatus, "campaigns:manage_all", "campaigns:manage")))
	api.POST("/campaigns/{id}/recover", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.RecoverCampaign, "campaigns:manage_all", "campaigns:manage")))
	api.POST("/campaigns/{id}/ledger/resolve-inflight", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.ResolveCampaignLedgerInflight, "campaigns:manage_all", "campaigns:manage")))
	api.PUT("/campaigns/{id}/archive", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.UpdateCampaignArchive, "campaigns:manage_all", "campaigns:manage")))
	api.DELETE("/campaigns", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.DeleteCampaigns, "campaigns:manage", "campaigns:manage_all")))
	api.DELETE("/campaigns/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.DeleteCampaign, "campaigns:manage_all", "campaigns:manage")))

	api.GET("/media", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.GetAllMedia, "media:get")))
	api.GET("/media/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(hasID(a.GetMedia), "media:get")))
	api.POST("/media", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.UploadMedia, "media:manage")))
	api.DELETE("/media/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(hasID(a.DeleteMedia), "media:manage")))

	api.GET("/templates", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.GetTemplates, "templates:get")))
	api.GET("/templates/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.GetTemplate, "templates:get")))
	api.GET("/templates/{id}/preview", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.PreviewTemplate, "templates:get")))
	api.POST("/templates/preview", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.PreviewTemplateBody, "templates:get")))
	api.POST("/templates", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.CreateTemplate, "templates:manage")))
	api.PUT("/templates/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.UpdateTemplate, "templates:manage")))
	api.PUT("/templates/{id}/default", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.TemplateSetDefault, "templates:manage")))
	api.DELETE("/templates/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.DeleteTemplate, "templates:manage")))

	api.POST("/ai/campaign-builder/jobs", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.CreateAICampaignBuilderJob, "campaigns:manage_all", "campaigns:manage", "templates:manage")))
	api.GET("/ai/campaign-builder/jobs/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.GetAICampaignBuilderJob, "campaigns:manage_all", "campaigns:manage", "templates:manage")))
	api.GET("/ai/campaign-builder/jobs/{id}/stream", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.StreamAICampaignBuilderJob, "campaigns:manage_all", "campaigns:manage", "templates:manage")))
	api.POST("/ai/campaign-builder/jobs/{id}/cancel", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.CancelAICampaignBuilderJob, "campaigns:manage_all", "campaigns:manage", "templates:manage")))

	api.DELETE("/maintenance/subscribers/{type}", wrapEcho(a, tpl, cfg, urlCfg, []string{"type"}, pm(a.GCSubscribers, "settings:maintain")))
	api.DELETE("/maintenance/analytics/{type}", wrapEcho(a, tpl, cfg, urlCfg, []string{"type"}, pm(a.GCCampaignAnalytics, "settings:maintain")))
	api.DELETE("/maintenance/subscriptions/unconfirmed", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.GCSubscriptions, "settings:maintain")))

	api.GET("/tx", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.GetTxMessages, "tx:get")))
	api.GET("/tx/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.GetTxMessage, "tx:get")))
	api.POST("/tx", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.SendTxMessage, "tx:send")))

	api.GET("/profile", wrapEcho(a, tpl, cfg, urlCfg, nil, a.GetUserProfile))
	api.PUT("/profile", wrapEcho(a, tpl, cfg, urlCfg, nil, a.UpdateUserProfile))
	api.GET("/users", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.GetUsers, "users:get")))
	api.GET("/users/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.GetUser, "users:get")))
	api.POST("/users", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.CreateUser, "users:manage")))
	api.PUT("/users/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.UpdateUser, "users:manage")))
	api.DELETE("/users", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.DeleteUsers, "users:manage")))
	api.DELETE("/users/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(a.DeleteUser, "users:manage")))
	api.POST("/logout", wrapEcho(a, tpl, cfg, urlCfg, nil, a.Logout))

	api.GET("/users/{id}/twofa/totp", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, a.GenerateTOTPQR))
	api.PUT("/users/{id}/twofa", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, a.EnableTOTP))
	api.DELETE("/users/{id}/twofa", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, a.DisableTOTP))

	api.GET("/roles/users", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.GetUserRoles, "roles:get")))
	api.GET("/roles/lists", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.GeListRoles, "roles:get")))
	api.POST("/roles/users", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.CreateUserRole, "roles:manage")))
	api.POST("/roles/lists", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.CreateListRole, "roles:manage")))
	api.PUT("/roles/users/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(hasID(a.UpdateUserRole), "roles:manage")))
	api.PUT("/roles/lists/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(hasID(a.UpdateListRole), "roles:manage")))
	api.DELETE("/roles/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, pm(hasID(a.DeleteRole), "roles:manage")))

	if a.cfg.BounceWebhooksEnabled {
		api.POST("/webhooks/bounce", wrapEcho(a, tpl, cfg, urlCfg, nil, pm(a.BounceWebhook, "webhooks:post_bounce")))
	}

	public := se.Group("")
	public.GET("/mailapi/events", wrapEcho(a, tpl, cfg, urlCfg, nil, a.auth.APIMiddleware(pm(a.EventStream, "settings:get"))))
	if a.cfg.BounceWebhooksEnabled {
		public.POST("/webhooks/service/{service}", wrapEcho(a, tpl, cfg, urlCfg, []string{"service"}, a.BounceWebhook))
	}
	public.POST("/webhooks/quo/{token}", wrapEcho(a, tpl, cfg, urlCfg, []string{"token"}, a.QuoMessageWebhook))
	public.POST("/webhooks/email-replies", wrapEcho(a, tpl, cfg, urlCfg, nil, a.InboundEmailReplyWebhookPublic))

	public.GET("/", wrapEcho(a, tpl, cfg, urlCfg, nil, func(c echo.Context) error {
		return c.Render(http.StatusOK, "home", publicTpl{Title: "listpocket"})
	}))

	public.GET("/mailapi/public/lists", wrapEcho(a, tpl, cfg, urlCfg, nil, a.GetPublicLists))
	public.POST("/mailapi/public/subscription", wrapEcho(a, tpl, cfg, urlCfg, nil, a.PublicSubscription))
	public.GET("/mailapi/public/captcha/altcha", wrapEcho(a, tpl, cfg, urlCfg, nil, a.AltchaChallenge))
	if a.cfg.EnablePublicArchive {
		public.GET("/mailapi/public/archive", wrapEcho(a, tpl, cfg, urlCfg, nil, a.GetCampaignArchives))
	}

	public.GET("/subscription/form", wrapEcho(a, tpl, cfg, urlCfg, nil, a.SubscriptionFormPage))
	public.POST("/subscription/form", wrapEcho(a, tpl, cfg, urlCfg, nil, a.SubscriptionForm))
	public.GET("/subscription/{campUUID}/{subUUID}", wrapEcho(a, tpl, cfg, urlCfg, []string{"campUUID", "subUUID"}, noIndex(a.hasUUID(a.hasSub(a.SubscriptionPage), "campUUID", "subUUID"))))
	public.POST("/subscription/{campUUID}/{subUUID}", wrapEcho(a, tpl, cfg, urlCfg, []string{"campUUID", "subUUID"}, a.hasUUID(a.hasSub(a.SubscriptionPrefs), "campUUID", "subUUID")))
	public.GET("/subscription/optin/{subUUID}", wrapEcho(a, tpl, cfg, urlCfg, []string{"subUUID"}, noIndex(a.hasUUID(a.hasSub(a.OptinPage), "subUUID"))))
	public.POST("/subscription/optin/{subUUID}", wrapEcho(a, tpl, cfg, urlCfg, []string{"subUUID"}, a.hasUUID(a.hasSub(a.OptinPage), "subUUID")))
	public.POST("/subscription/export/{subUUID}", wrapEcho(a, tpl, cfg, urlCfg, []string{"subUUID"}, a.hasUUID(a.hasSub(a.SelfExportSubscriberData), "subUUID")))
	public.POST("/subscription/wipe/{subUUID}", wrapEcho(a, tpl, cfg, urlCfg, []string{"subUUID"}, a.hasUUID(a.hasSub(a.WipeSubscriberData), "subUUID")))

	public.GET("/s/{campID}/{subID}", wrapEcho(a, tpl, cfg, urlCfg, []string{"campID", "subID"}, noIndex(a.hasRecordID(a.hasSub(a.SubscriptionPage), "campID", "subID"))))
	public.POST("/s/{campID}/{subID}", wrapEcho(a, tpl, cfg, urlCfg, []string{"campID", "subID"}, a.hasRecordID(a.hasSub(a.SubscriptionPrefs), "campID", "subID")))
	public.GET("/o/{subID}", wrapEcho(a, tpl, cfg, urlCfg, []string{"subID"}, noIndex(a.hasRecordID(a.hasSub(a.OptinPage), "subID"))))
	public.POST("/o/{subID}", wrapEcho(a, tpl, cfg, urlCfg, []string{"subID"}, a.hasRecordID(a.hasSub(a.OptinPage), "subID")))

	public.GET("/link/{linkUUID}/{campUUID}/{subUUID}", wrapEcho(a, tpl, cfg, urlCfg, []string{"linkUUID", "campUUID", "subUUID"}, noIndex(a.hasUUID(a.LinkRedirect, "linkUUID", "campUUID", "subUUID"))))
	public.GET("/l/{linkID}/{campID}/{subID}", wrapEcho(a, tpl, cfg, urlCfg, []string{"linkID", "campID", "subID"}, noIndex(a.hasRecordID(a.LinkRedirect, "linkID", "campID", "subID"))))
	public.GET("/tx/link/{linkUUID}/{msgUUID}", wrapEcho(a, tpl, cfg, urlCfg, []string{"linkUUID", "msgUUID"}, noIndex(a.hasUUID(a.TxLinkRedirect, "linkUUID", "msgUUID"))))
	public.GET("/tx/{msgUUID}/px.png", wrapEcho(a, tpl, cfg, urlCfg, []string{"msgUUID"}, noIndex(a.hasUUID(a.RegisterTxMessageView, "msgUUID"))))
	public.GET("/campaign/{campUUID}/{subUUID}", wrapEcho(a, tpl, cfg, urlCfg, []string{"campUUID", "subUUID"}, noIndex(a.hasUUID(a.ViewCampaignMessage, "campUUID", "subUUID"))))
	public.GET("/campaign/{campUUID}/{subUUID}/px.png", wrapEcho(a, tpl, cfg, urlCfg, []string{"campUUID", "subUUID"}, noIndex(a.hasUUID(a.RegisterCampaignView, "campUUID", "subUUID"))))

	public.GET("/m/{campID}/{subID}", wrapEcho(a, tpl, cfg, urlCfg, []string{"campID", "subID"}, noIndex(a.hasRecordID(a.ViewCampaignMessage, "campID", "subID"))))
	public.GET("/v/{campID}/{subID}/px.png", wrapEcho(a, tpl, cfg, urlCfg, []string{"campID", "subID"}, noIndex(a.hasRecordID(a.RegisterCampaignView, "campID", "subID"))))

	if a.cfg.EnablePublicArchive {
		public.GET("/archive", wrapEcho(a, tpl, cfg, urlCfg, nil, a.CampaignArchivesPage))
		public.GET("/archive.xml", wrapEcho(a, tpl, cfg, urlCfg, nil, a.GetCampaignArchivesFeed))
		public.GET("/archive/{id}", wrapEcho(a, tpl, cfg, urlCfg, []string{"id"}, a.CampaignArchivePage))
		public.GET("/archive/latest", wrapEcho(a, tpl, cfg, urlCfg, nil, a.CampaignArchivePageLatest))
	}

	public.GET("/public/custom.css", wrapEcho(a, tpl, cfg, urlCfg, nil, serveCustomAppearance("public.custom_css")))
	public.GET("/public/custom.js", wrapEcho(a, tpl, cfg, urlCfg, nil, serveCustomAppearance("public.custom_js")))
	public.GET("/health", wrapEcho(a, tpl, cfg, urlCfg, nil, a.HealthCheck))
}

func wrapEcho(a *App, tpl *template.Template, cfg *Config, urlCfg *UrlConfig, params []string, handler echo.HandlerFunc) func(e *pbcore.RequestEvent) error {
	return func(e *pbcore.RequestEvent) error {
		ec := echo.New()
		ec.HideBanner = true
		ec.HidePort = true
		ec.Renderer = &tplRenderer{
			templates:           tpl,
			SiteName:            cfg.SiteName,
			RootURL:             urlCfg.RootURL,
			LogoURL:             urlCfg.LogoURL,
			FaviconURL:          urlCfg.FaviconURL,
			AssetVersion:        cfg.AssetVersion,
			EnablePublicSubPage: cfg.EnablePublicSubPage,
			EnablePublicArchive: cfg.EnablePublicArchive,
			IndividualTracking:  cfg.Privacy.IndividualTracking,
		}
		ec.HTTPErrorHandler = func(err error, c echo.Context) {
			if _, ok := err.(*echo.HTTPError); !ok {
				a.log.Println(err.Error())
			}
			ec.DefaultHTTPErrorHandler(err, c)
		}

		c := ec.NewContext(e.Request, e.Response)
		c.Set("app", a)

		if len(params) > 0 {
			values := make([]string, len(params))
			for i, name := range params {
				values[i] = e.Request.PathValue(name)
			}
			c.SetParamNames(params...)
			c.SetParamValues(values...)
		}

		if e.Auth != nil {
			c.Set(auth.AuthRecordHTTPCtxKey, e.Auth)

			if strings.TrimSpace(e.Auth.Id) == "" {
				return echo.NewHTTPError(http.StatusForbidden, "invalid auth user")
			}

			user, err := a.core.GetUser(e.Auth.Id, "", "")
			if err != nil {
				return echo.NewHTTPError(http.StatusForbidden, "invalid auth user")
			}

			if roleID := auth.ExtractRoleIDFromRecord(e.Auth); roleID > 0 {
				user.UserRoleID = roleID
			}

			c.Set(auth.UserHTTPCtxKey, user)
		}

		return handler(c)
	}
}

// AdminPage is the root handler that renders the Javascript admin frontend.
func serveAdminSPAFallback(a *App) func(e *pbcore.RequestEvent) error {
	return func(e *pbcore.RequestEvent) error {
		if a.isSetupRequired() {
			return e.Redirect(http.StatusFound, path.Join(uriAdmin, "/setup"))
		}
		return e.FileFS(stuffbinSubFS{base: a.fs, root: "/admin"}, "index.html")
	}
}

// HealthCheck is a healthcheck endpoint that returns a 200 response.
func (a *App) HealthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, okResp{true})
}

// serveCustomAppearance serves the given custom CSS/JS appearance blob
// meant for customizing public and admin pages from the admin settings UI.
func serveCustomAppearance(name string) echo.HandlerFunc {
	return func(c echo.Context) error {
		var (
			app = c.Get("app").(*App)

			out []byte
			hdr string
		)

		switch name {
		case "admin.custom_css":
			out = app.cfg.Appearance.AdminCSS
			hdr = "text/css; charset=utf-8"

		case "admin.custom_js":
			out = app.cfg.Appearance.AdminJS
			hdr = "application/javascript; charset=utf-8"

		case "public.custom_css":
			out = app.cfg.Appearance.PublicCSS
			hdr = "text/css; charset=utf-8"

		case "public.custom_js":
			out = app.cfg.Appearance.PublicJS
			hdr = "application/javascript; charset=utf-8"
		}

		return c.Blob(http.StatusOK, hdr, out)
	}
}

// hasRecordID validates public tracking URL segments (PocketBase record ids or known sentinels).
func (a *App) hasRecordID(next echo.HandlerFunc, params ...string) echo.HandlerFunc {
	return func(c echo.Context) error {
		for _, p := range params {
			if !models.IsPublicTrackingPathID(c.Param(p)) {
				return c.Render(http.StatusBadRequest, tplMessage, makeMsgTpl(a.i18n.T("public.errorTitle"), "",
					a.i18n.T("globals.messages.invalidUUID")))
			}
		}
		return next(c)
	}
}

// hasUUID middleware validates the UUID string format for a given set of params.
func (a *App) hasUUID(next echo.HandlerFunc, params ...string) echo.HandlerFunc {
	return func(c echo.Context) error {
		for _, p := range params {
			if !reUUID.MatchString(c.Param(p)) {
				return c.Render(http.StatusBadRequest, tplMessage, makeMsgTpl(a.i18n.T("public.errorTitle"), "",
					a.i18n.T("globals.messages.invalidUUID")))
			}
		}
		return next(c)
	}
}

// hasID middleware validates the :id param in the URL and sets its int value in the context.
func hasID(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, _ := strconv.Atoi(c.Param("id"))
		if id < 1 {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid ID")
		}

		c.Set("id", id)
		return next(c)
	}
}

// hasSub middleware checks if a subscriber exists given the UUID
// param in a request.
func (a *App) hasSub(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		subUUID := strings.TrimSpace(c.Param("subUUID"))
		if subUUID == "" {
			subUUID = strings.TrimSpace(c.Param("subID"))
		}

		if _, err := a.core.GetSubscriber(0, subUUID, ""); err != nil {
			if er, ok := err.(*echo.HTTPError); ok && er.Code == http.StatusBadRequest {
				return c.Render(http.StatusNotFound, tplMessage,
					makeMsgTpl(a.i18n.T("public.notFoundTitle"), "", er.Message.(string)))
			}

			a.log.Printf("error checking subscriber existence: %v", err)
			return c.Render(http.StatusInternalServerError, tplMessage,
				makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.T("public.errorProcessingRequest")))
		}

		return next(c)
	}
}

// noIndex adds the HTTP header requesting robots to not crawl the page.
func noIndex(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		c.Response().Header().Set("X-Robots-Tag", "noindex")
		return next(c)
	}
}

// getID returns the :id param from the URL parsed and stored as an int by the hasID middleware.
func getID(c echo.Context) int {
	return c.Get("id").(int)
}
