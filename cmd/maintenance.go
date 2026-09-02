package main

import (
	"github.com/compdani/list_pocket/internal/apperr"
	pbcore "github.com/pocketbase/pocketbase/core"
	"log"
	"time"

	"github.com/compdani/list_pocket/internal/pbdb"
)

// GCSubscribers garbage collects (deletes) orphaned or blocklisted subscribers.
func (a *App) GCSubscribers(re *pbcore.RequestEvent) error {
	var (
		typ = pathParam(re, "type")

		n   int
		err error
	)

	switch typ {
	case "blocklisted":
		n, err = a.core.DeleteBlocklistedSubscribers()
	case "orphan":
		n, err = a.core.DeleteOrphanSubscribers()
	default:
		err = apperr.BadRequest(a.i18n.T("globals.messages.invalidData"))
	}

	if err != nil {
		return err
	}

	return okJSON(re, struct {
		Count int `json:"count"`
	}{n})
}

// GCSubscriptions garbage collects (deletes) orphaned or blocklisted subscribers.
func (a *App) GCSubscriptions(re *pbcore.RequestEvent) error {
	// Validate the date.
	t, err := time.Parse(time.RFC3339, re.Request.FormValue("before_date"))
	if err != nil {
		return apperr.BadRequest(a.i18n.T("globals.messages.invalidData"))
	}

	// Delete unconfirmed subscriptions from the DB in bulk.
	n, err := a.core.DeleteUnconfirmedSubscriptions(t)
	if err != nil {
		return err
	}

	return okJSON(re, struct {
		Count int `json:"count"`
	}{n})
}

// GCCampaignAnalytics garbage collects (deletes) campaign analytics.
func (a *App) GCCampaignAnalytics(re *pbcore.RequestEvent) error {

	t, err := time.Parse(time.RFC3339, re.Request.FormValue("before_date"))
	if err != nil {
		return apperr.BadRequest(a.i18n.T("globals.messages.invalidData"))
	}

	switch pathParam(re, "type") {
	case "all":
		if err := a.core.DeleteCampaignViews(t); err != nil {
			return err
		}
		err = a.core.DeleteCampaignLinkClicks(t)
	case "views":
		err = a.core.DeleteCampaignViews(t)
	case "clicks":
		err = a.core.DeleteCampaignLinkClicks(t)
	default:
		err = apperr.BadRequest(a.i18n.T("globals.messages.invalidData"))
	}

	if err != nil {
		return err
	}

	return okJSON(re, true)
}

// RunDBVacuum runs PRAGMA optimize, ANALYZE, incremental_vacuum, then VACUUM.
func RunDBVacuum(db *pbdb.DB, lo *log.Logger) {
	lo.Println("running database PRAGMA optimize")
	if _, err := db.Exec("PRAGMA optimize"); err != nil {
		lo.Printf("error running PRAGMA optimize: %v", err)
	}
	lo.Println("running database ANALYZE")
	if _, err := db.Exec("ANALYZE"); err != nil {
		lo.Printf("error running ANALYZE: %v", err)
	}
	lo.Println("running database incremental_vacuum")
	if _, err := db.Exec("PRAGMA incremental_vacuum"); err != nil {
		lo.Printf("error running PRAGMA incremental_vacuum: %v", err)
	}
	lo.Println("running database VACUUM")
	if _, err := db.Exec("VACUUM"); err != nil {
		lo.Printf("error running VACUUM: %v", err)
		return
	}
	lo.Println("finished database VACUUM and ANALYZE")
}
