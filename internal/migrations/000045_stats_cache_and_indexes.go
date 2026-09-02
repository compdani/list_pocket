package migrations

import (
	"strings"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func collectionHasIndex(col *core.Collection, name string) bool {
	needle := strings.ToLower(name)
	for _, idx := range col.Indexes {
		if strings.Contains(strings.ToLower(idx), needle) {
			return true
		}
	}
	return false
}

func init() {
	m.Register(func(app core.App) error {
		drop := func(name, indexName string) {
			col, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return
			}
			if !collectionHasIndex(col, indexName) {
				return
			}
			col.RemoveIndex(indexName)
			_ = app.Save(col)
		}
		// Wrong-order composites from an earlier 000044.
		drop("campaign_views", "idx_views_sub_camp")
		drop("link_clicks", "idx_clicks_sub_camp")

		add := func(name, indexName, fields string) error {
			col, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			if collectionHasIndex(col, indexName) {
				return nil
			}
			col.AddIndex(indexName, false, fields, "")
			return app.Save(col)
		}

		if err := add("campaign_views", "idx_views_camp_sub", "campaign_id, subscriber_id"); err != nil {
			return err
		}
		if err := add("link_clicks", "idx_clicks_camp_sub", "campaign_id, subscriber_id"); err != nil {
			return err
		}
		if err := add("campaign_views", "idx_views_camp_created", "campaign_id, created"); err != nil {
			return err
		}
		if err := add("link_clicks", "idx_clicks_camp_created", "campaign_id, created"); err != nil {
			return err
		}
		if err := add("campaign_send_ledger", "idx_campaign_ledger_status_created", "campaign_id, status, created"); err != nil {
			return err
		}

		if existing, err := app.FindCollectionByNameOrId("listpocket_stats_cache"); err == nil && existing != nil {
			return nil
		}

		col := core.NewBaseCollection("listpocket_stats_cache")
		col.Fields.Add(
			&core.TextField{
				Name:     "cache_key",
				Required: true,
			},
			&core.TextField{
				Name: "value",
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
		col.AddIndex("idx_stats_cache_key", true, "cache_key", "")
		return app.Save(col)
	}, func(app core.App) error {
		drop := func(name, indexName string) {
			col, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return
			}
			col.RemoveIndex(indexName)
			_ = app.Save(col)
		}
		drop("campaign_views", "idx_views_camp_sub")
		drop("link_clicks", "idx_clicks_camp_sub")
		drop("campaign_views", "idx_views_camp_created")
		drop("link_clicks", "idx_clicks_camp_created")
		drop("campaign_send_ledger", "idx_campaign_ledger_status_created")

		collection, err := app.FindCollectionByNameOrId("listpocket_stats_cache")
		if err != nil {
			return nil
		}
		return app.Delete(collection)
	})
}
