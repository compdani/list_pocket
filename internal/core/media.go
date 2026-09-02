package core

import (
	"database/sql"
	"encoding/json"
	"github.com/compdani/list_pocket/internal/apperr"
	"strings"

	"github.com/compdani/list_pocket/internal/media"
	"github.com/compdani/list_pocket/models"
	"github.com/gofrs/uuid/v5"
	"gopkg.in/volatiletech/null.v6"
)

type sqliteMediaRow struct {
	ID          string `db:"id"`
	RowID       int    `db:"row_id"`
	UUID        string `db:"uuid"`
	Filename    string `db:"filename"`
	ContentType string `db:"content_type"`
	Thumb       string `db:"thumb"`
	CreatedAt   string `db:"created_at"`
	Provider    string `db:"provider"`
	Meta        []byte `db:"meta"`
	Total       int    `db:"total"`
}

func sqliteMediaRowToModel(row sqliteMediaRow) media.Media {
	meta := models.JSON{}
	if len(row.Meta) > 0 && string(row.Meta) != "null" {
		_ = json.Unmarshal(row.Meta, &meta)
	}

	return media.Media{
		ID:          row.ID,
		RowID:       row.RowID,
		UUID:        row.UUID,
		Filename:    row.Filename,
		ContentType: row.ContentType,
		Thumb:       row.Thumb,
		CreatedAt:   parseNullTime(row.CreatedAt),
		Provider:    row.Provider,
		Meta:        meta,
		Total:       row.Total,
	}
}

const sqliteMediaSelect = `
			SELECT
				id,
				rowid AS row_id,
				uuid,
				filename,
				content_type,
				thumb,
				created AS created_at,
				provider,
				meta
`

// QueryMedia returns media entries optionally filtered by a query string.
func (c *Core) QueryMedia(provider string, s media.Store, query string, offset, limit int) ([]media.Media, int, error) {
	rows := []sqliteMediaRow{}
	q := sqliteMediaSelect + `,
				COUNT(*) OVER () AS total
			FROM media
			WHERE provider = ?
		`
	args := []any{provider}
	if query != "" {
		query = "%" + strings.ToLower(query) + "%"
		q += ` AND LOWER(filename) LIKE ?`
		args = append(args, query)
	}
	q += ` ORDER BY created DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	if err := c.db.Select(&rows, q, args...); err != nil {
		return nil, 0, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching",
			"name", "{globals.terms.media}", "error", dbErr(err)))
	}

	out := make([]media.Media, 0, len(rows))
	total := 0
	for _, row := range rows {
		m := sqliteMediaRowToModel(row)
		m.URL = s.GetURL(m.Filename)
		if m.Thumb != "" {
			m.ThumbURL = null.String{Valid: true, String: s.GetURL(m.Thumb)}
		}
		out = append(out, m)
		total = row.Total
	}

	return out, total, nil
}

// GetMedia returns a media item by PocketBase record id, uuid, or filename.
func (c *Core) GetMedia(recordID, uuid, fileName string, s media.Store) (media.Media, error) {
	q := sqliteMediaSelect + `,
				0 AS total
			FROM media
			WHERE `
	args := []any{}
	switch {
	case strings.TrimSpace(recordID) != "":
		q += `id = ?`
		args = append(args, strings.TrimSpace(recordID))
	case uuid != "":
		q += `uuid = ?`
		args = append(args, uuid)
	default:
		q += `filename = ?`
		args = append(args, fileName)
	}
	q += ` LIMIT 1`

	var row sqliteMediaRow
	if err := c.db.Get(&row, q, args...); err != nil {
		if err == sql.ErrNoRows {
			return media.Media{}, ErrNotFound
		}
		return media.Media{}, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.media}", "error", dbErr(err)))
	}

	out := sqliteMediaRowToModel(row)
	out.URL = s.GetURL(out.Filename)
	if out.Thumb != "" {
		out.ThumbURL = null.String{Valid: true, String: s.GetURL(out.Thumb)}
	}
	return out, nil
}

// GetMediaByRowID returns a media item by SQLite rowid (send-loop / attachments).
func (c *Core) GetMediaByRowID(rowID int, s media.Store) (media.Media, error) {
	if rowID < 1 {
		return media.Media{}, ErrNotFound
	}
	q := sqliteMediaSelect + `,
				0 AS total
			FROM media
			WHERE rowid = ?
			LIMIT 1`

	var row sqliteMediaRow
	if err := c.db.Get(&row, q, rowID); err != nil {
		if err == sql.ErrNoRows {
			return media.Media{}, ErrNotFound
		}
		return media.Media{}, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.media}", "error", dbErr(err)))
	}

	out := sqliteMediaRowToModel(row)
	out.URL = s.GetURL(out.Filename)
	if out.Thumb != "" {
		out.ThumbURL = null.String{Valid: true, String: s.GetURL(out.Thumb)}
	}
	return out, nil
}

// InsertMedia inserts a new media file into the DB.
func (c *Core) InsertMedia(fileName, thumbName, contentType string, meta models.JSON, provider string, s media.Store) (media.Media, error) {
	uu, err := uuid.NewV4()
	if err != nil {
		c.log.Printf("error generating UUID: %v", err)
		return media.Media{}, apperr.Internal(c.i18n.Ts("globals.messages.errorUUID", "error", err.Error()))
	}

	if _, err := c.db.Exec(`INSERT INTO media (uuid, filename, thumb, content_type, provider, meta, created, updated)
			VALUES (?, ?, ?, ?, ?, ?, strftime('%Y-%m-%d %H:%M:%fZ'), strftime('%Y-%m-%d %H:%M:%fZ'))`,
		uu.String(), fileName, thumbName, contentType, provider, meta); err != nil {
		c.log.Printf("error inserting uploaded file to db: %v", err)
		return media.Media{}, apperr.Internal(c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.media}", "error", dbErr(err)))
	}

	return c.GetMedia("", uu.String(), "", s)
}

// DeleteMedia deletes a given media item by PocketBase record id and returns the filename.
func (c *Core) DeleteMedia(recordID string) (string, error) {
	recordID = strings.TrimSpace(recordID)
	if recordID == "" {
		return "", apperr.BadRequest(c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.media}"))
	}

	var fname string
	if err := c.db.Get(&fname, `SELECT filename FROM media WHERE id = ?`, recordID); err != nil {
		c.log.Printf("error deleting uploaded file from db: %v", err)
		return "", apperr.Internal(c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.media}", "error", dbErr(err)))
	}
	if _, err := c.db.Exec(`DELETE FROM media WHERE id = ?`, recordID); err != nil {
		c.log.Printf("error deleting uploaded file from db: %v", err)
		return "", apperr.Internal(c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.media}", "error", dbErr(err)))
	}
	return fname, nil
}

// ResolveMediaRecordIDs resolves deprecated int rowids and/or record ids to media record ids.
func (c *Core) ResolveMediaRecordIDs(mediaIDs []int, mediaRecordIDs []string) ([]string, error) {
	out := make([]string, 0, len(mediaIDs)+len(mediaRecordIDs))
	seen := map[string]struct{}{}

	for _, id := range mediaRecordIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}

	if len(mediaIDs) == 0 {
		return out, nil
	}

	query := `SELECT id FROM media WHERE rowid IN (` + sqlitePlaceholders(len(mediaIDs)) + `)`
	args := make([]any, 0, len(mediaIDs))
	for _, id := range mediaIDs {
		args = append(args, id)
	}
	var resolved []string
	if err := c.db.Select(&resolved, query, args...); err != nil {
		return nil, err
	}
	for _, id := range resolved {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

// ResolveMediaRowIDs resolves media record ids to SQLite rowids (for MediaIDs send-loop fields).
func (c *Core) ResolveMediaRowIDs(mediaRecordIDs []string) ([]int64, error) {
	if len(mediaRecordIDs) == 0 {
		return nil, nil
	}
	query := `SELECT rowid FROM media WHERE id IN (` + sqlitePlaceholders(len(mediaRecordIDs)) + `)`
	args := make([]any, 0, len(mediaRecordIDs))
	for _, id := range mediaRecordIDs {
		args = append(args, id)
	}
	var out []int64
	if err := c.db.Select(&out, query, args...); err != nil {
		return nil, err
	}
	return out, nil
}
