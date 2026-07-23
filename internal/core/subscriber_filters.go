package core

import (
	"encoding/json"
	"fmt"
	"github.com/compdani/list_pocket/internal/apperr"
	"regexp"
	"strconv"
	"strings"
)

// SubscriberFilters is a structured AND/OR tree used to segment subscribers
// without raw SQL. Nested groups are limited to one level under the root.
type SubscriberFilters struct {
	Logic string                 `json:"logic"`
	Rules []SubscriberFilterNode `json:"rules"`
}

// SubscriberFilterNode is either a leaf rule (Field set) or a nested group (Rules set).
type SubscriberFilterNode struct {
	Logic string                 `json:"logic,omitempty"`
	Rules []SubscriberFilterNode `json:"rules,omitempty"`

	Field string          `json:"field,omitempty"`
	Op    string          `json:"op,omitempty"`
	Key   string          `json:"key,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

var attribKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_]+(\.[a-zA-Z0-9_]+)*$`)

const maxFilterDepth = 2

// ParseSubscriberFilters unmarshals and lightly validates a filters JSON payload.
// Empty or null payloads return nil (no filter).
func ParseSubscriberFilters(raw json.RawMessage) (*SubscriberFilters, error) {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return nil, nil
	}

	var f SubscriberFilters
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, apperr.BadRequest(fmt.Sprintf("invalid filters: %v", err))
	}
	if len(f.Rules) == 0 {
		return nil, nil
	}
	if err := validateFilterGroup(&f.Logic, f.Rules, 1); err != nil {
		return nil, err
	}
	return &f, nil
}

// CompileSubscriberFilters turns structured filters into a parameterized SQL fragment.
// List membership rules resolve PocketBase list record IDs to rowids via Core.
func (c *Core) CompileSubscriberFilters(raw json.RawMessage) (string, []any, error) {
	f, err := ParseSubscriberFilters(raw)
	if err != nil {
		return "", nil, err
	}
	if f == nil {
		return "", nil, nil
	}
	return c.compileFilterGroup(f.Logic, f.Rules)
}

func validateFilterGroup(logic *string, rules []SubscriberFilterNode, depth int) error {
	if depth > maxFilterDepth {
		return apperr.BadRequest("filters nest too deeply (max one nested group)")
	}
	l := strings.ToLower(strings.TrimSpace(*logic))
	if l == "" {
		l = "and"
	}
	if l != "and" && l != "or" {
		return apperr.BadRequest("filters.logic must be 'and' or 'or'")
	}
	*logic = l

	for i := range rules {
		if err := validateFilterNode(&rules[i], depth); err != nil {
			return err
		}
	}
	return nil
}

func validateFilterNode(n *SubscriberFilterNode, depth int) error {
	isGroup := len(n.Rules) > 0
	if isGroup {
		if strings.TrimSpace(n.Field) != "" {
			return apperr.BadRequest("filter group cannot also set field")
		}
		return validateFilterGroup(&n.Logic, n.Rules, depth+1)
	}

	field := strings.ToLower(strings.TrimSpace(n.Field))
	op := strings.ToLower(strings.TrimSpace(n.Op))
	n.Field = field
	n.Op = op

	switch field {
	case "tag":
		if !isOneOf(op, "has_any", "has_all", "has_none") {
			return apperr.BadRequest("invalid tag operator")
		}
	case "attrib":
		if !isOneOf(op, "eq", "neq", "contains", "exists", "not_exists", "gt", "gte", "lt", "lte") {
			return apperr.BadRequest("invalid attrib operator")
		}
		key := strings.TrimSpace(n.Key)
		if key == "" || !attribKeyPattern.MatchString(key) {
			return apperr.BadRequest("invalid attrib key")
		}
		if key == "tags" {
			return apperr.BadRequest("use the tag field to filter on tags")
		}
		n.Key = key
	case "email", "name", "phone":
		if !isOneOf(op, "eq", "contains", "starts_with", "ends_with") {
			return apperr.BadRequest("invalid " + field + " operator")
		}
	case "status":
		if !isOneOf(op, "eq", "neq") {
			return apperr.BadRequest("invalid status operator")
		}
	case "list":
		if !isOneOf(op, "in", "not_in") {
			return apperr.BadRequest("invalid list operator")
		}
	default:
		return apperr.BadRequest("unknown filter field: " + n.Field)
	}
	return nil
}

func (c *Core) compileFilterGroup(logic string, rules []SubscriberFilterNode) (string, []any, error) {
	parts := make([]string, 0, len(rules))
	args := []any{}
	for i := range rules {
		sql, a, err := c.compileFilterNode(&rules[i])
		if err != nil {
			return "", nil, err
		}
		if sql == "" {
			continue
		}
		parts = append(parts, sql)
		args = append(args, a...)
	}
	if len(parts) == 0 {
		return "", nil, nil
	}
	joiner := " AND "
	if logic == "or" {
		joiner = " OR "
	}
	return "(" + strings.Join(parts, joiner) + ")", args, nil
}

func (c *Core) compileFilterNode(n *SubscriberFilterNode) (string, []any, error) {
	if len(n.Rules) > 0 {
		return c.compileFilterGroup(n.Logic, n.Rules)
	}

	switch n.Field {
	case "tag":
		return compileTagRule(n)
	case "attrib":
		return compileAttribRule(n)
	case "email", "name", "phone":
		return compileTextFieldRule(n)
	case "status":
		return compileStatusRule(n)
	case "list":
		return c.compileListRule(n)
	default:
		return "", nil, apperr.BadRequest("unknown filter field: " + n.Field)
	}
}

func compileTagRule(n *SubscriberFilterNode) (string, []any, error) {
	tags, err := parseStringSlice(n.Value)
	if err != nil {
		return "", nil, apperr.BadRequest("invalid tag value")
	}
	tags = normalizeFilterStrings(tags)
	if len(tags) == 0 {
		return "", nil, nil
	}

	tagEach := `json_each(COALESCE(json_extract(subscribers.attribs, '$.tags'), '[]'))`
	match := `lower(trim(CAST(jt.value AS TEXT))) = ?`

	switch n.Op {
	case "has_any":
		placeholders := sqlitePlaceholders(len(tags))
		args := make([]any, len(tags))
		for i, t := range tags {
			args[i] = t
		}
		sql := `EXISTS (
			SELECT 1 FROM ` + tagEach + ` jt
			WHERE lower(trim(CAST(jt.value AS TEXT))) IN (` + placeholders + `)
		)`
		return sql, args, nil
	case "has_all":
		parts := make([]string, 0, len(tags))
		args := make([]any, 0, len(tags))
		for _, t := range tags {
			parts = append(parts, `EXISTS (SELECT 1 FROM `+tagEach+` jt WHERE `+match+`)`)
			args = append(args, t)
		}
		return "(" + strings.Join(parts, " AND ") + ")", args, nil
	case "has_none":
		placeholders := sqlitePlaceholders(len(tags))
		args := make([]any, len(tags))
		for i, t := range tags {
			args[i] = t
		}
		sql := `NOT EXISTS (
			SELECT 1 FROM ` + tagEach + ` jt
			WHERE lower(trim(CAST(jt.value AS TEXT))) IN (` + placeholders + `)
		)`
		return sql, args, nil
	default:
		return "", nil, apperr.BadRequest("invalid tag operator")
	}
}

func compileAttribRule(n *SubscriberFilterNode) (string, []any, error) {
	path := "$." + n.Key
	extract := `json_extract(subscribers.attribs, ?)`

	switch n.Op {
	case "exists":
		return `(` + extract + ` IS NOT NULL)`, []any{path}, nil
	case "not_exists":
		return `(` + extract + ` IS NULL)`, []any{path}, nil
	case "eq", "neq", "contains":
		val, err := parseScalarString(n.Value)
		if err != nil || strings.TrimSpace(val) == "" {
			return "", nil, nil
		}
		switch n.Op {
		case "eq":
			return `(CAST(` + extract + ` AS TEXT) = ? COLLATE NOCASE)`, []any{path, val}, nil
		case "neq":
			return `(CAST(` + extract + ` AS TEXT) != ? COLLATE NOCASE OR ` + extract + ` IS NULL)`, []any{path, val, path}, nil
		case "contains":
			return `(CAST(` + extract + ` AS TEXT) LIKE ? COLLATE NOCASE)`, []any{path, "%" + val + "%"}, nil
		}
	case "gt", "gte", "lt", "lte":
		num, err := parseScalarNumber(n.Value)
		if err != nil {
			return "", nil, apperr.BadRequest("attrib numeric comparison requires a number")
		}
		op := map[string]string{"gt": ">", "gte": ">=", "lt": "<", "lte": "<="}[n.Op]
		return `(CAST(` + extract + ` AS REAL) ` + op + ` ?)`, []any{path, num}, nil
	}
	return "", nil, apperr.BadRequest("invalid attrib operator")
}

func compileTextFieldRule(n *SubscriberFilterNode) (string, []any, error) {
	val, err := parseScalarString(n.Value)
	if err != nil {
		return "", nil, apperr.BadRequest("invalid " + n.Field + " value")
	}
	val = strings.TrimSpace(val)
	if val == "" {
		return "", nil, nil
	}
	col := "subscribers." + n.Field
	switch n.Op {
	case "eq":
		return `(` + col + ` = ? COLLATE NOCASE)`, []any{val}, nil
	case "contains":
		return `(` + col + ` LIKE ? COLLATE NOCASE)`, []any{"%" + val + "%"}, nil
	case "starts_with":
		return `(` + col + ` LIKE ? COLLATE NOCASE)`, []any{val + "%"}, nil
	case "ends_with":
		return `(` + col + ` LIKE ? COLLATE NOCASE)`, []any{"%" + val}, nil
	default:
		return "", nil, apperr.BadRequest("invalid " + n.Field + " operator")
	}
}

func compileStatusRule(n *SubscriberFilterNode) (string, []any, error) {
	val, err := parseScalarString(n.Value)
	if err != nil {
		return "", nil, apperr.BadRequest("invalid status value")
	}
	val = strings.ToLower(strings.TrimSpace(val))
	if !isOneOf(val, "enabled", "disabled", "blocklisted") {
		return "", nil, apperr.BadRequest("invalid status value")
	}
	switch n.Op {
	case "eq":
		return `(subscribers.status = ?)`, []any{val}, nil
	case "neq":
		return `(subscribers.status != ?)`, []any{val}, nil
	default:
		return "", nil, apperr.BadRequest("invalid status operator")
	}
}

func (c *Core) compileListRule(n *SubscriberFilterNode) (string, []any, error) {
	recordIDs, err := parseStringSlice(n.Value)
	if err != nil {
		return "", nil, apperr.BadRequest("invalid list value")
	}
	recordIDs = normalizeRecordIDs(recordIDs)
	if len(recordIDs) == 0 {
		return "", nil, nil
	}

	listIDs, err := c.ResolveListIDs(nil, recordIDs)
	if err != nil {
		return "", nil, apperr.BadRequest(fmt.Sprintf("invalid list ids: %v", err))
	}
	if len(listIDs) == 0 {
		// No matching lists → in matches nothing; not_in matches everything.
		if n.Op == "in" {
			return `(1=0)`, nil, nil
		}
		return `(1=1)`, nil, nil
	}

	placeholders := sqlitePlaceholders(len(listIDs))
	args := make([]any, len(listIDs))
	for i, id := range listIDs {
		args[i] = id
	}
	exists := `EXISTS (
		SELECT 1 FROM subscriber_lists sl
		JOIN lists l ON l.id = sl.list_id
		WHERE sl.subscriber_id = subscribers.id
		  AND l.rowid IN (` + placeholders + `)
	)`
	if n.Op == "not_in" {
		return `NOT ` + exists, args, nil
	}
	return exists, args, nil
}

func isOneOf(v string, opts ...string) bool {
	for _, o := range opts {
		if v == o {
			return true
		}
	}
	return false
}

func normalizeFilterStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
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

func normalizeRecordIDs(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
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

func parseScalarString(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String(), nil
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		if b {
			return "true", nil
		}
		return "false", nil
	}
	return "", fmt.Errorf("expected string")
}

func parseScalarNumber(raw json.RawMessage) (float64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, fmt.Errorf("missing number")
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return f, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strconv.ParseFloat(strings.TrimSpace(s), 64)
	}
	return 0, fmt.Errorf("expected number")
}

func parseStringSlice(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if strings.TrimSpace(s) == "" {
			return nil, nil
		}
		return []string{s}, nil
	}
	return nil, fmt.Errorf("expected string or string array")
}
