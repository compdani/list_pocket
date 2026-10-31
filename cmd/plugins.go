package main

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/compdani/list_pocket/internal/apperr"
	"github.com/compdani/list_pocket/internal/auth"
	"github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/internal/plugins"
	pbcore "github.com/pocketbase/pocketbase/core"
)

const maxPluginUploadBytes = 64 << 20

// pluginRouteMethods are the methods a plugin can declare in its manifest.
var pluginRouteMethods = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}

// requireSuperAdmin guards every plugin management route. Installing a plugin
// runs code with full app access, so no ordinary permission can grant this.
func requireSuperAdmin(next func(*pbcore.RequestEvent) error) func(*pbcore.RequestEvent) error {
	return func(re *pbcore.RequestEvent) error {
		if !isSuperAdmin(re) {
			return apperr.Forbidden("only super admins can manage plugins")
		}
		return next(re)
	}
}

func pluginAPIError(err error) error {
	if errors.Is(err, plugins.ErrNotFound) {
		return apperr.NotFound(err.Error())
	}
	return apperr.BadRequest(err.Error())
}

// ListPlugins returns every installed plugin.
func (a *App) ListPlugins(re *pbcore.RequestEvent) error {
	list, err := a.plugins.List()
	if err != nil {
		return apperr.Internal(err.Error())
	}
	return okJSON(re, list)
}

// InstallPlugin accepts a multipart zip upload in the `file` field.
func (a *App) InstallPlugin(re *pbcore.RequestEvent) error {
	re.Request.Body = http.MaxBytesReader(re.Response, re.Request.Body, maxPluginUploadBytes)
	file, header, err := re.Request.FormFile("file")
	if err != nil {
		return apperr.BadRequest("upload a plugin zip in the file field")
	}
	defer file.Close()

	res, err := a.plugins.Install(file, header.Size)
	if err != nil {
		return pluginAPIError(err)
	}
	if res.NeedsRestart {
		return a.handleSettingsRestart(re)
	}
	return okJSON(re, res)
}

// EnablePlugin enables a plugin, which takes effect after a restart.
func (a *App) EnablePlugin(re *pbcore.RequestEvent) error {
	return a.setPluginEnabled(re, true)
}

// DisablePlugin disables a plugin, which stops its routes straight away. Channels
// and UI go at the restart that follows.
func (a *App) DisablePlugin(re *pbcore.RequestEvent) error {
	return a.setPluginEnabled(re, false)
}

func (a *App) setPluginEnabled(re *pbcore.RequestEvent, enabled bool) error {
	if !enabled {
		if err := a.checkPluginChannelsIdle(pathParam(re, "id")); err != nil {
			return err
		}
	}
	changed, err := a.plugins.SetEnabled(pathParam(re, "id"), enabled)
	if err != nil {
		return pluginAPIError(err)
	}
	if !changed {
		return okJSON(re, true)
	}
	return a.handleSettingsRestart(re)
}

// checkPluginChannelsIdle refuses to turn off a plugin while a campaign depends on
// its channels, because that campaign could no longer send.
func (a *App) checkPluginChannelsIdle(id string) error {
	man, err := a.plugins.Manifest(id)
	if err != nil {
		return pluginAPIError(err)
	}
	channels := make([]string, 0, len(man.Channels))
	for _, c := range man.Channels {
		channels = append(channels, c.ID)
	}
	n, err := a.core.CountCampaignsOnChannels(channels)
	if err != nil {
		return apperr.Internal(err.Error())
	}
	if n > 0 {
		return apperr.Conflict(fmt.Sprintf("%d running or scheduled campaigns use this plugin's channels. Pause or finish them first.", n))
	}
	return nil
}

// UninstallPlugin removes a disabled plugin. ?purge=1 also deletes its data.
func (a *App) UninstallPlugin(re *pbcore.RequestEvent) error {
	if err := a.plugins.Uninstall(pathParam(re, "id"), queryParam(re, "purge") == "1"); err != nil {
		return pluginAPIError(err)
	}
	return okJSON(re, true)
}

// GetPluginSettings returns a plugin's settings without secret values.
func (a *App) GetPluginSettings(re *pbcore.RequestEvent) error {
	values, secrets, err := a.plugins.SettingsView(pathParam(re, "id"))
	if err != nil {
		return pluginAPIError(err)
	}
	return re.JSON(http.StatusOK, okResp{Data: map[string]any{"values": values, "secrets": secrets}})
}

// UpdatePluginSettings saves a plugin's settings. It needs no restart.
func (a *App) UpdatePluginSettings(re *pbcore.RequestEvent) error {
	var update map[string]any
	if err := bindJSON(re, &update); err != nil {
		return err
	}
	if err := a.plugins.SaveSettings(pathParam(re, "id"), update); err != nil {
		return pluginAPIError(err)
	}
	return okJSON(re, true)
}

// GetPluginsUI lists what the admin UI loads at startup, for any logged-in user.
func (a *App) GetPluginsUI(re *pbcore.RequestEvent) error {
	return okJSON(re, a.plugins.UIManifests(isSuperAdmin(re)))
}

// PluginWebhook is the public receiver for plugin webhooks.
func (a *App) PluginWebhook(re *pbcore.RequestEvent) error {
	return a.plugins.ServeWebhook(re)
}

// PluginAdminRoute runs an authenticated plugin route after checking the
// permission the manifest declared for it.
func (a *App) PluginAdminRoute(re *pbcore.RequestEvent) error {
	p := a.plugins.Loaded(pathParam(re, "id"))
	if p == nil {
		return apperr.NotFound("plugin not found")
	}
	path := strings.Trim(pathParam(re, "path"), "/")
	idx := slices.IndexFunc(p.Manifest.AdminRoutes, func(r plugins.AdminRoute) bool { return r.Path == path })
	if idx < 0 {
		return apperr.NotFound("plugin route not found")
	}
	route := p.Manifest.AdminRoutes[idx]
	if !slices.Contains(route.Methods, re.Request.Method) {
		return apperr.New(http.StatusMethodNotAllowed, "method not allowed")
	}
	return a.auth.PermRE(auth.RequestEventHandler(func(e *pbcore.RequestEvent) error {
		return a.plugins.ServeAdminRoute(e, p, path)
	}), route.Permission)(re)
}

// pluginChannelIDs lists the channels of every running plugin.
func (a *App) pluginChannelIDs() []string {
	var out []string
	for _, p := range a.plugins.LoadedPlugins() {
		for _, c := range p.Manifest.Channels {
			out = append(out, c.ID)
		}
	}
	return out
}

// GetSubscriberChannels returns a subscriber's addresses and consent on plugin channels.
func (a *App) GetSubscriberChannels(re *pbcore.RequestEvent) error {
	out, err := a.core.GetSubscriberChannelInfo(pathParam(re, "id"), a.pluginChannelIDs())
	if err != nil {
		return apperr.Internal(err.Error())
	}
	return re.JSON(http.StatusOK, okResp{Data: out})
}

type subscriberChannelReq struct {
	Channel string `json:"channel"`
	Address string `json:"address"`
	ListID  string `json:"list_id"`
	Status  string `json:"status"`
}

// AddSubscriberChannelAddress adds an address on a plugin channel.
func (a *App) AddSubscriberChannelAddress(re *pbcore.RequestEvent) error {
	var req subscriberChannelReq
	if err := bindJSON(re, &req); err != nil {
		return err
	}
	if err := a.core.SetChannelAddress(pathParam(re, "id"), req.Channel, req.Address); err != nil {
		return err
	}
	return okJSON(re, true)
}

// RemoveSubscriberChannelAddress removes an address on a plugin channel.
func (a *App) RemoveSubscriberChannelAddress(re *pbcore.RequestEvent) error {
	var req subscriberChannelReq
	if err := bindJSON(re, &req); err != nil {
		return err
	}
	if err := a.core.RemoveChannelAddress(pathParam(re, "id"), req.Channel, req.Address); err != nil {
		return apperr.Internal(err.Error())
	}
	return okJSON(re, true)
}

// SetSubscriberChannelConsent records consent for one list on a plugin channel.
func (a *App) SetSubscriberChannelConsent(re *pbcore.RequestEvent) error {
	var req subscriberChannelReq
	if err := bindJSON(re, &req); err != nil {
		return err
	}
	if err := a.core.SetChannelConsent(pathParam(re, "id"), req.ListID, req.Channel, req.Status, core.ConsentSourceAdmin); err != nil {
		return err
	}
	return okJSON(re, true)
}
