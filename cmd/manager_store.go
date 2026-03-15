package main

import (
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/gofrs/uuid/v5"
	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/internal/manager"
	"github.com/knadh/listmonk/internal/media"
	"github.com/knadh/listmonk/internal/pbdb"
	"github.com/knadh/listmonk/models"
	"github.com/lib/pq"
)

// store implements DataSource over the primary
// database.
type store struct {
	queries *models.Queries
	db      *pbdb.DB
	sqlite  bool
	core    *core.Core
	media   media.Store
}

type runningCamp struct {
	CampaignID       int    `db:"campaign_id"`
	CampaignType     string `db:"campaign_type"`
	LastSubscriberID int    `db:"last_subscriber_id"`
	MaxSubscriberID  int    `db:"max_subscriber_id"`
	ListID           int    `db:"list_id"`
}

func newManagerStore(q *models.Queries, db *pbdb.DB, c *core.Core, m media.Store) *store {
	return &store{
		queries: q,
		db:      db,
		sqlite:  isSQLiteDB(db),
		core:    c,
		media:   m,
	}
}

// NextCampaigns retrieves active campaigns ready to be processed excluding
// campaigns that are also being processed. Additionally, it takes a map of campaignID:sentCount
// of campaigns that are being processed and updates them in the DB.
func (s *store) NextCampaigns(currentIDs []int64, sentCounts []int64) ([]*models.Campaign, error) {
	if s.sqlite {
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
func (s *store) NextSubscribers(campID, limit int) ([]models.Subscriber, error) {
	if s.sqlite {
		return s.nextSubscribersSQLite(campID, limit)
	}

	var camps []runningCamp
	if err := s.queries.GetRunningCampaign.Select(&camps, campID); err != nil {
		return nil, err
	}

	var listIDs []int
	for _, c := range camps {
		listIDs = append(listIDs, c.ListID)
	}

	if len(listIDs) == 0 {
		return nil, nil
	}

	var out []models.Subscriber
	err := s.queries.NextCampaignSubscribers.Select(&out, camps[0].CampaignID, camps[0].CampaignType, camps[0].LastSubscriberID, camps[0].MaxSubscriberID, pq.Array(listIDs), limit)
	return out, err
}

// GetCampaign fetches a campaign from the database.
func (s *store) GetCampaign(campID int) (*models.Campaign, error) {
	if s.sqlite {
		c, err := s.core.GetCampaign(campID, "", "")
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
			SET status=(CASE WHEN send_at IS NOT NULL AND ? = 'running' THEN 'scheduled' ELSE ? END),
			    updated_at=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE id = ?`,
			status, status, campID)
		return err
	}

	_, err := s.queries.UpdateCampaignStatus.Exec(campID, status)
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
				updated_at=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE id=?`,
			toSend, toSend, sent, lastSubID, lastSubID, campID)
		return err
	}

	_, err := s.queries.UpdateCampaignCounts.Exec(campID, toSend, sent, lastSubID)
	return err
}

func (s *store) nextCampaignsSQLite(currentIDs []int64, sentCounts []int64) ([]*models.Campaign, error) {
	for i, id := range currentIDs {
		if i >= len(sentCounts) || sentCounts[i] == 0 {
			continue
		}

		if _, err := s.db.Exec(`
			UPDATE campaigns
			SET sent = sent + ?,
			    updated_at=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE id = ?`,
			sentCounts[i], id); err != nil {
			return nil, err
		}
	}

	base := `
		SELECT c.*,
		       COALESCE(t.body, (SELECT body FROM templates WHERE is_default = 1 LIMIT 1), '') AS template_body
		FROM campaigns c
		LEFT JOIN templates t ON t.id = c.template_id
		WHERE (
			c.status='running' OR
			(c.status='scheduled' AND c.send_at IS NOT NULL AND datetime('now') >= datetime(c.send_at))
		)
	`

	args := make([]any, 0, len(currentIDs))
	if len(currentIDs) > 0 {
		base += " AND c.id NOT IN (" + placeholders(len(currentIDs)) + ")"
		for _, id := range currentIDs {
			args = append(args, id)
		}
	}

	var campaigns []*models.Campaign
	if err := s.db.Select(&campaigns, base, args...); err != nil {
		return nil, err
	}

	for _, c := range campaigns {
		var meta struct {
			ToSend int `db:"to_send"`
			MaxID  int `db:"max_subscriber_id"`
		}
		if err := s.db.Get(&meta, `
			SELECT
				COUNT(DISTINCT sl.subscriber_id) AS to_send,
				COALESCE(MAX(sl.subscriber_id), 0) AS max_subscriber_id
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
		`, c.ID, c.Type, c.Type); err != nil {
			return nil, err
		}

		if _, err := s.db.Exec(`
			UPDATE campaigns
			SET to_send = ?,
			    status = (CASE WHEN status != 'running' THEN 'running' ELSE status END),
			    max_subscriber_id = ?,
			    started_at = (CASE WHEN started_at IS NULL THEN (strftime('%Y-%m-%d %H:%M:%fZ')) ELSE started_at END),
			    updated_at=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE id = ?`,
			meta.ToSend, meta.MaxID, c.ID); err != nil {
			return nil, err
		}
		c.ToSend = meta.ToSend

		var mediaIDs []int64
		if err := s.db.Select(&mediaIDs, `
			SELECT media_id
			FROM campaign_media
			WHERE campaign_id = ? AND media_id IS NOT NULL
			ORDER BY media_id`, c.ID); err != nil {
			return nil, err
		}
		c.MediaIDs = mediaIDs
	}

	return campaigns, nil
}

func (s *store) nextSubscribersSQLite(campID, limit int) ([]models.Subscriber, error) {
	var camps []runningCamp
	if err := s.db.Select(&camps, `
		SELECT campaigns.id AS campaign_id,
		       campaigns.type AS campaign_type,
		       campaigns.last_subscriber_id AS last_subscriber_id,
		       campaigns.max_subscriber_id AS max_subscriber_id,
		       lists.id AS list_id
		FROM campaigns
		LEFT JOIN campaign_lists ON campaign_lists.campaign_id = campaigns.id
		LEFT JOIN lists ON lists.id = campaign_lists.list_id
		WHERE campaigns.id = ? AND campaigns.status='running'
	`, campID); err != nil {
		return nil, err
	}

	listIDs := make([]int, 0, len(camps))
	for _, c := range camps {
		if c.ListID > 0 {
			listIDs = append(listIDs, c.ListID)
		}
	}
	if len(listIDs) == 0 {
		return nil, nil
	}

	c := camps[0]
	args := []any{c.CampaignType, c.CampaignType, c.LastSubscriberID, c.MaxSubscriberID}
	for _, id := range listIDs {
		args = append(args, id)
	}
	args = append(args, limit)

	q := `
		SELECT s.*
		FROM subscribers s
		JOIN subscriber_lists sl ON sl.subscriber_id = s.id
		JOIN lists l ON l.id = sl.list_id
		WHERE s.id > ?
		  AND s.id <= ?
		  AND s.status != 'blocklisted'
		  AND sl.list_id IN (` + placeholders(len(listIDs)) + `)
		  AND (
		    (? = 'optin' AND sl.status = 'unconfirmed' AND l.optin = 'double') OR
		    (? != 'optin' AND (
		      (l.optin = 'double' AND sl.status = 'confirmed') OR
		      (l.optin != 'double' AND sl.status != 'unsubscribed')
		    ))
		  )
		GROUP BY s.id
		ORDER BY s.id
		LIMIT ?
	`

	// Reorder args for placeholders in query.
	args = []any{c.LastSubscriberID, c.MaxSubscriberID}
	for _, id := range listIDs {
		args = append(args, id)
	}
	args = append(args, c.CampaignType, c.CampaignType, limit)

	var out []models.Subscriber
	if err := s.db.Select(&out, q, args...); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}

	lastID := out[len(out)-1].ID
	if _, err := s.db.Exec(`
		UPDATE campaigns
		SET last_subscriber_id = ?,
		    updated_at=(strftime('%Y-%m-%d %H:%M:%fZ'))
		WHERE id = ?`, lastID, campID); err != nil {
		return nil, err
	}

	return out, nil
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
			INSERT INTO bounces (subscriber_id, campaign_id, type, source, meta, created_at)
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
		if _, err := s.db.Exec(`
			UPDATE subscribers
			SET status='blocklisted', updated_at=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE id = ?`, id); err != nil {
			return err
		}

		_, err := s.db.Exec(`
			UPDATE subscriber_lists
			SET status='unsubscribed', updated_at=(strftime('%Y-%m-%d %H:%M:%fZ'))
			WHERE subscriber_id = ?`, id)
		return err
	}

	_, err := s.queries.BlocklistSubscribers.Exec(pq.Int64Array{id})
	return err
}

// DeleteSubscriber deletes a subscriber from the DB.
func (s *store) DeleteSubscriber(id int64) error {
	if s.sqlite {
		_, err := s.db.Exec(`DELETE FROM subscribers WHERE id = ?`, id)
		return err
	}

	_, err := s.queries.DeleteSubscribers.Exec(pq.Int64Array{id})
	return err
}
