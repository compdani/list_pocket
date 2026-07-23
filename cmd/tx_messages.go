package main

import (
	pbcore "github.com/pocketbase/pocketbase/core"
	"net/http"
	"strings"

	"github.com/compdani/list_pocket/models"
	"github.com/labstack/echo/v4"
)

func (a *App) resolveTxRouteID(re *pbcore.RequestEvent) (string, error) {
	recordID := strings.TrimSpace(pathParam(re, "id"))
	if recordID == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "invalid ID")
	}
	return recordID, nil
}

func (a *App) GetTxMessages(re *pbcore.RequestEvent) error {
	search := strings.TrimSpace(re.Request.FormValue("query"))
	pg := a.pg.NewFromURL(re.Request.URL.Query())

	res, total, err := a.core.QueryTransactionalMessages(search, pg.Offset, pg.Limit)
	if err != nil {
		return err
	}

	return okJSON(re, models.PageResults{
		Query:   search,
		Results: res,
		Total:   total,
		Page:    pg.Page,
		PerPage: pg.PerPage,
	})
}

func (a *App) GetTxMessage(re *pbcore.RequestEvent) error {
	recordID, err := a.resolveTxRouteID(re)
	if err != nil {
		return err
	}

	res, err := a.core.GetTransactionalMessage(recordID)
	if err != nil {
		return err
	}

	return okJSON(re, res)
}
