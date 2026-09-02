package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		add := func(name, indexName, fields, filter string, unique bool) error {
			col, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			col.AddIndex(indexName, unique, fields, filter)
			return app.Save(col)
		}

		if err := add("campaign_views", "idx_views_created", "created", "", false); err != nil {
			return err
		}
		if err := add("link_clicks", "idx_clicks_created", "created", "", false); err != nil {
			return err
		}
		if err := add("campaign_views", "idx_views_sub_camp", "subscriber_id, campaign_id", "", false); err != nil {
			return err
		}
		if err := add("link_clicks", "idx_clicks_sub_camp", "subscriber_id, campaign_id", "", false); err != nil {
			return err
		}
		if err := add("subscriber_lists", "idx_sub_lists_list_status", "list_id, status", "", false); err != nil {
			return err
		}
		return add("subscribers", "idx_subs_phone", "phone", "phone != ''", false)
	}, func(app core.App) error {
		drop := func(name, indexName string) error {
			col, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return nil
			}
			col.RemoveIndex(indexName)
			return app.Save(col)
		}
		_ = drop("campaign_views", "idx_views_created")
		_ = drop("link_clicks", "idx_clicks_created")
		_ = drop("campaign_views", "idx_views_sub_camp")
		_ = drop("link_clicks", "idx_clicks_sub_camp")
		_ = drop("subscriber_lists", "idx_sub_lists_list_status")
		_ = drop("subscribers", "idx_subs_phone")
		return nil
	})
}
