package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/compdani/list_pocket/internal/auth"
	"github.com/compdani/list_pocket/internal/config"
	"github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/internal/manager"
	"github.com/compdani/list_pocket/internal/plugins"
	"github.com/compdani/list_pocket/models"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	pbcore "github.com/pocketbase/pocketbase/core"
)

const pluginSettingsPrefix = "plugin:"

// pluginBridge implements plugins.HostServices on top of the app's services.
type pluginBridge struct {
	pb   *pocketbase.PocketBase
	core *core.Core
	send func(plugins.SendInput) error
}

func (b *pluginBridge) settingsRecord(id string) (*pbcore.Record, error) {
	return b.pb.FindFirstRecordByFilter("listpocket_settings", "type={:type}", dbx.Params{"type": pluginSettingsPrefix + id})
}

// LoadPluginSettings returns stored settings, or an empty map if none are saved.
func (b *pluginBridge) LoadPluginSettings(id string) (map[string]any, error) {
	out := map[string]any{}
	rec, err := b.settingsRecord(id)
	if err != nil || rec == nil {
		return out, nil
	}
	raw := rec.GetString("value")
	if raw == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("plugin %s settings: %w", id, err)
	}
	return out, nil
}

// SavePluginSettings writes the plugin's settings row.
func (b *pluginBridge) SavePluginSettings(id string, values map[string]any) error {
	raw, err := json.Marshal(values)
	if err != nil {
		return err
	}
	col, err := b.pb.FindCollectionByNameOrId("listpocket_settings")
	if err != nil {
		return err
	}
	rec, err := b.settingsRecord(id)
	if err != nil || rec == nil {
		rec = pbcore.NewRecord(col)
		rec.Set("type", pluginSettingsPrefix+id)
	}
	rec.Set("value", string(raw))
	return b.pb.Save(rec)
}

// DeletePluginSettings removes the plugin's settings row, if it has one.
func (b *pluginBridge) DeletePluginSettings(id string) error {
	rec, err := b.settingsRecord(id)
	if err != nil || rec == nil {
		return nil
	}
	return b.pb.Delete(rec)
}

// StripRolePermissions removes permissions from every role that holds them.
func (b *pluginBridge) StripRolePermissions(perms []string) error {
	roles, err := b.core.GetRoles()
	if err != nil {
		return err
	}
	var errs []error
	for _, role := range roles {
		kept := slices.DeleteFunc(slices.Clone([]string(role.Permissions)), func(p string) bool {
			return slices.Contains(perms, p)
		})
		if len(kept) == len(role.Permissions) {
			continue
		}
		role.Permissions = kept
		if _, err := b.core.UpdateUserRole(role.RecordID, role); err != nil {
			errs = append(errs, fmt.Errorf("role %d: %w", role.ID, err))
		}
	}
	return errors.Join(errs...)
}

// initPlugins boots the plugin manager and merges plugin permissions into the
// catalog, which must happen before the Super Admin role is synced.
func initPlugins(ko *config.Conf, pb *pocketbase.PocketBase, coreSvc *core.Core, campMgr *manager.Manager, cfg *Config) (*plugins.Manager, []manager.Messenger) {
	safeMode := ko.Bool("plugins.safe_mode")
	if safeMode {
		lo.Println("plugin safe mode is on; no plugins will load")
	}
	mgr := plugins.New(pb, plugins.Options{
		Dir:      filepath.Join(pb.DataDir(), "plugins"),
		SafeMode: safeMode,
		Log:      lo,
		Host:     &pluginBridge{pb: pb, core: coreSvc, send: newChannelSender(coreSvc, campMgr)},
	})
	if err := coreSvc.EnsureChannelTriggers(); err != nil {
		lo.Printf("error creating channel cleanup triggers: %v", err)
	}
	if err := coreSvc.EnsureCRMTriggers(); err != nil {
		lo.Printf("error creating CRM cleanup triggers: %v", err)
	}
	if err := mgr.Boot(); err != nil {
		// Plugins are optional. A broken plugin directory must not stop the app.
		lo.Printf("error booting plugins: %v", err)
		return mgr, nil
	}

	groups, err := mgr.InstalledPermissions()
	if err != nil {
		lo.Printf("error reading plugin permissions: %v", err)
	} else if err := mergePluginPermissions(cfg, groups); err != nil {
		lo.Printf("error merging plugin permissions: %v", err)
	}

	pluginMsgrs, specs := mgr.Messengers()
	for _, spec := range specs {
		models.RegisterChannel(spec)
	}
	added := make([]manager.Messenger, 0, len(pluginMsgrs))
	for _, m := range pluginMsgrs {
		if err := campMgr.AddMessenger(m); err != nil {
			lo.Printf("error registering plugin channel %s: %v", m.Name(), err)
			continue
		}
		added = append(added, m)
	}
	return mgr, added
}

// newChannelSender sends one message on a plugin channel, for host.messages.send
// and for transactional and workflow sends. It's refused unless the contact has
// consented on at least one list and has somewhere to receive it.
func newChannelSender(coreSvc *core.Core, campMgr *manager.Manager) func(plugins.SendInput) error {
	return func(in plugins.SendInput) error {
		spec := models.ChannelFor(in.Channel)
		if spec.Disabled || !campMgr.HasMessenger(in.Channel) {
			return fmt.Errorf("channel %s is not available", in.Channel)
		}
		sub, ok, err := coreSvc.GetSubscriberForChannel(in.SubscriberID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("unknown subscriber %q", in.SubscriberID)
		}
		to, err := coreSvc.ChannelRecipients(sub, in.Channel)
		if err != nil {
			return err
		}
		if len(to) == 0 {
			return plugins.ErrNoConsent
		}
		sub.Addresses = to
		return campMgr.PushMessage(models.Message{
			To:          to,
			Subject:     in.Subject,
			ContentType: models.CampaignContentTypePlain,
			Body:        []byte(in.Body),
			Subscriber:  sub,
			Messenger:   in.Channel,
		})
	}
}

func mergePluginPermissions(cfg *Config, groups []plugins.PermissionGroup) error {
	if len(groups) == 0 {
		return nil
	}
	var catalog []json.RawMessage
	if err := json.Unmarshal(cfg.PermissionsRaw, &catalog); err != nil {
		return err
	}
	for _, group := range groups {
		raw, err := json.Marshal(group)
		if err != nil {
			return err
		}
		catalog = append(catalog, raw)
		for _, perm := range group.Permissions {
			cfg.Permissions[perm] = struct{}{}
		}
	}
	merged, err := json.Marshal(catalog)
	if err != nil {
		return err
	}
	cfg.PermissionsRaw = merged
	return nil
}

func contactMap(sub models.Subscriber) map[string]any {
	email := sub.Email
	if models.IsPlaceholderEmail(email) {
		email = ""
	}
	return map[string]any{
		"id":     sub.RecordID,
		"uuid":   sub.UUID,
		"email":  email,
		"phone":  sub.Phone,
		"name":   sub.Name,
		"status": sub.Status,
	}
}

// FindContact looks a contact up by their address on a channel.
func (b *pluginBridge) FindContact(channel, address string) (map[string]any, bool, error) {
	sub, ok, err := b.core.FindSubscriberByChannelAddress(channel, address)
	if err != nil || !ok {
		return nil, false, err
	}
	return contactMap(sub), true, nil
}

// GetContact returns a contact by subscriber record ID.
func (b *pluginBridge) GetContact(id string) (map[string]any, bool, error) {
	sub, ok, err := b.core.GetSubscriberForChannel(id)
	if err != nil || !ok {
		return nil, false, err
	}
	return contactMap(sub), true, nil
}

// UpsertContact finds or creates a contact and applies lists and consent.
func (b *pluginBridge) UpsertContact(in plugins.ContactInput) (map[string]any, error) {
	sub, err := b.core.UpsertChannelContact(core.ChannelContact{
		Channel: in.Channel,
		Address: in.Address,
		Name:    in.Name,
		Lists:   in.Lists,
		Consent: in.Consent,
		Source:  in.Source,
	})
	if err != nil {
		return nil, err
	}
	return contactMap(sub), nil
}

// SetConsent records consent for one list on a channel.
func (b *pluginBridge) SetConsent(subscriberID, listID, channel, status, source string) error {
	return b.core.SetChannelConsent(subscriberID, listID, channel, status, source)
}

// MarkAddressInvalid retires an address the provider rejected permanently.
func (b *pluginBridge) MarkAddressInvalid(subscriberID, channel, address string) error {
	return b.core.MarkChannelAddressInvalid(subscriberID, channel, address)
}

// RecordEvent stores a channel event, returning false for a duplicate.
func (b *pluginBridge) RecordEvent(in plugins.EventInput) (bool, error) {
	return b.core.RecordChannelEvent(core.ChannelEvent{
		Channel:       in.Channel,
		Direction:     in.Direction,
		Kind:          in.Kind,
		Status:        in.Status,
		ProviderMsgID: in.ProviderMsgID,
		SubscriberID:  in.SubscriberID,
		CampaignID:    in.CampaignID,
		Address:       in.Address,
		Body:          in.Body,
		Payload:       in.Payload,
	})
}

// UpdateEventStatus applies a delivery update.
func (b *pluginBridge) UpdateEventStatus(channel, providerMsgID, status string, permanent bool) (bool, error) {
	return b.core.ApplyChannelStatus(channel, providerMsgID, status, permanent)
}

// RecordBounce goes straight to core, because the bounce manager is nil when
// email bounce processing is off.
func (b *pluginBridge) RecordBounce(in plugins.BounceInput) error {
	return b.core.RecordChannelBounce(in.Channel, in.SubscriberID, in.CampaignID, in.Type, in.Meta)
}

// OptOut withdraws channel consent for everyone who owns an address.
func (b *pluginBridge) OptOut(channel, address string) (int64, error) {
	return b.core.OptOutChannelAddress(channel, address)
}

// ImportSMSConsent grants channel consent from SMS consent, or only counts with dryRun.
func (b *pluginBridge) ImportSMSConsent(channel string, dryRun, includeInherited bool) (map[string]any, error) {
	out, err := b.core.ImportSMSConsent(channel, dryRun, includeInherited)
	if err != nil {
		return nil, err
	}
	return map[string]any{"explicit": out.Explicit, "inherited": out.Inherited, "applied": out.Applied}, nil
}

// ListEvents returns a subscriber's latest events on a channel.
func (b *pluginBridge) ListEvents(subscriberID, channel string, limit int) ([]map[string]any, error) {
	rows, err := b.core.ListChannelEvents(subscriberID, channel, limit)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"direction": row.Direction, "kind": row.Kind, "status": row.Status,
			"providerMsgId": row.ProviderMsgID, "address": row.Address, "body": row.Body, "occurredAt": row.OccurredAt,
		})
	}
	return out, nil
}

// CountEvents returns a campaign's outbound events by delivery status.
func (b *pluginBridge) CountEvents(campaignID string) (map[string]int, error) {
	return b.core.CountChannelEvents(campaignID)
}

// SendMessage sends one consent-checked message on a channel.
func (b *pluginBridge) SendMessage(in plugins.SendInput) error {
	return b.send(in)
}

func isSuperAdmin(re *pbcore.RequestEvent) bool {
	return auth.ExtractRoleIDRE(re) == auth.SuperAdminRoleID
}
