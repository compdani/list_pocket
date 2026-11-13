package core

import (
	pbcore "ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/compdani/list_pocket/internal/apperr"
	"github.com/compdani/list_pocket/models"
)

// PatchSubscriberProfile updates only supplied profile fields in a transaction.
// List membership, consent, status and notification hooks are untouched.
func (c *Core) PatchSubscriberProfile(recordID string, patch map[string]any) (models.Subscriber, error) {
	for key := range patch {
		switch key {
		case "email", "phone", "first_name", "last_name", "attribs":
		default:
			return models.Subscriber{}, apperr.BadRequest("unsupported profile field")
		}
	}
	err := c.db.PocketBase().RunInTransaction(func(tx pbcore.App) error {
		rec, err := tx.FindRecordById("subscribers", recordID)
		if err != nil {
			return apperr.NotFound("subscriber not found")
		}
		for key, value := range patch {
			rec.Set(key, value)
		}
		_, first := patch["first_name"]
		_, last := patch["last_name"]
		if first || last {
			rec.Set("name", models.JoinSubscriberName(rec.GetString("first_name"), rec.GetString("last_name")))
		}
		return tx.Save(rec)
	})
	if err != nil {
		return models.Subscriber{}, err
	}
	return c.GetSubscriber(0, recordID, "")
}
