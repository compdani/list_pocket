package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		if existing, err := app.FindCollectionByNameOrId("pictures"); err == nil && existing != nil {
			return nil
		}

		pictures := core.NewBaseCollection("pictures")
		publicRule := "1=1"
		authRule := "@request.auth.id != ''"

		pictures.ListRule = &publicRule
		pictures.ViewRule = &publicRule
		pictures.CreateRule = &authRule
		pictures.UpdateRule = &authRule
		pictures.DeleteRule = &authRule

		pictures.Fields.Add(
			&core.FileField{
				Name:      "file",
				Required:  true,
				MaxSelect: 1,
				MaxSize:   20 * 1024 * 1024,
				MimeTypes: []string{
					"image/jpeg",
					"image/png",
					"image/gif",
					"image/svg+xml",
					"image/webp",
					"image/avif",
				},
				Thumbs: []string{"300x0"},
			},
			&core.TextField{
				Name:     "original_name",
				Required: false,
			},
			&core.TextField{
				Name:     "content_type",
				Required: false,
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

		pictures.AddIndex("idx_pictures_original_name", false, "original_name", "")

		return app.Save(pictures)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("pictures")
		if err != nil {
			return nil
		}

		return app.Delete(collection)
	})
}
