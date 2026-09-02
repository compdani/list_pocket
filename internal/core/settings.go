package core

import (
	"encoding/json"
	"github.com/compdani/list_pocket/internal/apperr"

	"github.com/compdani/list_pocket/models"
	"github.com/jmoiron/sqlx/types"
)

// GetSettings returns settings from the DB.
func (c *Core) GetSettings() (models.Settings, error) {
	var (
		b   types.JSONText
		out models.Settings
	)

	if c.getSettings != nil {
		var err error
		b, err = c.getSettings()
		if err != nil {
			return out, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching",
				"name", "{globals.terms.settings}", "error", dbErr(err)))
		}
	} else {
		return out, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching",
			"name", "{globals.terms.settings}", "error", "settings not configured"))
	}

	// Unmarshal the settings and filter out sensitive fields.
	if err := json.Unmarshal([]byte(b), &out); err != nil {
		return out, apperr.Internal(c.i18n.Ts("settings.errorEncoding", "error", err.Error()))
	}

	return out, nil
}

// UpdateSettings updates settings.
func (c *Core) UpdateSettings(s models.Settings) error {
	// Marshal settings.
	b, err := json.Marshal(s)
	if err != nil {
		return apperr.Internal(c.i18n.Ts("settings.errorEncoding", "error", err.Error()))
	}

	if c.getSettings != nil {
		if existing, err := c.getSettings(); err == nil && len(existing) > 0 {
			var oldMap, newMap map[string]json.RawMessage
			if json.Unmarshal(existing, &oldMap) == nil && json.Unmarshal(b, &newMap) == nil {
				for k, v := range oldMap {
					if _, ok := newMap[k]; !ok {
						newMap[k] = v
					}
				}
				if merged, err := json.Marshal(newMap); err == nil {
					b = merged
				}
			}
		}
	}

	if c.setSettings != nil {
		if err := c.setSettings(b); err != nil {
			return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.settings}", "error", dbErr(err)))
		}
	} else {
		return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.settings}", "error", "settings not configured"))
	}

	return nil
}

// UpdateSettingsByKey updates a single setting by key.
func (c *Core) UpdateSettingsByKey(key string, value json.RawMessage) error {
	if c.setSettingsByKey != nil {
		if err := c.setSettingsByKey(key, value); err != nil {
			return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.settings}", "error", dbErr(err)))
		}
	} else {
		return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.settings}", "error", "settings not configured"))
	}

	return nil
}
