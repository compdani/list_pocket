package core

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/compdani/list_pocket/models"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

var bounceQuerySortFields = []string{"email", "campaign_name", "source", "created_at", "type"}

// QueryBounces retrieves paginated bounce entries based on the given params.
// It also returns the total number of bounce records in the DB.
func (c *Core) QueryBounces(campID, subID int, source, orderBy, order string, offset, limit int) ([]models.Bounce, int, error) {
	if c.isSQLite() {
		return c.queryBouncesSQLite(campID, subID, source, orderBy, order, offset, limit)
	}

	if !strSliceContains(orderBy, bounceQuerySortFields) {
		orderBy = "created_at"
	}
	if order != SortAsc && order != SortDesc {
		order = SortDesc
	}

	out := []models.Bounce{}
	stmt := strings.ReplaceAll(c.q.QueryBounces, "%order%", orderBy+" "+order)
	if err := c.db.Select(&out, stmt, 0, campID, subID, source, offset, limit); err != nil {
		c.log.Printf("error fetching bounces: %v", err)
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.bounce}", "error", pqErrMsg(err)))
	}

	total := 0
	if len(out) > 0 {
		total = out[0].Total
	}

	return out, total, nil
}

// GetBounce retrieves bounce entries based on the given params.
func (c *Core) GetBounce(id string) (models.Bounce, error) {
	if c.isSQLite() {
		out, _, err := c.queryBouncesSQLite(0, 0, "", "id", SortAsc, 0, 1, id)
		if err != nil {
			return models.Bounce{}, err
		}
		if len(out) == 0 {
			return models.Bounce{}, echo.NewHTTPError(http.StatusBadRequest,
				c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.bounce}"))
		}
		return out[0], nil
	}

	var out []models.Bounce
	stmt := strings.ReplaceAll(c.q.QueryBounces, "%order%", "id "+SortAsc)
	if err := c.db.Select(&out, stmt, id, 0, 0, "", 0, 1); err != nil {
		c.log.Printf("error fetching bounces: %v", err)
		return models.Bounce{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.bounce}", "error", pqErrMsg(err)))
	}

	if len(out) == 0 {
		return models.Bounce{}, echo.NewHTTPError(http.StatusBadRequest,
			c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.bounce}"))

	}

	return out[0], nil
}

// RecordBounce records a new bounce.
func (c *Core) RecordBounce(b models.Bounce) error {
	if c.isSQLite() {
		return c.recordBounceSQLite(b)
	}

	action, ok := c.consts.BounceActions[b.Type]
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, c.i18n.Ts("globals.messages.invalidData")+": "+b.Type)
	}

	_, err := c.q.RecordBounce.Exec(b.SubscriberUUID,
		b.Email,
		b.CampaignUUID,
		b.Type,
		b.Source,
		b.Meta,
		b.CreatedAt,
		action.Count,
		action.Action)

	if err != nil {
		// Ignore the error if it complained of no subscriber.
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Column == "subscriber_id" {
			c.log.Printf("bounced subscriber (%s / %s) not found", b.SubscriberUUID, b.Email)
			return nil
		}

		c.log.Printf("error recording bounce: %v", err)
	}

	return err
}

// BlocklistBouncedSubscribers blocklists all bounced subscribers.
func (c *Core) BlocklistBouncedSubscribers() error {
	if c.isSQLite() {
		if _, err := c.db.Exec(`
			UPDATE subscribers
			SET status='blocklisted', updated=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE id IN (SELECT DISTINCT subscriber_id FROM bounces)
		`); err != nil {
			c.log.Printf("error blocklisting bounced subscribers: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError, c.i18n.Ts("subscribers.errorBlocklisting", "error", err.Error()))
		}

		if _, err := c.db.Exec(`
			UPDATE subscriber_lists
			SET status='unsubscribed', updated=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE subscriber_id IN (SELECT DISTINCT subscriber_id FROM bounces)
		`); err != nil {
			c.log.Printf("error unsubscribing bounced subscriber lists: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError, c.i18n.Ts("subscribers.errorBlocklisting", "error", err.Error()))
		}

		return nil
	}

	if _, err := c.q.BlocklistBouncedSubscribers.Exec(); err != nil {
		c.log.Printf("error blocklisting bounced subscribers: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, c.i18n.Ts("subscribers.errorBlocklisting", "error", err.Error()))
	}

	return nil
}

// DeleteBounce deletes a list.
func (c *Core) DeleteBounce(id string) error {
	return c.DeleteBounces([]string{id}, false)
}

// DeleteBounces deletes multiple lists.
func (c *Core) DeleteBounces(ids []string, all bool) error {
	if c.isSQLite() {
		if all {
			if _, err := c.db.Exec(`DELETE FROM bounces`); err != nil {
				c.log.Printf("error deleting bounces: %v", err)
				return echo.NewHTTPError(http.StatusInternalServerError,
					c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.list}", "error", pqErrMsg(err)))
			}
			return nil
		}

		if len(ids) == 0 {
			return nil
		}

		q := `DELETE FROM bounces WHERE id IN (` + sqlitePlaceholders(len(ids)) + `)`
		args := make([]any, 0, len(ids))
		for _, id := range ids {
			args = append(args, id)
		}
		if _, err := c.db.Exec(q, args...); err != nil {
			c.log.Printf("error deleting bounces: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.list}", "error", pqErrMsg(err)))
		}
		return nil
	}

	if _, err := c.q.DeleteBounces.Exec(pq.Array(ids), all); err != nil {
		c.log.Printf("error deleting lists: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.list}", "error", pqErrMsg(err)))
	}
	return nil
}

func (c *Core) queryBouncesSQLite(campID, subID int, source, orderBy, order string, offset, limit int, onlyIDs ...string) ([]models.Bounce, int, error) {
	if !strSliceContains(orderBy, bounceQuerySortFields) {
		orderBy = "created_at"
	}
	if order != SortAsc && order != SortDesc {
		order = SortDesc
	}

	sortCol := map[string]string{
		"email":         "s.email",
		"campaign_name": "c.name",
		"source":        "b.source",
		"created_at":    "b.created",
		"type":          "b.type",
		"id":            "b.id",
	}[orderBy]
	if sortCol == "" {
		sortCol = "b.created"
	}

	q := `
		SELECT COUNT(*) OVER() AS total,
			b.id,
			b.type,
			b.source,
			b.meta,
			b.created AS created_at,
			s.id AS subscriber_id,
			s.uuid AS subscriber_uuid,
			s.email AS email,
			s.status AS subscriber_status,
			CASE WHEN b.campaign_id IS NOT NULL
			     THEN json_object('id', c.id, 'name', c.name)
			     ELSE NULL END AS campaign,
			COALESCE(c.name, '') AS campaign_name
		FROM bounces b
		LEFT JOIN subscribers s ON s.id = b.subscriber_id
		LEFT JOIN campaigns c ON c.id = b.campaign_id
		WHERE 1=1
	`

	args := []any{}
	if len(onlyIDs) > 0 && strings.TrimSpace(onlyIDs[0]) != "" {
		q += ` AND b.id = ?`
		args = append(args, onlyIDs[0])
	}
	if campID > 0 {
		q += ` AND b.campaign_id = ?`
		args = append(args, campID)
	}
	if subID > 0 {
		q += ` AND b.subscriber_id = ?`
		args = append(args, subID)
	}
	if source != "" {
		q += ` AND b.source = ?`
		args = append(args, source)
	}

	q += ` ORDER BY ` + sortCol + ` ` + strings.ToUpper(order)
	if limit > 0 {
		q += ` LIMIT ? OFFSET ?`
		args = append(args, limit, offset)
	}

	out := []models.Bounce{}
	if err := c.db.Select(&out, q, args...); err != nil {
		c.log.Printf("error fetching bounces: %v", err)
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.bounce}", "error", pqErrMsg(err)))
	}

	total := 0
	if len(out) > 0 {
		total = out[0].Total
	}
	return out, total, nil
}

func (c *Core) recordBounceSQLite(b models.Bounce) error {
	action, ok := c.consts.BounceActions[b.Type]
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, c.i18n.Ts("globals.messages.invalidData")+": "+b.Type)
	}

	tx, err := c.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var sub struct {
		ID     int    `db:"id"`
		Status string `db:"status"`
	}

	if b.SubscriberUUID != "" {
		err = tx.Get(&sub, `SELECT id, status FROM subscribers WHERE uuid = ? LIMIT 1`, b.SubscriberUUID)
	} else {
		err = tx.Get(&sub, `SELECT id, status FROM subscribers WHERE email = ? LIMIT 1`, b.Email)
	}
	if err == sql.ErrNoRows {
		c.log.Printf("bounced subscriber (%s / %s) not found", b.SubscriberUUID, b.Email)
		return nil
	}
	if err != nil {
		c.log.Printf("error recording bounce: %v", err)
		return err
	}

	campID := 0
	if b.CampaignUUID != "" {
		_ = tx.Get(&campID, `SELECT id FROM campaigns WHERE uuid = ? LIMIT 1`, b.CampaignUUID)
	}

	var num int
	if err := tx.Get(&num, `SELECT COUNT(*) + 1 AS num FROM bounces WHERE subscriber_id = ? AND type = ?`, sub.ID, b.Type); err != nil {
		c.log.Printf("error counting bounces: %v", err)
		return err
	}

	if sub.Status != models.SubscriberStatusBlockListed && num >= action.Count {
		switch action.Action {
		case "blocklist":
			if _, err := tx.Exec(`UPDATE subscribers SET status='blocklisted', updated=(strftime('%Y-%m-%d %H:%M:%fZ')) WHERE id = ?`, sub.ID); err != nil {
				return err
			}
		case "unsubscribe":
			if _, err := tx.Exec(`UPDATE subscriber_lists SET status='unsubscribed', updated=(strftime('%Y-%m-%d %H:%M:%fZ')) WHERE subscriber_id = ?`, sub.ID); err != nil {
				return err
			}
		case "delete":
			if _, err := tx.Exec(`DELETE FROM subscribers WHERE id = ?`, sub.ID); err != nil {
				return err
			}
		}
	}

	// Same behavior as legacy query: don't insert if already blocklisted or if over threshold.
	if sub.Status != models.SubscriberStatusBlockListed && num <= action.Count {
		meta := b.Meta
		if len(meta) == 0 {
			meta = json.RawMessage(`{}`)
		}
		createdAt := b.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now()
		}

		if _, err := tx.Exec(`
			INSERT INTO bounces (subscriber_id, campaign_id, type, source, meta, created)
			VALUES (?, NULLIF(?, 0), ?, ?, ?, ?)`,
			sub.ID, campID, b.Type, b.Source, meta, createdAt.UTC().Format("2006-01-02 15:04:05")); err != nil {
			c.log.Printf("error recording bounce: %v", err)
			return err
		}
	}

	return tx.Commit()
}
