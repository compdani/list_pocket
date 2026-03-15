package migrations

import (
	"database/sql"
	"errors"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collectionNames := []string{
			"listmonk_settings",
			"templates",
			"subscribers",
			"lists",
			"subscriber_lists",
			"campaigns",
			"campaign_lists",
			"campaign_media",
			"media",
			"links",
			"campaign_views",
			"link_clicks",
			"bounces",
		}

		for _, name := range collectionNames {
			collection, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					continue
				}
				return err
			}

			changed := false

			if collection.Fields.GetByName("created") == nil {
				collection.Fields.Add(&core.AutodateField{
					Name:     "created",
					OnCreate: true,
				})
				changed = true
			}

			if collection.Fields.GetByName("updated") == nil {
				collection.Fields.Add(&core.AutodateField{
					Name:     "updated",
					OnCreate: true,
					OnUpdate: true,
				})
				changed = true
			}

			if changed {
				if err := app.Save(collection); err != nil {
					return err
				}
			}
		}

		return nil
	}, func(app core.App) error {
		return nil
	})
}
