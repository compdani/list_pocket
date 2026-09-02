package core

import (
	"database/sql"
	"github.com/compdani/list_pocket/internal/apperr"
	"strings"

	"github.com/compdani/list_pocket/models"
	pbcore "github.com/pocketbase/pocketbase/core"
	null "gopkg.in/volatiletech/null.v6"
)

func (c *Core) sqliteTemplateLegacyID(recordID string) int {
	recordID = strings.TrimSpace(recordID)
	if recordID == "" {
		return 0
	}

	var id int
	if err := c.db.Get(&id, `SELECT rowid FROM templates WHERE id = ? LIMIT 1`, recordID); err != nil {
		if err != sql.ErrNoRows {
			c.log.Printf("error resolving template rowid for %q: %v", recordID, err)
		}
		return 0
	}

	return id
}

func (c *Core) sqliteTemplateFromRecord(rec *pbcore.Record) models.Template {
	bodySource := null.String{}
	if value := strings.TrimSpace(rec.GetString("body_source")); value != "" {
		bodySource = null.StringFrom(value)
	}

	return models.Template{
		Base: models.Base{
			ID:        c.sqliteTemplateLegacyID(rec.Id),
			RecordID:  rec.Id,
			CreatedAt: parseNullTime(rec.GetString("created")),
			UpdatedAt: parseNullTime(rec.GetString("updated")),
		},
		Name:       rec.GetString("name"),
		Subject:    rec.GetString("subject"),
		Type:       rec.GetString("type"),
		Body:       rec.GetString("body"),
		BodySource: bodySource,
		IsDefault:  rec.GetBool("is_default"),
	}
}

// GetTemplates retrieves all templates.
func (c *Core) GetTemplates(status string, noBody bool) ([]models.Template, error) {
	pb := c.db.PocketBase()
	if pb == nil {
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.templates}", "error", "pocketbase unavailable"))
	}

	recs, err := pb.FindRecordsByFilter("templates", "", "created", 0, 0)
	if err != nil {
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.templates}", "error", err.Error()))
	}

	out := make([]models.Template, 0, len(recs))
	for _, rec := range recs {
		tpl := c.sqliteTemplateFromRecord(rec)
		if noBody {
			tpl.Body = ""
		}
		out = append(out, tpl)
	}

	return out, nil
}

// GetTemplate retrieves a given template.
func (c *Core) GetTemplate(recordID string, noBody bool) (models.Template, error) {
	pb := c.db.PocketBase()
	if pb == nil {
		return models.Template{}, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.templates}", "error", "pocketbase unavailable"))
	}

	rec, err := pb.FindRecordById("templates", recordID)
	if err != nil {
		return models.Template{}, apperr.BadRequest(c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.template}"))
	}

	out := c.sqliteTemplateFromRecord(rec)
	if noBody {
		out.Body = ""
	}
	return out, nil
}

// CreateTemplate creates a new template.
func (c *Core) CreateTemplate(name, typ, subject string, body []byte, bodySource null.String) (models.Template, error) {
	pb := c.db.PocketBase()
	col, err := pb.FindCollectionByNameOrId("templates")
	if err != nil {
		return models.Template{}, apperr.Internal(c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.template}", "error", dbErr(err)))
	}
	rec := pbcore.NewRecord(col)
	rec.Set("name", name)
	rec.Set("type", typ)
	rec.Set("subject", subject)
	rec.Set("body", string(body))
	rec.Set("body_source", bodySource.String)
	rec.Set("is_default", false)
	if err := pb.Save(rec); err != nil {
		return models.Template{}, apperr.Internal(c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.template}", "error", dbErr(err)))
	}
	return c.sqliteTemplateFromRecord(rec), nil
}

// UpdateTemplate updates a given template.
func (c *Core) UpdateTemplate(recordID string, name, subject string, body []byte, bodySource null.String) (models.Template, error) {
	pb := c.db.PocketBase()
	rec, err := pb.FindRecordById("templates", recordID)
	if err != nil {
		return models.Template{}, apperr.BadRequest(c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.template}"))
	}
	rec.Set("name", name)
	rec.Set("subject", subject)
	rec.Set("body", string(body))
	rec.Set("body_source", bodySource.String)
	if err := pb.Save(rec); err != nil {
		return models.Template{}, apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.template}", "error", dbErr(err)))
	}
	return c.sqliteTemplateFromRecord(rec), nil
}

// SetDefaultTemplate sets a template as default. Defaults are scoped per
// template type so that each channel (e.g. `campaign` for email HTML and
// `campaign_sms` for SMS) can have its own default independently.
func (c *Core) SetDefaultTemplate(recordID string) error {
	var tplType string
	if err := c.db.Get(&tplType, `SELECT type FROM templates WHERE id = ? LIMIT 1`, recordID); err != nil {
		return apperr.BadRequest(c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.template}"))
	}
	if _, err := c.db.Exec(`UPDATE templates SET is_default = 0 WHERE type = ?`, tplType); err != nil {
		return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.template}", "error", dbErr(err)))
	}
	if _, err := c.db.Exec(`UPDATE templates SET is_default = 1 WHERE id = ?`, recordID); err != nil {
		return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.template}", "error", dbErr(err)))
	}
	return nil
}

// DeleteTemplate deletes a given template.
func (c *Core) DeleteTemplate(recordID string) error {
	var isDefault bool
	if err := c.db.Get(&isDefault, `SELECT is_default FROM templates WHERE id = ? LIMIT 1`, recordID); err != nil {
		return apperr.BadRequest(c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.template}"))
	}
	if isDefault {
		return apperr.BadRequest(c.i18n.T("templates.cantDeleteDefault"))
	}
	if _, err := c.db.Exec(`DELETE FROM templates WHERE id = ?`, recordID); err != nil {
		return apperr.Internal(c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.template}", "error", dbErr(err)))
	}
	return nil
}
