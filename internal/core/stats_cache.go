package core

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/compdani/list_pocket/models"
	"github.com/jmoiron/sqlx/types"
	"golang.org/x/sync/singleflight"
)

const (
	cacheKeyDashboardCounts      = "cache.dashboard_counts"
	cacheKeyListSubscriberCounts = "cache.list_subscriber_counts"

	statsCacheTable         = "listpocket_stats_cache"
	statsCacheRowDashboard  = "cDashboardCnts"
	statsCacheRowListCounts = "cListCountBlobs"
)

type cachedListCounts struct {
	Count    int                 `json:"count"`
	Statuses models.StringIntMap `json:"statuses"`
}

type statsCacheMem struct {
	mu         sync.RWMutex
	dashboard  types.JSONText
	dashOK     bool
	listCounts map[string]cachedListCounts
	listsOK    bool
	sf         singleflight.Group
}

func (c *Core) memDashboardCounts() (types.JSONText, bool) {
	c.stats.mu.RLock()
	defer c.stats.mu.RUnlock()
	if !c.stats.dashOK || len(c.stats.dashboard) == 0 {
		return nil, false
	}
	out := make(types.JSONText, len(c.stats.dashboard))
	copy(out, c.stats.dashboard)
	return out, true
}

func (c *Core) setMemDashboardCounts(v types.JSONText) {
	c.stats.mu.Lock()
	defer c.stats.mu.Unlock()
	c.stats.dashboard = append(types.JSONText(nil), v...)
	c.stats.dashOK = len(v) > 0
}

func (c *Core) memListCounts() (map[string]cachedListCounts, bool) {
	c.stats.mu.RLock()
	defer c.stats.mu.RUnlock()
	if !c.stats.listsOK || c.stats.listCounts == nil {
		return nil, false
	}
	out := make(map[string]cachedListCounts, len(c.stats.listCounts))
	for k, v := range c.stats.listCounts {
		statuses := models.StringIntMap{}
		for sk, sv := range v.Statuses {
			statuses[sk] = sv
		}
		out[k] = cachedListCounts{Count: v.Count, Statuses: statuses}
	}
	return out, true
}

func (c *Core) setMemListCounts(in map[string]cachedListCounts) {
	c.stats.mu.Lock()
	defer c.stats.mu.Unlock()
	c.stats.listCounts = in
	c.stats.listsOK = in != nil
}

func (c *Core) readCachedJSON(key string) (types.JSONText, bool) {
	if c.db == nil {
		return nil, false
	}
	var value string
	if err := c.db.Get(&value, `SELECT value FROM `+statsCacheTable+` WHERE cache_key = ? LIMIT 1`, key); err != nil {
		return nil, false
	}
	if strings.TrimSpace(value) == "" || value == "null" {
		return nil, false
	}
	return types.JSONText(value), true
}

func (c *Core) writeCachedJSON(key string, value types.JSONText) error {
	if c.db == nil {
		return nil
	}
	rowID := statsCacheRowDashboard
	if key == cacheKeyListSubscriberCounts {
		rowID = statsCacheRowListCounts
	}
	_, err := c.db.Exec(`
		INSERT INTO `+statsCacheTable+` (id, cache_key, value, created, updated)
		VALUES (?, ?, ?, strftime('%Y-%m-%d %H:%M:%fZ'), strftime('%Y-%m-%d %H:%M:%fZ'))
		ON CONFLICT(cache_key) DO UPDATE SET
			value = excluded.value,
			updated = strftime('%Y-%m-%d %H:%M:%fZ')
	`, rowID, key, string(value))
	return err
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
	_, err, _ := c.stats.sf.Do("refresh", func() (any, error) {
		return nil, c.refreshMatViewsOnce()
	})
	return err
}

func (c *Core) refreshMatViewsOnce() error {
	counts, err := c.getDashboardCountsSQLite()
	if err != nil {
		return err
	}
	c.setMemDashboardCounts(counts)
	if err := c.writeCachedJSON(cacheKeyDashboardCounts, counts); err != nil {
		if c.log != nil {
			c.log.Printf("error persisting dashboard count cache: %v", err)
		}
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
	payload := types.JSONText(b)
	c.setMemListCounts(out)
	if err := c.writeCachedJSON(cacheKeyListSubscriberCounts, payload); err != nil {
		if c.log != nil {
			c.log.Printf("error persisting list count cache: %v", err)
		}
	}
	return nil
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
		if cached, ok := c.memListCounts(); ok {
			applyCachedListCounts(lists, cached)
			return nil
		}
		if cached, ok := c.readCachedListCounts(); ok {
			c.setMemListCounts(cached)
			applyCachedListCounts(lists, cached)
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

func applyCachedListCounts(lists []models.List, cached map[string]cachedListCounts) {
	for i := range lists {
		if v, found := cached[lists[i].RecordID]; found {
			lists[i].SubscriberCount = v.Count
			lists[i].SubscriberCounts = v.Statuses
		}
	}
}
