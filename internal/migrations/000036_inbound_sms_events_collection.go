package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		if existing, err := app.FindCollectionByNameOrId("inbound_sms_events"); err == nil && existing != nil {
			return nil
		}

		subscribers, err := app.FindCollectionByNameOrId("subscribers")
		if err != nil {
			return err
		}
		lists, err := app.FindCollectionByNameOrId("lists")
		if err != nil {
			return err
		}

		collection := core.NewBaseCollection("inbound_sms_events")
		authRule := "@request.auth.id != ''"
		collection.ListRule = &authRule
		collection.ViewRule = &authRule
		collection.CreateRule = &authRule
		collection.UpdateRule = &authRule
		collection.DeleteRule = &authRule

		collection.Fields.Add(
			// Linkage to subscriber and list (nullable if match could not be determined)
			&core.RelationField{
				Name:         "subscriber_id",
				Required:     false,
				CollectionId: subscribers.Id,
			},
			&core.RelationField{
				Name:         "list_id",
				Required:     false,
				CollectionId: lists.Id,
			},
			// Phone number (normalized digits for matching)
			&core.TextField{
				Name:     "phone_number",
				Required: true,
			},
			// Provider metadata
			&core.TextField{
				Name:     "provider_id",
				Required: true,
			},
			&core.TextField{
				Name:     "provider_msg_id",
				Required: true,
			},
			&core.TextField{
				Name:     "from_number",
				Required: true,
			},
			// Message content
			&core.TextField{
				Name:     "message_body",
				Required: true,
			},
			&core.DateField{
				Name:     "received_at",
				Required: true,
			},
			// Classification
			&core.BoolField{
				Name:     "is_stop_keyword",
				Required: true,
			},
			&core.SelectField{
				Name:     "match_score",
				Required: true,
				Values:   []string{"exact", "fallback_10digit", "unmatched"},
			},
			// Audit trail
			&core.JSONField{
				Name:     "raw_payload",
				Required: false,
			},
			&core.DateField{
				Name:     "processed_at",
				Required: true,
			},
			// Idempotency key
			&core.TextField{
				Name:     "sender_hash",
				Required: true,
			},
			// Auto timestamps
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

		// Indexes for efficient querying and idempotency
		collection.AddIndex("idx_inbound_sms_subscriber_received", false, "subscriber_id, received_at", "")
		collection.AddIndex("idx_inbound_sms_phone_received", false, "phone_number, received_at", "")
		collection.AddIndex("idx_inbound_sms_dedup", true, "provider_id, provider_msg_id, received_at", "")

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("inbound_sms_events")
		if err != nil {
			return nil
		}
		return app.Delete(collection)
	})
}
