package pbdb

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
)

// DB wraps a sqlx connection that is sourced from a PocketBase app DB handle.
type DB struct {
	*sqlx.DB
	pb *pocketbase.PocketBase
}

// NewFromPocketBase creates a sqlx-compatible DB from the active PocketBase DB.
func NewFromPocketBase(pb *pocketbase.PocketBase) (*DB, error) {
	if pb == nil {
		return nil, fmt.Errorf("pocketbase instance is nil")
	}

	// pb.DB() may return PocketBase's dual routing builder wrapper, which doesn't
	// expose the raw *dbx.DB handle. Use concrete pools directly.
	var dbxDB *dbx.DB
	for _, builder := range []any{pb.NonconcurrentDB(), pb.ConcurrentDB()} {
		if direct, ok := builder.(*dbx.DB); ok && direct != nil {
			dbxDB = direct
			break
		}

		carrier, ok := builder.(interface{ DB() *dbx.DB })
		if !ok {
			continue
		}
		dbxDB = carrier.DB()
		if dbxDB != nil {
			break
		}
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

	return &DB{
		DB: sqlx.NewDb(sqlDB, driver).Unsafe(),
		pb: pb,
	}, nil
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
