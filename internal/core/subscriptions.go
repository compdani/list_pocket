package core

import (
	"encoding/json"
	"github.com/compdani/list_pocket/internal/apperr"
	"time"

	"github.com/compdani/list_pocket/models"
	null "gopkg.in/volatiletech/null.v6"
)

// GetSubscriptions retrieves the subscriptions for a subscriber.
func (c *Core) GetSubscriptions(subID int, subUUID string, allLists bool) ([]models.Subscription, error) {
	listType := models.ListTypePublic
	if allLists {
		listType = ""
	}

	lists, err := c.getSubscriberListsSQLite(subID, subUUID, nil, nil, "", listType)
	if err != nil {
		c.log.Printf("error getting subscriptions: %v", err)
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", err.Error()))
	}

	out := make([]models.Subscription, 0, len(lists))
	for _, list := range lists {
		createdAt := null.String{}
		if list.SubscriptionCreatedAt.Valid {
			createdAt = null.StringFrom(list.SubscriptionCreatedAt.Time.Format(time.RFC3339))
		}
		out = append(out, models.Subscription{
			List:                  list,
			SubscriptionStatus:    nullStringFrom(list.SubscriptionStatus),
			SubscriptionCreatedAt: createdAt,
			Meta:                  nil,
		})
	}
	return out, nil
}

func nullStringFrom(v string) null.String {
	if v == "" {
		return null.String{}
	}
	return null.StringFrom(v)
}

// AddSubscriptions adds list subscriptions to subscribers.
func (c *Core) AddSubscriptions(subIDs, listIDs []int, status string) error {
	if _, err := c.addSubscriptionsSQLite(subIDs, listIDs, status); err != nil {
		c.log.Printf("error adding subscriptions: %v", err)
		return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}

	return nil
}

// AddSubscriptionsByQuery adds list subscriptions to subscribers by a given arbitrary query expression.
// sourceListIDs is the list of list IDs to filter the subscriber query with.
func (c *Core) AddSubscriptionsByQuery(searchStr, queryExp string, filters json.RawMessage, sourceListIDs, targetListIDs []int, status string, subStatus string) (int, error) {
	subIDs, err := c.findSubscriberIDsSQLite(searchStr, queryExp, filters, sourceListIDs, subStatus, 0, 0)
	if err != nil {
		c.log.Printf("error adding subscriptions by query: %v", err)
		return 0, apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}

	affected, err := c.addSubscriptionsSQLite(subIDs, targetListIDs, status)
	if err != nil {
		c.log.Printf("error adding subscriptions by query: %v", err)
		return 0, apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}
	return affected, nil
}

// DeleteSubscriptions delete list subscriptions from subscribers.
func (c *Core) DeleteSubscriptions(subIDs, listIDs []int) error {
	if _, err := c.deleteSubscriptionsSQLite(subIDs, listIDs); err != nil {
		c.log.Printf("error deleting subscriptions: %v", err)
		return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))

	}

	return nil
}

// DeleteSubscriptionsByQuery deletes list subscriptions from subscribers by a given arbitrary query expression.
// sourceListIDs is the list of list IDs to filter the subscriber query with.
func (c *Core) DeleteSubscriptionsByQuery(searchStr, queryExp string, filters json.RawMessage, sourceListIDs, targetListIDs []int, subStatus string) (int, error) {
	subIDs, err := c.findSubscriberIDsSQLite(searchStr, queryExp, filters, sourceListIDs, subStatus, 0, 0)
	if err != nil {
		c.log.Printf("error deleting subscriptions by query: %v", err)
		return 0, apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}

	affected, err := c.deleteSubscriptionsSQLite(subIDs, targetListIDs)
	if err != nil {
		c.log.Printf("error deleting subscriptions by query: %v", err)
		return 0, apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}
	return affected, nil
}

// UnsubscribeLists sets list subscriptions to 'unsubscribed'.
func (c *Core) UnsubscribeLists(subIDs, listIDs []int, listUUIDs []string) error {
	if len(subIDs) == 0 {
		return nil
	}

	resolvedListIDs := append([]int{}, listIDs...)
	if len(listUUIDs) > 0 {
		lists, err := c.getSubscriberListsSQLite(subIDs[0], "", nil, listUUIDs, "", "")
		if err != nil {
			c.log.Printf("error unsubscribing from lists: %v", err)
			return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
		}

		for _, list := range lists {
			if list.ID > 0 {
				resolvedListIDs = append(resolvedListIDs, list.ID)
			}
		}
	}

	deduped := make([]int, 0, len(resolvedListIDs))
	seen := map[int]struct{}{}
	for _, id := range resolvedListIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		deduped = append(deduped, id)
	}

	if _, err := c.unsubscribeSubscriptionsSQLite(subIDs, deduped); err != nil {
		c.log.Printf("error unsubscribing from lists: %v", err)
		return apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}
	return nil
}

// UnsubscribeListsByQuery sets list subscriptions to 'unsubscribed' by a given arbitrary query expression.
// sourceListIDs is the list of list IDs to filter the subscriber query with.
func (c *Core) UnsubscribeListsByQuery(searchStr, queryExp string, filters json.RawMessage, sourceListIDs, targetListIDs []int, subStatus string) (int, error) {
	subIDs, err := c.findSubscriberIDsSQLite(searchStr, queryExp, filters, sourceListIDs, subStatus, 0, 0)
	if err != nil {
		c.log.Printf("error unsubscribing from lists by query: %v", err)
		return 0, apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}

	affected, err := c.unsubscribeSubscriptionsSQLite(subIDs, targetListIDs)
	if err != nil {
		c.log.Printf("error unsubscribing from lists by query: %v", err)
		return 0, apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}
	return affected, nil
}

// DeleteUnconfirmedSubscriptions sets list subscriptions to 'unsubscribed' by a given arbitrary query expression.
// sourceListIDs is the list of list IDs to filter the subscriber query with.
func (c *Core) DeleteUnconfirmedSubscriptions(beforeDate time.Time) (int, error) {
	res, err := c.db.Exec(`
		DELETE FROM subscriber_lists
		WHERE status = 'unconfirmed'
		  AND created < ?`,
		beforeDate.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		c.log.Printf("error deleting unconfirmed subscribers: %v", err)
		return 0, apperr.Internal(c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}

	n, _ := res.RowsAffected()
	return int(n), nil
}

func (c *Core) addSubscriptionsSQLite(subIDs, listIDs []int, status string) (int, error) {
	if len(subIDs) == 0 || len(listIDs) == 0 {
		return 0, nil
	}

	affected := 0

	for i := 0; i < len(subIDs); i += 300 {
		end := i + 300
		if end > len(subIDs) {
			end = len(subIDs)
		}
		chunk := subIDs[i:end]

		q := `
			INSERT INTO subscriber_lists (subscriber_id, list_id, status, sms_status, updated)
			SELECT s.id, l.id,
			       (CASE WHEN ? != '' THEN ? ELSE 'unconfirmed' END),
			       (CASE WHEN ? != '' THEN ? ELSE 'unconfirmed' END),
			       (strftime('%Y-%m-%d %H:%M:%fZ'))
			FROM subscribers s
			CROSS JOIN lists l
			WHERE s.rowid IN (` + sqlitePlaceholders(len(chunk)) + `)
			  AND l.rowid IN (` + sqlitePlaceholders(len(listIDs)) + `)
			ON CONFLICT (subscriber_id, list_id) DO NOTHING
		`

		args := make([]any, 0, 4+len(chunk)+len(listIDs))
		args = append(args, status, status, status, status)
		for _, id := range chunk {
			args = append(args, id)
		}
		for _, id := range listIDs {
			args = append(args, id)
		}

		res, err := c.db.Exec(q, args...)
		if err != nil {
			return 0, err
		}

		n, _ := res.RowsAffected()
		affected += int(n)
	}

	return affected, nil
}

func (c *Core) deleteSubscriptionsSQLite(subIDs, listIDs []int) (int, error) {
	if len(subIDs) == 0 || len(listIDs) == 0 {
		return 0, nil
	}

	affected := 0

	for i := 0; i < len(subIDs); i += 400 {
		end := i + 400
		if end > len(subIDs) {
			end = len(subIDs)
		}
		chunk := subIDs[i:end]

		q := `DELETE FROM subscriber_lists
			WHERE subscriber_id IN (
				SELECT id FROM subscribers WHERE rowid IN (` + sqlitePlaceholders(len(chunk)) + `)
			)
			  AND list_id IN (
				SELECT id FROM lists WHERE rowid IN (` + sqlitePlaceholders(len(listIDs)) + `)
			)`

		args := make([]any, 0, len(chunk)+len(listIDs))
		for _, id := range chunk {
			args = append(args, id)
		}
		for _, id := range listIDs {
			args = append(args, id)
		}

		res, err := c.db.Exec(q, args...)
		if err != nil {
			return 0, err
		}

		n, _ := res.RowsAffected()
		affected += int(n)
	}

	return affected, nil
}

func (c *Core) unsubscribeSubscriptionsSQLite(subIDs, listIDs []int) (int, error) {
	if len(subIDs) == 0 || len(listIDs) == 0 {
		return 0, nil
	}

	affected := 0

	for i := 0; i < len(subIDs); i += 400 {
		end := i + 400
		if end > len(subIDs) {
			end = len(subIDs)
		}
		chunk := subIDs[i:end]

		q := `UPDATE subscriber_lists
			SET status='unsubscribed',
			    updated=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE subscriber_id IN (
				SELECT id FROM subscribers WHERE rowid IN (` + sqlitePlaceholders(len(chunk)) + `)
			)
			  AND list_id IN (
				SELECT id FROM lists WHERE rowid IN (` + sqlitePlaceholders(len(listIDs)) + `)
			)`

		args := make([]any, 0, len(chunk)+len(listIDs))
		for _, id := range chunk {
			args = append(args, id)
		}
		for _, id := range listIDs {
			args = append(args, id)
		}

		res, err := c.db.Exec(q, args...)
		if err != nil {
			return 0, err
		}

		n, _ := res.RowsAffected()
		affected += int(n)
	}

	return affected, nil
}
