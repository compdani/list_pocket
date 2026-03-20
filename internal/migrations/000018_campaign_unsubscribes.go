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

		subscribers, err := app.FindCollectionByNameOrId("subscribers")
		if err != nil {
			return err
		}

		campaignUnsubscribes := core.NewBaseCollection("campaign_unsubscribes")
		campaignUnsubscribes.Fields.Add(
			&core.RelationField{
				Name:         "campaign_id",
				Required:     true,
				CollectionId: campaigns.Id,
			},
			&core.RelationField{
				Name:         "subscriber_id",
				Required:     true,
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
		campaignUnsubscribes.AddIndex("idx_campaign_unsubs_campaign_id", false, "campaign_id", "")
		campaignUnsubscribes.AddIndex("idx_campaign_unsubs_subscriber_id", false, "subscriber_id", "")
		campaignUnsubscribes.AddIndex("idx_campaign_unsubs_unique", true, "campaign_id, subscriber_id", "")

		return app.Save(campaignUnsubscribes)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("campaign_unsubscribes")
		if err != nil {
			return nil
		}
		return app.Delete(collection)
	})
}
