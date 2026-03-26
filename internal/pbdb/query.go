package pbdb

import (
	"database/sql"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/pocketbase/dbx"
)

var (
	dollarArgRegexp = regexp.MustCompile(`\$(\d+)`)
	scannerType     = reflect.TypeOf((*sql.Scanner)(nil)).Elem()
)

// Query wraps a SQL query and executes it through PocketBase's dbx builder.
type Query struct {
	db   *DB
	sql  string
	once sync.Once
	stmt *sql.Stmt
	err  error
}

func NewQuery(db *DB, sql string) *Query {
	return &Query{db: db, sql: sql}
}

func (q *Query) SQL() string {
	if q == nil {
		return ""
	}

	return q.sql
}

func (q *Query) Select(dest any, args ...any) error {
	nq, err := q.bind(args...)
	if err != nil {
		return err
	}

	return nq.All(dest)
}

func (q *Query) Get(dest any, args ...any) error {
	nq, err := q.bind(args...)
	if err != nil {
		return err
	}

	if isColumnDest(dest) {
		return nq.Column(dest)
	}

	return nq.One(dest)
}

func (q *Query) Exec(args ...any) (sql.Result, error) {
	nq, err := q.bind(args...)
	if err != nil {
		return nil, err
	}

	return nq.Execute()
}

// SQLStmt returns a cached prepared *sql.Stmt for call sites that require it.
func (q *Query) SQLStmt() (*sql.Stmt, error) {
	if q == nil || q.db == nil || q.db.DB == nil || q.db.DB.DB == nil {
		return nil, fmt.Errorf("query is not initialized")
	}

	q.once.Do(func() {
		q.stmt, q.err = q.db.DB.DB.Prepare(q.sql)
	})

	return q.stmt, q.err
}

func (q *Query) bind(args ...any) (*dbx.Query, error) {
	if q == nil || q.db == nil || q.db.PocketBase() == nil {
		return nil, fmt.Errorf("query is not initialized")
	}

	sqlStr, params, err := toNamedParams(q.sql, args...)
	if err != nil {
		return nil, err
	}

	nq := q.db.PocketBase().DB().NewQuery(sqlStr)
	if len(params) > 0 {
		nq.Bind(params)
	}

	return nq, nil
}

func toNamedParams(sqlStr string, args ...any) (string, dbx.Params, error) {
	if len(args) == 0 {
		return sqlStr, nil, nil
	}

	if strings.Contains(sqlStr, "$") {
		params := dbx.Params{}
		sqlStr = dollarArgRegexp.ReplaceAllStringFunc(sqlStr, func(ph string) string {
			m := dollarArgRegexp.FindStringSubmatch(ph)
			if len(m) != 2 {
				return ph
			}

			n, err := strconv.Atoi(m[1])
			if err != nil || n < 1 || n > len(args) {
				// Leave out-of-range placeholders untouched. This avoids false
				// positives from placeholders that appear in SQL comments.
				return ph
			}

			params["p"+m[1]] = args[n-1]
			return "{:p" + m[1] + "}"
		})

		return sqlStr, params, nil
	}

	if strings.Contains(sqlStr, "?") {
		params := dbx.Params{}
		var (
			i int
			b strings.Builder
		)
		b.Grow(len(sqlStr) + (len(args) * 6))

		for _, ch := range sqlStr {
			if ch != '?' {
				b.WriteRune(ch)
				continue
			}

			i++
			if i > len(args) {
				return "", nil, fmt.Errorf("placeholder ? references missing arg %d", i)
			}
			name := fmt.Sprintf("p%d", i)
			params[name] = args[i-1]
			b.WriteString("{:" + name + "}")
		}

		return b.String(), params, nil
	}

	return sqlStr, nil, nil
}

func isColumnDest(dest any) bool {
	if dest == nil {
		return false
	}

	t := reflect.TypeOf(dest)
	if t.Kind() != reflect.Ptr {
		return false
	}

	e := t.Elem()
	if e.Kind() == reflect.Invalid {
		return false
	}

	if e.Kind() == reflect.Slice && e.Elem().Kind() == reflect.Uint8 {
		return true
	}

	if e.Kind() != reflect.Struct {
		return true
	}

	if e.Implements(scannerType) || reflect.PointerTo(e).Implements(scannerType) {
		return true
	}

	return false
}
