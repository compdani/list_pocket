package campaignledger

import (
	"github.com/jmoiron/sqlx"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// RegisterHooks adds PocketBase hooks that keep campaign_send_ledger in sync when
// subscriber_lists membership changes while a campaign is active.
func RegisterHooks(pb *pocketbase.PocketBase) {
	pb.OnRecordAfterCreateSuccess("subscriber_lists").BindFunc(func(e *core.RecordEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		return syncSubscriberListHook(e.App, e.Record)
	})

	pb.OnRecordAfterUpdateSuccess("subscriber_lists").BindFunc(func(e *core.RecordEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		return syncSubscriberListHook(e.App, e.Record)
	})
}

func syncSubscriberListHook(app core.App, record *core.Record) error {
	listID := record.GetString("list_id")
	subscriberID := record.GetString("subscriber_id")
	if listID == "" || subscriberID == "" {
		return nil
	}

	bxp, ok := app.NonconcurrentDB().(*dbx.DB)
	if !ok || bxp == nil {
		return nil
	}

	driver := bxp.DriverName()
	if driver == "" {
		driver = "sqlite3"
	}
	db := sqlx.NewDb(bxp.DB(), driver).Unsafe()

	return InsertPendingIfEligible(db, listID, subscriberID)
}
