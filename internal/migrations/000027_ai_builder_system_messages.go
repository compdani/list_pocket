package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		if existing, err := app.FindCollectionByNameOrId("ai_builder_system_messages"); err == nil && existing != nil {
			return nil
		}

		col := core.NewBaseCollection("ai_builder_system_messages")
		col.Fields.Add(
			&core.SelectField{
				Name:      "editor_mode",
				Required:  true,
				MaxSelect: 1,
				Values:    []string{"richtext", "html", "markdown", "plain", "visual", "grapes_mjml"},
			},
			&core.TextField{
				Name:     "prompt",
				Required: true,
			},
			&core.AutodateField{
				Name:     "created",
				OnCreate: true,
			},
			&core.AutodateField{
				Name:     "updated",
				OnCreate: true,
				OnUpdate: true,
			},
		)
		col.AddIndex("idx_ai_builder_system_messages_editor_mode", true, "editor_mode", "")

		if err := app.Save(col); err != nil {
			return err
		}

		defaultPrompts := map[string]string{
			"richtext":    "You generate email campaign drafts for a rich text editor. Return strict JSON only with keys: subject, preheader, contentType, body, notes. Keep body as valid HTML suitable for rich text editing.",
			"html":        "You generate email campaign drafts for an HTML editor. Return strict JSON only with keys: subject, preheader, contentType, body, notes. Keep body as clean HTML.",
			"markdown":    "You generate email campaign drafts for a markdown editor. Return strict JSON only with keys: subject, preheader, contentType, body, notes. Keep body in markdown format.",
			"plain":       "You generate email campaign drafts for a plain text editor. Return strict JSON only with keys: subject, preheader, contentType, body, notes. Keep body in plain text without HTML.",
			"visual":      "You generate email campaign drafts for a visual editor. Return strict JSON only with keys: subject, preheader, contentType, body, notes. For visual contentType, body must contain source JSON, not rendered HTML.",
			"grapes_mjml": "You generate email campaign drafts for a GrapesJS MJML editor. Return strict JSON only with keys: subject, preheader, contentType, body, notes. For grapes_mjml contentType, body must contain valid MJML source.",
		}

		for mode, prompt := range defaultPrompts {
			rec := core.NewRecord(col)
			rec.Set("editor_mode", mode)
			rec.Set("prompt", prompt)
			if err := app.Save(rec); err != nil {
				return err
			}
		}

		return nil
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("ai_builder_system_messages")
		if err != nil {
			return nil
		}

		return app.Delete(col)
	})
}
