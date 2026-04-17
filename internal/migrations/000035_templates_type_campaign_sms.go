package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Extends the `templates.type` select values to include `campaign_sms` so
// plain-text SMS templates can be saved alongside the existing email template
// types.
func init() {
	m.Register(func(app core.App) error {
		templates, err := app.FindCollectionByNameOrId("templates")
		if err != nil {
			return err
		}

		templateType := templates.Fields.GetByName("type")
		if templateType != nil {
			if selectField, ok := templateType.(*core.SelectField); ok {
				selectField.Values = []string{"campaign", "campaign_visual", "campaign_grapes_mjml", "campaign_sms", "tx"}
			}
		}
		return app.Save(templates)
	}, func(app core.App) error {
		templates, err := app.FindCollectionByNameOrId("templates")
		if err != nil {
			return err
		}

		templateType := templates.Fields.GetByName("type")
		if templateType != nil {
			if selectField, ok := templateType.(*core.SelectField); ok {
				selectField.Values = []string{"campaign", "campaign_visual", "campaign_grapes_mjml", "tx"}
			}
		}
		return app.Save(templates)
	})
}
