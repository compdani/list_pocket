package core

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/compdani/list_pocket/models"
)

func toFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case string:
		var f float64
		_, _ = fmt.Sscanf(t, "%f", &f)
		return f
	}
	return 0
}

// toString converts an arbitrary value to string.
func toString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return fmt.Sprint(t)
	}
}

func parseSQLiteDateTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid datetime: %q", raw)
}

func decodeJSONFieldToModelJSON(v any) (models.JSON, error) {
	out := models.JSON{}
	if v == nil {
		return out, nil
	}
	var data []byte
	switch t := v.(type) {
	case []byte:
		data = t
	case string:
		data = []byte(t)
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return nil, err
		}
		data = b
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Core) subscriberRecordIDByRowID(subscriberID int) (string, error) {
	var recID string
	if err := c.db.Get(&recID, `SELECT id FROM subscribers WHERE rowid = ?`, subscriberID); err != nil {
		return "", err
	}
	return recID, nil
}

func (c *Core) inferSingleListRecordIDForSubscriberRow(subscriberID int) (string, error) {
	rows := []struct {
		ListRecordID string `db:"list_record_id"`
	}{}
	err := c.db.Select(&rows, `
		SELECT DISTINCT l.id AS list_record_id
		FROM subscriber_lists sl
		JOIN subscribers s ON s.id = sl.subscriber_id
		JOIN lists l ON l.id = sl.list_id
		WHERE s.rowid = ?
		ORDER BY l.rowid ASC
		LIMIT 2
	`, subscriberID)
	if err != nil {
		return "", err
	}
	if len(rows) != 1 {
		return "", nil
	}
	return rows[0].ListRecordID, nil
}
