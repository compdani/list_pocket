package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		if existing, err := app.FindCollectionByNameOrId("campaign_tags"); err == nil && existing != nil {
			return nil
		}

		collection := core.NewBaseCollection("campaign_tags")
		authRule := "@request.auth.id != ''"
		collection.ListRule = &authRule
		collection.ViewRule = &authRule
		collection.CreateRule = &authRule
		collection.UpdateRule = &authRule
		collection.DeleteRule = &authRule

		collection.Fields.Add(
			&core.TextField{
				Name:     "tag",
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

		collection.AddIndex("idx_campaign_tags_tag_unique", true, "tag", "")

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("campaign_tags")
		if err != nil {
			return nil
		}
		return app.Delete(collection)
	})
}
