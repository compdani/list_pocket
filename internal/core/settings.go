package core

import (
	"encoding/json"
	"net/http"

	"github.com/jmoiron/sqlx/types"
	"github.com/compdani/list_pocket/models"
	"github.com/labstack/echo/v4"
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
			return out, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorFetching",
					"name", "{globals.terms.settings}", "error", pqErrMsg(err)))
		}
	} else {
		return out, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching",
				"name", "{globals.terms.settings}", "error", "settings not configured"))
	}

	// Unmarshal the settings and filter out sensitive fields.
	if err := json.Unmarshal([]byte(b), &out); err != nil {
		return out, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("settings.errorEncoding", "error", err.Error()))
	}

	return out, nil
}

// UpdateSettings updates settings.
func (c *Core) UpdateSettings(s models.Settings) error {
	// Marshal settings.
	b, err := json.Marshal(s)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("settings.errorEncoding", "error", err.Error()))
	}

	// Update the settings in the DB.
	if c.setSettings != nil {
		if err := c.setSettings(b); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.settings}", "error", pqErrMsg(err)))
		}
	} else {
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.settings}", "error", "settings not configured"))
	}

	return nil
}

// UpdateSettingsByKey updates a single setting by key.
func (c *Core) UpdateSettingsByKey(key string, value json.RawMessage) error {
	if c.setSettingsByKey != nil {
		if err := c.setSettingsByKey(key, value); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.settings}", "error", pqErrMsg(err)))
		}
	} else {
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.settings}", "error", "settings not configured"))
	}

	return nil
}
