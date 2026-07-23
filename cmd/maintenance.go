package main

import (
	pbcore "github.com/pocketbase/pocketbase/core"
	"log"
	"net/http"
	"time"

	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/labstack/echo/v4"
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
		err = echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidData"))
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
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidData"))
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
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidData"))
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
		err = echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidData"))
	}

	if err != nil {
		return err
	}

	return okJSON(re, true)
}

// RunDBVacuum runs a full VACUUM on the PostgreSQL database.
// VACUUM reclaims storage occupied by dead tuples and updates planner statistics.
func RunDBVacuum(db *pbdb.DB, lo *log.Logger) {
	lo.Println("running database VACUUM ANALYZE")
	if _, err := db.Exec("VACUUM ANALYZE"); err != nil {
		lo.Printf("error running VACUUM ANALYZE: %v", err)
		return
	}
	lo.Println("finished database VACUUM ANALYZE")
}
