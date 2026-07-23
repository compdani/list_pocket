package core

import (
	"encoding/json"
	"github.com/compdani/list_pocket/internal/apperr"
	"strings"

	"github.com/compdani/list_pocket/models"
	"github.com/gofrs/uuid/v5"
	"github.com/lib/pq"
	pbcore "github.com/pocketbase/pocketbase/core"
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

func (c *Core) ResolveListRecordIDs(listIDs []int) ([]string, error) {
	if len(listIDs) == 0 {
		return []string{}, nil
	}

	query := `SELECT id FROM lists WHERE rowid IN (` + sqlitePlaceholders(len(listIDs)) + `)`
	args := make([]any, 0, len(listIDs))
	for _, id := range listIDs {
		args = append(args, id)
	}

	var out []string
	if err := c.db.Select(&out, query, args...); err != nil {
		return nil, err
	}
	return out, nil
}

// GetLists gets all lists optionally filtered by type and status.
func (c *Core) GetLists(typ, status string, getAll bool, permittedIDs []int) ([]models.List, error) {
	return c.getListsSQLite(typ, status, getAll, permittedIDs)
}

// QueryLists gets multiple lists based on multiple query params. Along with the  paginated and sliced
// results, the total number of lists in the DB is returned.
func (c *Core) QueryLists(searchStr, typ, optin, status string, tags []string, orderBy, order string, getAll bool, permittedIDs []int, offset, limit int) ([]models.List, int, error) {
	return c.queryListsSQLite(searchStr, typ, optin, status, tags, orderBy, order, getAll, permittedIDs, offset, limit)
}

// GetList gets a list by its record ID or UUID.
func (c *Core) GetList(recordID, uuid string) (models.List, error) {
	return c.getListSQLite(recordID, uuid)
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
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
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
		return nil, 0, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
	}

	out := sqliteListRowsToModels(rows)
	total := 0
	if len(out) > 0 {
		total = out[0].Total
	}

	return out, total, nil
}

func (c *Core) getListSQLite(recordID, uuid string) (models.List, error) {
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
	if recordID != "" {
		query += ` AND l.id = ?`
		args = append(args, recordID)
	}
	if uuid != "" {
		query += ` AND l.uuid = ?`
		args = append(args, uuid)
	}
	query += ` LIMIT 1`

	var rows []sqliteListRow
	if err := c.db.Select(&rows, query, args...); err != nil {
		c.log.Printf("error fetching lists: %v", err)
		return models.List{}, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
	}
	if len(rows) == 0 {
		return models.List{}, apperr.BadRequest(c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.list}"))
	}

	out := sqliteListRowsToModels(rows)
	return out[0], nil
}

// GetListsByOptin returns lists by optin type.
func (c *Core) GetListsByOptin(ids []int, optinType string) ([]models.List, error) {
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
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.list}", "error", pqErrMsg(err)))
	}

	return sqliteListRowsToModels(rows), nil
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
		return nil, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.list}", "error", pqErrMsg(err)))
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

// CreateList creates a new list via the PocketBase Record API.
func (c *Core) CreateList(l models.List) (models.List, error) {
	uu, err := uuid.NewV4()
	if err != nil {
		c.log.Printf("error generating UUID: %v", err)
		return models.List{}, apperr.Internal(c.i18n.Ts("globals.messages.errorUUID", "error", err.Error()))
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

	pb := c.db.PocketBase()
	if pb == nil {
		return models.List{}, apperr.Internal(c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.list}", "error", "pocketbase is not initialized"))
	}

	col, err := pb.FindCollectionByNameOrId("lists")
	if err != nil {
		c.log.Printf("error finding lists collection: %v", err)
		return models.List{}, apperr.Internal(c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.list}", "error", pqErrMsg(err)))
	}

	rec := pbcore.NewRecord(col)
	rec.Set("uuid", uu.String())
	rec.Set("name", l.Name)
	rec.Set("type", l.Type)
	rec.Set("optin", l.Optin)
	rec.Set("status", l.Status)
	rec.Set("tags", normalizeTags(l.Tags))
	rec.Set("description", l.Description)

	if err := pb.Save(rec); err != nil {
		c.log.Printf("error creating list: %v", err)
		return models.List{}, apperr.Internal(c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.list}", "error", pqErrMsg(err)))
	}

	return c.GetList(rec.Id, "")
}

// UpdateList updates a given list via the PocketBase Record API.
func (c *Core) UpdateList(recordID string, l models.List) (models.List, error) {
	recordID = strings.TrimSpace(recordID)
	if recordID == "" {
		return models.List{}, apperr.BadRequest(c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.list}"))
	}

	pb := c.db.PocketBase()
	if pb == nil {
		return models.List{}, apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.list}", "error", "pocketbase is not initialized"))
	}

	rec, err := pb.FindRecordById("lists", recordID)
	if err != nil {
		c.log.Printf("error updating list: %v", err)
		return models.List{}, apperr.BadRequest(c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.list}"))
	}

	if l.Name != "" {
		rec.Set("name", l.Name)
	}
	if l.Type != "" {
		rec.Set("type", l.Type)
	}
	if l.Optin != "" {
		rec.Set("optin", l.Optin)
	}
	if l.Status != "" {
		rec.Set("status", l.Status)
	}
	rec.Set("tags", normalizeTags(l.Tags))
	if l.Description != "" {
		rec.Set("description", l.Description)
	}

	if err := pb.Save(rec); err != nil {
		c.log.Printf("error updating list: %v", err)
		return models.List{}, apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.list}", "error", pqErrMsg(err)))
	}

	return c.GetList(recordID, "")
}

// DeleteList deletes a list.
func (c *Core) DeleteList(recordID string) error {
	return c.DeleteLists([]string{recordID}, "", true, nil)
}

// DeleteLists deletes multiple lists via the PocketBase Record API.
// Query/filter matching still uses SQL to resolve record ids; deletes go through PB.
func (c *Core) DeleteLists(recordIDs []string, query string, getAll bool, permittedIDs []int) error {
	ids := make([]string, 0, len(recordIDs))
	for _, recordID := range recordIDs {
		recordID = strings.TrimSpace(recordID)
		if recordID != "" {
			ids = append(ids, recordID)
		}
	}

	if len(ids) == 0 {
		q := `SELECT id FROM lists WHERE 1=1`
		args := []any{}

		queryStr := strings.TrimSpace(query)
		if queryStr != "" {
			q += ` AND name LIKE ?`
			args = append(args, "%"+queryStr+"%")
		}

		if !getAll {
			if len(permittedIDs) == 0 {
				return nil
			}
			q += ` AND rowid IN (` + sqlitePlaceholders(len(permittedIDs)) + `)`
			for _, id := range permittedIDs {
				args = append(args, id)
			}
		}

		if err := c.db.Select(&ids, q, args...); err != nil {
			c.log.Printf("error resolving lists for delete: %v", err)
			return apperr.Internal(c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
		}
	}

	if len(ids) == 0 {
		return nil
	}

	pb := c.db.PocketBase()
	if pb == nil {
		return apperr.Internal(c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.lists}", "error", "pocketbase is not initialized"))
	}

	for _, id := range ids {
		rec, err := pb.FindRecordById("lists", id)
		if err != nil {
			// Skip missing records so bulk delete remains idempotent.
			continue
		}
		if err := pb.Delete(rec); err != nil {
			c.log.Printf("error deleting list %s: %v", id, err)
			return apperr.Internal(c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
		}
	}
	return nil
}
