package pbdb

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
)

// DB wraps a sqlx connection that is sourced from a PocketBase app DB handle.
//
// The embedded *sqlx.DB is PocketBase's single-connection write pool. Read-only
// queries issued through Select/Get/Queryx/QueryRowx are routed to the
// concurrent (multi-connection) pool instead, so they don't queue behind writes.
// SQLite runs in WAL mode, so readers see every committed write.
type DB struct {
	*sqlx.DB
	reader *sqlx.DB
	pb     *pocketbase.PocketBase
}

// NewFromPocketBase creates a sqlx-compatible DB from the active PocketBase DB.
func NewFromPocketBase(pb *pocketbase.PocketBase) (*DB, error) {
	if pb == nil {
		return nil, fmt.Errorf("pocketbase instance is nil")
	}

	writer, err := unwrapDBX(pb.NonconcurrentDB())
	if err != nil {
		return nil, err
	}

	d := &DB{DB: writer, pb: pb}

	// The read pool is optional; without it every query uses the writer.
	if reader, err := unwrapDBX(pb.ConcurrentDB()); err == nil {
		d.reader = reader
	}

	return d, nil
}

// unwrapDBX returns a sqlx handle for one of PocketBase's dbx pools. The pools
// may be wrapped in a builder that doesn't expose *dbx.DB directly.
func unwrapDBX(builder any) (*sqlx.DB, error) {
	var dbxDB *dbx.DB
	if direct, ok := builder.(*dbx.DB); ok {
		dbxDB = direct
	} else if carrier, ok := builder.(interface{ DB() *dbx.DB }); ok {
		dbxDB = carrier.DB()
	}
	if dbxDB == nil {
		return nil, fmt.Errorf("pocketbase db handle is nil")
	}

	sqlDB := dbxDB.DB()
	if sqlDB == nil {
		return nil, fmt.Errorf("pocketbase sql db handle is nil")
	}

	driver := dbxDB.DriverName()
	if driver == "" {
		driver = "sqlite3"
	}

	return sqlx.NewDb(sqlDB, driver).Unsafe(), nil
}

func MustNewFromPocketBase(pb *pocketbase.PocketBase) *DB {
	db, err := NewFromPocketBase(pb)
	if err != nil {
		panic(err)
	}

	return db
}

func NewFromSQLX(db *sqlx.DB) *DB {
	if db == nil {
		return nil
	}

	return &DB{DB: db}
}

func (d *DB) PocketBase() *pocketbase.PocketBase {
	if d == nil {
		return nil
	}

	return d.pb
}

var (
	sqlCommentRegexp = regexp.MustCompile(`(?s)^\s*(--[^\n]*\n|/\*.*?\*/)`)
	sqlWriteRegexp   = regexp.MustCompile(`(?i)\b(insert|update|delete|replace|returning)\b`)
)

// isReadOnlySQL reports whether query is a plain SELECT, or a WITH ... SELECT
// that has no write keywords. Anything it is unsure about counts as a write.
func isReadOnlySQL(query string) bool {
	q := query
	for {
		loc := sqlCommentRegexp.FindStringIndex(q)
		if loc == nil {
			break
		}
		q = q[loc[1]:]
	}
	q = strings.ToLower(strings.TrimSpace(q))

	switch {
	case strings.HasPrefix(q, "select"):
		return !strings.Contains(q, "returning")
	case strings.HasPrefix(q, "with"):
		return !sqlWriteRegexp.MatchString(q)
	}
	return false
}

func (d *DB) pick(query string) *sqlx.DB {
	if d.reader != nil && isReadOnlySQL(query) {
		return d.reader
	}
	return d.DB
}

func (d *DB) Select(dest any, query string, args ...any) error {
	return d.pick(query).Select(dest, query, args...)
}

func (d *DB) SelectContext(ctx context.Context, dest any, query string, args ...any) error {
	return d.pick(query).SelectContext(ctx, dest, query, args...)
}

func (d *DB) Get(dest any, query string, args ...any) error {
	return d.pick(query).Get(dest, query, args...)
}

func (d *DB) GetContext(ctx context.Context, dest any, query string, args ...any) error {
	return d.pick(query).GetContext(ctx, dest, query, args...)
}

func (d *DB) Queryx(query string, args ...any) (*sqlx.Rows, error) {
	return d.pick(query).Queryx(query, args...)
}

func (d *DB) QueryRowx(query string, args ...any) *sqlx.Row {
	return d.pick(query).QueryRowx(query, args...)
}

func (d *DB) Query(query string, args ...any) (*sql.Rows, error) {
	return d.pick(query).Query(query, args...)
}

func (d *DB) QueryRow(query string, args ...any) *sql.Row {
	return d.pick(query).QueryRow(query, args...)
}
