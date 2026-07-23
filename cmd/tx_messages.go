package main

import (
	"github.com/compdani/list_pocket/internal/apperr"
	pbcore "github.com/pocketbase/pocketbase/core"
	"strings"

	"github.com/compdani/list_pocket/models"
)

func (a *App) resolveTxRouteID(re *pbcore.RequestEvent) (string, error) {
	recordID := strings.TrimSpace(pathParam(re, "id"))
	if recordID == "" {
		return "", apperr.BadRequest("invalid ID")
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
