package migrations

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/compdani/list_pocket/models"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("subscribers")
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		}

		changed := false
		if collection.Fields.GetByName("first_name") == nil {
			collection.Fields.Add(&core.TextField{Name: "first_name"})
			changed = true
		}
		if collection.Fields.GetByName("last_name") == nil {
			collection.Fields.Add(&core.TextField{Name: "last_name"})
			changed = true
		}
		if changed {
			if err := app.Save(collection); err != nil {
				return err
			}
		}

		records, err := app.FindAllRecords("subscribers")
		if err != nil {
			return err
		}
		for _, rec := range records {
			firstName := strings.TrimSpace(rec.GetString("first_name"))
			lastName := strings.TrimSpace(rec.GetString("last_name"))
			name := strings.TrimSpace(rec.GetString("name"))
			if firstName == "" && lastName == "" && name != "" {
				firstName, lastName = models.SplitSubscriberName(name)
			}

			rec.Set("first_name", firstName)
			rec.Set("last_name", lastName)
			rec.Set("name", models.JoinSubscriberName(firstName, lastName))

			if err := app.Save(rec); err != nil {
				return err
			}
		}

		return nil
	}, func(app core.App) error {
		return nil
	})
}
