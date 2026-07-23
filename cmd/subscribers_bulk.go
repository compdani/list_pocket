package main

import (
	"encoding/json"
	"github.com/compdani/list_pocket/internal/apperr"
	pbcore "github.com/pocketbase/pocketbase/core"
	"strings"

	"github.com/compdani/list_pocket/internal/auth"
	corepkg "github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/internal/subimporter"
	"github.com/compdani/list_pocket/models"
)

const bulkContactMaxBatch = 5000

type bulkContactOpsReq struct {
	Contacts           []json.RawMessage `json:"contacts"`
	TagsAdd            []string          `json:"tags_add"`
	TagsRemove         []string          `json:"tags_remove"`
	ListRemove         []string          `json:"list_remove"`
	ListUpdate         []string          `json:"list_update"`
	SubscriptionStatus string            `json:"subscription_status"`
	OverrideDetails    bool              `json:"override_details"`
}

type bulkContactResp struct {
	OK           bool `json:"ok"`
	Matched      int  `json:"matched,omitempty"`
	Skipped      int  `json:"skipped,omitempty"`
	Created      int  `json:"created,omitempty"`
	Updated      int  `json:"updated,omitempty"`
	TagsUpdated  int  `json:"tags_updated,omitempty"`
	ListsRemoved int  `json:"lists_removed,omitempty"`
	ListsUpdated int  `json:"lists_updated,omitempty"`
}

func (a *App) BulkUpdateSubscribers(re *pbcore.RequestEvent) error {
	user := auth.GetUserRE(re)

	var req bulkContactOpsReq
	if err := bindJSON(re, &req); err != nil {
		return err
	}

	emails, err := a.parseBulkUpdateEmails(req.Contacts)
	if err != nil {
		return err
	}
	if len(emails) == 0 {
		return apperr.BadRequest(a.i18n.T("subscribers.errorNoIDs"))
	}
	if len(emails) > bulkContactMaxBatch {
		return apperr.BadRequest(a.i18n.T("globals.messages.invalidData"))
	}
	if !bulkContactHasOps(req, false) {
		return apperr.BadRequest(a.i18n.T("globals.messages.invalidData"))
	}

	listRemove, listUpdate, err := a.filterBulkListOps(user, req.ListRemove, req.ListUpdate)
	if err != nil {
		return err
	}

	subs, _, err := a.core.LookupSubscribersByEmailsForBulk(emails)
	if err != nil {
		return err
	}
	if len(subs) > 0 {
		subIDs := make([]int, 0, len(subs))
		for _, sub := range subs {
			subIDs = append(subIDs, sub.ID)
		}
		if err := a.hasSubPerm(user, subIDs); err != nil {
			return err
		}
	}

	result, err := a.core.BulkUpdateSubscribersByEmail(
		emails,
		req.TagsAdd,
		req.TagsRemove,
		listRemove,
		listUpdate,
		req.SubscriptionStatus,
	)
	if err != nil {
		return err
	}

	if resultMatchedOrChanged(result) {
		_ = a.core.RefreshMatViews(true)
	}

	return okJSON(re, bulkContactResp{
		OK:           true,
		Matched:      result.Matched,
		Skipped:      result.Skipped,
		TagsUpdated:  result.TagsUpdated,
		ListsRemoved: result.ListsRemoved,
		ListsUpdated: result.ListsUpdated,
	})
}

func (a *App) BulkAddSubscribers(re *pbcore.RequestEvent) error {
	user := auth.GetUserRE(re)

	var req bulkContactOpsReq
	if err := bindJSON(re, &req); err != nil {
		return err
	}
	if len(req.Contacts) == 0 {
		return apperr.BadRequest(a.i18n.T("subscribers.errorNoIDs"))
	}
	if len(req.Contacts) > bulkContactMaxBatch {
		return apperr.BadRequest(a.i18n.T("globals.messages.invalidData"))
	}

	contacts, emails, err := a.parseBulkAddContacts(req.Contacts)
	if err != nil {
		return err
	}

	listRemove, listUpdate, err := a.filterBulkListOps(user, req.ListRemove, req.ListUpdate)
	if err != nil {
		return err
	}

	if len(emails) > 0 {
		subs, _, err := a.core.LookupSubscribersByEmailsForBulk(emails)
		if err != nil {
			return err
		}
		if len(subs) > 0 {
			subIDs := make([]int, 0, len(subs))
			for _, sub := range subs {
				subIDs = append(subIDs, sub.ID)
			}
			if err := a.hasSubPerm(user, subIDs); err != nil {
				return err
			}
		}
	}

	result, err := a.importer.BulkUpsertContacts(contacts, subimporter.BulkUpsertOpt{
		TagsAdd:            req.TagsAdd,
		TagsRemove:         req.TagsRemove,
		ListUpdate:         listUpdate,
		ListRemove:         listRemove,
		SubscriptionStatus: req.SubscriptionStatus,
		OverrideDetails:    req.OverrideDetails,
	})
	if err != nil {
		return err
	}

	if bulkAddChanged(result) {
		_ = a.core.RefreshMatViews(true)
	}

	return okJSON(re, bulkContactResp{
		OK:           true,
		Matched:      result.Created + result.Updated,
		Skipped:      result.Skipped,
		Created:      result.Created,
		Updated:      result.Updated,
		TagsUpdated:  result.TagsUpdated,
		ListsRemoved: result.ListsRemoved,
		ListsUpdated: result.ListsUpdated,
	})
}

func (a *App) parseBulkUpdateEmails(raw []json.RawMessage) ([]string, error) {
	emails := make([]string, 0, len(raw))
	for _, item := range raw {
		var email string
		if err := json.Unmarshal(item, &email); err != nil {
			return nil, apperr.BadRequest(a.i18n.T("globals.messages.invalidData"))
		}
		email = strings.TrimSpace(email)
		if email == "" {
			continue
		}
		sanitized, err := a.importer.SanitizeEmail(email)
		if err != nil {
			return nil, apperr.BadRequest(err.Error())
		}
		emails = append(emails, sanitized)
	}
	return uniqueNonEmptyStrings(emails), nil
}

func (a *App) parseBulkAddContacts(raw []json.RawMessage) ([]subimporter.SubReq, []string, error) {
	contacts := make([]subimporter.SubReq, 0, len(raw))
	emails := make([]string, 0, len(raw))

	for _, item := range raw {
		var contact subimporter.SubReq
		if err := json.Unmarshal(item, &contact); err != nil {
			return nil, nil, apperr.BadRequest(a.i18n.T("globals.messages.invalidData"))
		}

		// Accept CSV-style "attributes" alias for attribs.
		var alias struct {
			Attributes models.JSON `json:"attributes"`
		}
		_ = json.Unmarshal(item, &alias)
		if len(alias.Attributes) > 0 && len(contact.Attribs) == 0 {
			contact.Attribs = alias.Attributes
		}

		validated, err := a.importer.ValidateFields(contact)
		if err != nil {
			return nil, nil, apperr.BadRequest(err.Error())
		}
		contacts = append(contacts, validated)
		emails = append(emails, validated.Email)
	}

	return contacts, uniqueNonEmptyStrings(emails), nil
}

func (a *App) filterBulkListOps(user auth.User, listRemove, listUpdate []string) ([]string, []string, error) {
	removeIDs := uniqueNonEmptyStrings(listRemove)
	updateIDs := uniqueNonEmptyStrings(listUpdate)

	if len(removeIDs) > 0 {
		filtered := user.FilterListsByPerm(auth.PermTypeManage, removeIDs)
		if len(filtered) == 0 {
			return nil, nil, apperr.BadRequest(a.i18n.T("subscribers.errorNoListsGiven"))
		}
		resolved, err := a.core.ResolveListIDs(nil, filtered)
		if err != nil {
			return nil, nil, apperr.BadRequest(a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
		}
		if len(resolved) == 0 {
			return nil, nil, apperr.BadRequest(a.i18n.T("subscribers.errorNoListsGiven"))
		}
		removeIDs = filtered
	}

	if len(updateIDs) > 0 {
		filtered := user.FilterListsByPerm(auth.PermTypeManage, updateIDs)
		if len(filtered) == 0 {
			return nil, nil, apperr.BadRequest(a.i18n.T("subscribers.errorNoListsGiven"))
		}
		resolved, err := a.core.ResolveListIDs(nil, filtered)
		if err != nil {
			return nil, nil, apperr.BadRequest(a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
		}
		if len(resolved) == 0 {
			return nil, nil, apperr.BadRequest(a.i18n.T("subscribers.errorNoListsGiven"))
		}
		updateIDs = filtered
	}

	return removeIDs, updateIDs, nil
}

func bulkContactHasOps(req bulkContactOpsReq, allowCreateOnly bool) bool {
	if allowCreateOnly && len(req.Contacts) > 0 {
		return true
	}
	return len(req.TagsAdd) > 0 ||
		len(req.TagsRemove) > 0 ||
		len(req.ListRemove) > 0 ||
		len(req.ListUpdate) > 0
}

func resultMatchedOrChanged(result corepkg.BulkUpdateResult) bool {
	return result.Matched > 0 || result.TagsUpdated > 0 || result.ListsRemoved > 0 || result.ListsUpdated > 0
}

func bulkAddChanged(result subimporter.BulkUpsertResult) bool {
	return result.Created > 0 || result.Updated > 0 || result.TagsUpdated > 0 || result.ListsRemoved > 0 || result.ListsUpdated > 0
}
