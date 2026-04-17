package core

import (
	"github.com/compdani/list_pocket/internal/phoneutil"
)

// SMSOptOutSubscriberByPhone sets sms_status to unsubscribed for all list memberships
// for subscribers whose phone matches (digit-normalized). Does not change email list status.
func (c *Core) SMSOptOutSubscriberByPhone(phone string) (int64, error) {
	d := phoneutil.NormalizeDigits(phone)
	if d == "" {
		return 0, nil
	}
	res, err := c.db.Exec(`
UPDATE subscriber_lists
SET sms_status = 'unsubscribed',
    updated = (strftime('%Y-%m-%d %H:%M:%fZ', 'now'))
WHERE subscriber_id IN (
  SELECT id FROM subscribers
  WHERE replace(replace(replace(replace(replace(replace(COALESCE(phone, ''), '+', ''), ' ', ''), '-', ''), '(', ''), ')', ''), '.', '') = ?
)`, d)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
