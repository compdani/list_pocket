package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/internal/events"
	"github.com/compdani/list_pocket/internal/manager"
	"github.com/compdani/list_pocket/internal/media"
	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/compdani/list_pocket/models"
	"github.com/gofrs/uuid/v5"
	"github.com/lib/pq"
	null "gopkg.in/volatiletech/null.v6"
)

// store implements DataSource over the primary
// database.
type store struct {
	queries *models.Queries
	db      *pbdb.DB
	sqlite  bool
	core    *core.Core
	media   media.Store
	log     *log.Logger
	events  *events.Events
}

type runningCamp struct {
	CampaignID       int    `db:"campaign_id"`
	CampaignType     string `db:"campaign_type"`
	LastSubscriberID int    `db:"last_subscriber_id"`
	MaxSubscriberID  int    `db:"max_subscriber_id"`
	ListID           int    `db:"list_id"`
}

type sqliteStoreSubscriberRow struct {
	ID        int    `db:"id"`
	CreatedAt string `db:"created_at"`
	UpdatedAt string `db:"updated_at"`
	UUID      string `db:"uuid"`
	Email     string `db:"email"`
	Name      string `db:"name"`
	Attribs   []byte `db:"attribs"`
	Status    string `db:"status"`
}

func newManagerStore(q *models.Queries, db *pbdb.DB, c *core.Core, m media.Store, l *log.Logger, ev *events.Events) *store {
	return &store{
		queries: q,
		db:      db,
		sqlite:  isSQLiteDB(db),
		core:    c,
		media:   m,
		log:     l,
		events:  ev,
	}
}

func (s *store) publishCampaignStatsEvent(reason string, campaignID int) {
	if s.events == nil {
		return
	}

	if err := s.events.Publish(events.Event{
		Type: events.TypeCampaignStats,
		Data: map[string]any{
			"campaign_id": campaignID,
			"reason":      reason,
		},
	}); err != nil {
		s.log.Printf("error publishing campaign stats event: %v", err)
	}
}

func parseStoreNullTime(value string) null.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return null.Time{}
	}

	t, err := time.Parse("2006-01-02 15:04:05.000Z", value)
	if err != nil {
		t, err = time.Parse(time.RFC3339, value)
		if err != nil {
			return null.Time{}
		}
	}

	return null.NewTime(t, true)
}

func sqliteStoreSubscriberRowsToModels(rows []sqliteStoreSubscriberRow) []models.Subscriber {
	out := make([]models.Subscriber, 0, len(rows))
	for _, row := range rows {
		attribs := models.JSON{}
		if len(row.Attribs) > 0 && string(row.Attribs) != "null" {
			_ = json.Unmarshal(row.Attribs, &attribs)
		}

		out = append(out, models.Subscriber{
			Base: models.Base{
				ID:        row.ID,
				CreatedAt: parseStoreNullTime(row.CreatedAt),
				UpdatedAt: parseStoreNullTime(row.UpdatedAt),
			},
			UUID:    row.UUID,
			Email:   row.Email,
			Name:    row.Name,
			Attribs: attribs,
			Status:  row.Status,
		})
	}
	return out
}

func (s *store) sqliteCampaignRecordID(campID int) (string, error) {
	var recID string
	if err := s.db.Get(&recID, `SELECT id FROM campaigns WHERE rowid = ?`, campID); err != nil {
		return "", err
	}
	return recID, nil
}

func (s *store) sqliteSubscriberRecordID(subID int64) (string, error) {
	var recID string
	if err := s.db.Get(&recID, `SELECT id FROM subscribers WHERE rowid = ?`, subID); err != nil {
		return "", err
	}
	return recID, nil
}

// NextCampaigns retrieves active campaigns ready to be processed excluding
// campaigns that are also being processed. Additionally, it takes a map of campaignID:sentCount
// of campaigns that are being processed and updates them in the DB.
func (s *store) NextCampaigns(currentIDs []int64, sentCounts []int64) ([]*models.Campaign, error) {
	if s.sqlite {
		s.log.Printf("manager store sqlite: next campaigns current_ids=%v sent_counts=%v", currentIDs, sentCounts)
		return s.nextCampaignsSQLite(currentIDs, sentCounts)
	}

	var out []*models.Campaign
	err := s.queries.NextCampaigns.Select(&out, pq.Int64Array(currentIDs), pq.Int64Array(sentCounts))
	return out, err
}

// NextSubscribers retrieves a subset of subscribers of a given campaign.
// Since batches are processed sequentially, the retrieval is ordered by ID,
// and every batch takes the last ID of the last batch and fetches the next
// batch above that.
func (s *store) NextSubscribers(campID, limit int) ([]models.Subscriber, bool, error) {
	if s.sqlite {
		return s.nextSubscribersSQLite(campID, limit)
	}

	var camps []runningCamp
	if err := s.queries.GetRunningCampaign.Select(&camps, campID); err != nil {
		return nil, false, err
	}

	var listIDs []int
	for _, c := range camps {
		listIDs = append(listIDs, c.ListID)
	}

	if len(listIDs) == 0 {
		return nil, false, nil
	}

	var out []models.Subscriber
	err := s.queries.NextCampaignSubscribers.Select(&out, camps[0].CampaignID, camps[0].CampaignType, camps[0].LastSubscriberID, camps[0].MaxSubscriberID, pq.Array(listIDs), limit+1)
	if err != nil {
		return nil, false, err
	}
	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}
	return out, hasMore, nil
}

// GetCampaign fetches a campaign from the database.
func (s *store) GetCampaign(campID int) (*models.Campaign, error) {
	if s.sqlite {
		recordID, err := s.sqliteCampaignRecordID(campID)
		if err != nil {
			return nil, err
		}

		c, err := s.core.GetCampaign(recordID, "", "")
		if err != nil {
			return nil, err
		}
		return &c, nil
	}

	var out = &models.Campaign{}
	err := s.queries.GetCampaign.Get(out, campID, nil, nil, "default")
	return out, err
}

// UpdateCampaignStatus updates a campaign's status.
func (s *store) UpdateCampaignStatus(campID int, status string) error {
	if s.sqlite {
		_, err := s.db.Exec(`
			UPDATE campaigns
			SET status=(CASE WHEN send_at IS NOT NULL AND send_at != '' AND ? = 'running' THEN 'scheduled' ELSE ? END),
			    updated=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE rowid = ?`,
			status, status, campID)
		if err == nil {
			s.publishCampaignStatsEvent("status", campID)
		}
		return err
	}

	_, err := s.queries.UpdateCampaignStatus.Exec(campID, status)
	if err == nil {
		s.publishCampaignStatsEvent("status", campID)
	}
	return err
}

func (s *store) ScheduleCampaignBatch(campID int, sendAt time.Time) error {
	if s.sqlite {
		_, err := s.db.Exec(`
			UPDATE campaigns
			SET status = 'scheduled',
			    send_at = ?,
			    updated=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE rowid = ?`,
			sendAt.UTC().Format("2006-01-02 15:04:05.000Z"), campID)
		if err == nil {
			s.publishCampaignStatsEvent("schedule-batch", campID)
		}
		return err
	}

	_, err := s.db.Exec(`
		UPDATE campaigns
		SET status='scheduled', send_at=$1, updated_at=NOW()
		WHERE id=$2`, sendAt.UTC(), campID)
	if err == nil {
		s.publishCampaignStatsEvent("schedule-batch", campID)
	}
	return err
}

// UpdateCampaignCounts updates a campaign's status.
func (s *store) UpdateCampaignCounts(campID int, toSend int, sent int, lastSubID int) error {
	if s.sqlite {
		_, err := s.db.Exec(`
			UPDATE campaigns SET
				to_send=(CASE WHEN ? != 0 THEN ? ELSE to_send END),
				sent=sent+?,
				last_subscriber_id=(CASE WHEN ? > 0 THEN ? ELSE to_send END),
				updated=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE rowid=?`,
			toSend, toSend, sent, lastSubID, lastSubID, campID)
		if err == nil {
			s.publishCampaignStatsEvent("counts", campID)
		}
		return err
	}

	_, err := s.queries.UpdateCampaignCounts.Exec(campID, toSend, sent, lastSubID)
	if err == nil {
		s.publishCampaignStatsEvent("counts", campID)
	}
	return err
}

func (s *store) nextCampaignsSQLite(currentIDs []int64, sentCounts []int64) ([]*models.Campaign, error) {
	statsChanged := false

	for i, id := range currentIDs {
		if i >= len(sentCounts) || sentCounts[i] == 0 {
			continue
		}

		if _, err := s.db.Exec(`
			UPDATE campaigns
			SET sent = sent + ?,
			    updated=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE rowid = ?`,
			sentCounts[i], id); err != nil {
			return nil, err
		}
		statsChanged = true
	}

	base := `
		SELECT c.rowid
		FROM campaigns c
		WHERE (
			c.status='running' OR
			(c.status='scheduled' AND c.send_at IS NOT NULL AND c.send_at != '' AND datetime('now') >= datetime(c.send_at))
		)
	`

	args := make([]any, 0, len(currentIDs))
	if len(currentIDs) > 0 {
		base += " AND c.rowid NOT IN (" + placeholders(len(currentIDs)) + ")"
		for _, id := range currentIDs {
			args = append(args, id)
		}
	}

	base += " ORDER BY c.rowid"

	var rowIDs []int
	if err := s.db.Select(&rowIDs, base, args...); err != nil {
		return nil, err
	}
	s.log.Printf("manager store sqlite: runnable campaign rowids=%v", rowIDs)

	campaigns := make([]*models.Campaign, 0, len(rowIDs))
	for _, rowID := range rowIDs {
		campaignRecID, err := s.sqliteCampaignRecordID(rowID)
		if err != nil {
			return nil, err
		}

		campaign, err := s.core.GetCampaign(campaignRecID, "", "")
		if err != nil {
			return nil, err
		}
		campaigns = append(campaigns, &campaign)
	}

	for _, c := range campaigns {
		campaignRecID, err := s.sqliteCampaignRecordID(c.ID)
		if err != nil {
			return nil, err
		}

		var meta struct {
			ToSend int `db:"to_send"`
			MaxID  int `db:"max_subscriber_id"`
		}
		if err := s.db.Get(&meta, `
			SELECT
				COUNT(DISTINCT s.rowid) AS to_send,
				COALESCE(MAX(s.rowid), 0) AS max_subscriber_id
			FROM campaign_lists cl
			JOIN lists l ON l.id = cl.list_id
			JOIN subscriber_lists sl ON sl.list_id = cl.list_id
			JOIN subscribers s ON s.id = sl.subscriber_id
			WHERE cl.campaign_id = ?
			  AND s.status != 'blocklisted'
			  AND (
			    (? = 'optin' AND sl.status = 'unconfirmed' AND l.optin = 'double') OR
			    (? != 'optin' AND (
			      (l.optin = 'double' AND sl.status = 'confirmed') OR
			      (l.optin != 'double' AND sl.status != 'unsubscribed')
			    ))
			  )
		`, campaignRecID, c.Type, c.Type); err != nil {
			return nil, err
		}
		s.log.Printf("manager store sqlite: campaign rowid=%d record_id=%q type=%q to_send=%d max_subscriber_id=%d", c.ID, campaignRecID, c.Type, meta.ToSend, meta.MaxID)

		if _, err := s.db.Exec(`
				UPDATE campaigns
				SET to_send = ?,
				    status = (CASE WHEN status != 'running' THEN 'running' ELSE status END),
				    max_subscriber_id = ?,
				    started_at = (CASE WHEN started_at IS NULL THEN (strftime('%Y-%m-%d %H:%M:%fZ')) ELSE started_at END),
				    updated=(strftime('%Y-%m-%d %H:%M:%fZ'))
				WHERE rowid = ?`,
			meta.ToSend, meta.MaxID, c.ID); err != nil {
			return nil, err
		}
		statsChanged = true
		c.ToSend = meta.ToSend

		var mediaIDs []int64
		if err := s.db.Select(&mediaIDs, `
			SELECT m.rowid
			FROM campaign_media cm
			JOIN media m ON m.id = cm.media_id
			WHERE cm.campaign_id = ? AND cm.media_id IS NOT NULL
			ORDER BY m.rowid`, campaignRecID); err != nil {
			return nil, err
		}
		c.MediaIDs = mediaIDs
	}

	if statsChanged {
		s.publishCampaignStatsEvent("scan", 0)
	}

	return campaigns, nil
}

func (s *store) nextSubscribersSQLite(campID, limit int) ([]models.Subscriber, bool, error) {
	var camps []runningCamp
	if err := s.db.Select(&camps, `
		SELECT campaigns.rowid AS campaign_id,
		       campaigns.type AS campaign_type,
		       campaigns.last_subscriber_id AS last_subscriber_id,
		       campaigns.max_subscriber_id AS max_subscriber_id,
		       lists.rowid AS list_id
		FROM campaigns
		LEFT JOIN campaign_lists ON campaign_lists.campaign_id = campaigns.id
		LEFT JOIN lists ON lists.id = campaign_lists.list_id
		WHERE campaigns.rowid = ? AND campaigns.status='running'
	`, campID); err != nil {
		return nil, false, err
	}

	listIDs := make([]int, 0, len(camps))
	for _, c := range camps {
		if c.ListID > 0 {
			listIDs = append(listIDs, c.ListID)
		}
	}
	if len(listIDs) == 0 {
		s.log.Printf("manager store sqlite: next subscribers campaign_id=%d has no lists", campID)
		return nil, false, nil
	}

	c := camps[0]
	args := []any{c.CampaignType, c.CampaignType, c.LastSubscriberID, c.MaxSubscriberID}
	for _, id := range listIDs {
		args = append(args, id)
	}
	args = append(args, limit)

	// Reorder args for placeholders in query.
	args = []any{c.LastSubscriberID, c.MaxSubscriberID}
	for _, id := range listIDs {
		args = append(args, id)
	}
	args = append(args, c.CampaignType, c.CampaignType, limit+1)

	var rows []sqliteStoreSubscriberRow
	if err := s.db.Select(&rows, `
		SELECT s.rowid AS id,
		       s.created AS created_at,
		       s.updated AS updated_at,
		       s.uuid,
		       s.email,
		       s.name,
		       s.attribs,
		       s.status
		FROM subscribers s
		JOIN subscriber_lists sl ON sl.subscriber_id = s.id
		JOIN lists l ON l.id = sl.list_id
		WHERE s.rowid > ?
		  AND s.rowid <= ?
		  AND s.status != 'blocklisted'
		  AND l.rowid IN (`+placeholders(len(listIDs))+`)
		  AND (
		    (? = 'optin' AND sl.status = 'unconfirmed' AND l.optin = 'double') OR
		    (? != 'optin' AND (
		      (l.optin = 'double' AND sl.status = 'confirmed') OR
		      (l.optin != 'double' AND sl.status != 'unsubscribed')
		    ))
		  )
		GROUP BY s.rowid
		ORDER BY s.rowid
		LIMIT ?
	`, args...); err != nil {
		return nil, false, err
	}
	out := sqliteStoreSubscriberRowsToModels(rows)
	s.log.Printf("manager store sqlite: next subscribers campaign_id=%d list_ids=%v fetched=%d limit=%d", campID, listIDs, len(out), limit)
	if len(out) == 0 {
		return nil, false, nil
	}

	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}

	lastID := out[len(out)-1].ID
	if _, err := s.db.Exec(`
		UPDATE campaigns
		SET last_subscriber_id = ?,
		    updated=(strftime('%Y-%m-%d %H:%M:%fZ'))
		WHERE rowid = ?`, lastID, campID); err != nil {
		return nil, false, err
	}

	return out, hasMore, nil
}

func placeholders(n int) string {
	if n < 1 {
		return ""
	}

	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// GetAttachment fetches a media attachment blob.
func (s *store) GetAttachment(mediaID int) (models.Attachment, error) {
	m, err := s.core.GetMedia(mediaID, "", "", s.media)
	if err != nil {
		return models.Attachment{}, err
	}

	b, err := s.media.GetBlob(m.URL)
	if err != nil {
		return models.Attachment{}, err
	}

	return models.Attachment{
		Name:    m.Filename,
		Content: b,
		Header:  manager.MakeAttachmentHeader(m.Filename, "base64", m.ContentType),
	}, nil
}

// CreateLink registers a URL with a UUID for tracking clicks and returns the UUID.
func (s *store) CreateLink(url string) (string, error) {
	// Create a new UUID for the URL. If the URL already exists in the DB
	// the UUID in the database is returned.
	uu, err := uuid.NewV4()
	if err != nil {
		return "", err
	}

	if s.sqlite {
		var out string
		if err := s.db.Get(&out, `
			INSERT INTO links (uuid, url)
			VALUES (?, ?)
			ON CONFLICT(url) DO UPDATE SET url=excluded.url
			RETURNING uuid`,
			uu, url); err != nil {
			return "", err
		}
		return out, nil
	}

	var out string
	if err := s.queries.CreateLink.Get(&out, uu, url); err != nil {
		return "", err
	}

	return out, nil
}

// RecordBounce records a bounce event and returns the bounce count.
func (s *store) RecordBounce(b models.Bounce) (int64, int, error) {
	if s.sqlite {
		subID := int64(0)
		if b.SubscriberUUID != "" {
			if err := s.db.Get(&subID, `SELECT id FROM subscribers WHERE uuid = ? LIMIT 1`, b.SubscriberUUID); err != nil {
				if err == sql.ErrNoRows {
					return 0, 0, nil
				}
				return 0, 0, err
			}
		} else if b.Email != "" {
			if err := s.db.Get(&subID, `SELECT id FROM subscribers WHERE email = ? LIMIT 1`, b.Email); err != nil {
				if err == sql.ErrNoRows {
					return 0, 0, nil
				}
				return 0, 0, err
			}
		}

		if subID == 0 {
			return 0, 0, nil
		}

		var campID sql.NullInt64
		if b.CampaignUUID != "" {
			_ = s.db.Get(&campID, `SELECT id FROM campaigns WHERE uuid = ? LIMIT 1`, b.CampaignUUID)
		}

		metaJSON := b.Meta
		if len(metaJSON) == 0 {
			metaJSON = json.RawMessage(`{}`)
		}

		if _, err := s.db.Exec(`
			INSERT INTO bounces (subscriber_id, campaign_id, type, source, meta, created)
			VALUES (?, ?, ?, ?, ?, (strftime('%Y-%m-%d %H:%M:%fZ')))`,
			subID, campID, b.Type, b.Source, string(metaJSON)); err != nil {
			return 0, 0, err
		}

		num := 0
		if err := s.db.Get(&num, `SELECT COUNT(*) FROM bounces WHERE subscriber_id = ? AND type = ?`, subID, b.Type); err != nil {
			return subID, 0, err
		}
		return subID, num, nil
	}

	var res = struct {
		SubscriberID int64 `db:"subscriber_id"`
		Num          int   `db:"num"`
	}{}

	err := s.queries.UpdateCampaignStatus.Select(&res,
		b.SubscriberUUID,
		b.Email,
		b.CampaignUUID,
		b.Type,
		b.Source,
		b.Meta)

	return res.SubscriberID, res.Num, err
}

// BlocklistSubscriber blocklists a subscriber permanently.
func (s *store) BlocklistSubscriber(id int64) error {
	if s.sqlite {
		recID, err := s.sqliteSubscriberRecordID(id)
		if err != nil {
			return err
		}

		pb := s.db.PocketBase()
		rec, err := pb.FindRecordById("subscribers", recID)
		if err != nil {
			return err
		}
		rec.Set("status", "blocklisted")
		if err := pb.Save(rec); err != nil {
			return err
		}

		_, err = s.db.Exec(`
			UPDATE subscriber_lists
			SET status='unsubscribed', updated=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE subscriber_id = ?`, recID)
		return err
	}

	_, err := s.queries.BlocklistSubscribers.Exec(pq.Int64Array{id})
	return err
}

// DeleteSubscriber deletes a subscriber from the DB.
func (s *store) DeleteSubscriber(id int64) error {
	if s.sqlite {
		recID, err := s.sqliteSubscriberRecordID(id)
		if err != nil {
			return err
		}
		pb := s.db.PocketBase()
		rec, err := pb.FindRecordById("subscribers", recID)
		if err != nil {
			return err
		}
		return pb.Delete(rec)
	}

	_, err := s.queries.DeleteSubscribers.Exec(pq.Int64Array{id})
	return err
}
