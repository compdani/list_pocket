package main

import (
	"encoding/json"
	"strings"

	pbcore "ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/compdani/list_pocket/internal/apperr"
	"github.com/compdani/list_pocket/internal/auth"
	"github.com/compdani/list_pocket/internal/utils"
)

// PatchSubscriber changes profile fields without touching list membership or
// consent and without invoking the opt-in notification hook.
func (a *App) PatchSubscriber(re *pbcore.RequestEvent) error {
	id, err := a.resolveSubscriberRouteID(re)
	if err != nil {
		return err
	}
	if err := a.hasSubPerm(auth.GetUserRE(re), []int{id}); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := bindJSON(re, &raw); err != nil {
		return err
	}
	if len(raw) == 0 {
		return apperr.BadRequest("provide at least one profile field")
	}
	patch := map[string]any{}
	for key, value := range raw {
		if string(value) == "null" {
			return apperr.BadRequest("profile fields cannot be null")
		}
		switch key {
		case "email", "phone", "first_name", "last_name":
			var text string
			if err := json.Unmarshal(value, &text); err != nil {
				return apperr.BadRequest("profile fields must be strings")
			}
			text = strings.TrimSpace(text)
			if len(text) > stdInputMaxLen {
				return apperr.BadRequest("profile field is too long")
			}
			if key == "email" {
				text, err = a.importer.SanitizeEmail(text)
				if err != nil {
					return apperr.BadRequest(err.Error())
				}
			}
			if key == "phone" {
				text = utils.NormalizePhone(text)
				if len(text) > 64 {
					return apperr.BadRequest("phone is too long")
				}
			}
			patch[key] = text
		case "attribs":
			var attrs map[string]any
			if err := json.Unmarshal(value, &attrs); err != nil || attrs == nil {
				return apperr.BadRequest("attribs must be an object")
			}
			patch[key] = attrs
		default:
			return apperr.BadRequest("unsupported profile field: " + key)
		}
	}
	out, err := a.core.PatchSubscriberProfile(pathParam(re, "id"), patch)
	if err != nil {
		return err
	}
	return okJSON(re, out)
}
