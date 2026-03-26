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

		ledger := core.NewBaseCollection("campaign_send_ledger")
		ledger.Fields.Add(
			&core.RelationField{
				Name:         "campaign_id",
				Required:     true,
				CollectionId: campaigns.Id,
				CascadeDelete: true,
			},
			&core.RelationField{
				Name:         "subscriber_id",
				Required:     true,
				CollectionId: subscribers.Id,
				CascadeDelete: true,
			},
			&core.SelectField{
				Name:     "status",
				Required: true,
				Values:   []string{"pending", "inflight", "sent", "skipped"},
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
		ledger.AddIndex("idx_campaign_ledger_unique", true, "campaign_id, subscriber_id", "")
		ledger.AddIndex("idx_campaign_ledger_campaign_status", false, "campaign_id, status", "")

		return app.Save(ledger)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("campaign_send_ledger")
		if err != nil {
			return nil
		}
		return app.Delete(collection)
	})
}
