package core

import (
	"encoding/json"
	"strings"

	"github.com/compdani/list_pocket/models"
	"github.com/jmoiron/sqlx/types"
)

const (
	cacheKeyDashboardCounts      = "cache.dashboard_counts"
	cacheKeyListSubscriberCounts = "cache.list_subscriber_counts"
)

type cachedListCounts struct {
	Count    int                 `json:"count"`
	Statuses models.StringIntMap `json:"statuses"`
}

func (c *Core) readCachedJSON(key string) (types.JSONText, bool) {
	if c.getSettings == nil {
		return nil, false
	}
	raw, err := c.getSettings()
	if err != nil || len(raw) == 0 {
		return nil, false
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, false
	}
	v, ok := m[key]
	if !ok || len(v) == 0 || string(v) == "null" {
		return nil, false
	}
	return types.JSONText(v), true
}

func (c *Core) writeCachedJSON(key string, value types.JSONText) error {
	if c.setSettingsByKey == nil {
		return nil
	}
	return c.setSettingsByKey(key, json.RawMessage(value))
}

func (c *Core) readCachedListCounts() (map[string]cachedListCounts, bool) {
	raw, ok := c.readCachedJSON(cacheKeyListSubscriberCounts)
	if !ok {
		return nil, false
	}
	var out map[string]cachedListCounts
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, false
	}
	return out, true
}

// RefreshMatViews refreshes cached dashboard and list subscriber counts.
func (c *Core) RefreshMatViews(_ bool) error {
	counts, err := c.getDashboardCountsSQLite()
	if err != nil {
		return err
	}
	if err := c.writeCachedJSON(cacheKeyDashboardCounts, counts); err != nil {
		return err
	}

	type row struct {
		ListID string `db:"list_id"`
		Status string `db:"status"`
		Count  int    `db:"subscriber_count"`
	}
	var rows []row
	if err := c.db.Select(&rows, `
		SELECT list_id, status, COUNT(*) AS subscriber_count
		FROM subscriber_lists
		GROUP BY list_id, status
	`); err != nil {
		return err
	}
	out := map[string]cachedListCounts{}
	for _, r := range rows {
		id := strings.TrimSpace(r.ListID)
		if id == "" {
			continue
		}
		cur := out[id]
		if cur.Statuses == nil {
			cur.Statuses = models.StringIntMap{}
		}
		cur.Statuses[r.Status] = r.Count
		cur.Count += r.Count
		out[id] = cur
	}
	b, err := json.Marshal(out)
	if err != nil {
		return err
	}
	return c.writeCachedJSON(cacheKeyListSubscriberCounts, types.JSONText(b))
}

// RefreshMatView refreshes a single cache blob. Empty name refreshes all.
func (c *Core) RefreshMatView(name string, concurrent bool) error {
	_ = name
	return c.RefreshMatViews(concurrent)
}

func (c *Core) attachListSubscriberCounts(lists []models.List) error {
	if len(lists) == 0 {
		return nil
	}
	if c.consts.CacheSlowQueries {
		if cached, ok := c.readCachedListCounts(); ok {
			for i := range lists {
				if v, found := cached[lists[i].RecordID]; found {
					lists[i].SubscriberCount = v.Count
					lists[i].SubscriberCounts = v.Statuses
				}
			}
			return nil
		}
	}

	ids := make([]string, 0, len(lists))
	idx := make(map[string]int, len(lists))
	for i, l := range lists {
		if l.RecordID == "" {
			continue
		}
		idx[l.RecordID] = i
		ids = append(ids, l.RecordID)
	}
	if len(ids) == 0 {
		return nil
	}

	type row struct {
		ListID string `db:"list_id"`
		Status string `db:"status"`
		Count  int    `db:"subscriber_count"`
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	var rows []row
	if err := c.db.Select(&rows, `
		SELECT list_id, status, COUNT(*) AS subscriber_count
		FROM subscriber_lists
		WHERE list_id IN (`+sqlitePlaceholders(len(ids))+`)
		GROUP BY list_id, status
	`, args...); err != nil {
		return err
	}
	for _, r := range rows {
		i, ok := idx[r.ListID]
		if !ok {
			continue
		}
		if lists[i].SubscriberCounts == nil {
			lists[i].SubscriberCounts = models.StringIntMap{}
		}
		lists[i].SubscriberCounts[r.Status] = r.Count
		lists[i].SubscriberCount += r.Count
	}
	return nil
}
