package core

import (
	"encoding/json"
	"github.com/compdani/list_pocket/internal/apperr"

	"github.com/compdani/list_pocket/internal/subimporter"
	"github.com/compdani/list_pocket/models"
)

const bulkContactBatchSize = 5000

// BulkUpdateResult summarizes a bulk update by email.
type BulkUpdateResult struct {
	Matched      int `json:"matched"`
	Skipped      int `json:"skipped"`
	TagsUpdated  int `json:"tags_updated"`
	ListsRemoved int `json:"lists_removed"`
	ListsUpdated int `json:"lists_updated"`
}

// BulkUpdateSubscribersByEmail applies tag and list operations to existing subscribers matched by email.
func (c *Core) BulkUpdateSubscribersByEmail(emails, tagsAdd, tagsRemove, listRemoveRecordIDs, listUpdateRecordIDs []string, subStatus string) (BulkUpdateResult, error) {
	var result BulkUpdateResult
	if len(emails) == 0 {
		return result, apperr.BadRequest(c.i18n.T("campaigns.noKnownSubsToTest"))
	}
	if len(emails) > bulkContactBatchSize {
		return result, apperr.BadRequest(c.i18n.T("globals.messages.invalidData"))
	}

	tagsAdd = subimporter.NormalizeImportTags(tagsAdd)
	tagsRemove = subimporter.NormalizeImportTags(tagsRemove)

	if subStatus == "" {
		subStatus = models.SubscriptionStatusUnconfirmed
	}

	subs, skipped, err := c.lookupSubscribersByEmails(emails)
	if err != nil {
		return result, err
	}
	result.Skipped = skipped
	result.Matched = len(subs)
	if result.Matched == 0 {
		return result, nil
	}

	subIDs := make([]int, 0, len(subs))
	emailsFound := make([]string, 0, len(subs))
	for _, sub := range subs {
		subIDs = append(subIDs, sub.ID)
		emailsFound = append(emailsFound, sub.Email)
	}

	if len(tagsAdd) > 0 || len(tagsRemove) > 0 {
		n, err := c.bulkUpdateSubscriberTags(emailsFound, tagsAdd, tagsRemove)
		if err != nil {
			return result, err
		}
		result.TagsUpdated = n
	}

	listRemoveIDs, err := c.ResolveListIDs(nil, listRemoveRecordIDs)
	if err != nil {
		return result, apperr.BadRequest(c.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}
	if len(listRemoveIDs) > 0 {
		n, err := c.deleteSubscriptionsSQLite(subIDs, listRemoveIDs)
		if err != nil {
			c.log.Printf("error removing bulk subscriptions: %v", err)
			return result, apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
		}
		result.ListsRemoved = n
	}

	listUpdateIDs, err := c.ResolveListIDs(nil, listUpdateRecordIDs)
	if err != nil {
		return result, apperr.BadRequest(c.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}
	if len(listUpdateIDs) > 0 {
		n, err := c.upsertSubscriptionsSQLite(subIDs, listUpdateIDs, subStatus)
		if err != nil {
			c.log.Printf("error upserting bulk subscriptions: %v", err)
			return result, apperr.Internal(c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
		}
		result.ListsUpdated = n
	}

	return result, nil
}

// LookupSubscribersByEmailsForBulk returns subscribers matching the given emails and a skipped count for unknown emails.
func (c *Core) LookupSubscribersByEmailsForBulk(emails []string) (models.Subscribers, int, error) {
	return c.lookupSubscribersByEmails(emails)
}

func (c *Core) lookupSubscribersByEmails(emails []string) (models.Subscribers, int, error) {
	seen := make(map[string]struct{}, len(emails))
	unique := make([]string, 0, len(emails))
	for _, email := range emails {
		if email == "" {
			continue
		}
		key := email
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, email)
	}

	if len(unique) == 0 {
		return nil, len(emails), nil
	}

	args := make([]any, 0, len(unique))
	for _, email := range unique {
		args = append(args, email)
	}

	var rows []sqliteSubscriberRow
	q := `SELECT rowid AS id, id AS record_id, created AS created_at, updated AS updated_at, uuid, email, phone, first_name, last_name, name, attribs, status FROM subscribers WHERE email IN (` + sqlitePlaceholders(len(unique)) + `) ORDER BY rowid`
	if err := c.db.Select(&rows, q, args...); err != nil {
		c.log.Printf("error fetching subscribers by email: %v", err)
		return nil, 0, apperr.Internal(c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", pqErrMsg(err)))
	}

	found := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		found[row.Email] = struct{}{}
	}

	skipped := 0
	for _, email := range unique {
		if _, ok := found[email]; !ok {
			skipped++
		}
	}

	return sqliteSubscriberRowsToModels(rows), skipped, nil
}

func (c *Core) bulkUpdateSubscriberTags(emails, tagsAdd, tagsRemove []string) (int, error) {
	if len(emails) == 0 {
		return 0, nil
	}

	updated := 0
	for i := 0; i < len(emails); i += 300 {
		end := i + 300
		if end > len(emails) {
			end = len(emails)
		}
		chunk := emails[i:end]

		args := make([]any, 0, len(chunk))
		for _, email := range chunk {
			args = append(args, email)
		}

		var rows []struct {
			Email   string `db:"email"`
			Attribs []byte `db:"attribs"`
		}
		q := `SELECT email, attribs FROM subscribers WHERE email IN (` + sqlitePlaceholders(len(chunk)) + `)`
		if err := c.db.Select(&rows, q, args...); err != nil {
			return updated, err
		}

		for _, row := range rows {
			attribs := models.JSON{}
			if len(row.Attribs) > 0 && string(row.Attribs) != "null" {
				_ = json.Unmarshal(row.Attribs, &attribs)
			}
			merged := subimporter.ApplyImportTagChanges(attribs, tagsAdd, tagsRemove)
			if _, err := c.db.Exec(`
UPDATE subscribers
SET attribs = ?, updated = (strftime('%Y-%m-%d %H:%M:%fZ'))
WHERE email = ?;
`, merged, row.Email); err != nil {
				return updated, err
			}
			updated++
		}
	}

	return updated, nil
}

func (c *Core) upsertSubscriptionsSQLite(subIDs, listIDs []int, status string) (int, error) {
	if len(subIDs) == 0 || len(listIDs) == 0 {
		return 0, nil
	}

	affected := 0
	for i := 0; i < len(subIDs); i += 300 {
		end := i + 300
		if end > len(subIDs) {
			end = len(subIDs)
		}
		subChunk := subIDs[i:end]

		for j := 0; j < len(listIDs); j += 100 {
			listEnd := j + 100
			if listEnd > len(listIDs) {
				listEnd = len(listIDs)
			}
			listChunk := listIDs[j:listEnd]

			q := `
				INSERT INTO subscriber_lists (subscriber_id, list_id, status, sms_status, updated)
				SELECT s.id, l.id,
				       CASE WHEN s.status = 'blocklisted' THEN 'unsubscribed' ELSE ? END,
				       CASE WHEN s.status = 'blocklisted' THEN 'unsubscribed' ELSE ? END,
				       (strftime('%Y-%m-%d %H:%M:%fZ'))
				FROM subscribers s
				CROSS JOIN lists l
				WHERE s.rowid IN (` + sqlitePlaceholders(len(subChunk)) + `)
				  AND l.rowid IN (` + sqlitePlaceholders(len(listChunk)) + `)
				ON CONFLICT (subscriber_id, list_id) DO UPDATE SET
					status = excluded.status,
					sms_status = excluded.sms_status,
					updated = excluded.updated
			`

			args := make([]any, 0, 2+len(subChunk)+len(listChunk))
			args = append(args, status, status)
			for _, id := range subChunk {
				args = append(args, id)
			}
			for _, id := range listChunk {
				args = append(args, id)
			}

			res, err := c.db.Exec(q, args...)
			if err != nil {
				return affected, err
			}
			n, _ := res.RowsAffected()
			affected += int(n)
		}
	}

	return affected, nil
}
