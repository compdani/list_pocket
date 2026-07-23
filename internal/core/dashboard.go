package core

import (
	"database/sql"
	"encoding/json"
	"github.com/compdani/list_pocket/internal/apperr"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx/types"
)

// GetDashboardCharts returns chart data points to render on the dashboard.
func (c *Core) GetDashboardCharts(timeZone string) (types.JSONText, error) {
	return c.getDashboardChartsSQLite(timeZone)
}

// GetDashboardCounts returns stats counts to show on the dashboard.
func (c *Core) GetDashboardCounts() (types.JSONText, error) {
	return c.getDashboardCountsSQLite()
}

func (c *Core) getDashboardChartsSQLite(timeZone string) (types.JSONText, error) {
	tzName := strings.TrimSpace(timeZone)
	if tzName == "" {
		tzName = "UTC"
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		c.log.Printf("invalid dashboard timezone %q; falling back to UTC", tzName)
		loc = time.UTC
	}

	nowLocal := time.Now().In(loc)
	endLocal := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 23, 59, 59, 0, loc)
	startLocal := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -30)
	const tsFmt = "2006-01-02 15:04:05"
	startUTC := startLocal.UTC().Format(tsFmt)
	endUTC := endLocal.UTC().Format(tsFmt)

	type clickRow struct {
		Created string `db:"created"`
	}
	var clicks []clickRow
	if err := c.db.Select(&clicks, `
		SELECT created
		FROM link_clicks
		WHERE datetime(created) >= datetime(?) AND datetime(created) <= datetime(?)`, startUTC, endUTC); err != nil {
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard charts", "error", pqErrMsg(err)))
	}

	type viewRow struct {
		RowID      int64          `db:"rowid"`
		CampaignID string         `db:"campaign_id"`
		Subscriber sql.NullString `db:"subscriber_id"`
		Suspected  int            `db:"suspected"`
		Created    string         `db:"created"`
	}
	var views []viewRow
	if err := c.db.Select(&views, `
		SELECT rowid, campaign_id, subscriber_id, COALESCE(is_suspected_privacy_open, 0) AS suspected, created
		FROM campaign_views
		WHERE datetime(created) >= datetime(?) AND datetime(created) <= datetime(?)`, startUTC, endUTC); err != nil {
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard charts", "error", pqErrMsg(err)))
	}

	linkClicksByDay := map[string]int{}
	for _, row := range clicks {
		t, err := parseDashboardTimestamp(row.Created)
		if err != nil {
			continue
		}
		day := t.In(loc).Format("2006-01-02")
		linkClicksByDay[day]++
	}

	confirmedUniqueByDay := map[string]int{}
	rawUniqueByDay := map[string]int{}
	rawAllByDay := map[string]int{}
	suspectedUniqueByDay := map[string]int{}
	confirmedSeen := map[string]struct{}{}
	rawSeen := map[string]struct{}{}
	suspectedSeen := map[string]struct{}{}

	for _, row := range views {
		t, err := parseDashboardTimestamp(row.Created)
		if err != nil {
			continue
		}
		day := t.In(loc).Format("2006-01-02")
		rawAllByDay[day]++

		identity := ""
		if row.Subscriber.Valid {
			identity = row.CampaignID + ":sub:" + row.Subscriber.String
		} else {
			identity = row.CampaignID + ":anon:" + stringInt64(row.RowID)
		}
		key := day + "|" + identity

		if _, ok := rawSeen[key]; !ok {
			rawSeen[key] = struct{}{}
			rawUniqueByDay[day]++
		}

		if row.Suspected == 1 {
			if _, ok := suspectedSeen[key]; !ok {
				suspectedSeen[key] = struct{}{}
				suspectedUniqueByDay[day]++
			}
			continue
		}

		if _, ok := confirmedSeen[key]; !ok {
			confirmedSeen[key] = struct{}{}
			confirmedUniqueByDay[day]++
		}
	}

	type chartPoint struct {
		Count int    `json:"count"`
		Date  string `json:"date"`
	}
	toPoints := func(m map[string]int) []chartPoint {
		dates := make([]string, 0, len(m))
		for day := range m {
			dates = append(dates, day)
		}
		sort.Strings(dates)
		out := make([]chartPoint, 0, len(dates))
		for _, day := range dates {
			out = append(out, chartPoint{Count: m[day], Date: day})
		}
		return out
	}

	payload := map[string]any{
		"link_clicks":              toPoints(linkClicksByDay),
		"campaign_views":           toPoints(confirmedUniqueByDay),
		"campaign_views_all_raw":   toPoints(rawAllByDay),
		"campaign_views_raw":       toPoints(rawUniqueByDay),
		"campaign_views_suspected": toPoints(suspectedUniqueByDay),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard charts", "error", pqErrMsg(err)))
	}
	return types.JSONText(b), nil
}

func stringInt64(v int64) string {
	return strconv.FormatInt(v, 10)
}

func parseDashboardTimestamp(value string) (time.Time, error) {
	normalized, err := normalizeAnalyticsDateInput(value, false)
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse("2006-01-02 15:04:05", normalized)
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
		'unsubscribes', COALESCE((SELECT COUNT(*) FROM campaign_unsubscribes), 0),
		'messages', COALESCE((SELECT SUM(sent) FROM campaigns), 0)
	) AS data`

	var out types.JSONText
	if err := c.db.Get(&out, q); err != nil {
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard stats", "error", pqErrMsg(err)))
	}

	return out, nil
}
