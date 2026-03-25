package core

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/compdani/list_pocket/models"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx/types"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
	"github.com/pocketbase/dbx"
	pbcore "github.com/pocketbase/pocketbase/core"
	null "gopkg.in/volatiletech/null.v6"
)

func sqliteCampaignAltBodyValue(v null.String) string {
	if v.Valid {
		return v.String
	}
	return ""
}

func sqliteCampaignStringValue(v null.String) string {
	if v.Valid {
		return v.String
	}
	return ""
}

func sqliteCampaignTimeValue(v null.Time) string {
	if v.Valid {
		return v.Time.Format("2006-01-02 15:04:05.000Z")
	}
	return ""
}

func normalizeAnalyticsDateInput(value string, endOfDay bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "missing analytics date")
	}

	layouts := []string{
		"2006-01-02",
		"2006-01-02T15:04",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04",
		"2006-01-02 15:04:05",
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05.999Z",
		"2006-01-02T15:04:05.000-07:00",
	}

	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			if layout == "2006-01-02" && endOfDay {
				parsed = parsed.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
			}
			return parsed.UTC().Format("2006-01-02 15:04:05"), nil
		}
	}

	return "", echo.NewHTTPError(http.StatusBadRequest, "invalid analytics date")
}

func (c *Core) sqliteTemplateRecordID(id null.String) (string, error) {
	if !id.Valid || strings.TrimSpace(id.String) == "" {
		return "", nil
	}
	return strings.TrimSpace(id.String), nil
}

type sqliteCampaignListRecordRow struct {
	ID   string `db:"id"`
	Name string `db:"name"`
}

type sqliteCampaignMediaRecordRow struct {
	ID       string `db:"id"`
	Filename string `db:"filename"`
}

type sqliteCampaignRow struct {
	ID                int            `db:"id"`
	RecordID          string         `db:"record_id"`
	CreatedAt         string         `db:"created_at"`
	UpdatedAt         string         `db:"updated_at"`
	UUID              string         `db:"uuid"`
	Type              string         `db:"type"`
	Name              string         `db:"name"`
	Subject           string         `db:"subject"`
	FromEmail         string         `db:"from_email"`
	Body              string         `db:"body"`
	BodySource        string         `db:"body_source"`
	AltBody           string         `db:"altbody"`
	SendAt            string         `db:"send_at"`
	Status            string         `db:"status"`
	ContentType       string         `db:"content_type"`
	Tags              []byte         `db:"tags"`
	Headers           []byte         `db:"headers"`
	Attribs           []byte         `db:"attribs"`
	TemplateID        sql.NullString `db:"template_id"`
	Messenger         string         `db:"messenger"`
	Archive           bool           `db:"archive"`
	ArchiveSlug       string         `db:"archive_slug"`
	ArchiveTemplateID sql.NullString `db:"archive_template_id"`
	ArchiveMeta       []byte         `db:"archive_meta"`
	StartedAt         string         `db:"started_at"`
	ToSend            int            `db:"to_send"`
	Sent              int            `db:"sent"`
	TemplateBody      string         `db:"template_body"`
	Lists             []byte         `db:"lists"`
	Media             []byte         `db:"media"`
	Views             int            `db:"views"`
	RawViews          int            `db:"raw_views"`
	SuspectedViews    int            `db:"suspected_views"`
	Clicks            int            `db:"clicks"`
	Bounces           int            `db:"bounces"`
	Total             int            `db:"total"`
}

type sqliteCampaignStatsRow struct {
	ID        int    `db:"id"`
	Status    string `db:"status"`
	ToSend    int    `db:"to_send"`
	Sent      int    `db:"sent"`
	StartedAt string `db:"started_at"`
	UpdatedAt string `db:"updated_at"`
}

type campaignAnalyticsSQLiteRow struct {
	CampaignID int    `db:"campaign_id"`
	Count      int    `db:"count"`
	TS         string `db:"ts"`
}

func sqliteCampaignRowToModel(row sqliteCampaignRow) models.Campaign {
	tags := pq.StringArray{}
	if len(row.Tags) > 0 && string(row.Tags) != "null" {
		_ = json.Unmarshal(row.Tags, &tags)
	}

	headers := models.Headers{}
	if len(row.Headers) > 0 && string(row.Headers) != "null" {
		_ = json.Unmarshal(row.Headers, &headers)
	}

	attribs := models.JSON{}
	if len(row.Attribs) > 0 && string(row.Attribs) != "null" {
		_ = json.Unmarshal(row.Attribs, &attribs)
	}

	var archiveMeta json.RawMessage
	if len(row.ArchiveMeta) > 0 && string(row.ArchiveMeta) != "null" {
		archiveMeta = json.RawMessage(row.ArchiveMeta)
	}

	templateID := null.String{}
	if row.TemplateID.Valid {
		templateID = null.StringFrom(row.TemplateID.String)
	}

	archiveTemplateID := null.String{}
	if row.ArchiveTemplateID.Valid {
		archiveTemplateID = null.StringFrom(row.ArchiveTemplateID.String)
	}

	bodySource := null.String{}
	if strings.TrimSpace(row.BodySource) != "" {
		bodySource = null.StringFrom(row.BodySource)
	}

	altBody := null.String{}
	if row.AltBody != "" {
		altBody = null.StringFrom(row.AltBody)
	}

	archiveSlug := null.String{}
	if strings.TrimSpace(row.ArchiveSlug) != "" {
		archiveSlug = null.StringFrom(row.ArchiveSlug)
	}

	return models.Campaign{
		Base: models.Base{
			ID:        row.ID,
			RecordID:  row.RecordID,
			CreatedAt: parseNullTime(row.CreatedAt),
			UpdatedAt: parseNullTime(row.UpdatedAt),
		},
		CampaignMeta: models.CampaignMeta{
			Views:          row.Views,
			RawViews:       row.RawViews,
			SuspectedViews: row.SuspectedViews,
			Clicks:         row.Clicks,
			Bounces:        row.Bounces,
			Lists:          types.JSONText(row.Lists),
			Media:          types.JSONText(row.Media),
			StartedAt:      parseNullTime(row.StartedAt),
			ToSend:         row.ToSend,
			Sent:           row.Sent,
		},
		UUID:              row.UUID,
		Type:              row.Type,
		Name:              row.Name,
		Subject:           row.Subject,
		FromEmail:         row.FromEmail,
		Body:              row.Body,
		BodySource:        bodySource,
		AltBody:           altBody,
		SendAt:            parseNullTime(row.SendAt),
		Status:            row.Status,
		ContentType:       row.ContentType,
		Tags:              tags,
		Headers:           headers,
		Attribs:           attribs,
		TemplateID:        templateID,
		Messenger:         row.Messenger,
		Archive:           row.Archive,
		ArchiveSlug:       archiveSlug,
		ArchiveTemplateID: archiveTemplateID,
		ArchiveMeta:       archiveMeta,
		TemplateBody:      row.TemplateBody,
		Total:             row.Total,
	}
}

func (c *Core) ResolveCampaignIDs(campaignIDs []int, campaignRecordIDs []string) ([]int, error) {
	if len(campaignRecordIDs) == 0 {
		return appendUniqueInts([]int{}, campaignIDs), nil
	}

	query := `SELECT rowid FROM campaigns WHERE id IN (` + sqlitePlaceholders(len(campaignRecordIDs)) + `)`
	args := make([]any, 0, len(campaignRecordIDs))
	for _, id := range campaignRecordIDs {
		args = append(args, id)
	}

	var resolved []int
	if err := c.db.Select(&resolved, query, args...); err != nil {
		return nil, err
	}

	return appendUniqueInts(append([]int{}, campaignIDs...), resolved), nil
}

func (c *Core) ResolveCampaignRecordIDs(campaignIDs []int) ([]string, error) {
	if len(campaignIDs) == 0 {
		return nil, nil
	}

	query := `SELECT rowid AS row_id, id FROM campaigns WHERE rowid IN (` + sqlitePlaceholders(len(campaignIDs)) + `)`
	args := make([]any, 0, len(campaignIDs))
	for _, id := range campaignIDs {
		args = append(args, id)
	}

	var rows []struct {
		RowID int    `db:"row_id"`
		ID    string `db:"id"`
	}
	if err := c.db.Select(&rows, query, args...); err != nil {
		return nil, err
	}

	idMap := make(map[int]string, len(rows))
	for _, row := range rows {
		idMap[row.RowID] = row.ID
	}

	resolved := make([]string, 0, len(campaignIDs))
	for _, id := range campaignIDs {
		if recordID := idMap[id]; recordID != "" {
			resolved = append(resolved, recordID)
		}
	}

	return resolved, nil
}

func sqliteCampaignRowsToModels(rows []sqliteCampaignRow) models.Campaigns {
	out := make(models.Campaigns, 0, len(rows))
	for _, row := range rows {
		out = append(out, sqliteCampaignRowToModel(row))
	}
	return out
}

func sqliteCampaignStatsRowsToModels(rows []sqliteCampaignStatsRow) []models.CampaignStats {
	out := make([]models.CampaignStats, 0, len(rows))
	for _, row := range rows {
		out = append(out, models.CampaignStats{
			ID:        row.ID,
			Status:    row.Status,
			ToSend:    row.ToSend,
			Sent:      row.Sent,
			Started:   parseNullTime(row.StartedAt),
			UpdatedAt: parseNullTime(row.UpdatedAt),
		})
	}
	return out
}

func (c *Core) sqliteCampaignListRecordRows(listIDs []int) ([]sqliteCampaignListRecordRow, error) {
	if len(listIDs) == 0 {
		return nil, nil
	}

	rows := []sqliteCampaignListRecordRow{}
	query := `SELECT id, name FROM lists WHERE rowid IN (` + sqlitePlaceholders(len(listIDs)) + `)`
	args := make([]any, 0, len(listIDs))
	for _, id := range listIDs {
		args = append(args, id)
	}
	if err := c.db.Select(&rows, query, args...); err != nil {
		return nil, err
	}
	return rows, nil
}

func (c *Core) sqliteCampaignMediaRecordRows(mediaIDs []int) ([]sqliteCampaignMediaRecordRow, error) {
	if len(mediaIDs) == 0 {
		return nil, nil
	}

	rows := []sqliteCampaignMediaRecordRow{}
	query := `SELECT id, filename FROM media WHERE rowid IN (` + sqlitePlaceholders(len(mediaIDs)) + `)`
	args := make([]any, 0, len(mediaIDs))
	for _, id := range mediaIDs {
		args = append(args, id)
	}
	if err := c.db.Select(&rows, query, args...); err != nil {
		return nil, err
	}
	return rows, nil
}

func (c *Core) sqliteCampaignDeleteRelationRecords(txApp pbcore.App, collectionName, campaignID string) error {
	records, err := txApp.FindAllRecords(
		collectionName,
		dbx.NewExp("campaign_id = {:campaign_id}", dbx.Params{"campaign_id": campaignID}),
	)
	if err != nil {
		return err
	}

	for _, rec := range records {
		if err := txApp.Delete(rec); err != nil {
			return err
		}
	}

	return nil
}

const (
	CampaignAnalyticsViews   = "views"
	CampaignAnalyticsClicks  = "clicks"
	CampaignAnalyticsBounces = "bounces"

	campaignTplDefault = "default"
	campaignTplArchive = "archive"
)

// QueryCampaigns retrieves paginated campaigns optionally filtering them by the given arbitrary
// query expression. It also returns the total number of records in the DB.
func (c *Core) QueryCampaigns(searchStr string, statuses, tags []string, orderBy, order string, getAll bool, permittedLists []int, offset, limit int) (models.Campaigns, int, error) {
	return c.queryCampaignsSQLite(searchStr, statuses, tags, orderBy, order, getAll, permittedLists, offset, limit)
}

// GetCampaign retrieves a campaign.
func (c *Core) GetCampaign(recordID, uuid, archiveSlug string) (models.Campaign, error) {
	return c.getCampaign(recordID, uuid, archiveSlug, campaignTplDefault)
}

// GetArchivedCampaign retrieves a campaign with the archive template body.
func (c *Core) GetArchivedCampaign(recordID, uuid, archiveSlug string) (models.Campaign, error) {
	out, err := c.getCampaign(recordID, uuid, archiveSlug, campaignTplArchive)
	if err != nil {
		return out, err
	}

	if !out.Archive {
		return models.Campaign{}, echo.NewHTTPError(http.StatusBadRequest,
			c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.campaign}"))
	}

	return out, nil
}

// getCampaign retrieves a campaign. If typlType=default, then the campaign's
// template body is returned as "template_body". If tplType="archive",
// the archive template is returned.
func (c *Core) getCampaign(recordID, uuid, archiveSlug string, tplType string) (models.Campaign, error) {
	return c.getCampaignSQLite(recordID, uuid, archiveSlug, tplType)
}

// GetCampaignForPreview retrieves a campaign with a template body. If the optional tplID is > 0
// that particular template is used, otherwise, the template saved on the campaign is.
func (c *Core) GetCampaignForPreview(recordID string, tplID string) (models.Campaign, error) {
	return c.getCampaignForPreviewSQLite(recordID, tplID)
}

// GetArchivedCampaigns retrieves campaigns with a template body.
func (c *Core) GetArchivedCampaigns(offset, limit int) (models.Campaigns, int, error) {
	return c.getArchivedCampaignsSQLite(offset, limit)
}

// CreateCampaign creates a new campaign.
func (c *Core) CreateCampaign(o models.Campaign, listIDs []int, mediaIDs []int) (models.Campaign, error) {
	c.log.Printf("core create campaign: name=%q content_type=%q list_ids=%v media_ids=%v sqlite=%v", o.Name, o.ContentType, listIDs, mediaIDs, c.isSQLite())
	return c.createCampaignSQLite(o, listIDs, mediaIDs)
}

// UpdateCampaign updates a campaign.
func (c *Core) UpdateCampaign(recordID string, o models.Campaign, listIDs []int, mediaIDs []int) (models.Campaign, error) {
	return c.updateCampaignSQLite(recordID, o, listIDs, mediaIDs)
}

// UpdateCampaignStatus updates a campaign's status, eg: draft to running.
func (c *Core) UpdateCampaignStatus(recordID string, status string) (models.Campaign, error) {
	cm, err := c.GetCampaign(recordID, "", "")
	if err != nil {
		return models.Campaign{}, err
	}

	errMsg := ""
	switch status {
	case models.CampaignStatusDraft:
		if cm.Status != models.CampaignStatusScheduled {
			errMsg = c.i18n.T("campaigns.onlyScheduledAsDraft")
		}
	case models.CampaignStatusScheduled:
		if cm.Status != models.CampaignStatusDraft && cm.Status != models.CampaignStatusPaused {
			errMsg = c.i18n.T("campaigns.onlyDraftAsScheduled")
		}
		if !cm.SendAt.Valid {
			errMsg = c.i18n.T("campaigns.needsSendAt")
		}

	case models.CampaignStatusRunning:
		if cm.Status != models.CampaignStatusPaused && cm.Status != models.CampaignStatusDraft {
			errMsg = c.i18n.T("campaigns.onlyPausedDraft")
		}
	case models.CampaignStatusPaused:
		if cm.Status != models.CampaignStatusRunning {
			errMsg = c.i18n.T("campaigns.onlyActivePause")
		}
	case models.CampaignStatusCancelled:
		if cm.Status != models.CampaignStatusRunning && cm.Status != models.CampaignStatusPaused {
			errMsg = c.i18n.T("campaigns.onlyActiveCancel")
		}
	}

	if len(errMsg) > 0 {
		return models.Campaign{}, echo.NewHTTPError(http.StatusBadRequest, errMsg)
	}

	res, err := c.db.Exec(`UPDATE campaigns SET
		status=(CASE WHEN send_at IS NOT NULL AND send_at != '' AND ? = 'running' THEN 'scheduled' ELSE ? END),
		updated=(strftime('%Y-%m-%d %H:%M:%fZ'))
		WHERE id = ?`, status, status, cm.RecordID)
	if err != nil {
		c.log.Printf("error updating campaign status: %v", err)

		return models.Campaign{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return models.Campaign{}, echo.NewHTTPError(http.StatusBadRequest,
			c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}

	cm.Status = status
	return cm, nil
}

// UpdateCampaignArchive updates a campaign's archive properties.
func (c *Core) UpdateCampaignArchive(recordID string, enabled bool, tplID string, meta models.JSON, archiveSlug string) error {
	metaJSON, _ := json.Marshal(meta)
	if _, err := c.db.Exec(`UPDATE campaigns SET
		archive=?,
		archive_slug=(CASE WHEN ? = '' THEN NULL ELSE ? END),
		archive_template_id=(CASE WHEN ? != '' THEN ? ELSE archive_template_id END),
		archive_meta=(CASE WHEN ? != '' THEN ? ELSE archive_meta END),
		updated=(strftime('%Y-%m-%d %H:%M:%fZ'))
		WHERE id=?`,
		enabled, archiveSlug, archiveSlug, tplID, tplID, string(metaJSON), string(metaJSON), recordID); err != nil {
		c.log.Printf("error updating campaign: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}
	return nil
}

// DeleteCampaign deletes a campaign.
func (c *Core) DeleteCampaign(recordID string) error {
	if strings.TrimSpace(recordID) == "" {
		return echo.NewHTTPError(http.StatusBadRequest,
			c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.campaign}"))
	}

	if _, err := c.db.Exec(`DELETE FROM campaign_lists WHERE campaign_id = ?`, recordID); err != nil {
		c.log.Printf("error deleting campaign: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}
	if _, err := c.db.Exec(`DELETE FROM campaign_media WHERE campaign_id = ?`, recordID); err != nil {
		c.log.Printf("error deleting campaign: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}
	if _, err := c.db.Exec(`DELETE FROM campaign_views WHERE campaign_id = ?`, recordID); err != nil {
		c.log.Printf("error deleting campaign: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}
	if _, err := c.db.Exec(`DELETE FROM link_clicks WHERE campaign_id = ?`, recordID); err != nil {
		c.log.Printf("error deleting campaign: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}
	if _, err := c.db.Exec(`DELETE FROM bounces WHERE campaign_id = ?`, recordID); err != nil {
		c.log.Printf("error deleting campaign: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}
	if _, err := c.db.Exec(`DELETE FROM campaign_unsubscribes WHERE campaign_id = ?`, recordID); err != nil {
		c.log.Printf("error deleting campaign: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}

	res, err := c.db.Exec(`DELETE FROM campaigns WHERE id = ?`, recordID)
	if err != nil {
		c.log.Printf("error deleting campaign: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return echo.NewHTTPError(http.StatusBadRequest,
			c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.campaign}"))
	}

	return nil
}

// DeleteCampaigns deletes multiple campaigns by IDs or by query.
func (c *Core) DeleteCampaigns(recordIDs []string, query string, hasAllPerm bool, permittedLists []int) error {
	targetRecordIDs := append([]string{}, recordIDs...)
	if len(targetRecordIDs) == 0 {
		searchStr := makeSearchString(query)
		q := `SELECT DISTINCT c.id
			FROM campaigns c
			WHERE 1=1`
		args := []any{}

		if searchStr != "" {
			q += ` AND (c.name LIKE ? OR c.subject LIKE ?)`
			args = append(args, searchStr, searchStr)
		}

		if !hasAllPerm {
			if len(permittedLists) == 0 {
				return nil
			}
			q += ` AND EXISTS (
				SELECT 1
				FROM campaign_lists cl
				LEFT JOIN lists l ON l.id = cl.list_id
				WHERE cl.campaign_id = c.id
				  AND l.rowid IN (` + sqlitePlaceholders(len(permittedLists)) + `)
			)`
			for _, listID := range permittedLists {
				args = append(args, listID)
			}
		}

		if err := c.db.Select(&targetRecordIDs, q, args...); err != nil {
			c.log.Printf("error deleting campaigns: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.campaigns}", "error", pqErrMsg(err)))
		}
	}

	for _, recordID := range targetRecordIDs {
		if err := c.DeleteCampaign(recordID); err != nil {
			return err
		}
	}

	return nil
}

// CampaignHasLists checks if a campaign has any of the given list IDs.
func (c *Core) CampaignHasLists(recordID string, listIDs []int) (bool, error) {
	if len(listIDs) == 0 {
		return false, nil
	}

	q := `SELECT EXISTS (
		SELECT 1 FROM campaign_lists WHERE campaign_id = ? AND list_id IN (` + sqlitePlaceholders(len(listIDs)) + `)
	);`
	args := []any{recordID}
	for _, lid := range listIDs {
		args = append(args, lid)
	}

	has := false
	if err := c.db.Get(&has, q, args...); err != nil {
		c.log.Printf("error checking campaign lists: %v", err)
		return false, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}
	return has, nil
}

func (c *Core) queryCampaignsSQLite(searchStr string, statuses, tags []string, orderBy, order string, getAll bool, permittedLists []int, offset, limit int) (models.Campaigns, int, error) {
	query := `
	SELECT c.rowid AS id, c.id AS record_id, c.created AS created_at, c.updated AS updated_at, c.uuid, c.type, c.name,
		c.subject, c.from_email, c.body, c.body_source, c.altbody, c.send_at, c.status, c.content_type,
		c.tags, c.headers, c.attribs, tpl.id AS template_id, c.messenger, c.archive, c.archive_slug,
		atpl.id AS archive_template_id, c.archive_meta, c.started_at, c.to_send, c.sent,
		'' AS template_body,
		COUNT(*) OVER() AS total,
		COALESCE((
			SELECT json_group_array(json_object('id', COALESCE(l.id, cl.list_id), 'name', cl.list_name))
			FROM campaign_lists cl
			LEFT JOIN lists l ON l.id = cl.list_id
			WHERE cl.campaign_id = c.id
		), '[]') AS lists,
		COALESCE((
			SELECT json_group_array(json_object('id', m.rowid, 'filename', cm.filename))
			FROM campaign_media cm
			LEFT JOIN media m ON m.id = cm.media_id
			WHERE cm.campaign_id = c.id
		), '[]') AS media,
		(SELECT ` + sqliteUniqueCampaignViewsExpr("cv", "COALESCE(cv.is_suspected_privacy_open, 0) = 0") + ` FROM campaign_views cv WHERE cv.campaign_id = c.id) AS views,
		(SELECT ` + sqliteUniqueCampaignViewsExpr("cv", "") + ` FROM campaign_views cv WHERE cv.campaign_id = c.id) AS raw_views,
		(SELECT ` + sqliteUniqueCampaignViewsExpr("cv", "COALESCE(cv.is_suspected_privacy_open, 0) = 1") + ` FROM campaign_views cv WHERE cv.campaign_id = c.id) AS suspected_views,
		(SELECT COUNT(*) FROM link_clicks lc WHERE lc.campaign_id = c.id) AS clicks,
		(SELECT COUNT(*) FROM bounces b WHERE b.campaign_id = c.id) AS bounces
	FROM campaigns c
	LEFT JOIN templates tpl ON tpl.id = c.template_id
	LEFT JOIN templates atpl ON atpl.id = c.archive_template_id
	WHERE 1=1
	`

	args := []any{}
	if len(statuses) > 0 {
		query += ` AND c.status IN (` + sqlitePlaceholders(len(statuses)) + `)`
		for _, s := range statuses {
			args = append(args, s)
		}
	}

	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		query += ` AND INSTR(c.tags, ?) > 0`
		args = append(args, t)
	}

	if searchStr != "" {
		query += ` AND (c.name LIKE ? OR c.subject LIKE ?)`
		s := "%" + searchStr + "%"
		args = append(args, s, s)
	}

	if !getAll && len(permittedLists) > 0 {
		query += ` AND EXISTS (
			SELECT 1
			FROM campaign_lists cl
			LEFT JOIN lists l ON l.id = cl.list_id
			WHERE cl.campaign_id = c.id
			  AND l.rowid IN (` + sqlitePlaceholders(len(permittedLists)) + `)
		)`
		for _, id := range permittedLists {
			args = append(args, id)
		}
	}

	orderMap := map[string]string{
		"id":         "c.rowid",
		"name":       "c.name",
		"subject":    "c.subject",
		"status":     "c.status",
		"created_at": "c.created",
		"updated_at": "c.updated",
	}
	sortCol, ok := orderMap[orderBy]
	if !ok {
		sortCol = "c.created"
	}
	if order != SortAsc && order != SortDesc {
		order = SortDesc
	}
	query += ` ORDER BY ` + sortCol + ` ` + strings.ToUpper(order) + ` LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows := []sqliteCampaignRow{}
	if err := c.db.Select(&rows, query, args...); err != nil {
		c.log.Printf("error fetching campaigns: %v", err)
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}

	out := sqliteCampaignRowsToModels(rows)
	total := 0
	if len(out) > 0 {
		total = out[0].Total
	}
	return out, total, nil
}

func (c *Core) getCampaignSQLite(recordID, uuid, archiveSlug string, tplType string) (models.Campaign, error) {
	q := `
	SELECT c.rowid AS id, c.id AS record_id, c.created AS created_at, c.updated AS updated_at, c.uuid, c.type, c.name,
		c.subject, c.from_email, c.body, c.body_source, c.altbody, c.send_at, c.status, c.content_type,
		c.tags, c.headers, c.attribs, tpl.id AS template_id, c.messenger, c.archive, c.archive_slug,
		atpl.id AS archive_template_id, c.archive_meta, c.started_at, c.to_send, c.sent,
		COALESCE(t.body, (SELECT body FROM templates WHERE is_default = 1 LIMIT 1), '') AS template_body,
		COALESCE((
			SELECT json_group_array(json_object('id', COALESCE(l.id, cl.list_id), 'name', cl.list_name))
			FROM campaign_lists cl
			LEFT JOIN lists l ON l.id = cl.list_id
			WHERE cl.campaign_id = c.id
		), '[]') AS lists,
		COALESCE((
			SELECT json_group_array(json_object('id', m.rowid, 'filename', cm.filename))
			FROM campaign_media cm
			LEFT JOIN media m ON m.id = cm.media_id
			WHERE cm.campaign_id = c.id
		), '[]') AS media,
		(SELECT ` + sqliteUniqueCampaignViewsExpr("cv", "COALESCE(cv.is_suspected_privacy_open, 0) = 0") + ` FROM campaign_views cv WHERE cv.campaign_id = c.id) AS views,
		(SELECT ` + sqliteUniqueCampaignViewsExpr("cv", "") + ` FROM campaign_views cv WHERE cv.campaign_id = c.id) AS raw_views,
		(SELECT ` + sqliteUniqueCampaignViewsExpr("cv", "COALESCE(cv.is_suspected_privacy_open, 0) = 1") + ` FROM campaign_views cv WHERE cv.campaign_id = c.id) AS suspected_views,
		(SELECT COUNT(*) FROM link_clicks lc WHERE lc.campaign_id = c.id) AS clicks,
		(SELECT COUNT(*) FROM bounces b WHERE b.campaign_id = c.id) AS bounces
	FROM campaigns c
	LEFT JOIN templates tpl ON tpl.id = c.template_id
	LEFT JOIN templates atpl ON atpl.id = c.archive_template_id
	LEFT JOIN templates t ON t.id = (CASE WHEN ? = 'default' THEN c.template_id ELSE c.archive_template_id END)
	WHERE `

	args := []any{tplType}
	switch {
	case strings.TrimSpace(recordID) != "":
		q += "c.id = ?"
		args = append(args, recordID)
	case archiveSlug != "":
		q += "c.archive_slug = ?"
		args = append(args, archiveSlug)
	default:
		q += "c.uuid = ?"
		args = append(args, uuid)
	}
	q += " LIMIT 1"

	var row sqliteCampaignRow
	if err := c.db.Get(&row, q, args...); err != nil {
		if err == sql.ErrNoRows {
			return models.Campaign{}, echo.NewHTTPError(http.StatusBadRequest,
				c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.campaign}"))
		}
		c.log.Printf("error fetching campaign: %v", err)
		return models.Campaign{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}

	return sqliteCampaignRowToModel(row), nil
}

func (c *Core) getCampaignForPreviewSQLite(recordID string, tplID string) (models.Campaign, error) {
	var row sqliteCampaignRow
	if err := c.db.Get(&row, `
		SELECT c.rowid AS id, c.id AS record_id, c.created AS created_at, c.updated AS updated_at, c.uuid, c.type, c.name,
			c.subject, c.from_email, c.body, c.body_source, c.altbody, c.send_at, c.status, c.content_type,
			c.tags, c.headers, c.attribs, tpl.id AS template_id, c.messenger, c.archive, c.archive_slug,
			atpl.id AS archive_template_id, c.archive_meta, c.started_at, c.to_send, c.sent,
			COALESCE(t.body, '') AS template_body,
			COALESCE((
				SELECT json_group_array(json_object('id', COALESCE(l.id, cl.list_id), 'name', cl.list_name))
				FROM campaign_lists cl
				LEFT JOIN lists l ON l.id = cl.list_id
				WHERE cl.campaign_id = c.id
			), '[]') AS lists,
			'[]' AS media,
			(SELECT `+sqliteUniqueCampaignViewsExpr("cv", "COALESCE(cv.is_suspected_privacy_open, 0) = 0")+` FROM campaign_views cv WHERE cv.campaign_id = c.id) AS views,
			(SELECT `+sqliteUniqueCampaignViewsExpr("cv", "")+` FROM campaign_views cv WHERE cv.campaign_id = c.id) AS raw_views,
			(SELECT `+sqliteUniqueCampaignViewsExpr("cv", "COALESCE(cv.is_suspected_privacy_open, 0) = 1")+` FROM campaign_views cv WHERE cv.campaign_id = c.id) AS suspected_views,
			(SELECT COUNT(*) FROM link_clicks lc WHERE lc.campaign_id = c.id) AS clicks,
			(SELECT COUNT(*) FROM bounces b WHERE b.campaign_id = c.id) AS bounces,
			0 AS total
		FROM campaigns c
		LEFT JOIN templates tpl ON tpl.id = c.template_id
		LEFT JOIN templates atpl ON atpl.id = c.archive_template_id
		LEFT JOIN templates t ON t.id = (CASE WHEN ? = '' THEN c.template_id ELSE ? END)
		WHERE c.id = ?
	`, tplID, tplID, recordID); err != nil {
		if err == sql.ErrNoRows {
			return models.Campaign{}, echo.NewHTTPError(http.StatusBadRequest,
				c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.campaign}"))
		}
		c.log.Printf("error fetching campaign: %v", err)
		return models.Campaign{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}

	return sqliteCampaignRowToModel(row), nil
}

func (c *Core) getArchivedCampaignsSQLite(offset, limit int) (models.Campaigns, int, error) {
	rows := []sqliteCampaignRow{}
	if err := c.db.Select(&rows, `
		SELECT COUNT(*) OVER() AS total, c.rowid AS id, c.id AS record_id, c.created AS created_at, c.updated AS updated_at, c.uuid, c.type, c.name,
			c.subject, c.from_email, c.body, c.body_source, c.altbody, c.send_at, c.status, c.content_type,
			c.tags, c.headers, c.attribs, tpl.id AS template_id, c.messenger, c.archive, c.archive_slug,
			atpl.id AS archive_template_id, c.archive_meta, c.started_at, c.to_send, c.sent,
			COALESCE(t.body, (SELECT body FROM templates WHERE is_default = 1 LIMIT 1), '') AS template_body,
			'[]' AS lists,
			'[]' AS media,
			(SELECT `+sqliteUniqueCampaignViewsExpr("cv", "COALESCE(cv.is_suspected_privacy_open, 0) = 0")+` FROM campaign_views cv WHERE cv.campaign_id = c.id) AS views,
			(SELECT `+sqliteUniqueCampaignViewsExpr("cv", "")+` FROM campaign_views cv WHERE cv.campaign_id = c.id) AS raw_views,
			(SELECT `+sqliteUniqueCampaignViewsExpr("cv", "COALESCE(cv.is_suspected_privacy_open, 0) = 1")+` FROM campaign_views cv WHERE cv.campaign_id = c.id) AS suspected_views,
			(SELECT COUNT(*) FROM link_clicks lc WHERE lc.campaign_id = c.id) AS clicks,
			(SELECT COUNT(*) FROM bounces b WHERE b.campaign_id = c.id) AS bounces
		FROM campaigns c
		LEFT JOIN templates tpl ON tpl.id = c.template_id
		LEFT JOIN templates atpl ON atpl.id = c.archive_template_id
		LEFT JOIN templates t ON t.id = c.archive_template_id
		WHERE c.archive = 1 AND c.type = 'regular' AND c.status IN ('running', 'paused', 'finished')
		ORDER BY c.created DESC
		LIMIT ? OFFSET ?
	`, limit, offset); err != nil {
		c.log.Printf("error fetching public campaigns: %v", err)
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}

	out := sqliteCampaignRowsToModels(rows)
	total := 0
	if len(out) > 0 {
		total = out[0].Total
	}
	return out, total, nil
}

func (c *Core) createCampaignSQLite(o models.Campaign, listIDs []int, mediaIDs []int) (models.Campaign, error) {
	c.log.Printf("create campaign sqlite: begin name=%q content_type=%q archive=%v altbody_valid=%v archive_slug_valid=%v send_at_valid=%v template_id_valid=%v archive_template_id_valid=%v",
		o.Name, o.ContentType, o.Archive, o.AltBody.Valid, o.ArchiveSlug.Valid, o.SendAt.Valid, o.TemplateID.Valid, o.ArchiveTemplateID.Valid)
	uu, err := uuid.NewV4()
	if err != nil {
		c.log.Printf("error generating UUID: %v", err)
		return models.Campaign{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUUID", "error", err.Error()))
	}

	type tplInfo struct {
		ID         sql.NullInt64  `db:"id"`
		Type       string         `db:"type"`
		Body       string         `db:"body"`
		BodySource sql.NullString `db:"body_source"`
	}

	var tpl tplInfo
	if o.TemplateID.Valid && strings.TrimSpace(o.TemplateID.String) != "" {
		_ = c.db.Get(&tpl, `SELECT id, type, body, body_source FROM templates WHERE id = ? LIMIT 1`, o.TemplateID.String)
	} else if o.ContentType != models.CampaignContentTypeVisual {
		_ = c.db.Get(&tpl, `SELECT id, type, body, body_source FROM templates WHERE is_default = 1 LIMIT 1`)
	}
	c.log.Printf("create campaign sqlite: template resolved name=%q tpl_type=%q tpl_id_valid=%v", o.Name, tpl.Type, tpl.ID.Valid)

	contentType := o.ContentType
	if contentType == "" {
		contentType = models.CampaignContentTypeRichtext
	}
	body := o.Body
	bodySource := o.BodySource
	templateID := o.TemplateID

	if tpl.Type == models.TemplateTypeCampaignVisual {
		contentType = models.CampaignContentTypeVisual
		templateID.Valid = false
		templateID.String = ""
		if body == "" {
			body = tpl.Body
		}
		if !bodySource.Valid && tpl.BodySource.Valid {
			bodySource.Valid = true
			bodySource.String = tpl.BodySource.String
		}
	} else if body == "" && tpl.Body != "" {
		body = tpl.Body
		if !bodySource.Valid && tpl.BodySource.Valid {
			bodySource.Valid = true
			bodySource.String = tpl.BodySource.String
		}
	}

	templateRecordID, err := c.sqliteTemplateRecordID(templateID)
	if err != nil {
		c.log.Printf("create campaign sqlite: template record id lookup failed name=%q error=%v", o.Name, err)
		return models.Campaign{}, err
	}
	archiveTemplateRecordID, err := c.sqliteTemplateRecordID(o.ArchiveTemplateID)
	if err != nil {
		c.log.Printf("create campaign sqlite: archive template record id lookup failed name=%q error=%v", o.Name, err)
		return models.Campaign{}, err
	}
	c.log.Printf("create campaign sqlite: normalized fields name=%q send_at=%q template_record_id=%q archive_template_record_id=%q archive_slug=%q body_source_len=%d",
		o.Name, sqliteCampaignTimeValue(o.SendAt), templateRecordID, archiveTemplateRecordID, sqliteCampaignStringValue(o.ArchiveSlug), len(sqliteCampaignStringValue(bodySource)))

	listRows, err := c.sqliteCampaignListRecordRows(listIDs)
	if err != nil {
		return models.Campaign{}, err
	}
	mediaRows, err := c.sqliteCampaignMediaRecordRows(mediaIDs)
	if err != nil {
		return models.Campaign{}, err
	}
	c.log.Printf("create campaign sqlite: resolved %d list records and %d media records for name=%q", len(listRows), len(mediaRows), o.Name)

	pb := c.db.PocketBase()
	if pb == nil {
		return models.Campaign{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.campaign}", "error", "pocketbase is not initialized"))
	}

	if err := pb.RunInTransaction(func(txApp pbcore.App) error {
		campaignsCol, err := txApp.FindCollectionByNameOrId("campaigns")
		if err != nil {
			return err
		}

		campaignRec := pbcore.NewRecord(campaignsCol)
		campaignType := o.Type
		if campaignType == "" {
			campaignType = models.CampaignTypeRegular
		}
		messenger := o.Messenger
		if messenger == "" {
			messenger = "email"
		}
		status := o.Status
		if status == "" {
			status = models.CampaignStatusDraft
		}

		campaignRec.Set("uuid", uu.String())
		campaignRec.Set("type", campaignType)
		campaignRec.Set("name", o.Name)
		campaignRec.Set("subject", o.Subject)
		campaignRec.Set("from_email", o.FromEmail)
		campaignRec.Set("body", body)
		campaignRec.Set("body_source", sqliteCampaignStringValue(bodySource))
		campaignRec.Set("altbody", sqliteCampaignAltBodyValue(o.AltBody))
		campaignRec.Set("content_type", contentType)
		campaignRec.Set("send_at", sqliteCampaignTimeValue(o.SendAt))
		campaignRec.Set("headers", o.Headers)
		campaignRec.Set("attribs", o.Attribs)
		campaignRec.Set("status", status)
		campaignRec.Set("tags", normalizeTags(o.Tags))
		campaignRec.Set("messenger", messenger)
		campaignRec.Set("template_id", templateRecordID)
		campaignRec.Set("to_send", 0)
		campaignRec.Set("sent", 0)
		campaignRec.Set("max_subscriber_id", 0)
		campaignRec.Set("last_subscriber_id", 0)
		campaignRec.Set("archive", o.Archive)
		campaignRec.Set("archive_slug", sqliteCampaignStringValue(o.ArchiveSlug))
		campaignRec.Set("archive_template_id", archiveTemplateRecordID)
		campaignRec.Set("archive_meta", o.ArchiveMeta)
		if err := txApp.Save(campaignRec); err != nil {
			return err
		}
		c.log.Printf("create campaign sqlite: saved campaign record name=%q record_id=%q", o.Name, campaignRec.Id)

		if len(listRows) > 0 {
			campaignListsCol, err := txApp.FindCollectionByNameOrId("campaign_lists")
			if err != nil {
				return err
			}
			for _, row := range listRows {
				rec := pbcore.NewRecord(campaignListsCol)
				rec.Set("campaign_id", campaignRec.Id)
				rec.Set("list_id", row.ID)
				rec.Set("list_name", row.Name)
				if err := txApp.Save(rec); err != nil {
					return err
				}
			}
		}

		if len(mediaRows) > 0 {
			campaignMediaCol, err := txApp.FindCollectionByNameOrId("campaign_media")
			if err != nil {
				return err
			}
			for _, row := range mediaRows {
				rec := pbcore.NewRecord(campaignMediaCol)
				rec.Set("campaign_id", campaignRec.Id)
				rec.Set("media_id", row.ID)
				rec.Set("filename", row.Filename)
				if err := txApp.Save(rec); err != nil {
					return err
				}
			}
		}

		return nil
	}); err != nil {
		c.log.Printf("error creating campaign: %v", err)
		return models.Campaign{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}
	c.log.Printf("create campaign sqlite: committed name=%q uuid=%q", o.Name, uu.String())

	return c.GetCampaign("", uu.String(), "")
}

func (c *Core) updateCampaignSQLite(recordID string, o models.Campaign, listIDs []int, mediaIDs []int) (models.Campaign, error) {
	campaignRecID := recordID

	templateRecordID, err := c.sqliteTemplateRecordID(o.TemplateID)
	if err != nil {
		return models.Campaign{}, err
	}
	archiveTemplateRecordID, err := c.sqliteTemplateRecordID(o.ArchiveTemplateID)
	if err != nil {
		return models.Campaign{}, err
	}
	listRows, err := c.sqliteCampaignListRecordRows(listIDs)
	if err != nil {
		return models.Campaign{}, err
	}
	mediaRows, err := c.sqliteCampaignMediaRecordRows(mediaIDs)
	if err != nil {
		return models.Campaign{}, err
	}
	pb := c.db.PocketBase()
	if pb == nil {
		return models.Campaign{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.campaign}", "error", "pocketbase is not initialized"))
	}

	if err := pb.RunInTransaction(func(txApp pbcore.App) error {
		rec, err := txApp.FindRecordById("campaigns", campaignRecID)
		if err != nil {
			return err
		}

		rec.Set("name", o.Name)
		rec.Set("subject", o.Subject)
		rec.Set("from_email", o.FromEmail)
		rec.Set("body", o.Body)
		rec.Set("altbody", sqliteCampaignAltBodyValue(o.AltBody))
		rec.Set("content_type", o.ContentType)
		rec.Set("send_at", sqliteCampaignTimeValue(o.SendAt))
		rec.Set("headers", o.Headers)
		rec.Set("attribs", o.Attribs)
		rec.Set("tags", normalizeTags(o.Tags))
		rec.Set("messenger", o.Messenger)
		rec.Set("template_id", templateRecordID)
		rec.Set("archive", o.Archive)
		rec.Set("archive_slug", sqliteCampaignStringValue(o.ArchiveSlug))
		rec.Set("archive_template_id", archiveTemplateRecordID)
		rec.Set("archive_meta", o.ArchiveMeta)
		rec.Set("body_source", sqliteCampaignStringValue(o.BodySource))
		if err := txApp.Save(rec); err != nil {
			return err
		}

		if err := c.sqliteCampaignDeleteRelationRecords(txApp, "campaign_lists", campaignRecID); err != nil {
			return err
		}
		if len(listRows) > 0 {
			campaignListsCol, err := txApp.FindCollectionByNameOrId("campaign_lists")
			if err != nil {
				return err
			}
			for _, row := range listRows {
				link := pbcore.NewRecord(campaignListsCol)
				link.Set("campaign_id", campaignRecID)
				link.Set("list_id", row.ID)
				link.Set("list_name", row.Name)
				if err := txApp.Save(link); err != nil {
					return err
				}
			}
		}
		if err := c.sqliteCampaignDeleteRelationRecords(txApp, "campaign_media", campaignRecID); err != nil {
			return err
		}
		if len(mediaRows) > 0 {
			campaignMediaCol, err := txApp.FindCollectionByNameOrId("campaign_media")
			if err != nil {
				return err
			}
			for _, row := range mediaRows {
				link := pbcore.NewRecord(campaignMediaCol)
				link.Set("campaign_id", campaignRecID)
				link.Set("media_id", row.ID)
				link.Set("filename", row.Filename)
				if err := txApp.Save(link); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		c.log.Printf("error updating campaign: %v", err)
		return models.Campaign{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}
	return c.GetCampaign(recordID, "", "")
}

func (c *Core) isSQLite() bool {
	return c.db != nil && strings.Contains(strings.ToLower(c.db.DriverName()), "sqlite")
}

func sqlitePlaceholders(n int) string {
	if n < 1 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// GetRunningCampaignStats returns the progress stats of running campaigns.
func (c *Core) GetRunningCampaignStats() ([]models.CampaignStats, error) {
	rows := []sqliteCampaignStatsRow{}
	if err := c.db.Select(&rows, `SELECT
		rowid AS id,
		status,
		to_send,
		sent,
		started_at,
		updated AS updated_at
	FROM campaigns
	WHERE status = ?`, models.CampaignStatusRunning); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}

		c.log.Printf("error fetching campaign stats: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	} else if len(rows) == 0 {
		return nil, nil
	}

	return sqliteCampaignStatsRowsToModels(rows), nil
}

func (c *Core) GetCampaignAnalyticsCounts(campIDs []int, typ, fromDate, toDate string) ([]models.CampaignAnalyticsCount, error) {
	return c.getCampaignAnalyticsCountsSQLite(campIDs, typ, fromDate, toDate)
}

// GetCampaignAnalyticsLinks returns link click analytics for the given campaign IDs.
func (c *Core) GetCampaignAnalyticsLinks(campIDs []int, typ, fromDate, toDate string) ([]models.CampaignAnalyticsLink, error) {
	return c.getCampaignAnalyticsLinksSQLite(campIDs, fromDate, toDate)
}

// RegisterCampaignView registers a subscriber's view on a campaign.
func (c *Core) RegisterCampaignView(campUUID, subUUID string, event models.OpenEvent) error {
	event = normalizeOpenEvent(event)

	var row struct {
		CampaignID   string         `db:"campaign_id"`
		SubscriberID sql.NullString `db:"subscriber_id"`
		StartedAt    string         `db:"started_at"`
		SendAt       string         `db:"send_at"`
	}
	if err := c.db.Get(&row, `
		SELECT c.id AS campaign_id, s.id AS subscriber_id, COALESCE(c.started_at, '') AS started_at, COALESCE(c.send_at, '') AS send_at
		FROM campaigns c
		LEFT JOIN subscribers s ON s.uuid = ?
		WHERE c.uuid = ?
		LIMIT 1
	`, subUUID, campUUID); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		c.log.Printf("error resolving campaign view target: %s", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}

	startedAt := time.Time{}
	if parsed := parseNullTime(row.StartedAt); parsed.Valid {
		startedAt = parsed.Time
	}
	sendAt := time.Time{}
	if parsed := parseNullTime(row.SendAt); parsed.Valid {
		sendAt = parsed.Time
	}
	referenceAt, referenceType := campaignPrivacyReference(sendAt, startedAt)
	suspected, meta, err := classifyPrivacyOpen(event, referenceAt, referenceType)
	if err != nil {
		c.log.Printf("error marshaling campaign view metadata: %s", err)
		meta = "{}"
	}

	if _, err := c.db.Exec(`
		INSERT INTO campaign_views (campaign_id, subscriber_id, meta, is_suspected_privacy_open, created)
		VALUES (?, ?, ?, ?, ?)
	`, row.CampaignID, row.SubscriberID, meta, suspected, sqliteTimestampValue(event.OpenedAt)); err != nil {
		c.log.Printf("error registering campaign view: %s", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}
	return nil
}

// GetLinkURL returns the original URL for a link UUID without recording a click.
func (c *Core) GetLinkURL(linkUUID string) (string, error) {
	var url string
	if err := c.db.Get(&url, `SELECT url FROM links WHERE uuid = ?`, linkUUID); err != nil {
		c.log.Printf("error getting link URL: %s", err)
		return "", echo.NewHTTPError(http.StatusInternalServerError, c.i18n.Ts("public.errorProcessingRequest"))
	}
	return url, nil
}

// RegisterCampaignLinkClick registers a subscriber's link click on a campaign.
func (c *Core) RegisterCampaignLinkClick(linkUUID, campUUID, subUUID string) (string, error) {
	var out struct {
		ID  string `db:"id"`
		URL string `db:"url"`
	}

	if err := c.db.Get(&out, `SELECT id, url FROM links WHERE uuid = ?`, linkUUID); err != nil {
		if err == sql.ErrNoRows {
			return "", echo.NewHTTPError(http.StatusBadRequest, c.i18n.Ts("public.invalidLink"))
		}
		c.log.Printf("error registering link click: %s", err)
		return "", echo.NewHTTPError(http.StatusInternalServerError, c.i18n.Ts("public.errorProcessingRequest"))
	}

	if _, err := c.db.Exec(`
		INSERT INTO link_clicks (campaign_id, subscriber_id, link_id, created)
		SELECT c.id, s.id, ?, (strftime('%Y-%m-%d %H:%M:%fZ'))
		FROM campaigns c
		LEFT JOIN subscribers s ON s.uuid = ?
		WHERE c.uuid = ?
		LIMIT 1
	`, out.ID, subUUID, campUUID); err != nil {
		c.log.Printf("error registering link click: %s", err)
		return "", echo.NewHTTPError(http.StatusInternalServerError, c.i18n.Ts("public.errorProcessingRequest"))
	}

	return out.URL, nil
}

func (c *Core) getCampaignAnalyticsCountsSQLite(campIDs []int, typ, fromDate, toDate string) ([]models.CampaignAnalyticsCount, error) {
	if !strHasLen(fromDate, 10, 30) || !strHasLen(toDate, 10, 30) {
		return nil, echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("analytics.invalidDates"))
	}
	if len(campIDs) == 0 {
		return []models.CampaignAnalyticsCount{}, nil
	}

	table := ""
	countExpr := "COUNT(*)"
	groupByPeriod := true
	switch typ {
	case "views":
		table = "campaign_views"
		countExpr = "COUNT(CASE WHEN COALESCE(" + table + ".is_suspected_privacy_open, 0) = 0 THEN 1 END)"
	case "views_total":
		table = "campaign_views"
		countExpr = "COUNT(CASE WHEN COALESCE(" + table + ".is_suspected_privacy_open, 0) = 0 THEN 1 END)"
		groupByPeriod = false
	case "views_unique":
		table = "campaign_views"
		countExpr = sqliteUniqueCampaignViewsExpr(table, "COALESCE("+table+".is_suspected_privacy_open, 0) = 0")
	case "views_unique_total":
		table = "campaign_views"
		countExpr = sqliteUniqueCampaignViewsExpr(table, "COALESCE("+table+".is_suspected_privacy_open, 0) = 0")
		groupByPeriod = false
	case "views_raw":
		table = "campaign_views"
	case "views_raw_total":
		table = "campaign_views"
		groupByPeriod = false
	case "views_unique_raw":
		table = "campaign_views"
		countExpr = sqliteUniqueCampaignViewsExpr(table, "")
	case "views_unique_raw_total":
		table = "campaign_views"
		countExpr = sqliteUniqueCampaignViewsExpr(table, "")
		groupByPeriod = false
	case "views_suspected":
		table = "campaign_views"
		countExpr = "COUNT(CASE WHEN COALESCE(" + table + ".is_suspected_privacy_open, 0) = 1 THEN 1 END)"
	case "views_suspected_total":
		table = "campaign_views"
		countExpr = "COUNT(CASE WHEN COALESCE(" + table + ".is_suspected_privacy_open, 0) = 1 THEN 1 END)"
		groupByPeriod = false
	case "views_unique_suspected":
		table = "campaign_views"
		countExpr = sqliteUniqueCampaignViewsExpr(table, "COALESCE("+table+".is_suspected_privacy_open, 0) = 1")
	case "views_unique_suspected_total":
		table = "campaign_views"
		countExpr = sqliteUniqueCampaignViewsExpr(table, "COALESCE("+table+".is_suspected_privacy_open, 0) = 1")
		groupByPeriod = false
	case "clicks":
		table = "link_clicks"
	case "bounces":
		table = "bounces"
	case "unsubscribes":
		table = "campaign_unsubscribes"
	default:
		return nil, echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("globals.messages.invalidData"))
	}

	fromSQL, err := normalizeAnalyticsDateInput(fromDate, false)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("analytics.invalidDates"))
	}
	toSQL, err := normalizeAnalyticsDateInput(toDate, true)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("analytics.invalidDates"))
	}
	fromTime, _ := time.Parse("2006-01-02 15:04:05", fromSQL)
	toTime, _ := time.Parse("2006-01-02 15:04:05", toSQL)
	splitTime := fromTime
	if candidate := toTime.Add(-12 * time.Hour); candidate.After(fromTime) {
		splitTime = candidate
	}
	splitSQL := splitTime.Format("2006-01-02 15:04:05")

	recordIDs, err := c.ResolveCampaignRecordIDs(campIDs)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.analytics}", "error", pqErrMsg(err)))
	}
	if len(recordIDs) == 0 {
		return []models.CampaignAnalyticsCount{}, nil
	}

	q := ""
	args := []any{}
	if groupByPeriod {
		q = `
			SELECT
				campaign_id,
				` + countExpr + ` AS count,
				CASE
					WHEN created >= ? THEN 'hour'
					ELSE 'day'
				END AS bucket,
				CASE
					WHEN created >= ? THEN strftime('%Y-%m-%d %H:00:00', created)
					ELSE strftime('%Y-%m-%d 00:00:00', created)
				END AS ts
			FROM ` + table + `
			WHERE campaign_id IN (` + sqlitePlaceholders(len(recordIDs)) + `)
			  AND created >= ?
			  AND created <= ?
			GROUP BY campaign_id, bucket, ts
			ORDER BY ts ASC`
		args = []any{splitSQL, splitSQL}
	} else {
		q = `
			SELECT
				campaign_id,
				` + countExpr + ` AS count,
				'day' AS bucket,
				? AS ts
			FROM ` + table + `
			WHERE campaign_id IN (` + sqlitePlaceholders(len(recordIDs)) + `)
			  AND created >= ?
			  AND created <= ?
			GROUP BY campaign_id
			ORDER BY campaign_id ASC`
		args = []any{fromSQL}
	}
	for _, id := range recordIDs {
		args = append(args, id)
	}
	args = append(args, fromSQL, toSQL)

	var rows []struct {
		CampaignID string `db:"campaign_id"`
		Bucket     string `db:"bucket"`
		Count      int    `db:"count"`
		TS         string `db:"ts"`
	}
	if err := c.db.Select(&rows, q, args...); err != nil {
		c.log.Printf("error fetching campaign %s: %v", typ, err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.analytics}", "error", pqErrMsg(err)))
	}

	out := make([]models.CampaignAnalyticsCount, 0, len(rows))
	for _, r := range rows {
		t, err := time.Parse("2006-01-02 15:04:05", r.TS)
		if err != nil {
			continue
		}
		out = append(out, models.CampaignAnalyticsCount{
			CampaignID: r.CampaignID,
			Bucket:     r.Bucket,
			Count:      r.Count,
			Timestamp:  t,
		})
	}

	return out, nil
}

func (c *Core) getCampaignAnalyticsLinksSQLite(campIDs []int, fromDate, toDate string) ([]models.CampaignAnalyticsLink, error) {
	if len(campIDs) == 0 {
		return []models.CampaignAnalyticsLink{}, nil
	}

	fromSQL, err := normalizeAnalyticsDateInput(fromDate, false)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("analytics.invalidDates"))
	}
	toSQL, err := normalizeAnalyticsDateInput(toDate, true)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("analytics.invalidDates"))
	}

	recordIDs, err := c.ResolveCampaignRecordIDs(campIDs)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.analytics}", "error", pqErrMsg(err)))
	}
	if len(recordIDs) == 0 {
		return []models.CampaignAnalyticsLink{}, nil
	}

	q := `
		SELECT COUNT(*) AS count, links.url
		FROM link_clicks
		LEFT JOIN links ON link_clicks.link_id = links.id
		WHERE campaign_id IN (` + sqlitePlaceholders(len(recordIDs)) + `)
		  AND link_clicks.created >= ?
		  AND link_clicks.created <= ?
		GROUP BY links.url
		ORDER BY count DESC
		LIMIT 50`

	args := make([]any, 0, len(recordIDs)+2)
	for _, id := range recordIDs {
		args = append(args, id)
	}
	args = append(args, fromSQL, toSQL)

	out := []models.CampaignAnalyticsLink{}
	if err := c.db.Select(&out, q, args...); err != nil {
		c.log.Printf("error fetching campaign links: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.analytics}", "error", pqErrMsg(err)))
	}
	return out, nil
}

// DeleteCampaignViews deletes campaign views older than a given date.
func (c *Core) DeleteCampaignViews(before time.Time) error {
	if _, err := c.db.Exec(`DELETE FROM campaign_views WHERE created < ?`, before.UTC().Format("2006-01-02 15:04:05")); err != nil {
		c.log.Printf("error deleting campaign views: %s", err)
		return echo.NewHTTPError(http.StatusInternalServerError, c.i18n.Ts("public.errorProcessingRequest"))
	}

	return nil
}

// DeleteCampaignLinkClicks deletes campaign views older than a given date.
func (c *Core) DeleteCampaignLinkClicks(before time.Time) error {
	if _, err := c.db.Exec(`DELETE FROM link_clicks WHERE created < ?`, before.UTC().Format("2006-01-02 15:04:05")); err != nil {
		c.log.Printf("error deleting campaign link clicks: %s", err)
		return echo.NewHTTPError(http.StatusInternalServerError, c.i18n.Ts("public.errorProcessingRequest"))
	}

	return nil
}
