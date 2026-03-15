package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// Get existing collection references
		campaigns, err := app.FindCollectionByNameOrId("campaigns")
		if err != nil {
			return err
		}

		subscribers, err := app.FindCollectionByNameOrId("subscribers")
		if err != nil {
			return err
		}

		links, err := app.FindCollectionByNameOrId("links")
		if err != nil {
			return err
		}

		// Create campaign_views collection
		campaignViews := core.NewBaseCollection("campaign_views")
		campaignViews.Fields.Add(
			&core.RelationField{
				Name:         "campaign_id",
				Required:     true,
				CollectionId: campaigns.Id,
			},
			&core.RelationField{
				Name:         "subscriber_id",
				Required:     false,
				CollectionId: subscribers.Id,
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
		campaignViews.AddIndex("idx_views_camp_id", false, "campaign_id", "")
		campaignViews.AddIndex("idx_views_subscriber_id", false, "subscriber_id", "")

		if err := app.Save(campaignViews); err != nil {
			return err
		}

		// Create link_clicks collection
		linkClicks := core.NewBaseCollection("link_clicks")
		linkClicks.Fields.Add(
			&core.RelationField{
				Name:         "campaign_id",
				Required:     false,
				CollectionId: campaigns.Id,
			},
			&core.RelationField{
				Name:         "link_id",
				Required:     true,
				CollectionId: links.Id,
			},
			&core.RelationField{
				Name:         "subscriber_id",
				Required:     false,
				CollectionId: subscribers.Id,
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
		linkClicks.AddIndex("idx_clicks_camp_id", false, "campaign_id", "")
		linkClicks.AddIndex("idx_clicks_link_id", false, "link_id", "")
		linkClicks.AddIndex("idx_clicks_sub_id", false, "subscriber_id", "")

		if err := app.Save(linkClicks); err != nil {
			return err
		}

		// Create bounces collection
		bounces := core.NewBaseCollection("bounces")
		bounces.Fields.Add(
			&core.RelationField{
				Name:         "subscriber_id",
				Required:     true,
				CollectionId: subscribers.Id,
			},
			&core.RelationField{
				Name:         "campaign_id",
				Required:     false,
				CollectionId: campaigns.Id,
			},
			&core.SelectField{
				Name:     "type",
				Required: true,
				Values:   []string{"hard", "soft", "complaint"},
			},
			&core.TextField{
				Name:     "source",
				Required: false,
			},
			&core.JSONField{
				Name:     "meta",
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
		bounces.AddIndex("idx_bounces_sub_id", false, "subscriber_id", "")
		bounces.AddIndex("idx_bounces_camp_id", false, "campaign_id", "")
		bounces.AddIndex("idx_bounces_source", false, "source", "")

		return app.Save(bounces)
	}, func(app core.App) error {
		collections := []string{"bounces", "link_clicks", "campaign_views"}
		for _, name := range collections {
			collection, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				continue
			}
			if err := app.Delete(collection); err != nil {
				return err
			}
		}
		return nil
	})
}
