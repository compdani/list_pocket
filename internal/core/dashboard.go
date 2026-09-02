package core

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/apperr"
	"github.com/jmoiron/sqlx/types"
)

// GetDashboardCharts returns chart data points to render on the dashboard.
func (c *Core) GetDashboardCharts(timeZone string) (types.JSONText, error) {
	return c.getDashboardChartsSQLite(timeZone)
}

// GetDashboardCounts returns stats counts to show on the dashboard.
func (c *Core) GetDashboardCounts() (types.JSONText, error) {
	if c.consts.CacheSlowQueries {
		if cached, ok := c.memDashboardCounts(); ok {
			return cached, nil
		}
		if cached, ok := c.readCachedJSON(cacheKeyDashboardCounts); ok {
			c.setMemDashboardCounts(cached)
			return cached, nil
		}
	}
	v, err, _ := c.stats.sf.Do("dashboard-counts", func() (any, error) {
		return c.getDashboardCountsSQLite()
	})
	if err != nil {
		return nil, err
	}
	return v.(types.JSONText), nil
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
	const tsFmt = "2006-01-02 15:04:05.000Z"
	startUTC := startLocal.UTC().Format(tsFmt)
	endUTC := endLocal.UTC().Format(tsFmt)
	tzOff := sqliteTZOffset(loc)

	type dayCount struct {
		Day   string `db:"day"`
		Count int    `db:"count"`
	}
	var clicks []dayCount
	if err := c.db.Select(&clicks, `
		SELECT strftime('%Y-%m-%d', created, ?) AS day, COUNT(*) AS count
		FROM link_clicks
		WHERE created >= ? AND created <= ?
		GROUP BY day
		ORDER BY day`, tzOff, startUTC, endUTC); err != nil {
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard charts", "error", dbErr(err)))
	}

	type viewDay struct {
		Day             string `db:"day"`
		RawAll          int    `db:"raw_all"`
		RawUnique       int    `db:"raw_unique"`
		ConfirmedUnique int    `db:"confirmed_unique"`
		SuspectedUnique int    `db:"suspected_unique"`
	}
	ident := `campaign_id || ':' || COALESCE(CAST(subscriber_id AS TEXT), 'anon:' || rowid)`
	var views []viewDay
	if err := c.db.Select(&views, `
		SELECT strftime('%Y-%m-%d', created, ?) AS day,
			COUNT(*) AS raw_all,
			COUNT(DISTINCT `+ident+`) AS raw_unique,
			COUNT(DISTINCT CASE WHEN COALESCE(is_suspected_privacy_open, 0) = 0 THEN `+ident+` END) AS confirmed_unique,
			COUNT(DISTINCT CASE WHEN COALESCE(is_suspected_privacy_open, 0) = 1 THEN `+ident+` END) AS suspected_unique
		FROM campaign_views
		WHERE created >= ? AND created <= ?
		GROUP BY day
		ORDER BY day`, tzOff, startUTC, endUTC); err != nil {
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard charts", "error", dbErr(err)))
	}

	type chartPoint struct {
		Count int    `json:"count"`
		Date  string `json:"date"`
	}
	toPoints := func(rows []dayCount) []chartPoint {
		out := make([]chartPoint, 0, len(rows))
		for _, r := range rows {
			if strings.TrimSpace(r.Day) == "" {
				continue
			}
			out = append(out, chartPoint{Count: r.Count, Date: r.Day})
		}
		return out
	}
	viewPoints := func(pick func(viewDay) int) []chartPoint {
		out := make([]chartPoint, 0, len(views))
		for _, r := range views {
			if strings.TrimSpace(r.Day) == "" {
				continue
			}
			out = append(out, chartPoint{Count: pick(r), Date: r.Day})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
		return out
	}

	payload := map[string]any{
		"link_clicks":              toPoints(clicks),
		"campaign_views":           viewPoints(func(r viewDay) int { return r.ConfirmedUnique }),
		"campaign_views_all_raw":   viewPoints(func(r viewDay) int { return r.RawAll }),
		"campaign_views_raw":       viewPoints(func(r viewDay) int { return r.RawUnique }),
		"campaign_views_suspected": viewPoints(func(r viewDay) int { return r.SuspectedUnique }),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard charts", "error", dbErr(err)))
	}
	return types.JSONText(b), nil
}

func sqliteTZOffset(loc *time.Location) string {
	_, off := time.Now().In(loc).Zone()
	sign := "+"
	if off < 0 {
		sign = "-"
		off = -off
	}
	return fmt.Sprintf("%s%02d:%02d", sign, off/3600, (off%3600)/60)
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
				SELECT COUNT(*) FROM subscribers s
				WHERE NOT EXISTS (
					SELECT 1 FROM subscriber_lists sl WHERE sl.subscriber_id = s.id
				)
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
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard stats", "error", dbErr(err)))
	}

	return out, nil
}
