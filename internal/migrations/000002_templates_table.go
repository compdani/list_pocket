package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection := core.NewBaseCollection("templates")

		// Add fields
		collection.Fields.Add(
			&core.TextField{
				Name:     "name",
				Required: true,
			},
			&core.SelectField{
				Name:     "type",
				Required: true,
				Values:   []string{"campaign", "tx"},
			},
			&core.TextField{
				Name:     "subject",
				Required: false,
			},
			&core.TextField{
				Name:     "body",
				Required: false,
			},
			&core.TextField{
				Name:     "body_source",
				Required: false,
			},
			&core.BoolField{
				Name: "is_default",
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

		// Add unique index for is_default when true
		collection.AddIndex("idx_templates_is_default_true", true, "is_default", "is_default = TRUE")

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("templates")
		if err != nil {
			return err
		}

		return app.Delete(collection)
	})
}
