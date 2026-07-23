package main

import (
	"net/http"
	"path"
	"regexp"

	"github.com/compdani/list_pocket/internal/apperr"
	"github.com/compdani/list_pocket/internal/auth"
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
func registerHandlers(se *router.Router[*pbcore.RequestEvent], a *App) {

	//Token exchange routes
	auth.RegisterExchangeRoutes(se)

	admin := se.Group("")
	admin.GET(path.Join(uriAdmin, "/setup"), asHandler(a.LoginSetupPage))
	admin.POST(path.Join(uriAdmin, "/setup"), asHandler(a.LoginSetupPage))
	admin.GET(path.Join(uriAdmin, "/custom.css"), asHandler(serveCustomAppearance("admin.custom_css")))
	admin.GET(path.Join(uriAdmin, "/custom.js"), asHandler(serveCustomAppearance("admin.custom_js")))
	admin.GET("/custom.css", asHandler(serveCustomAppearance("admin.custom_css")))
	admin.GET("/custom.js", asHandler(serveCustomAppearance("admin.custom_js")))

	// Admin SPA routing: static assets are served separately via apis.Static in init.go.
	// All other GET requests under /admin fall back to the Vue SPA index unless
	// initial setup is still required, in which case they redirect to /admin/setup.
	se.GET(path.Join(uriAdmin, ""), serveAdminSPAFallback(a))
	se.GET(path.Join(uriAdmin, "/{path...}"), serveAdminSPAFallback(a))

	authAPI := se.Group("/mailapi/auth")
	authAPI.POST("/login", asHandler(a.AuthLogin))
	authAPI.POST("/twofa", asHandler(a.AuthVerifyTwoFA))
	authAPI.POST("/forgot", asHandler(a.AuthForgotPassword))
	authAPI.POST("/reset", asHandler(a.AuthResetPassword))

	pmRE := a.auth.PermRE
	api := se.Group("/mailapi").Bind(apis.RequireAuth()).BindFunc(hydrateAuthUser)

	api.GET("/health", asHandler(a.HealthCheck))
	api.GET("/config", asHandler(a.GetServerConfig))
	api.GET("/lang/{lang}", asHandler(a.GetI18nLang))
	api.GET("/dashboard/charts", asHandler(a.GetDashboardCharts))
	api.GET("/dashboard/counts", asHandler(a.GetDashboardCounts))

	api.GET("/settings", asHandler(pmRE(a.GetSettings, "settings:get")))
	api.PUT("/settings", asHandler(pmRE(a.UpdateSettings, "settings:manage")))
	api.GET("/settings/ai-builder", asHandler(pmRE(a.GetAIBuilderSettings, "settings:get")))
	api.PUT("/settings/ai-builder", asHandler(pmRE(a.UpdateAIBuilderSettings, "settings:manage")))
	api.PUT("/settings/{key}", asHandler(pmRE(a.UpdateSettingsByKey, "settings:manage")))
	api.POST("/settings/smtp/test", asHandler(pmRE(a.TestSMTPSettings, "settings:manage")))
	api.GET("/settings/text-messaging", asHandler(pmRE(a.GetTextMessagingSettings, "settings:get")))
	api.PUT("/settings/text-messaging", asHandler(pmRE(a.UpdateTextMessagingSettings, "settings:manage")))
	api.POST("/settings/text-messaging/test", asHandler(pmRE(a.TestTextMessagingSettings, "settings:manage")))
	api.POST("/webhooks/email-replies", asHandler(pmRE(a.InboundEmailReplyWebhook, "webhooks:post_bounce")))
	api.POST("/admin/reload", asHandler(pmRE(a.ReloadApp, "settings:manage")))
	api.GET("/logs", asHandler(pmRE(a.GetLogs, "settings:get")))
	api.GET("/about", asHandler(a.GetAboutInfo))

	api.GET("/subscribers", asHandler(pmRE(a.QuerySubscribers, "subscribers:get_all", "subscribers:get")))
	api.GET("/subscribers/{id}", asHandler(pmRE(a.GetSubscriber, "subscribers:get_all", "subscribers:get")))
	api.GET("/subscribers/{id}/activity", asHandler(pmRE(a.GetSubscriberActivity, "subscribers:get_all", "subscribers:get")))
	api.GET("/subscribers/{id}/timeline", asHandler(pmRE(a.GetSubscriberTimeline, "subscribers:get_all", "subscribers:get")))
	api.GET("/subscribers/{id}/export", asHandler(pmRE(a.ExportSubscriberData, "subscribers:get_all", "subscribers:get")))
	api.GET("/inbound-email-replies/{replyId}/attachments", asHandler(pmRE(a.GetInboundEmailAttachments, "subscribers:get_all", "subscribers:get")))
	api.GET("/inbound-email-attachments/{id}/download", asHandler(pmRE(a.DownloadInboundEmailAttachment, "subscribers:get_all", "subscribers:get")))
	api.GET("/inbound-emails", asHandler(pmRE(a.GetInboundEmailInbox, "inbox:get")))
	api.GET("/inbound-emails/{id}", asHandler(pmRE(a.GetInboundEmailByID, "inbox:get")))
	api.PUT("/inbound-emails/{id}/spam", asHandler(pmRE(a.UpdateInboundEmailSpamStatus, "inbox:manage")))
	api.GET("/inbound-email-spam-rules", asHandler(pmRE(a.GetInboundSpamRules, "inbox:manage")))
	api.DELETE("/inbound-email-spam-rules/{id}", asHandler(pmRE(a.DeleteInboundSpamRule, "inbox:manage")))
	api.DELETE("/maintenance/inbound-emails/spam", asHandler(pmRE(a.GCSpamInboundEmails, "inbox:manage")))
	api.GET("/subscribers/{id}/bounces", asHandler(pmRE(a.GetSubscriberBounces, "bounces:get")))
	api.DELETE("/subscribers/{id}/bounces", asHandler(pmRE(a.DeleteSubscriberBounces, "bounces:manage")))
	api.POST("/subscribers", asHandler(pmRE(a.CreateSubscriber, "subscribers:manage")))
	api.PUT("/subscribers/{id}", asHandler(pmRE(a.UpdateSubscriber, "subscribers:manage")))
	api.POST("/subscribers/{id}/optin", asHandler(pmRE(a.SubscriberSendOptin, "subscribers:manage")))
	api.POST("/subscribers/{id}/sms-opt-out", asHandler(pmRE(a.SubscriberSMSOptOut, "subscribers:manage")))
	api.PUT("/subscribers/blocklist", asHandler(pmRE(a.BlocklistSubscribers, "subscribers:manage")))
	api.PUT("/subscribers/{first}/{second}", asHandler(func(re *pbcore.RequestEvent) error {
		switch {
		case pathParam(re, "first") == "lists":
			re.Set("override_id", pathParam(re, "second"))
			return pmRE(a.ManageSubscriberLists, "subscribers:manage")(re)
		case pathParam(re, "second") == "blocklist":
			re.Set("override_id", pathParam(re, "first"))
			return pmRE(a.BlocklistSubscriber, "subscribers:manage")(re)
		default:
			return apperr.NotFound("404 unknown endpoint")
		}
	}))
	api.PUT("/subscribers/lists", asHandler(pmRE(a.ManageSubscriberLists, "subscribers:manage")))
	api.PUT("/subscribers/bulk-update", asHandler(pmRE(a.BulkUpdateSubscribers, "subscribers:manage")))
	api.POST("/subscribers/bulk-add", asHandler(pmRE(a.BulkAddSubscribers, "subscribers:import")))
	api.DELETE("/subscribers/{id}", asHandler(pmRE(a.DeleteSubscriber, "subscribers:manage")))
	api.DELETE("/subscribers", asHandler(pmRE(a.DeleteSubscribers, "subscribers:manage")))

	api.GET("/bounces", asHandler(pmRE(a.GetBounces, "bounces:get")))
	api.PUT("/bounces/blocklist", asHandler(pmRE(a.BlocklistBouncedSubscribers, "bounces:manage")))
	api.GET("/bounces/{id}", asHandler(pmRE(a.GetBounce, "bounces:get")))
	api.DELETE("/bounces", asHandler(pmRE(a.DeleteBounces, "bounces:manage")))
	api.DELETE("/bounces/{id}", asHandler(pmRE(a.DeleteBounce, "bounces:manage")))

	api.POST("/subscribers/query/delete", asHandler(pmRE(a.DeleteSubscribersByQuery, "subscribers:manage")))
	api.PUT("/subscribers/query/blocklist", asHandler(pmRE(a.BlocklistSubscribersByQuery, "subscribers:manage")))
	api.PUT("/subscribers/query/lists", asHandler(pmRE(a.ManageSubscriberListsByQuery, "subscribers:manage")))
	api.GET("/subscribers/export", asHandler(pmRE(a.ExportSubscribers, "subscribers:get_all", "subscribers:get")))

	api.GET("/import/subscribers", asHandler(pmRE(a.GetImportSubscribers, "subscribers:import")))
	api.GET("/import/subscribers/logs", asHandler(pmRE(a.GetImportSubscriberStats, "subscribers:import")))
	api.POST("/import/subscribers", asHandler(pmRE(a.ImportSubscribers, "subscribers:import")))
	api.DELETE("/import/subscribers", asHandler(pmRE(a.StopImportSubscribers, "subscribers:import")))

	api.GET("/lists", asHandler(a.GetLists))
	api.GET("/lists/{id}", asHandler(a.GetList))
	api.POST("/lists", asHandler(pmRE(a.CreateList, "lists:manage_all")))
	api.PUT("/lists/{id}", asHandler(a.UpdateList))
	api.DELETE("/lists", asHandler(a.DeleteLists))
	api.DELETE("/lists/{id}", asHandler(a.DeleteList))

	api.GET("/campaigns", asHandler(pmRE(a.GetCampaigns, "campaigns:get_all", "campaigns:get")))
	api.GET("/campaigns/running/stats", asHandler(pmRE(a.GetRunningCampaignStats, "campaigns:get_all", "campaigns:get")))
	api.GET("/campaigns/{id}/recover", asHandler(pmRE(a.GetCampaignRecover, "campaigns:manage_all", "campaigns:manage")))
	api.GET("/campaigns/{id}", asHandler(pmRE(a.GetCampaign, "campaigns:get_all", "campaigns:get")))
	api.GET("/campaigns/{first}/{second}", asHandler(func(re *pbcore.RequestEvent) error {
		switch {
		case pathParam(re, "first") == "analytics":
			re.Set("override_type", pathParam(re, "second"))
			return pmRE(a.GetCampaignViewAnalytics, "campaigns:get_analytics")(re)
		case pathParam(re, "second") == "preview":
			re.Set("override_id", pathParam(re, "first"))
			return pmRE(a.PreviewCampaign, "campaigns:get_all", "campaigns:get")(re)
		default:
			return apperr.NotFound("404 unknown endpoint")
		}
	}))
	api.POST("/campaigns/{id}/preview/archive", asHandler(pmRE(a.PreviewCampaignArchive, "campaigns:get_all", "campaigns:get")))
	api.POST("/campaigns/{id}/preview", asHandler(pmRE(a.PreviewCampaign, "campaigns:get_all", "campaigns:get")))
	api.POST("/campaigns/{id}/content", asHandler(pmRE(a.CampaignContent, "campaigns:manage_all", "campaigns:manage")))
	api.POST("/campaigns/{id}/text", asHandler(pmRE(a.PreviewCampaign, "campaigns:get")))
	api.POST("/campaigns/{id}/test", asHandler(pmRE(a.TestCampaign, "campaigns:manage_all", "campaigns:manage")))
	api.POST("/campaigns", asHandler(pmRE(a.CreateCampaign, "campaigns:manage_all", "campaigns:manage")))
	api.PUT("/campaigns/{id}", asHandler(pmRE(a.UpdateCampaign, "campaigns:manage_all", "campaigns:manage")))
	api.PUT("/campaigns/{id}/status", asHandler(pmRE(a.UpdateCampaignStatus, "campaigns:manage_all", "campaigns:manage")))
	api.POST("/campaigns/{id}/recover", asHandler(pmRE(a.RecoverCampaign, "campaigns:manage_all", "campaigns:manage")))
	api.POST("/campaigns/{id}/ledger/resolve-inflight", asHandler(pmRE(a.ResolveCampaignLedgerInflight, "campaigns:manage_all", "campaigns:manage")))
	api.PUT("/campaigns/{id}/archive", asHandler(pmRE(a.UpdateCampaignArchive, "campaigns:manage_all", "campaigns:manage")))
	api.DELETE("/campaigns", asHandler(pmRE(a.DeleteCampaigns, "campaigns:manage", "campaigns:manage_all")))
	api.DELETE("/campaigns/{id}", asHandler(pmRE(a.DeleteCampaign, "campaigns:manage_all", "campaigns:manage")))

	api.GET("/media", asHandler(pmRE(a.GetAllMedia, "media:get")))
	api.GET("/media/{id}", asHandler(pmRE(a.GetMedia, "media:get")))
	api.POST("/media", asHandler(pmRE(a.UploadMedia, "media:manage")))
	api.DELETE("/media/{id}", asHandler(pmRE(a.DeleteMedia, "media:manage")))

	api.GET("/templates", asHandler(pmRE(a.GetTemplates, "templates:get")))
	api.GET("/templates/{id}", asHandler(pmRE(a.GetTemplate, "templates:get")))
	api.GET("/templates/{id}/preview", asHandler(pmRE(a.PreviewTemplate, "templates:get")))
	api.POST("/templates/preview", asHandler(pmRE(a.PreviewTemplateBody, "templates:get")))
	api.POST("/templates", asHandler(pmRE(a.CreateTemplate, "templates:manage")))
	api.PUT("/templates/{id}", asHandler(pmRE(a.UpdateTemplate, "templates:manage")))
	api.PUT("/templates/{id}/default", asHandler(pmRE(a.TemplateSetDefault, "templates:manage")))
	api.DELETE("/templates/{id}", asHandler(pmRE(a.DeleteTemplate, "templates:manage")))

	api.POST("/ai/campaign-builder/jobs", asHandler(pmRE(a.CreateAICampaignBuilderJob, "campaigns:manage_all", "campaigns:manage", "templates:manage")))
	api.GET("/ai/campaign-builder/jobs/{id}", asHandler(pmRE(a.GetAICampaignBuilderJob, "campaigns:manage_all", "campaigns:manage", "templates:manage")))
	api.GET("/ai/campaign-builder/jobs/{id}/stream", asHandler(pmRE(a.StreamAICampaignBuilderJob, "campaigns:manage_all", "campaigns:manage", "templates:manage")))
	api.POST("/ai/campaign-builder/jobs/{id}/cancel", asHandler(pmRE(a.CancelAICampaignBuilderJob, "campaigns:manage_all", "campaigns:manage", "templates:manage")))

	api.DELETE("/maintenance/subscribers/{type}", asHandler(pmRE(a.GCSubscribers, "settings:maintain")))
	api.DELETE("/maintenance/analytics/{type}", asHandler(pmRE(a.GCCampaignAnalytics, "settings:maintain")))
	api.DELETE("/maintenance/subscriptions/unconfirmed", asHandler(pmRE(a.GCSubscriptions, "settings:maintain")))

	api.GET("/tx", asHandler(pmRE(a.GetTxMessages, "tx:get")))
	api.GET("/tx/{id}", asHandler(pmRE(a.GetTxMessage, "tx:get")))
	api.POST("/tx", asHandler(pmRE(a.SendTxMessage, "tx:send")))

	api.GET("/profile", asHandler(a.GetUserProfile))
	api.PUT("/profile", asHandler(a.UpdateUserProfile))
	api.GET("/users", asHandler(pmRE(a.GetUsers, "users:get")))
	api.GET("/users/{id}", asHandler(pmRE(a.GetUser, "users:get")))
	api.POST("/users", asHandler(pmRE(a.CreateUser, "users:manage")))
	api.PUT("/users/{id}", asHandler(pmRE(a.UpdateUser, "users:manage")))
	api.DELETE("/users", asHandler(pmRE(a.DeleteUsers, "users:manage")))
	api.DELETE("/users/{id}", asHandler(pmRE(a.DeleteUser, "users:manage")))
	api.POST("/logout", asHandler(a.Logout))

	api.GET("/users/{id}/twofa/totp", asHandler(a.GenerateTOTPQR))
	api.PUT("/users/{id}/twofa", asHandler(a.EnableTOTP))
	api.DELETE("/users/{id}/twofa", asHandler(a.DisableTOTP))

	api.GET("/roles/users", asHandler(pmRE(a.GetUserRoles, "roles:get")))
	api.GET("/roles/lists", asHandler(pmRE(a.GeListRoles, "roles:get")))
	api.POST("/roles/users", asHandler(pmRE(a.CreateUserRole, "roles:manage")))
	api.POST("/roles/lists", asHandler(pmRE(a.CreateListRole, "roles:manage")))
	api.PUT("/roles/users/{id}", asHandler(pmRE(a.UpdateUserRole, "roles:manage")))
	api.PUT("/roles/lists/{id}", asHandler(pmRE(a.UpdateListRole, "roles:manage")))
	api.DELETE("/roles/{id}", asHandler(pmRE(a.DeleteRole, "roles:manage")))

	if a.cfg.BounceWebhooksEnabled {
		api.POST("/webhooks/bounce", asHandler(pmRE(a.BounceWebhook, "webhooks:post_bounce")))
	}

	public := se.Group("")
	public.GET("/mailapi/events", asHandler(a.auth.APIMiddlewareRE(pmRE(a.EventStream, "settings:get"))))
	if a.cfg.BounceWebhooksEnabled {
		public.POST("/webhooks/service/{service}", asHandler(a.BounceWebhook))
	}
	public.POST("/webhooks/quo/{token}", asHandler(a.QuoMessageWebhook))
	public.POST("/webhooks/email-replies", asHandler(a.InboundEmailReplyWebhookPublic))

	public.GET("/", asHandler(func(re *pbcore.RequestEvent) error {
		return renderTpl(re, http.StatusOK, "home", publicTpl{Title: "listpocket"})
	}))

	public.GET("/mailapi/public/lists", asHandler(a.GetPublicLists))
	public.POST("/mailapi/public/subscription", asHandler(a.PublicSubscription))
	public.GET("/mailapi/public/captcha/altcha", asHandler(a.AltchaChallenge))
	if a.cfg.EnablePublicArchive {
		public.GET("/mailapi/public/archive", asHandler(a.GetCampaignArchives))
	}

	public.GET("/subscription/form", asHandler(a.SubscriptionFormPage))
	public.POST("/subscription/form", asHandler(a.SubscriptionForm))
	public.GET("/subscription/{campUUID}/{subUUID}", asHandler(noIndexRE(a.hasUUIDRE(a.hasSubRE(a.SubscriptionPage), "campUUID", "subUUID"))))
	public.POST("/subscription/{campUUID}/{subUUID}", asHandler(a.hasUUIDRE(a.hasSubRE(a.SubscriptionPrefs), "campUUID", "subUUID")))
	public.GET("/subscription/optin/{subUUID}", asHandler(noIndexRE(a.hasUUIDRE(a.hasSubRE(a.OptinPage), "subUUID"))))
	public.POST("/subscription/optin/{subUUID}", asHandler(a.hasUUIDRE(a.hasSubRE(a.OptinPage), "subUUID")))
	public.POST("/subscription/export/{subUUID}", asHandler(a.hasUUIDRE(a.hasSubRE(a.SelfExportSubscriberData), "subUUID")))
	public.POST("/subscription/wipe/{subUUID}", asHandler(a.hasUUIDRE(a.hasSubRE(a.WipeSubscriberData), "subUUID")))

	public.GET("/s/{campID}/{subID}", asHandler(noIndexRE(a.hasRecordIDRE(a.hasSubRE(a.SubscriptionPage), "campID", "subID"))))
	public.POST("/s/{campID}/{subID}", asHandler(a.hasRecordIDRE(a.hasSubRE(a.SubscriptionPrefs), "campID", "subID")))
	public.GET("/o/{subID}", asHandler(noIndexRE(a.hasRecordIDRE(a.hasSubRE(a.OptinPage), "subID"))))
	public.POST("/o/{subID}", asHandler(a.hasRecordIDRE(a.hasSubRE(a.OptinPage), "subID")))

	public.GET("/link/{linkUUID}/{campUUID}/{subUUID}", asHandler(noIndexRE(a.hasUUIDRE(a.LinkRedirect, "linkUUID", "campUUID", "subUUID"))))
	public.GET("/l/{linkID}/{campID}/{subID}", asHandler(noIndexRE(a.hasRecordIDRE(a.LinkRedirect, "linkID", "campID", "subID"))))
	public.GET("/tx/link/{linkUUID}/{msgUUID}", asHandler(noIndexRE(a.hasUUIDRE(a.TxLinkRedirect, "linkUUID", "msgUUID"))))
	public.GET("/tx/{msgUUID}/px.png", asHandler(noIndexRE(a.hasUUIDRE(a.RegisterTxMessageView, "msgUUID"))))
	public.GET("/campaign/{campUUID}/{subUUID}", asHandler(noIndexRE(a.hasUUIDRE(a.ViewCampaignMessage, "campUUID", "subUUID"))))
	public.GET("/campaign/{campUUID}/{subUUID}/px.png", asHandler(noIndexRE(a.hasUUIDRE(a.RegisterCampaignView, "campUUID", "subUUID"))))

	public.GET("/m/{campID}/{subID}", asHandler(noIndexRE(a.hasRecordIDRE(a.ViewCampaignMessage, "campID", "subID"))))
	public.GET("/v/{campID}/{subID}/px.png", asHandler(noIndexRE(a.hasRecordIDRE(a.RegisterCampaignView, "campID", "subID"))))

	if a.cfg.EnablePublicArchive {
		public.GET("/archive", asHandler(a.CampaignArchivesPage))
		public.GET("/archive.xml", asHandler(a.GetCampaignArchivesFeed))
		public.GET("/archive/{id}", asHandler(a.CampaignArchivePage))
		public.GET("/archive/latest", asHandler(a.CampaignArchivePageLatest))
	}

	public.GET("/public/custom.css", asHandler(serveCustomAppearance("public.custom_css")))
	public.GET("/public/custom.js", asHandler(serveCustomAppearance("public.custom_js")))
	public.GET("/health", asHandler(a.HealthCheck))
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
func (a *App) HealthCheck(re *pbcore.RequestEvent) error {
	return okJSON(re, true)
}

// serveCustomAppearance serves the given custom CSS/JS appearance blob
// meant for customizing public and admin pages from the admin settings UI.
func serveCustomAppearance(name string) func(*pbcore.RequestEvent) error {
	return func(re *pbcore.RequestEvent) error {
		app := getApp(re)
		if app == nil {
			return apperr.Internal("app unavailable")
		}

		var (
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

		return writeBlob(re, http.StatusOK, hdr, out)
	}
}
