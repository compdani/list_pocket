package core

import (
	"net/http"
	"strconv"

	"github.com/jmoiron/sqlx/types"
	"github.com/labstack/echo/v4"
)

// GetDashboardCharts returns chart data points to render on the dashboard.
func (c *Core) GetDashboardCharts(tzOffsetMins int) (types.JSONText, error) {
	if c.isSQLite() {
		return c.getDashboardChartsSQLite(tzOffsetMins)
	}

	_ = c.refreshCache(matDashboardCharts, false)

	var out types.JSONText
	if err := c.q.GetDashboardCharts.Get(&out); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard charts", "error", pqErrMsg(err)))
	}

	return out, nil
}

// GetDashboardCounts returns stats counts to show on the dashboard.
func (c *Core) GetDashboardCounts() (types.JSONText, error) {
	if c.isSQLite() {
		return c.getDashboardCountsSQLite()
	}

	_ = c.refreshCache(matDashboardCounts, false)

	var out types.JSONText
	if err := c.q.GetDashboardCounts.Get(&out); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard stats", "error", pqErrMsg(err)))
	}

	return out, nil
}

func sqliteTimezoneModifier(tzOffsetMins int) string {
	if tzOffsetMins == 0 {
		return "0 minutes"
	}

	return strconv.Itoa(-tzOffsetMins) + " minutes"
}

func (c *Core) getDashboardChartsSQLite(tzOffsetMins int) (types.JSONText, error) {
	const q = `
	SELECT json_object(
		'link_clicks', COALESCE((
			SELECT json_group_array(json_object('count', count, 'date', date))
			FROM (
				SELECT COUNT(*) AS count, DATE(datetime(created, ?)) AS date
				FROM link_clicks
				WHERE DATE(datetime(created, ?)) >= DATE(datetime('now', ?), '-30 day')
				GROUP BY DATE(datetime(created, ?))
				ORDER BY DATE(datetime(created, ?))
			)
		), '[]'),
		'campaign_views', COALESCE((
			SELECT json_group_array(json_object('count', count, 'date', date))
			FROM (
				SELECT COUNT(DISTINCT campaign_id || ':' || COALESCE(CAST(subscriber_id AS TEXT), 'anon:' || rowid)) AS count, DATE(datetime(created, ?)) AS date
				FROM campaign_views
				WHERE DATE(datetime(created, ?)) >= DATE(datetime('now', ?), '-30 day')
				GROUP BY DATE(datetime(created, ?))
				ORDER BY DATE(datetime(created, ?))
			)
		), '[]')
	) AS data`

	tzModifier := sqliteTimezoneModifier(tzOffsetMins)

	var out types.JSONText
	if err := c.db.Get(&out, q,
		tzModifier, tzModifier, tzModifier, tzModifier, tzModifier,
		tzModifier, tzModifier, tzModifier, tzModifier, tzModifier,
	); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard charts", "error", pqErrMsg(err)))
	}

	return out, nil
}

func (c *Core) getDashboardCountsSQLite() (types.JSONText, error) {
	const q = `
	WITH subs AS (
		SELECT COUNT(*) AS num, status FROM subscribers GROUP BY status
	),
	campaign_statuses AS (
		SELECT status, COUNT(*) AS num FROM campaigns GROUP BY status
	)
	SELECT json_object(
		'subscribers', json_object(
			'total', COALESCE((SELECT SUM(num) FROM subs), 0),
			'blocklisted', COALESCE((SELECT num FROM subs WHERE status='blocklisted'), 0),
			'orphans', COALESCE((
				SELECT COUNT(s.id) FROM subscribers s
				LEFT JOIN subscriber_lists sl ON s.id = sl.subscriber_id
				WHERE sl.subscriber_id IS NULL
			), 0)
		),
		'lists', json_object(
			'total', COALESCE((SELECT COUNT(*) FROM lists), 0),
			'private', COALESCE((SELECT COUNT(*) FROM lists WHERE type='private'), 0),
			'public', COALESCE((SELECT COUNT(*) FROM lists WHERE type='public'), 0),
			'optin_single', COALESCE((SELECT COUNT(*) FROM lists WHERE optin='single'), 0),
			'optin_double', COALESCE((SELECT COUNT(*) FROM lists WHERE optin='double'), 0)
		),
		'campaigns', json_object(
			'total', COALESCE((SELECT COUNT(*) FROM campaigns), 0),
			'by_status', COALESCE((SELECT json_group_object(status, num) FROM campaign_statuses), '{}')
		),
		'messages', COALESCE((SELECT SUM(sent) FROM campaigns), 0)
	) AS data`

	var out types.JSONText
	if err := c.db.Get(&out, q); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard stats", "error", pqErrMsg(err)))
	}

	return out, nil
}
