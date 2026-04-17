package core

import (
	"github.com/compdani/list_pocket/internal/phoneutil"
)

// SMSOptOutSubscriberByPhone sets sms_status to unsubscribed for all list
// memberships belonging to subscribers whose phone matches (digit-normalized).
// It does NOT change email list status.
//
// We match in two passes so Quo STOP replies reliably opt out the right
// subscriber regardless of how the phone was originally stored:
//
//  1. Exact match on the fully-digit-normalized phone. This covers the common
//     case where both sides were run through NormalizePhone and look like
//     "+15551234567".
//  2. Last-10-digits match, when both sides have at least 10 digits. This
//     rescues historical subscribers imported without a country code (e.g.
//     "(555) 123-4567" stored raw -> "5551234567") — Quo always sends the
//     full E.164 form (+15551234567), so without this fallback the exact
//     match above fails and the opt-out silently updates zero rows.
//
// The last-10-digit fallback is the industry norm (Twilio, OpenPhone, Sinch
// all effectively dedupe US/CA numbers by last 10 digits) and is safe enough
// in this codebase because subscribers are overwhelmingly North American and
// the downside of a false-positive opt-out ("I still got matched against
// another country's number that happens to share my last 10 digits") is
// negligible compared to failing to honor an opt-out.
func (c *Core) SMSOptOutSubscriberByPhone(phone string) (int64, error) {
	d := phoneutil.NormalizeDigits(phone)
	if d == "" {
		return 0, nil
	}

	// SQL-side digit normalization for the stored phone. Kept identical in
	// both the equality branch and the last-10-digits branch so both see the
	// same cleaned string.
	const normalizePhoneSQL = `REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(COALESCE(phone, ''), '+', ''), ' ', ''), '-', ''), '(', ''), ')', ''), '.', '')`

	// Match either:
	//   - exact digit match
	//   - both sides have >= 10 digits AND last 10 digits agree
	query := `
UPDATE subscriber_lists
SET sms_status = 'unsubscribed',
    updated = (strftime('%Y-%m-%d %H:%M:%fZ', 'now'))
WHERE subscriber_id IN (
  SELECT id FROM subscribers
  WHERE ` + normalizePhoneSQL + ` = ?
     OR (
       LENGTH(` + normalizePhoneSQL + `) >= 10
       AND LENGTH(?) >= 10
       AND SUBSTR(` + normalizePhoneSQL + `, -10) = SUBSTR(?, -10)
     )
)`

	res, err := c.db.Exec(query, d, d, d)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
