package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		for _, name := range []string{"pictures", "subscriber_tags", "campaign_tags"} {
			col, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			// Mailapi owns role authorization. Public picture file URLs remain usable in email.
			col.ListRule = nil
			col.ViewRule = nil
			col.CreateRule = nil
			col.UpdateRule = nil
			col.DeleteRule = nil
			if err := app.Save(col); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		// Explicit rollback restores the prior schema's authorization behavior.
		authRule, publicRule := "@request.auth.id != ''", "1=1"
		for _, name := range []string{"pictures", "subscriber_tags", "campaign_tags"} {
			col, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			col.ListRule, col.ViewRule = &authRule, &authRule
			if name == "pictures" {
				col.ListRule, col.ViewRule = &publicRule, &publicRule
			}
			col.CreateRule, col.UpdateRule, col.DeleteRule = &authRule, &authRule, &authRule
			if err := app.Save(col); err != nil {
				return err
			}
		}
		return nil
	})
}
