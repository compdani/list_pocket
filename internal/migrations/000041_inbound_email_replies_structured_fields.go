package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("inbound_email_replies")
		if err != nil {
			return nil
		}

		// Add structured content fields if not already present.
		if collection.Fields.GetByName("body_html") == nil {
			collection.Fields.Add(&core.TextField{
				Name:     "body_html",
				Required: false,
				Max:      largeBodyFieldMax,
			})
		}
		if collection.Fields.GetByName("body_text") == nil {
			collection.Fields.Add(&core.TextField{
				Name:     "body_text",
				Required: false,
				Max:      largeBodyFieldMax,
			})
		}
		if collection.Fields.GetByName("to_address") == nil {
			collection.Fields.Add(&core.TextField{
				Name:     "to_address",
				Required: false,
			})
		}
		if collection.Fields.GetByName("cc") == nil {
			collection.Fields.Add(&core.TextField{
				Name:     "cc",
				Required: false,
			})
		}
		if collection.Fields.GetByName("reply_to") == nil {
			collection.Fields.Add(&core.TextField{
				Name:     "reply_to",
				Required: false,
			})
		}
		if collection.Fields.GetByName("structured_headers") == nil {
			collection.Fields.Add(&core.JSONField{
				Name:     "structured_headers",
				Required: false,
			})
		}
		if collection.Fields.GetByName("spam_status") == nil {
			collection.Fields.Add(&core.SelectField{
				Name:     "spam_status",
				Required: false,
				Values:   []string{"suspected", "spam", "confirmed_spam"},
			})
		}
		if collection.Fields.GetByName("spam_score") == nil {
			collection.Fields.Add(&core.NumberField{
				Name:     "spam_score",
				Required: false,
			})
		}

		if err := app.Save(collection); err != nil {
			return err
		}

		// Migrate existing body content from raw_body JSON into the new dedicated fields.
		// json_extract is available in SQLite 3.38+; gracefully ignores rows without the key.
		if _, err := app.DB().NewQuery(`
UPDATE inbound_email_replies
SET
  body_html = COALESCE(json_extract(raw_body, '$.html'), ''),
  body_text = COALESCE(json_extract(raw_body, '$.text'), '')
WHERE (body_html IS NULL OR body_html = '')
  AND (body_text IS NULL OR body_text = '')
  AND raw_body IS NOT NULL
  AND raw_body != ''
`).Execute(); err != nil {
			// Non-fatal: migration still proceeds if JSON extraction is unavailable.
			app.Logger().Warn("inbound_email_replies body migration skipped", "error", err)
		}

		// Remove the raw blob fields.
		collection.Fields.RemoveByName("raw_headers")
		collection.Fields.RemoveByName("raw_body")

		if err := app.Save(collection); err != nil {
			return err
		}

		// Index for spam-status-based queries and cron cleanup.
		collection.AddIndex("idx_inbound_email_spam_status", false, "spam_status, received_at", "")
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("inbound_email_replies")
		if err != nil {
			return nil
		}

		for _, name := range []string{"body_html", "body_text", "to_address", "cc", "reply_to", "structured_headers", "spam_status", "spam_score"} {
			collection.Fields.RemoveByName(name)
		}

		// Restore raw blob fields.
		collection.Fields.Add(
			&core.JSONField{Name: "raw_headers", Required: false},
			&core.JSONField{Name: "raw_body", Required: false},
		)

		return app.Save(collection)
	})
}
