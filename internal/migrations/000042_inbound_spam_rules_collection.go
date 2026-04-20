package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		if existing, err := app.FindCollectionByNameOrId("inbound_spam_rules"); err == nil && existing != nil {
			return nil
		}

		collection := core.NewBaseCollection("inbound_spam_rules")
		authRule := "@request.auth.id != ''"
		collection.ListRule = &authRule
		collection.ViewRule = &authRule
		collection.CreateRule = &authRule
		collection.UpdateRule = &authRule
		collection.DeleteRule = &authRule

		collection.Fields.Add(
			// Rule classification: sender address, sender domain, or keyword.
			&core.SelectField{
				Name:     "type",
				Required: true,
				Values:   []string{"sender", "domain", "keyword"},
			},
			// The matched value: email address, domain string, or keyword.
			&core.TextField{
				Name:     "value",
				Required: true,
			},
			// Relative relevance weight for keyword scoring (default 1.0).
			&core.NumberField{
				Name:     "weight",
				Required: false,
			},
			// How many times user-driven spam marking has reinforced this rule.
			&core.NumberField{
				Name:     "hit_count",
				Required: false,
				OnlyInt:  true,
			},
			// Highest spam level implied by this rule.
			&core.SelectField{
				Name:     "spam_level",
				Required: false,
				Values:   []string{"suspected", "spam", "confirmed_spam"},
			},
			&core.BoolField{
				Name:     "is_active",
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

		// Unique constraint ensures we upsert rather than duplicate rules.
		collection.AddIndex("idx_spam_rules_type_value", true, "type, value", "")

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("inbound_spam_rules")
		if err != nil {
			return nil
		}
		return app.Delete(collection)
	})
}
