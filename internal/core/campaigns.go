package core

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/knadh/listmonk/internal/pbdb"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

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
	if c.isSQLite() {
		return c.queryCampaignsSQLite(searchStr, statuses, tags, orderBy, order, getAll, permittedLists, offset, limit)
	}

	queryStr, stmt := makeSearchQuery(searchStr, orderBy, order, c.q.QueryCampaigns, campQuerySortFields)

	if statuses == nil {
		statuses = []string{}
	}

	if tags == nil {
		tags = []string{}
	}

	// Unsafe to ignore scanning fields not present in models.Campaigns.
	var out models.Campaigns
	if err := c.db.Select(&out, stmt, 0, pq.StringArray(statuses), pq.StringArray(tags), queryStr, getAll, pq.Array(permittedLists), offset, limit); err != nil {
		c.log.Printf("error fetching campaigns: %v", err)
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}

	for i := range out {
		// Replace null tags.
		if out[i].Tags == nil {
			out[i].Tags = []string{}
		}
	}

	// Lazy load stats.
	if err := out.LoadStats(c.q.GetCampaignStats); err != nil {
		c.log.Printf("error fetching campaign stats: %v", err)
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaigns}", "error", pqErrMsg(err)))
	}

	total := 0
	if len(out) > 0 {
		total = out[0].Total
	}

	return out, total, nil
}

// GetCampaign retrieves a campaign.
func (c *Core) GetCampaign(id int, uuid, archiveSlug string) (models.Campaign, error) {
	return c.getCampaign(id, uuid, archiveSlug, campaignTplDefault)
}

// GetArchivedCampaign retrieves a campaign with the archive template body.
func (c *Core) GetArchivedCampaign(id int, uuid, archiveSlug string) (models.Campaign, error) {
	out, err := c.getCampaign(id, uuid, archiveSlug, campaignTplArchive)
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
func (c *Core) getCampaign(id int, uuid, archiveSlug string, tplType string) (models.Campaign, error) {
	if c.isSQLite() {
		return c.getCampaignSQLite(id, uuid, archiveSlug, tplType)
	}

	// Unsafe to ignore scanning fields not present in models.Campaigns.
	var uu any
	if uuid != "" {
		uu = uuid
	}

	var out models.Campaigns
	if err := c.q.GetCampaign.Select(&out, id, uu, archiveSlug, tplType); err != nil {
		// if err := c.db.Select(&out, stmt, 0, pq.Array([]string{}), queryStr, 0, 1); err != nil {
		c.log.Printf("error fetching campaign: %v", err)
		return models.Campaign{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}

	if len(out) == 0 {
		return models.Campaign{}, echo.NewHTTPError(http.StatusBadRequest,
			c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.campaign}"))
	}

	for i := 0; i < len(out); i++ {
		// Replace null tags.
		if out[i].Tags == nil {
			out[i].Tags = []string{}
		}
	}

	// Lazy load stats.
	if err := out.LoadStats(c.q.GetCampaignStats); err != nil {
		c.log.Printf("error fetching campaign stats: %v", err)
		return models.Campaign{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}

	return out[0], nil
}

// GetCampaignForPreview retrieves a campaign with a template body. If the optional tplID is > 0
// that particular template is used, otherwise, the template saved on the campaign is.
func (c *Core) GetCampaignForPreview(id, tplID int) (models.Campaign, error) {
	if c.isSQLite() {
		return c.getCampaignForPreviewSQLite(id, tplID)
	}

	var out models.Campaign
	if err := c.q.GetCampaignForPreview.Get(&out, id, tplID); err != nil {
		if err == sql.ErrNoRows {
			return models.Campaign{}, echo.NewHTTPError(http.StatusBadRequest,
				c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.campaign}"))
		}

		c.log.Printf("error fetching campaign: %v", err)
		return models.Campaign{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}

	return out, nil
}

// GetArchivedCampaigns retrieves campaigns with a template body.
func (c *Core) GetArchivedCampaigns(offset, limit int) (models.Campaigns, int, error) {
	if c.isSQLite() {
		return c.getArchivedCampaignsSQLite(offset, limit)
	}

	var out models.Campaigns
	if err := c.q.GetArchivedCampaigns.Select(&out, offset, limit, campaignTplArchive); err != nil {
		c.log.Printf("error fetching public campaigns: %v", err)
		return models.Campaigns{}, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}

	total := 0
	if len(out) > 0 {
		total = out[0].Total
	}

	return out, total, nil
}

// CreateCampaign creates a new campaign.
func (c *Core) CreateCampaign(o models.Campaign, listIDs []int, mediaIDs []int) (models.Campaign, error) {
	if c.isSQLite() {
		return c.createCampaignSQLite(o, listIDs, mediaIDs)
	}

	uu, err := uuid.NewV4()
	if err != nil {
		c.log.Printf("error generating UUID: %v", err)
		return models.Campaign{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUUID", "error", err.Error()))
	}

	// Insert and read ID.
	var newID int
	if err := c.q.CreateCampaign.Get(&newID,
		uu,
		o.Type,
		o.Name,
		o.Subject,
		o.FromEmail,
		o.Body,
		o.AltBody,
		o.ContentType,
		o.SendAt,
		o.Headers,
		o.Attribs,
		pq.StringArray(normalizeTags(o.Tags)),
		o.Messenger,
		o.TemplateID,
		pq.Array(listIDs),
		o.Archive,
		o.ArchiveSlug,
		o.ArchiveTemplateID,
		o.ArchiveMeta,
		pq.Array(mediaIDs),
		o.BodySource,
	); err != nil {
		if err == sql.ErrNoRows {
			return models.Campaign{}, echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("campaigns.noSubs"))
		}

		c.log.Printf("error creating campaign: %v", err)
		return models.Campaign{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}

	out, err := c.GetCampaign(newID, "", "")
	if err != nil {
		return models.Campaign{}, err
	}

	return out, nil
}

// UpdateCampaign updates a campaign.
func (c *Core) UpdateCampaign(id int, o models.Campaign, listIDs []int, mediaIDs []int) (models.Campaign, error) {
	if c.isSQLite() {
		return c.updateCampaignSQLite(id, o, listIDs, mediaIDs)
	}

	_, err := c.q.UpdateCampaign.Exec(id,
		o.Name,
		o.Subject,
		o.FromEmail,
		o.Body,
		o.AltBody,
		o.ContentType,
		o.SendAt,
		o.Headers,
		o.Attribs,
		pq.StringArray(normalizeTags(o.Tags)),
		o.Messenger,
		o.TemplateID,
		pq.Array(listIDs),
		o.Archive,
		o.ArchiveSlug,
		o.ArchiveTemplateID,
		o.ArchiveMeta,
		pq.Array(mediaIDs),
		o.BodySource)
	if err != nil {
		c.log.Printf("error updating campaign: %v", err)
		return models.Campaign{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}

	out, err := c.GetCampaign(id, "", "")
	if err != nil {
		return models.Campaign{}, err
	}

	return out, nil
}

// UpdateCampaignStatus updates a campaign's status, eg: draft to running.
func (c *Core) UpdateCampaignStatus(id int, status string) (models.Campaign, error) {
	cm, err := c.GetCampaign(id, "", "")
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

	var res sql.Result
	if c.isSQLite() {
		res, err = c.db.Exec(`UPDATE campaigns SET
			status=(CASE WHEN send_at IS NOT NULL AND ? = 'running' THEN 'scheduled' ELSE ? END),
			updated_at=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE id = ?`, status, status, cm.ID)
	} else {
		res, err = c.q.UpdateCampaignStatus.Exec(cm.ID, status)
	}
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
func (c *Core) UpdateCampaignArchive(id int, enabled bool, tplID int, meta models.JSON, archiveSlug string) error {
	if c.isSQLite() {
		metaJSON, _ := json.Marshal(meta)
		if _, err := c.db.Exec(`UPDATE campaigns SET
			archive=?,
			archive_slug=(CASE WHEN ? = '' THEN NULL ELSE ? END),
			archive_template_id=(CASE WHEN ? > 0 THEN ? ELSE archive_template_id END),
			archive_meta=(CASE WHEN ? != '' THEN ? ELSE archive_meta END),
			updated_at=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE id=?`,
			enabled, archiveSlug, archiveSlug, tplID, tplID, string(metaJSON), string(metaJSON), id); err != nil {
			c.log.Printf("error updating campaign: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
		}
		return nil
	}

	if _, err := c.q.UpdateCampaignArchive.Exec(id, enabled, archiveSlug, tplID, meta); err != nil {
		c.log.Printf("error updating campaign: %v", err)

		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}

	return nil
}

// DeleteCampaign deletes a campaign.
func (c *Core) DeleteCampaign(id int) error {
	res, err := c.q.DeleteCampaign.Exec(id)
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
func (c *Core) DeleteCampaigns(ids []int, query string, hasAllPerm bool, permittedLists []int) error {
	var queryStr string

	if len(ids) > 0 {
		queryStr = ""
	} else {
		queryStr = makeSearchString(query)
	}

	if _, err := c.q.DeleteCampaigns.Exec(pq.Array(ids), queryStr, hasAllPerm, pq.Array(permittedLists)); err != nil {
		c.log.Printf("error deleting campaigns: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.campaigns}", "error", pqErrMsg(err)))
	}

	return nil
}

// CampaignHasLists checks if a campaign has any of the given list IDs.
func (c *Core) CampaignHasLists(id int, listIDs []int) (bool, error) {
	if c.isSQLite() {
		if len(listIDs) == 0 {
			return false, nil
		}

		q := `SELECT EXISTS (
			SELECT 1 FROM campaign_lists WHERE campaign_id = ? AND list_id IN (` + sqlitePlaceholders(len(listIDs)) + `)
		);`
		args := []any{id}
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

	has := false
	if err := c.q.CampaignHasLists.Get(&has, id, pq.Array(listIDs)); err != nil {
		c.log.Printf("error checking campaign lists: %v", err)
		return false, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}

	return has, nil
}

func (c *Core) queryCampaignsSQLite(searchStr string, statuses, tags []string, orderBy, order string, getAll bool, permittedLists []int, offset, limit int) (models.Campaigns, int, error) {
	query := `
	SELECT c.*,
		COUNT(*) OVER() AS total,
		COALESCE((
			SELECT json_group_array(json_object('id', COALESCE(cl.list_id, 0), 'name', cl.list_name))
			FROM campaign_lists cl
			WHERE cl.campaign_id = c.id
		), '[]') AS lists,
		COALESCE((
			SELECT json_group_array(json_object('id', COALESCE(cm.media_id, 0), 'filename', cm.filename))
			FROM campaign_media cm
			WHERE cm.campaign_id = c.id
		), '[]') AS media,
		0 AS views, 0 AS clicks, 0 AS bounces
	FROM campaigns c
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
		query += ` AND EXISTS (SELECT 1 FROM campaign_lists cl WHERE cl.campaign_id = c.id AND cl.list_id IN (` + sqlitePlaceholders(len(permittedLists)) + `))`
		for _, id := range permittedLists {
			args = append(args, id)
		}
	}

	if !strSliceContains(orderBy, campQuerySortFields) {
		orderBy = "created_at"
	}
	if order != SortAsc && order != SortDesc {
		order = SortDesc
	}
	query += ` ORDER BY c.` + orderBy + ` ` + strings.ToUpper(order) + ` LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	var out models.Campaigns
	if err := c.db.Select(&out, query, args...); err != nil {
		c.log.Printf("error fetching campaigns: %v", err)
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}

	total := 0
	if len(out) > 0 {
		total = out[0].Total
	}
	return out, total, nil
}

func (c *Core) getCampaignSQLite(id int, uuid, archiveSlug string, tplType string) (models.Campaign, error) {
	q := `
	SELECT c.*,
		COALESCE(t.body, (SELECT body FROM templates WHERE is_default = 1 LIMIT 1), '') AS template_body,
		COALESCE((
			SELECT json_group_array(json_object('id', COALESCE(cl.list_id, 0), 'name', cl.list_name))
			FROM campaign_lists cl
			WHERE cl.campaign_id = c.id
		), '[]') AS lists,
		COALESCE((
			SELECT json_group_array(json_object('id', COALESCE(cm.media_id, 0), 'filename', cm.filename))
			FROM campaign_media cm
			WHERE cm.campaign_id = c.id
		), '[]') AS media,
		0 AS views, 0 AS clicks, 0 AS bounces
	FROM campaigns c
	LEFT JOIN templates t ON t.id = (CASE WHEN ? = 'default' THEN c.template_id ELSE c.archive_template_id END)
	WHERE `

	args := []any{tplType}
	switch {
	case id > 0:
		q += "c.id = ?"
		args = append(args, id)
	case archiveSlug != "":
		q += "c.archive_slug = ?"
		args = append(args, archiveSlug)
	default:
		q += "c.uuid = ?"
		args = append(args, uuid)
	}
	q += " LIMIT 1"

	var out models.Campaign
	if err := c.db.Get(&out, q, args...); err != nil {
		if err == sql.ErrNoRows {
			return models.Campaign{}, echo.NewHTTPError(http.StatusBadRequest,
				c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.campaign}"))
		}
		c.log.Printf("error fetching campaign: %v", err)
		return models.Campaign{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}

	return out, nil
}

func (c *Core) getCampaignForPreviewSQLite(id, tplID int) (models.Campaign, error) {
	var out models.Campaign
	if err := c.db.Get(&out, `
		SELECT c.*,
			COALESCE(t.body, '') AS template_body,
			COALESCE((
				SELECT json_group_array(json_object('id', COALESCE(cl.list_id, 0), 'name', cl.list_name))
				FROM campaign_lists cl
				WHERE cl.campaign_id = c.id
			), '[]') AS lists
		FROM campaigns c
		LEFT JOIN templates t ON t.id = (CASE WHEN ? = 0 THEN c.template_id ELSE ? END)
		WHERE c.id = ?
	`, tplID, tplID, id); err != nil {
		if err == sql.ErrNoRows {
			return models.Campaign{}, echo.NewHTTPError(http.StatusBadRequest,
				c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.campaign}"))
		}
		c.log.Printf("error fetching campaign: %v", err)
		return models.Campaign{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}

	return out, nil
}

func (c *Core) getArchivedCampaignsSQLite(offset, limit int) (models.Campaigns, int, error) {
	var out models.Campaigns
	if err := c.db.Select(&out, `
		SELECT COUNT(*) OVER() AS total, c.*,
			COALESCE(t.body, (SELECT body FROM templates WHERE is_default = 1 LIMIT 1), '') AS template_body
		FROM campaigns c
		LEFT JOIN templates t ON t.id = c.archive_template_id
		WHERE c.archive = 1 AND c.type = 'regular' AND c.status IN ('running', 'paused', 'finished')
		ORDER BY c.created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset); err != nil {
		c.log.Printf("error fetching public campaigns: %v", err)
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}

	total := 0
	if len(out) > 0 {
		total = out[0].Total
	}
	return out, total, nil
}

func (c *Core) createCampaignSQLite(o models.Campaign, listIDs []int, mediaIDs []int) (models.Campaign, error) {
	uu, err := uuid.NewV4()
	if err != nil {
		c.log.Printf("error generating UUID: %v", err)
		return models.Campaign{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUUID", "error", err.Error()))
	}

	tx, err := c.db.Beginx()
	if err != nil {
		return models.Campaign{}, err
	}
	defer tx.Rollback()

	type tplInfo struct {
		ID         sql.NullInt64  `db:"id"`
		Type       string         `db:"type"`
		Body       string         `db:"body"`
		BodySource sql.NullString `db:"body_source"`
	}

	var tpl tplInfo
	if o.TemplateID.Valid && o.TemplateID.Int > 0 {
		_ = tx.Get(&tpl, `SELECT id, type, body, body_source FROM templates WHERE id = ? LIMIT 1`, o.TemplateID.Int)
	} else if o.ContentType != models.CampaignContentTypeVisual {
		_ = tx.Get(&tpl, `SELECT id, type, body, body_source FROM templates WHERE is_default = 1 LIMIT 1`)
	}

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
		templateID.Int = 0
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

	var newID int
	if err := tx.Get(&newID, `
		INSERT INTO campaigns (
			uuid, type, name, subject, from_email, body, altbody, content_type, send_at,
			headers, attribs, tags, messenger, template_id, to_send, max_subscriber_id,
			archive, archive_slug, archive_template_id, archive_meta, body_source
		) VALUES (
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, ?, ?, ?, ?, ?
		) RETURNING id
	`,
		uu.String(),
		o.Type,
		o.Name,
		o.Subject,
		o.FromEmail,
		body,
		o.AltBody,
		contentType,
		o.SendAt,
		o.Headers,
		o.Attribs,
		pq.StringArray(normalizeTags(o.Tags)),
		o.Messenger,
		templateID,
		o.Archive,
		o.ArchiveSlug,
		o.ArchiveTemplateID,
		o.ArchiveMeta,
		bodySource,
	); err != nil {
		c.log.Printf("error creating campaign: %v", err)
		return models.Campaign{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}

	if len(listIDs) > 0 {
		q := `INSERT OR IGNORE INTO campaign_lists (campaign_id, list_id, list_name)
		      SELECT ?, id, name FROM lists WHERE id IN (` + sqlitePlaceholders(len(listIDs)) + `)`
		args := []any{newID}
		for _, id := range listIDs {
			args = append(args, id)
		}
		if _, err := tx.Exec(q, args...); err != nil {
			return models.Campaign{}, err
		}
	}

	if len(mediaIDs) > 0 {
		q := `INSERT OR IGNORE INTO campaign_media (campaign_id, media_id, filename)
		      SELECT ?, id, filename FROM media WHERE id IN (` + sqlitePlaceholders(len(mediaIDs)) + `)`
		args := []any{newID}
		for _, id := range mediaIDs {
			args = append(args, id)
		}
		if _, err := tx.Exec(q, args...); err != nil {
			return models.Campaign{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return models.Campaign{}, err
	}

	return c.GetCampaign(newID, "", "")
}

func (c *Core) updateCampaignSQLite(id int, o models.Campaign, listIDs []int, mediaIDs []int) (models.Campaign, error) {
	tx, err := c.db.Beginx()
	if err != nil {
		return models.Campaign{}, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		UPDATE campaigns SET
			name=?, subject=?, from_email=?, body=?, altbody=?,
			content_type=?, send_at=?,
			headers=?, attribs=?, tags=?,
			messenger=?, template_id=?,
			archive=?, archive_slug=?, archive_template_id=?, archive_meta=?,
			body_source=?, updated_at=(strftime('%Y-%m-%d %H:%M:%fZ'))
		WHERE id=?
	`,
		o.Name, o.Subject, o.FromEmail, o.Body, o.AltBody,
		o.ContentType, o.SendAt,
		o.Headers, o.Attribs, pq.StringArray(normalizeTags(o.Tags)),
		o.Messenger, o.TemplateID,
		o.Archive, o.ArchiveSlug, o.ArchiveTemplateID, o.ArchiveMeta,
		o.BodySource, id,
	); err != nil {
		c.log.Printf("error updating campaign: %v", err)
		return models.Campaign{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}

	if _, err := tx.Exec(`DELETE FROM campaign_lists WHERE campaign_id = ?`, id); err != nil {
		return models.Campaign{}, err
	}
	if len(listIDs) > 0 {
		q := `INSERT OR IGNORE INTO campaign_lists (campaign_id, list_id, list_name)
		      SELECT ?, id, name FROM lists WHERE id IN (` + sqlitePlaceholders(len(listIDs)) + `)`
		args := []any{id}
		for _, lid := range listIDs {
			args = append(args, lid)
		}
		if _, err := tx.Exec(q, args...); err != nil {
			return models.Campaign{}, err
		}
	}

	if _, err := tx.Exec(`DELETE FROM campaign_media WHERE campaign_id = ?`, id); err != nil {
		return models.Campaign{}, err
	}
	if len(mediaIDs) > 0 {
		q := `INSERT OR IGNORE INTO campaign_media (campaign_id, media_id, filename)
		      SELECT ?, id, filename FROM media WHERE id IN (` + sqlitePlaceholders(len(mediaIDs)) + `)`
		args := []any{id}
		for _, mid := range mediaIDs {
			args = append(args, mid)
		}
		if _, err := tx.Exec(q, args...); err != nil {
			return models.Campaign{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return models.Campaign{}, err
	}
	return c.GetCampaign(id, "", "")
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
	out := []models.CampaignStats{}
	if err := c.q.GetCampaignStatus.Select(&out, models.CampaignStatusRunning); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}

		c.log.Printf("error fetching campaign stats: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	} else if len(out) == 0 {
		return nil, nil
	}

	return out, nil
}

func (c *Core) GetCampaignAnalyticsCounts(campIDs []int, typ, fromDate, toDate string) ([]models.CampaignAnalyticsCount, error) {
	if c.isSQLite() {
		return c.getCampaignAnalyticsCountsSQLite(campIDs, typ, fromDate, toDate)
	}

	// Pick campaign view counts or click counts.
	var stmt *pbdb.Query
	switch typ {
	case "views":
		stmt = c.q.GetCampaignViewCounts
	case "clicks":
		stmt = c.q.GetCampaignClickCounts
	case "bounces":
		stmt = c.q.GetCampaignBounceCounts
	default:
		return nil, echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("globals.messages.invalidData"))
	}

	if !strHasLen(fromDate, 10, 30) || !strHasLen(toDate, 10, 30) {
		return nil, echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("analytics.invalidDates"))
	}

	out := []models.CampaignAnalyticsCount{}
	if err := stmt.Select(&out, pq.Array(campIDs), fromDate, toDate); err != nil {
		c.log.Printf("error fetching campaign %s: %v", typ, err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.analytics}", "error", pqErrMsg(err)))
	}

	return out, nil
}

// GetCampaignAnalyticsLinks returns link click analytics for the given campaign IDs.
func (c *Core) GetCampaignAnalyticsLinks(campIDs []int, typ, fromDate, toDate string) ([]models.CampaignAnalyticsLink, error) {
	if c.isSQLite() {
		return c.getCampaignAnalyticsLinksSQLite(campIDs, fromDate, toDate)
	}

	out := []models.CampaignAnalyticsLink{}
	if err := c.q.GetCampaignLinkCounts.Select(&out, pq.Array(campIDs), fromDate, toDate); err != nil {
		c.log.Printf("error fetching campaign %s: %v", typ, err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.analytics}", "error", pqErrMsg(err)))
	}

	return out, nil
}

// RegisterCampaignView registers a subscriber's view on a campaign.
func (c *Core) RegisterCampaignView(campUUID, subUUID string) error {
	if c.isSQLite() {
		if _, err := c.db.Exec(`
			INSERT INTO campaign_views (campaign_id, subscriber_id, created_at)
			SELECT c.id, s.id, (strftime('%Y-%m-%d %H:%M:%fZ'))
			FROM campaigns c
			LEFT JOIN subscribers s ON s.uuid = ?
			WHERE c.uuid = ?
			LIMIT 1
		`, subUUID, campUUID); err != nil {
			c.log.Printf("error registering campaign view: %s", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
		}
		return nil
	}

	if _, err := c.q.RegisterCampaignView.Exec(campUUID, subUUID); err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Column == "campaign_id" {
			return nil
		}

		c.log.Printf("error registering campaign view: %s", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}
	return nil
}

// GetLinkURL returns the original URL for a link UUID without recording a click.
func (c *Core) GetLinkURL(linkUUID string) (string, error) {
	var url string
	if err := c.q.GetLinkURL.Get(&url, linkUUID); err != nil {
		c.log.Printf("error getting link URL: %s", err)
		return "", echo.NewHTTPError(http.StatusInternalServerError, c.i18n.Ts("public.errorProcessingRequest"))
	}
	return url, nil
}

// RegisterCampaignLinkClick registers a subscriber's link click on a campaign.
func (c *Core) RegisterCampaignLinkClick(linkUUID, campUUID, subUUID string) (string, error) {
	if c.isSQLite() {
		var out struct {
			ID  int    `db:"id"`
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
			INSERT INTO link_clicks (campaign_id, subscriber_id, link_id, created_at)
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

	var url string
	if err := c.q.RegisterLinkClick.Get(&url, linkUUID, campUUID, subUUID); err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Column == "link_id" {
			return "", echo.NewHTTPError(http.StatusBadRequest, c.i18n.Ts("public.invalidLink"))
		}

		c.log.Printf("error registering link click: %s", err)
		return "", echo.NewHTTPError(http.StatusInternalServerError, c.i18n.Ts("public.errorProcessingRequest"))
	}

	return url, nil
}

func (c *Core) getCampaignAnalyticsCountsSQLite(campIDs []int, typ, fromDate, toDate string) ([]models.CampaignAnalyticsCount, error) {
	if !strHasLen(fromDate, 10, 30) || !strHasLen(toDate, 10, 30) {
		return nil, echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("analytics.invalidDates"))
	}
	if len(campIDs) == 0 {
		return []models.CampaignAnalyticsCount{}, nil
	}

	table := ""
	switch typ {
	case "views":
		table = "campaign_views"
	case "clicks":
		table = "link_clicks"
	case "bounces":
		table = "bounces"
	default:
		return nil, echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("globals.messages.invalidData"))
	}

	fromTime, err := time.Parse("2006-01-02", fromDate)
	if err != nil {
		fromTime = time.Now().AddDate(0, 0, -7)
	}
	toTime, err := time.Parse("2006-01-02", toDate)
	if err != nil {
		toTime = time.Now()
	}
	groupFmt := "%Y-%m-%d %H:00:00"
	if toTime.Sub(fromTime).Hours()/24 >= 7 {
		groupFmt = "%Y-%m-%d 00:00:00"
	}

	q := `
		SELECT campaign_id, COUNT(*) AS count, strftime(? , created_at) AS ts
		FROM ` + table + `
		WHERE campaign_id IN (` + sqlitePlaceholders(len(campIDs)) + `)
		  AND created_at >= ?
		  AND created_at <= ?
		GROUP BY campaign_id, ts
		ORDER BY ts ASC`

	args := []any{groupFmt}
	for _, id := range campIDs {
		args = append(args, id)
	}
	args = append(args, fromDate, toDate+" 23:59:59")

	type row struct {
		CampaignID int    `db:"campaign_id"`
		Count      int    `db:"count"`
		TS         string `db:"ts"`
	}
	var rows []row
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

	q := `
		SELECT COUNT(*) AS count, links.url
		FROM link_clicks
		LEFT JOIN links ON link_clicks.link_id = links.id
		WHERE campaign_id IN (` + sqlitePlaceholders(len(campIDs)) + `)
		  AND link_clicks.created_at >= ?
		  AND link_clicks.created_at <= ?
		GROUP BY links.url
		ORDER BY count DESC
		LIMIT 50`

	args := make([]any, 0, len(campIDs)+2)
	for _, id := range campIDs {
		args = append(args, id)
	}
	args = append(args, fromDate, toDate+" 23:59:59")

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
	if _, err := c.q.DeleteCampaignViews.Exec(before); err != nil {
		c.log.Printf("error deleting campaign views: %s", err)
		return echo.NewHTTPError(http.StatusInternalServerError, c.i18n.Ts("public.errorProcessingRequest"))
	}

	return nil
}

// DeleteCampaignLinkClicks deletes campaign views older than a given date.
func (c *Core) DeleteCampaignLinkClicks(before time.Time) error {
	if _, err := c.q.DeleteCampaignLinkClicks.Exec(before); err != nil {
		c.log.Printf("error deleting campaign link clicks: %s", err)
		return echo.NewHTTPError(http.StatusInternalServerError, c.i18n.Ts("public.errorProcessingRequest"))
	}

	return nil
}
