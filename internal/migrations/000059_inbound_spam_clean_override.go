package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("inbound_spam_rules")
		if err != nil {
			return err
		}
		if collection.Fields.GetByName("clean_override") != nil {
			return nil
		}
		// Exact-sender clean feedback. While set, classification ignores this
		// sender's stored spam level plus domain and keyword rules. A later
		// explicit spam mark clears it.
		collection.Fields.Add(&core.BoolField{
			Name:     "clean_override",
			Required: false,
		})
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("inbound_spam_rules")
		if err != nil {
			return nil
		}
		collection.Fields.RemoveByName("clean_override")
		return app.Save(collection)
	})
}
