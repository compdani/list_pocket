package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		campaigns, err := app.FindCollectionByNameOrId("campaigns")
		if err != nil {
			return err
		}

		campaignContentType := campaigns.Fields.GetByName("content_type")
		if campaignContentType != nil {
			if selectField, ok := campaignContentType.(*core.SelectField); ok {
				selectField.Values = []string{"richtext", "html", "markdown", "plain", "visual", "grapes_mjml"}
			}
		}
		if err := app.Save(campaigns); err != nil {
			return err
		}

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
	}, func(app core.App) error {
		campaigns, err := app.FindCollectionByNameOrId("campaigns")
		if err != nil {
			return err
		}

		campaignContentType := campaigns.Fields.GetByName("content_type")
		if campaignContentType != nil {
			if selectField, ok := campaignContentType.(*core.SelectField); ok {
				selectField.Values = []string{"richtext", "html", "markdown", "plain", "visual"}
			}
		}
		if err := app.Save(campaigns); err != nil {
			return err
		}

		templates, err := app.FindCollectionByNameOrId("templates")
		if err != nil {
			return err
		}

		templateType := templates.Fields.GetByName("type")
		if templateType != nil {
			if selectField, ok := templateType.(*core.SelectField); ok {
				selectField.Values = []string{"campaign", "campaign_visual", "tx"}
			}
		}
		return app.Save(templates)
	})
}
