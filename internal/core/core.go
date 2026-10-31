// package core is the collection of re-usable functions that primarily provides data (DB / CRUD) operations
// to the app. For instance, creating and mutating objects like lists, subscribers etc.
// All such methods return an *apperr.Error (which implements error) that can be directly returned
// as a response to HTTP handlers without further processing.
package core

import (
	"bytes"
	"encoding/json"
	"log"
	"regexp"
	"slices"
	"strings"

	"github.com/compdani/list_pocket/internal/apperr"
	"github.com/compdani/list_pocket/internal/i18n"
	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/compdani/list_pocket/models"
	"github.com/jmoiron/sqlx/types"
)

const (
	SortAsc  = "asc"
	SortDesc = "desc"
)

// Core represents the listmonk core with all shared, global functions.
type Core struct {
	h *Hooks

	consts Constants
	i18n   *i18n.I18n
	db     *pbdb.DB
	log    *log.Logger

	getSettings      func() (types.JSONText, error)
	setSettings      func(types.JSONText) error
	setSettingsByKey func(string, json.RawMessage) error

	stats statsCacheMem
}

// Constants represents constant config.
type Constants struct {
	SendOptinConfirmation bool
	BounceActions         map[string]struct {
		Count  int
		Action string
	}
	CacheSlowQueries bool
}

// Hooks contains external function hooks that are required by the core package.
type Hooks struct {
	SendOptinConfirmation func(models.Subscriber, []int) (int, error)
	// OnChannelInbound is told about each new inbound message on a plugin channel.
	OnChannelInbound func(ChannelEvent)
}

// Opt contains the controllers required to start the core.
type Opt struct {
	Constants Constants
	I18n      *i18n.I18n
	DB        *pbdb.DB
	Log       *log.Logger

	GetSettings      func() (types.JSONText, error)
	SetSettings      func(types.JSONText) error
	SetSettingsByKey func(string, json.RawMessage) error
}

var (
	ErrNotFound = apperr.NotFound("not found")
)

var (
	regexpSpaces        = regexp.MustCompile(`[\s]+`)
	listQuerySortFields = []string{"name", "status", "created_at", "updated_at", "subscriber_count"}
)

// New returns a new instance of the core.
func New(o *Opt, h *Hooks) *Core {
	return &Core{
		h:      h,
		consts: o.Constants,
		i18n:   o.I18n,
		db:     o.DB,
		log:    o.Log,

		getSettings:      o.GetSettings,
		setSettings:      o.SetSettings,
		setSettingsByKey: o.SetSettingsByKey,
	}
}

func dbErr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// likeEsc is appended after a `LIKE ?` whose argument was built with the
// like* helpers, so that % and _ typed by users match literally.
const likeEsc = ` ESCAPE '\'`

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// likeContains returns a LIKE pattern that matches s anywhere.
func likeContains(s string) string { return "%" + likeEscaper.Replace(s) + "%" }

// likePrefix returns a LIKE pattern that matches values starting with s.
func likePrefix(s string) string { return likeEscaper.Replace(s) + "%" }

// likeSuffix returns a LIKE pattern that matches values ending with s.
func likeSuffix(s string) string { return "%" + likeEscaper.Replace(s) }

// strSliceContains checks if a string is present in the string slice.
func strSliceContains(str string, sl []string) bool {
	return slices.Contains(sl, str)
}

// normalizeTags takes a list of string tags and normalizes them by
// lower casing and removing all special characters except for dashes.
func normalizeTags(tags []string) []string {
	var (
		out  []string
		dash = []byte("-")
	)

	for _, t := range tags {
		rep := regexpSpaces.ReplaceAll(bytes.TrimSpace([]byte(t)), dash)

		if len(rep) > 0 {
			out = append(out, string(rep))
		}
	}
	return out
}

// sanitizeSQLExp does basic sanitisation on arbitrary
// SQL query expressions coming from the frontend.
func sanitizeSQLExp(q string) string {
	if len(q) == 0 {
		return ""
	}
	q = strings.TrimSpace(q)

	// Remove semicolon suffix.
	if q[len(q)-1] == ';' {
		q = q[:len(q)-1]
	}
	return q
}

// strHasLen checks if the given string has a length within min-max.
func strHasLen(str string, min, max int) bool {
	return len(str) >= min && len(str) <= max
}
