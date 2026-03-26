package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("campaign_send_ledger")
		if err != nil {
			return nil
		}
		f := col.Fields.GetByName("status")
		if f == nil {
			return nil
		}
		sf, ok := f.(*core.SelectField)
		if !ok {
			return nil
		}
		for _, v := range sf.Values {
			if v == "inflight" {
				return nil
			}
		}
		sf.Values = append(sf.Values, "inflight")
		return app.Save(col)
	}, func(app core.App) error {
		return nil
	})
}
