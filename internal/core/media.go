package core

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/compdani/list_pocket/internal/media"
	"github.com/compdani/list_pocket/models"
	"github.com/gofrs/uuid/v5"
	"github.com/labstack/echo/v4"
	"gopkg.in/volatiletech/null.v6"
)

type sqliteMediaRow struct {
	ID          int    `db:"id"`
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

// QueryMedia returns media entries optionally filtered by a query string.
func (c *Core) QueryMedia(provider string, s media.Store, query string, offset, limit int) ([]media.Media, int, error) {
	rows := []sqliteMediaRow{}
	q := `
			SELECT
				COUNT(*) OVER () AS total,
				rowid AS id,
				uuid,
				filename,
				content_type,
				thumb,
				created AS created_at,
				provider,
				meta
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
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching",
				"name", "{globals.terms.media}", "error", pqErrMsg(err)))
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

// GetMedia returns a media item.
func (c *Core) GetMedia(id int, uuid, fileName string, s media.Store) (media.Media, error) {
	q := `
			SELECT
				rowid AS id,
				uuid,
				filename,
				content_type,
				thumb,
				created AS created_at,
				provider,
				meta,
				0 AS total
			FROM media
			WHERE `
	args := []any{}
	switch {
	case id > 0:
		q += `rowid = ?`
		args = append(args, id)
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
		return media.Media{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.media}", "error", pqErrMsg(err)))
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
		return media.Media{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUUID", "error", err.Error()))
	}

	if _, err := c.db.Exec(`INSERT INTO media (uuid, filename, thumb, content_type, provider, meta, created, updated)
			VALUES (?, ?, ?, ?, ?, ?, strftime('%Y-%m-%d %H:%M:%fZ'), strftime('%Y-%m-%d %H:%M:%fZ'))`,
		uu.String(), fileName, thumbName, contentType, provider, meta); err != nil {
		c.log.Printf("error inserting uploaded file to db: %v", err)
		return media.Media{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.media}", "error", pqErrMsg(err)))
	}

	return c.GetMedia(0, uu.String(), "", s)
}

// DeleteMedia deletes a given media item and returns the filename of the deleted item.
func (c *Core) DeleteMedia(id int) (string, error) {
	var fname string
	if err := c.db.Get(&fname, `SELECT filename FROM media WHERE rowid = ?`, id); err != nil {
		c.log.Printf("error deleting uploaded file from db: %v", err)
		return "", echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.media}", "error", pqErrMsg(err)))
	}
	if _, err := c.db.Exec(`DELETE FROM media WHERE rowid = ?`, id); err != nil {
		c.log.Printf("error deleting uploaded file from db: %v", err)
		return "", echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.media}", "error", pqErrMsg(err)))
	}
	return fname, nil
}
