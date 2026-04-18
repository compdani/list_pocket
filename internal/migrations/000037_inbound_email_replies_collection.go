package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		if existing, err := app.FindCollectionByNameOrId("inbound_email_replies"); err == nil && existing != nil {
			return nil
		}

		subscribers, err := app.FindCollectionByNameOrId("subscribers")
		if err != nil {
			return err
		}
		campaignLedger, err := app.FindCollectionByNameOrId("campaign_send_ledger")
		if err != nil {
			return err
		}

		collection := core.NewBaseCollection("inbound_email_replies")
		authRule := "@request.auth.id != ''"
		collection.ListRule = &authRule
		collection.ViewRule = &authRule
		collection.CreateRule = &authRule
		collection.UpdateRule = &authRule
		collection.DeleteRule = &authRule

		collection.Fields.Add(
			// Linkage to subscriber and outbound message (nullable if match could not be determined)
			&core.RelationField{
				Name:         "subscriber_id",
				Required:     false,
				CollectionId: subscribers.Id,
			},
			&core.RelationField{
				Name:         "linked_message_id",
				Required:     false,
				CollectionId: campaignLedger.Id,
			},
			// Message identification
			&core.EmailField{
				Name:     "from_address",
				Required: true,
			},
			&core.TextField{
				Name:     "subject",
				Required: true,
			},
			// RFC 5322 threading headers
			&core.TextField{
				Name:     "message_id",
				Required: true,
			},
			&core.TextField{
				Name:     "in_reply_to",
				Required: false,
			},
			&core.TextField{
				Name:     "references",
				Required: false,
			},
			&core.DateField{
				Name:     "received_at",
				Required: true,
			},
			// Content (snippet + full for preview + compliance)
			&core.TextField{
				Name:     "body_snippet",
				Required: true,
			},
			&core.BoolField{
				Name:     "has_attachments",
				Required: true,
			},
			// Match classification
			&core.SelectField{
				Name:     "match_score",
				Required: true,
				Values:   []string{"exact_messageID", "exact_email", "unmatched"},
			},
			// Audit trail
			&core.JSONField{
				Name:     "raw_headers",
				Required: false,
			},
			&core.JSONField{
				Name:     "raw_body",
				Required: false,
			},
			&core.DateField{
				Name:     "processed_at",
				Required: true,
			},
			// Idempotency key
			&core.TextField{
				Name:     "dedupe_key",
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
		collection.AddIndex("idx_inbound_email_subscriber_received", false, "subscriber_id, received_at", "")
		collection.AddIndex("idx_inbound_email_from_received", false, "from_address, received_at", "")
		collection.AddIndex("idx_inbound_email_dedup", true, "message_id, from_address, received_at", "")
		collection.AddIndex("idx_inbound_email_threading", false, "in_reply_to, message_id", "")

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("inbound_email_replies")
		if err != nil {
			return nil
		}
		return app.Delete(collection)
	})
}
