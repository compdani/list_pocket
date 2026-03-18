package core

import (
	"net/http"
	"time"

	"github.com/compdani/list_pocket/models"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// GetSubscriptions retrieves the subscriptions for a subscriber.
func (c *Core) GetSubscriptions(subID int, subUUID string, allLists bool) ([]models.Subscription, error) {
	var out []models.Subscription
	err := c.q.GetSubscriptions.Select(&out, subID, subUUID, allLists)
	if err != nil {
		c.log.Printf("error getting subscriptions: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", err.Error()))
	}

	return out, err
}

// AddSubscriptions adds list subscriptions to subscribers.
func (c *Core) AddSubscriptions(subIDs, listIDs []int, status string) error {
	if _, err := c.q.AddSubscribersToLists.Exec(pq.Array(subIDs), pq.Array(listIDs), status); err != nil {
		c.log.Printf("error adding subscriptions: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", err.Error()))
	}

	return nil
}

// AddSubscriptionsByQuery adds list subscriptions to subscribers by a given arbitrary query expression.
// sourceListIDs is the list of list IDs to filter the subscriber query with.
func (c *Core) AddSubscriptionsByQuery(searchStr, queryExp string, sourceListIDs, targetListIDs []int, status string, subStatus string) error {
	if c.isSQLite() {
		subIDs, err := c.findSubscriberIDsSQLite(searchStr, queryExp, sourceListIDs, subStatus, 0, 0)
		if err != nil {
			c.log.Printf("error adding subscriptions by query: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
		}

		if err := c.addSubscriptionsSQLite(subIDs, targetListIDs, status); err != nil {
			c.log.Printf("error adding subscriptions by query: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
		}
		return nil
	}

	if sourceListIDs == nil {
		sourceListIDs = []int{}
	}

	err := c.q.ExecSubQueryTpl(searchStr, queryExp, c.q.AddSubscribersToListsByQuery, sourceListIDs, c.db, subStatus, pq.Array(targetListIDs), status)
	if err != nil {
		c.log.Printf("error adding subscriptions by query: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}

	return nil
}

// DeleteSubscriptions delete list subscriptions from subscribers.
func (c *Core) DeleteSubscriptions(subIDs, listIDs []int) error {
	if _, err := c.q.DeleteSubscriptions.Exec(pq.Array(subIDs), pq.Array(listIDs)); err != nil {
		c.log.Printf("error deleting subscriptions: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", err.Error()))

	}

	return nil
}

// DeleteSubscriptionsByQuery deletes list subscriptions from subscribers by a given arbitrary query expression.
// sourceListIDs is the list of list IDs to filter the subscriber query with.
func (c *Core) DeleteSubscriptionsByQuery(searchStr, queryExp string, sourceListIDs, targetListIDs []int, subStatus string) error {
	if c.isSQLite() {
		subIDs, err := c.findSubscriberIDsSQLite(searchStr, queryExp, sourceListIDs, subStatus, 0, 0)
		if err != nil {
			c.log.Printf("error deleting subscriptions by query: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
		}

		if err := c.deleteSubscriptionsSQLite(subIDs, targetListIDs); err != nil {
			c.log.Printf("error deleting subscriptions by query: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
		}
		return nil
	}

	if sourceListIDs == nil {
		sourceListIDs = []int{}
	}

	err := c.q.ExecSubQueryTpl(searchStr, queryExp, c.q.DeleteSubscriptionsByQuery, sourceListIDs, c.db, subStatus, pq.Array(targetListIDs))
	if err != nil {
		c.log.Printf("error deleting subscriptions by query: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}

	return nil
}

// UnsubscribeLists sets list subscriptions to 'unsubscribed'.
func (c *Core) UnsubscribeLists(subIDs, listIDs []int, listUUIDs []string) error {
	if _, err := c.q.UnsubscribeSubscribersFromLists.Exec(pq.Array(subIDs), pq.Array(listIDs), pq.StringArray(listUUIDs)); err != nil {
		c.log.Printf("error unsubscribing from lists: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", err.Error()))
	}

	return nil
}

// UnsubscribeListsByQuery sets list subscriptions to 'unsubscribed' by a given arbitrary query expression.
// sourceListIDs is the list of list IDs to filter the subscriber query with.
func (c *Core) UnsubscribeListsByQuery(searchStr, queryExp string, sourceListIDs, targetListIDs []int, subStatus string) error {
	if c.isSQLite() {
		subIDs, err := c.findSubscriberIDsSQLite(searchStr, queryExp, sourceListIDs, subStatus, 0, 0)
		if err != nil {
			c.log.Printf("error unsubscribing from lists by query: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
		}

		if err := c.unsubscribeSubscriptionsSQLite(subIDs, targetListIDs); err != nil {
			c.log.Printf("error unsubscribing from lists by query: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
		}
		return nil
	}

	if sourceListIDs == nil {
		sourceListIDs = []int{}
	}

	err := c.q.ExecSubQueryTpl(searchStr, queryExp, c.q.UnsubscribeSubscribersFromListsByQuery, sourceListIDs, c.db, subStatus, pq.Array(targetListIDs))
	if err != nil {
		c.log.Printf("error unsubscribing from lists by query: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}

	return nil
}

// DeleteUnconfirmedSubscriptions sets list subscriptions to 'unsubscribed' by a given arbitrary query expression.
// sourceListIDs is the list of list IDs to filter the subscriber query with.
func (c *Core) DeleteUnconfirmedSubscriptions(beforeDate time.Time) (int, error) {
	res, err := c.q.DeleteUnconfirmedSubscriptions.Exec(beforeDate)
	if err != nil {
		c.log.Printf("error deleting unconfirmed subscribers: %v", err)
		return 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}

	n, _ := res.RowsAffected()
	return int(n), nil
}

func (c *Core) addSubscriptionsSQLite(subIDs, listIDs []int, status string) error {
	if len(subIDs) == 0 || len(listIDs) == 0 {
		return nil
	}

	for i := 0; i < len(subIDs); i += 300 {
		end := i + 300
		if end > len(subIDs) {
			end = len(subIDs)
		}
		chunk := subIDs[i:end]

		q := `
			INSERT INTO subscriber_lists (subscriber_id, list_id, status, updated_at)
			SELECT s.id, l.id,
			       (CASE WHEN ? != '' THEN ? ELSE 'unconfirmed' END),
			       (strftime('%Y-%m-%d %H:%M:%fZ'))
			FROM subscribers s
			CROSS JOIN lists l
			WHERE s.id IN (` + sqlitePlaceholders(len(chunk)) + `)
			  AND l.id IN (` + sqlitePlaceholders(len(listIDs)) + `)
			ON CONFLICT (subscriber_id, list_id) DO NOTHING
		`

		args := make([]any, 0, 2+len(chunk)+len(listIDs))
		args = append(args, status, status)
		for _, id := range chunk {
			args = append(args, id)
		}
		for _, id := range listIDs {
			args = append(args, id)
		}

		if _, err := c.db.Exec(q, args...); err != nil {
			return err
		}
	}

	return nil
}

func (c *Core) deleteSubscriptionsSQLite(subIDs, listIDs []int) error {
	if len(subIDs) == 0 || len(listIDs) == 0 {
		return nil
	}

	for i := 0; i < len(subIDs); i += 400 {
		end := i + 400
		if end > len(subIDs) {
			end = len(subIDs)
		}
		chunk := subIDs[i:end]

		q := `DELETE FROM subscriber_lists
			WHERE subscriber_id IN (` + sqlitePlaceholders(len(chunk)) + `)
			  AND list_id IN (` + sqlitePlaceholders(len(listIDs)) + `)`

		args := make([]any, 0, len(chunk)+len(listIDs))
		for _, id := range chunk {
			args = append(args, id)
		}
		for _, id := range listIDs {
			args = append(args, id)
		}

		if _, err := c.db.Exec(q, args...); err != nil {
			return err
		}
	}

	return nil
}

func (c *Core) unsubscribeSubscriptionsSQLite(subIDs, listIDs []int) error {
	if len(subIDs) == 0 || len(listIDs) == 0 {
		return nil
	}

	for i := 0; i < len(subIDs); i += 400 {
		end := i + 400
		if end > len(subIDs) {
			end = len(subIDs)
		}
		chunk := subIDs[i:end]

		q := `UPDATE subscriber_lists
			SET status='unsubscribed',
			    updated=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE subscriber_id IN (` + sqlitePlaceholders(len(chunk)) + `)
			  AND list_id IN (` + sqlitePlaceholders(len(listIDs)) + `)`

		args := make([]any, 0, len(chunk)+len(listIDs))
		for _, id := range chunk {
			args = append(args, id)
		}
		for _, id := range listIDs {
			args = append(args, id)
		}

		if _, err := c.db.Exec(q, args...); err != nil {
			return err
		}
	}

	return nil
}
