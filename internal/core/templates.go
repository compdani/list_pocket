package core

import (
	"net/http"
	"strconv"

	"github.com/compdani/list_pocket/models"
	"github.com/labstack/echo/v4"
	pbcore "github.com/pocketbase/pocketbase/core"
	null "gopkg.in/volatiletech/null.v6"
)

// GetTemplates retrieves all templates.
func (c *Core) GetTemplates(status string, noBody bool) ([]models.Template, error) {
	if c.isSQLite() {
		bodySel := "body"
		if noBody {
			bodySel = "'' AS body"
		}
		out := []models.Template{}
		if err := c.db.Select(&out, `
			SELECT rowid AS id, id AS record_id, created AS created_at, updated AS updated_at,
				name, subject, type, `+bodySel+`, body_source, is_default
			FROM templates
			ORDER BY created ASC`); err != nil {
			return nil, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.templates}", "error", pqErrMsg(err)))
		}
		return out, nil
	}

	out := []models.Template{}
	if err := c.q.GetTemplates.Select(&out, 0, noBody, status); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.templates}", "error", pqErrMsg(err)))
	}

	return out, nil
}

// GetTemplate retrieves a given template.
func (c *Core) GetTemplate(recordID string, noBody bool) (models.Template, error) {
	if c.isSQLite() {
		bodySel := "body"
		if noBody {
			bodySel = "'' AS body"
		}
		var out models.Template
		if err := c.db.Get(&out, `
			SELECT rowid AS id, id AS record_id, created AS created_at, updated AS updated_at,
				name, subject, type, `+bodySel+`, body_source, is_default
			FROM templates
			WHERE id = ?
			LIMIT 1`, recordID); err != nil {
			return models.Template{}, echo.NewHTTPError(http.StatusBadRequest,
				c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.template}"))
		}
		return out, nil
	}

	var out []models.Template
	if err := c.q.GetTemplates.Select(&out, recordID, noBody, ""); err != nil {
		return models.Template{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.templates}", "error", pqErrMsg(err)))
	}

	if len(out) == 0 {
		return models.Template{}, echo.NewHTTPError(http.StatusBadRequest,
			c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.template}"))
	}

	return out[0], nil
}

// CreateTemplate creates a new template.
func (c *Core) CreateTemplate(name, typ, subject string, body []byte, bodySource null.String) (models.Template, error) {
	if c.isSQLite() {
		pb := c.db.PocketBase()
		col, err := pb.FindCollectionByNameOrId("templates")
		if err != nil {
			return models.Template{}, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.template}", "error", pqErrMsg(err)))
		}
		rec := pbcore.NewRecord(col)
		rec.Set("name", name)
		rec.Set("type", typ)
		rec.Set("subject", subject)
		rec.Set("body", string(body))
		rec.Set("body_source", bodySource.String)
		rec.Set("is_default", false)
		if err := pb.Save(rec); err != nil {
			return models.Template{}, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.template}", "error", pqErrMsg(err)))
		}
		return c.GetTemplate(rec.Id, false)
	}

	var newID int
	if err := c.q.CreateTemplate.Get(&newID, name, typ, subject, body, bodySource); err != nil {
		return models.Template{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.template}", "error", pqErrMsg(err)))
	}

	return c.GetTemplate(strconv.Itoa(newID), false)
}

// UpdateTemplate updates a given template.
func (c *Core) UpdateTemplate(recordID string, name, subject string, body []byte, bodySource null.String) (models.Template, error) {
	if c.isSQLite() {
		pb := c.db.PocketBase()
		rec, err := pb.FindRecordById("templates", recordID)
		if err != nil {
			return models.Template{}, echo.NewHTTPError(http.StatusBadRequest,
				c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.template}"))
		}
		rec.Set("name", name)
		rec.Set("subject", subject)
		rec.Set("body", string(body))
		rec.Set("body_source", bodySource.String)
		if err := pb.Save(rec); err != nil {
			return models.Template{}, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.template}", "error", pqErrMsg(err)))
		}
		return c.GetTemplate(recordID, false)
	}

	res, err := c.q.UpdateTemplate.Exec(recordID, name, subject, body, bodySource)
	if err != nil {
		return models.Template{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.template}", "error", pqErrMsg(err)))
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return models.Template{}, echo.NewHTTPError(http.StatusBadRequest,
			c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.template}"))
	}

	return c.GetTemplate(recordID, false)
}

// SetDefaultTemplate sets a template as default.
func (c *Core) SetDefaultTemplate(recordID string) error {
	if c.isSQLite() {
		if _, err := c.db.Exec(`UPDATE templates SET is_default = 0`); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.template}", "error", pqErrMsg(err)))
		}
		if _, err := c.db.Exec(`UPDATE templates SET is_default = 1 WHERE id = ?`, recordID); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.template}", "error", pqErrMsg(err)))
		}
		return nil
	}

	if _, err := c.q.SetDefaultTemplate.Exec(recordID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.template}", "error", pqErrMsg(err)))
	}

	return nil
}

// DeleteTemplate deletes a given template.
func (c *Core) DeleteTemplate(recordID string) error {
	if c.isSQLite() {
		var isDefault bool
		if err := c.db.Get(&isDefault, `SELECT is_default FROM templates WHERE id = ? LIMIT 1`, recordID); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.template}"))
		}
		if isDefault {
			return echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("templates.cantDeleteDefault"))
		}
		if _, err := c.db.Exec(`DELETE FROM templates WHERE id = ?`, recordID); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.template}", "error", pqErrMsg(err)))
		}
		return nil
	}

	var delID int
	if err := c.q.DeleteTemplate.Get(&delID, recordID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.template}", "error", pqErrMsg(err)))
	}
	if delID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("templates.cantDeleteDefault"))
	}

	return nil
}
