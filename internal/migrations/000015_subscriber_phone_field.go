package migrations

import (
	"database/sql"
	"errors"
	"strings"

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

		if collection.Fields.GetByName("phone") == nil {
			collection.Fields.Add(&core.TextField{Name: "phone"})
			if err := app.Save(collection); err != nil {
				return err
			}
		}

		records, err := app.FindAllRecords("subscribers")
		if err != nil {
			return err
		}
		for _, rec := range records {
			rec.Set("phone", strings.TrimSpace(rec.GetString("phone")))
			if err := app.Save(rec); err != nil {
				return err
			}
		}

		return nil
	}, func(app core.App) error {
		return nil
	})
}
