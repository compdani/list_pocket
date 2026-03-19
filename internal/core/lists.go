package core

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/compdani/list_pocket/models"
	"github.com/gofrs/uuid/v5"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

type listType struct {
	ID   int    `json:"id"`
	UUID string `json:"uuid"`
	Type string `json:"type"`
}

type sqliteListRow struct {
	ID               int                 `db:"id"`
	RecordID         string              `db:"record_id"`
	CreatedAt        string              `db:"created_at"`
	UpdatedAt        string              `db:"updated_at"`
	UUID             string              `db:"uuid"`
	Name             string              `db:"name"`
	Type             string              `db:"type"`
	Optin            string              `db:"optin"`
	Status           string              `db:"status"`
	Tags             []byte              `db:"tags"`
	Description      string              `db:"description"`
	SubscriberCount  int                 `db:"subscriber_count"`
	SubscriberCounts models.StringIntMap `db:"subscriber_statuses"`
	Total            int                 `db:"total"`
}

func sqliteListRowsToModels(rows []sqliteListRow) []models.List {
	out := make([]models.List, 0, len(rows))
	for _, row := range rows {
		tags := pq.StringArray{}
		if len(row.Tags) > 0 && string(row.Tags) != "null" {
			_ = json.Unmarshal(row.Tags, &tags)
		}

		out = append(out, models.List{
			Base: models.Base{
				ID:        row.ID,
				RecordID:  row.RecordID,
				CreatedAt: parseNullTime(row.CreatedAt),
				UpdatedAt: parseNullTime(row.UpdatedAt),
			},
			UUID:             row.UUID,
			Name:             row.Name,
			Type:             row.Type,
			Optin:            row.Optin,
			Status:           row.Status,
			Tags:             tags,
			Description:      row.Description,
			SubscriberCount:  row.SubscriberCount,
			SubscriberCounts: row.SubscriberCounts,
			Total:            row.Total,
		})
	}
	return out
}

// GetLists gets all lists optionally filtered by type and status.
func (c *Core) GetLists(typ, status string, getAll bool, permittedIDs []int) ([]models.List, error) {
	if c.isSQLite() {
		return c.getListsSQLite(typ, status, getAll, permittedIDs)
	}

	out := []models.List{}

	if err := c.q.GetLists.Select(&out, typ, status, "id", getAll, pq.Array(permittedIDs)); err != nil {
		c.log.Printf("error fetching lists: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
	}

	// Replace null tags.
	for i, l := range out {
		if l.Tags == nil {
			out[i].Tags = []string{}
		}

		// Total counts.
		for _, c := range l.SubscriberCounts {
			out[i].SubscriberCount += c
		}
	}

	return out, nil
}

// QueryLists gets multiple lists based on multiple query params. Along with the  paginated and sliced
// results, the total number of lists in the DB is returned.
func (c *Core) QueryLists(searchStr, typ, optin, status string, tags []string, orderBy, order string, getAll bool, permittedIDs []int, offset, limit int) ([]models.List, int, error) {
	if c.isSQLite() {
		return c.queryListsSQLite(searchStr, typ, optin, status, tags, orderBy, order, getAll, permittedIDs, offset, limit)
	}

	_ = c.refreshCache(matListSubStats, false)

	if tags == nil {
		tags = []string{}
	}

	var (
		out            = []models.List{}
		queryStr, stmt = makeSearchQuery(searchStr, orderBy, order, c.q.QueryLists, listQuerySortFields)
	)
	if err := c.db.Select(&out, stmt, 0, "", queryStr, typ, optin, status, pq.StringArray(tags), getAll, pq.Array(permittedIDs), offset, limit); err != nil {
		c.log.Printf("error fetching lists: %v", err)
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
	}

	total := 0
	if len(out) > 0 {
		total = out[0].Total

		// Replace null tags.
		for i, l := range out {
			if l.Tags == nil {
				out[i].Tags = []string{}
			}
		}
	}

	return out, total, nil
}

// GetList gets a list by its ID or UUID.
func (c *Core) GetList(id int, uuid string) (models.List, error) {
	if c.isSQLite() {
		return c.getListSQLite(id, uuid)
	}

	var uu any
	if uuid != "" {
		uu = uuid
	}

	var res []models.List
	queryStr, stmt := makeSearchQuery("", "", "", c.q.QueryLists, nil)
	if err := c.db.Select(&res, stmt, id, uu, queryStr, "", "", "", pq.StringArray{}, true, nil, 0, 1); err != nil {
		c.log.Printf("error fetching lists: %v", err)
		return models.List{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
	}

	if len(res) == 0 {
		return models.List{}, echo.NewHTTPError(http.StatusBadRequest,
			c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.list}"))
	}

	out := res[0]
	if out.Tags == nil {
		out.Tags = []string{}
	}
	// Total counts.
	for _, c := range out.SubscriberCounts {
		out.SubscriberCount += c
	}

	return out, nil
}

func (c *Core) getListsSQLite(typ, status string, getAll bool, permittedIDs []int) ([]models.List, error) {
	query := `
	SELECT
		l.rowid AS id,
		l.id AS record_id,
		l.created AS created_at,
		l.updated AS updated_at,
		l.uuid,
		l.name,
		l.type,
		l.optin,
		l.status,
		l.tags,
		l.description,
		COALESCE(ss.subscriber_statuses, '{}') AS subscriber_statuses,
		COALESCE(ss.subscriber_count, 0) AS subscriber_count
	FROM lists l
	LEFT JOIN (
		SELECT
			list_id,
			COALESCE(json_group_object(status, subscriber_count), '{}') AS subscriber_statuses,
			COALESCE(SUM(subscriber_count), 0) AS subscriber_count
		FROM (
			SELECT sl.list_id, sl.status, COUNT(*) AS subscriber_count
			FROM subscriber_lists sl
			GROUP BY sl.list_id, sl.status
		)
		GROUP BY list_id
	) ss ON ss.list_id = l.id
	WHERE 1=1`

	args := make([]any, 0, 2+len(permittedIDs))
	if typ != "" {
		query += ` AND l.type = ?`
		args = append(args, typ)
	}
	if status != "" {
		query += ` AND l.status = ?`
		args = append(args, status)
	}
	if !getAll {
		if len(permittedIDs) == 0 {
			query += ` AND 1=0`
		} else {
			query += ` AND l.rowid IN (` + sqlitePlaceholders(len(permittedIDs)) + `)`
			for _, id := range permittedIDs {
				args = append(args, id)
			}
		}
	}
	query += ` ORDER BY l.id ASC`

	rows := []sqliteListRow{}
	if err := c.db.Select(&rows, query, args...); err != nil {
		c.log.Printf("error fetching lists: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
	}

	return sqliteListRowsToModels(rows), nil
}

func (c *Core) queryListsSQLite(searchStr, typ, optin, status string, tags []string, orderBy, order string, getAll bool, permittedIDs []int, offset, limit int) ([]models.List, int, error) {
	query := `
	SELECT
		COUNT(*) OVER() AS total,
		l.rowid AS id,
		l.id AS record_id,
		l.created AS created_at,
		l.updated AS updated_at,
		l.uuid,
		l.name,
		l.type,
		l.optin,
		l.status,
		l.tags,
		l.description,
		COALESCE(ss.subscriber_statuses, '{}') AS subscriber_statuses,
		COALESCE(ss.subscriber_count, 0) AS subscriber_count
	FROM lists l
	LEFT JOIN (
		SELECT
			list_id,
			COALESCE(json_group_object(status, subscriber_count), '{}') AS subscriber_statuses,
			COALESCE(SUM(subscriber_count), 0) AS subscriber_count
		FROM (
			SELECT sl.list_id, sl.status, COUNT(*) AS subscriber_count
			FROM subscriber_lists sl
			GROUP BY sl.list_id, sl.status
		)
		GROUP BY list_id
	) ss ON ss.list_id = l.id
	WHERE 1=1`

	args := make([]any, 0, 6+len(tags)+len(permittedIDs))
	if searchStr != "" {
		query += ` AND l.name LIKE ?`
		args = append(args, "%"+searchStr+"%")
	}
	if typ != "" {
		query += ` AND l.type = ?`
		args = append(args, typ)
	}
	if optin != "" {
		query += ` AND l.optin = ?`
		args = append(args, optin)
	}
	if status != "" {
		query += ` AND l.status = ?`
		args = append(args, status)
	}
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		query += ` AND INSTR(l.tags, ?) > 0`
		args = append(args, t)
	}
	if !getAll {
		if len(permittedIDs) == 0 {
			query += ` AND 1=0`
		} else {
			query += ` AND l.rowid IN (` + sqlitePlaceholders(len(permittedIDs)) + `)`
			for _, id := range permittedIDs {
				args = append(args, id)
			}
		}
	}

	if !strSliceContains(orderBy, listQuerySortFields) {
		orderBy = "created_at"
	}
	if order != SortAsc && order != SortDesc {
		order = SortDesc
	}
	switch orderBy {
	case "subscriber_count":
		query += ` ORDER BY subscriber_count ` + strings.ToUpper(order)
	case "created_at":
		query += ` ORDER BY l.created ` + strings.ToUpper(order)
	case "updated_at":
		query += ` ORDER BY l.updated ` + strings.ToUpper(order)
	default:
		query += ` ORDER BY l.` + orderBy + ` ` + strings.ToUpper(order)
	}

	if limit > 0 {
		query += ` LIMIT ? OFFSET ?`
		args = append(args, limit, offset)
	}

	rows := []sqliteListRow{}
	if err := c.db.Select(&rows, query, args...); err != nil {
		c.log.Printf("error fetching lists: %v", err)
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
	}

	out := sqliteListRowsToModels(rows)
	total := 0
	if len(out) > 0 {
		total = out[0].Total
	}

	return out, total, nil
}

func (c *Core) getListSQLite(id int, uuid string) (models.List, error) {
	query := `
	SELECT
		l.rowid AS id,
		l.id AS record_id,
		l.created AS created_at,
		l.updated AS updated_at,
		l.uuid,
		l.name,
		l.type,
		l.optin,
		l.status,
		l.tags,
		l.description,
		COALESCE(ss.subscriber_statuses, '{}') AS subscriber_statuses,
		COALESCE(ss.subscriber_count, 0) AS subscriber_count
	FROM lists l
	LEFT JOIN (
		SELECT
			list_id,
			COALESCE(json_group_object(status, subscriber_count), '{}') AS subscriber_statuses,
			COALESCE(SUM(subscriber_count), 0) AS subscriber_count
		FROM (
			SELECT sl.list_id, sl.status, COUNT(*) AS subscriber_count
			FROM subscriber_lists sl
			GROUP BY sl.list_id, sl.status
		)
		GROUP BY list_id
	) ss ON ss.list_id = l.id
	WHERE 1=1`

	args := make([]any, 0, 2)
	if id > 0 {
		query += ` AND l.rowid = ?`
		args = append(args, id)
	}
	if uuid != "" {
		query += ` AND l.uuid = ?`
		args = append(args, uuid)
	}
	query += ` LIMIT 1`

	var rows []sqliteListRow
	if err := c.db.Select(&rows, query, args...); err != nil {
		c.log.Printf("error fetching lists: %v", err)
		return models.List{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
	}
	if len(rows) == 0 {
		return models.List{}, echo.NewHTTPError(http.StatusBadRequest,
			c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.list}"))
	}

	out := sqliteListRowsToModels(rows)
	return out[0], nil
}

// GetListsByOptin returns lists by optin type.
func (c *Core) GetListsByOptin(ids []int, optinType string) ([]models.List, error) {
	if c.isSQLite() {
		rows := []sqliteListRow{}
		q := `
			SELECT
				l.rowid AS id,
				l.id AS record_id,
				l.created AS created_at,
				l.updated AS updated_at,
				l.uuid,
				l.name,
				l.type,
				l.optin,
				l.status,
				l.tags,
				l.description,
				0 AS subscriber_count,
				'{}' AS subscriber_statuses,
				0 AS total
			FROM lists l
			WHERE l.optin = ?
		`
		args := []any{optinType}
		if len(ids) > 0 {
			q += ` AND l.rowid IN (` + sqlitePlaceholders(len(ids)) + `)`
			for _, id := range ids {
				args = append(args, id)
			}
		}
		q += ` ORDER BY l.rowid`

		if err := c.db.Select(&rows, q, args...); err != nil {
			c.log.Printf("error fetching lists for opt-in: %s", pqErrMsg(err))
			return nil, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.list}", "error", pqErrMsg(err)))
		}

		return sqliteListRowsToModels(rows), nil
	}

	out := []models.List{}
	if err := c.q.GetListsByOptin.Select(&out, optinType, pq.Array(ids), nil); err != nil {
		c.log.Printf("error fetching lists for opt-in: %s", pqErrMsg(err))
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.list}", "error", pqErrMsg(err)))
	}

	return out, nil
}

// GetListTypes returns lists by their IDs or UUIDs.
// If ids is given, then the map returned has the list IDs as keys,
// otherwise, they have UUIDs as the keys.
// Note: This is a really weird and awkward API. Ideally, Go Generics
// should've somehow supported generic struct methods.
func (c *Core) GetListTypes(ids []int, uuids []string) (map[any]string, error) {
	res := []listType{}

	out := map[any]string{}
	if len(ids) == 0 && len(uuids) == 0 {
		return out, nil
	}

	if c.isSQLite() {
		q := `SELECT rowid AS id, uuid, type FROM lists WHERE `
		args := make([]any, 0, max(len(ids), len(uuids)))

		switch {
		case len(ids) > 0:
			q += `rowid IN (` + sqlitePlaceholders(len(ids)) + `)`
			for _, id := range ids {
				args = append(args, id)
			}
		case len(uuids) > 0:
			q += `uuid IN (` + sqlitePlaceholders(len(uuids)) + `)`
			for _, uuid := range uuids {
				args = append(args, uuid)
			}
		}

		if err := c.db.Select(&res, q, args...); err != nil {
			c.log.Printf("error fetching list types: %v", err)
			return nil, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.list}", "error", pqErrMsg(err)))
		}

		isIDs := ids != nil
		for _, r := range res {
			if isIDs {
				out[r.ID] = r.Type
			} else {
				out[r.UUID] = r.Type
			}
		}

		return out, nil
	}

	if err := c.q.GetListTypes.Select(&res, pq.Array(ids), pq.StringArray(uuids)); err != nil {
		c.log.Printf("error fetching list types: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.list}", "error", pqErrMsg(err)))
	}

	isIDs := ids != nil
	for _, r := range res {
		if isIDs {
			out[r.ID] = r.Type
		} else {
			out[r.UUID] = r.Type
		}
	}

	return out, nil
}

// CreateList creates a new list.
func (c *Core) CreateList(l models.List) (models.List, error) {
	uu, err := uuid.NewV4()
	if err != nil {
		c.log.Printf("error generating UUID: %v", err)
		return models.List{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUUID", "error", err.Error()))
	}

	if l.Type == "" {
		l.Type = models.ListTypePrivate
	}
	if l.Optin == "" {
		l.Optin = models.ListOptinSingle
	}
	if l.Status == "" {
		l.Status = models.ListStatusActive
	}

	l.UUID = uu.String()
	if c.isSQLite() {
		tags, err := json.Marshal(normalizeTags(l.Tags))
		if err != nil {
			c.log.Printf("error marshaling list tags: %v", err)
			return models.List{}, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.list}", "error", err.Error()))
		}

		if _, err := c.db.Exec(`
			INSERT INTO lists (uuid, name, type, optin, status, tags, description)
			VALUES (?, ?, ?, ?, ?, json(?), ?)`,
			l.UUID, l.Name, l.Type, l.Optin, l.Status, string(tags), l.Description); err != nil {
			c.log.Printf("error creating list: %v", err)
			return models.List{}, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.list}", "error", pqErrMsg(err)))
		}

		return c.GetList(0, l.UUID)
	}

	// Insert and read ID.
	var newID int
	if err := c.q.CreateList.Get(&newID, l.UUID, l.Name, l.Type, l.Optin, l.Status, pq.StringArray(normalizeTags(l.Tags)), l.Description); err != nil {
		c.log.Printf("error creating list: %v", err)
		return models.List{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.list}", "error", pqErrMsg(err)))
	}

	return c.GetList(newID, "")
}

// UpdateList updates a given list.
func (c *Core) UpdateList(id int, l models.List) (models.List, error) {
	if c.isSQLite() {
		tags, err := json.Marshal(normalizeTags(l.Tags))
		if err != nil {
			c.log.Printf("error marshaling list tags: %v", err)
			return models.List{}, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.list}", "error", err.Error()))
		}

		res, err := c.db.Exec(`
			UPDATE lists SET
				name=(CASE WHEN ? != '' THEN ? ELSE name END),
				type=(CASE WHEN ? != '' THEN ? ELSE type END),
				optin=(CASE WHEN ? != '' THEN ? ELSE optin END),
				status=(CASE WHEN ? != '' THEN ? ELSE status END),
				tags=json(?),
				description=(CASE WHEN ? != '' THEN ? ELSE description END),
				updated=strftime('%Y-%m-%d %H:%M:%fZ', 'now')
			WHERE rowid = ?`,
			l.Name, l.Name, l.Type, l.Type, l.Optin, l.Optin, l.Status, l.Status, string(tags), l.Description, l.Description, id)
		if err != nil {
			c.log.Printf("error updating list: %v", err)
			return models.List{}, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.list}", "error", pqErrMsg(err)))
		}

		if n, _ := res.RowsAffected(); n == 0 {
			return models.List{}, echo.NewHTTPError(http.StatusBadRequest,
				c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.list}"))
		}

		return c.GetList(id, "")
	}

	res, err := c.q.UpdateList.Exec(id, l.Name, l.Type, l.Optin, l.Status, pq.StringArray(normalizeTags(l.Tags)), l.Description)
	if err != nil {
		c.log.Printf("error updating list: %v", err)
		return models.List{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.list}", "error", pqErrMsg(err)))
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return models.List{}, echo.NewHTTPError(http.StatusBadRequest,
			c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.list}"))
	}

	return c.GetList(id, "")
}

// DeleteList deletes a list.
func (c *Core) DeleteList(id int) error {
	return c.DeleteLists([]int{id}, "", true, nil)
}

// DeleteLists deletes multiple lists.
func (c *Core) DeleteLists(ids []int, query string, getAll bool, permittedIDs []int) error {
	if c.isSQLite() {
		q := `DELETE FROM lists WHERE 1=1`
		args := []any{}

		if len(ids) > 0 {
			q += ` AND rowid IN (` + sqlitePlaceholders(len(ids)) + `)`
			for _, id := range ids {
				args = append(args, id)
			}
		} else {
			queryStr := strings.TrimSpace(query)
			if queryStr != "" {
				q += ` AND name LIKE ?`
				args = append(args, "%"+queryStr+"%")
			}
		}

		if !getAll {
			if len(permittedIDs) == 0 {
				q += ` AND 1=0`
			} else {
				q += ` AND rowid IN (` + sqlitePlaceholders(len(permittedIDs)) + `)`
				for _, id := range permittedIDs {
					args = append(args, id)
				}
			}
		}

		if _, err := c.db.Exec(q, args...); err != nil {
			c.log.Printf("error deleting lists: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
		}
		return nil
	}

	var queryStr string

	if len(ids) > 0 {
		queryStr = ""
	} else {
		queryStr = makeSearchString(query)
	}

	if _, err := c.q.DeleteLists.Exec(pq.Array(ids), queryStr, getAll, pq.Array(permittedIDs)); err != nil {
		c.log.Printf("error deleting lists: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
	}
	return nil
}
