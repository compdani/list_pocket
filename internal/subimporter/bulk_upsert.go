package subimporter

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/compdani/list_pocket/models"
	"github.com/gofrs/uuid/v5"
)

const bulkUpsertBatchSize = 500

// BulkUpsertOpt configures a synchronous JSON bulk upsert.
type BulkUpsertOpt struct {
	TagsAdd            []string
	TagsRemove         []string
	ListUpdate         []string
	ListRemove         []string
	SubscriptionStatus string
	OverrideDetails    bool
}

// BulkUpsertResult summarizes a bulk upsert run.
type BulkUpsertResult struct {
	Created      int `json:"created"`
	Updated      int `json:"updated"`
	Skipped      int `json:"skipped"`
	TagsUpdated  int `json:"tags_updated"`
	ListsRemoved int `json:"lists_removed"`
	ListsUpdated int `json:"lists_updated"`
}

// BulkUpsertContacts upserts contacts synchronously without using the CSV import session.
func (im *Importer) BulkUpsertContacts(contacts []SubReq, opt BulkUpsertOpt) (BulkUpsertResult, error) {
	var result BulkUpsertResult
	if len(contacts) == 0 {
		return result, nil
	}

	opt.TagsAdd = NormalizeImportTags(opt.TagsAdd)
	opt.TagsRemove = NormalizeImportTags(opt.TagsRemove)
	opt.ListUpdate = uniqueNonEmptyStrings(opt.ListUpdate)
	opt.ListRemove = uniqueNonEmptyStrings(opt.ListRemove)

	subStatus := opt.SubscriptionStatus
	if subStatus == "" {
		subStatus = models.SubscriptionStatusUnconfirmed
	}

	listUpdateJSON := "[]"
	if len(opt.ListUpdate) > 0 {
		b, err := json.Marshal(opt.ListUpdate)
		if err != nil {
			return result, err
		}
		listUpdateJSON = string(b)
	}

	overwrite := opt.OverrideDetails
	overwriteListStatus := len(opt.ListUpdate) > 0

	for i := 0; i < len(contacts); i += bulkUpsertBatchSize {
		end := i + bulkUpsertBatchSize
		if end > len(contacts) {
			end = len(contacts)
		}
		batch := contacts[i:end]

		tx, err := im.db.Begin()
		if err != nil {
			return result, err
		}

		batchResult, err := im.bulkUpsertContactsTx(tx, batch, opt, listUpdateJSON, subStatus, overwrite, overwriteListStatus)
		if err != nil {
			_ = tx.Rollback()
			return result, err
		}

		if err := tx.Commit(); err != nil {
			return result, err
		}

		result.Created += batchResult.Created
		result.Updated += batchResult.Updated
		result.Skipped += batchResult.Skipped
		result.TagsUpdated += batchResult.TagsUpdated
		result.ListsRemoved += batchResult.ListsRemoved
		result.ListsUpdated += batchResult.ListsUpdated
	}

	return result, nil
}

func (im *Importer) bulkUpsertContactsTx(tx *sql.Tx, contacts []SubReq, opt BulkUpsertOpt, listUpdateJSON, subStatus string, overwrite, overwriteListStatus bool) (BulkUpsertResult, error) {
	var result BulkUpsertResult

	for _, contact := range contacts {
		email := contact.Email
		if email == "" {
			result.Skipped++
			continue
		}

		existed, err := subscriberEmailExistsTx(tx, email)
		if err != nil {
			return result, err
		}

		uu, err := uuid.NewV4()
		if err != nil {
			return result, err
		}

		if _, err = tx.Exec(sqliteUpsertSubscriber,
			uu, email, contact.Phone, contact.FirstName, contact.LastName, contact.Name, contact.Attribs,
			overwrite, overwrite, overwrite, overwrite, overwrite,
		); err != nil {
			return result, fmt.Errorf("upsert subscriber %q: %w", email, err)
		}

		if existed {
			result.Updated++
		} else {
			result.Created++
		}

		if len(opt.ListUpdate) > 0 {
			res, err := tx.Exec(sqliteUpsertSubscriberLists,
				subStatus, subStatus, listUpdateJSON, email,
				overwriteListStatus, overwriteListStatus,
			)
			if err != nil {
				return result, fmt.Errorf("upsert lists for %q: %w", email, err)
			}
			n, _ := res.RowsAffected()
			result.ListsUpdated += int(n)
		}

		if len(opt.TagsAdd) > 0 || len(opt.TagsRemove) > 0 {
			if err := applyBulkTagChangesTx(tx, email, opt.TagsAdd, opt.TagsRemove); err != nil {
				return result, fmt.Errorf("update tags for %q: %w", email, err)
			}
			result.TagsUpdated++
		}

		if len(opt.ListRemove) > 0 {
			n, err := removeSubscriberListsTx(tx, email, opt.ListRemove)
			if err != nil {
				return result, fmt.Errorf("remove lists for %q: %w", email, err)
			}
			result.ListsRemoved += n
		}
	}

	return result, nil
}

func subscriberEmailExistsTx(tx *sql.Tx, email string) (bool, error) {
	var n int
	err := tx.QueryRow(`SELECT COUNT(1) FROM subscribers WHERE email = ?`, email).Scan(&n)
	return n > 0, err
}

func applyBulkTagChangesTx(tx *sql.Tx, email string, tagsAdd, tagsRemove []string) error {
	var rawAttribs sql.NullString
	if err := tx.QueryRow(`SELECT attribs FROM subscribers WHERE email = ?`, email).Scan(&rawAttribs); err != nil {
		return err
	}

	var attribs models.JSON
	if rawAttribs.Valid && rawAttribs.String != "" {
		if err := json.Unmarshal([]byte(rawAttribs.String), &attribs); err != nil {
			attribs = nil
		}
	}

	merged := ApplyImportTagChanges(attribs, tagsAdd, tagsRemove)
	_, err := tx.Exec(`
UPDATE subscribers
SET attribs = ?, updated = (strftime('%Y-%m-%d %H:%M:%fZ'))
WHERE email = ?;
`, merged, email)
	return err
}

func removeSubscriberListsTx(tx *sql.Tx, email string, listRecordIDs []string) (int, error) {
	if len(listRecordIDs) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(listRecordIDs))
	args := make([]any, 0, len(listRecordIDs)+1)
	args = append(args, email)
	for i, id := range listRecordIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}

	q := `DELETE FROM subscriber_lists
WHERE subscriber_id = (SELECT id FROM subscribers WHERE email = ?)
  AND list_id IN (` + strings.Join(placeholders, ",") + `)`

	res, err := tx.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func uniqueNonEmptyStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
