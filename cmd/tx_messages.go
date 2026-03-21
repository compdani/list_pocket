package main

import (
	"net/http"
	"strings"

	"github.com/compdani/list_pocket/models"
	"github.com/labstack/echo/v4"
)

func (a *App) resolveTxRouteID(c echo.Context) (string, error) {
	recordID := strings.TrimSpace(c.Param("id"))
	if recordID == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "invalid ID")
	}
	return recordID, nil
}

func (a *App) GetTxMessages(c echo.Context) error {
	search := strings.TrimSpace(c.FormValue("query"))
	pg := a.pg.NewFromURL(c.Request().URL.Query())

	res, total, err := a.core.QueryTransactionalMessages(search, pg.Offset, pg.Limit)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{models.PageResults{
		Query:   search,
		Results: res,
		Total:   total,
		Page:    pg.Page,
		PerPage: pg.PerPage,
	}})
}

func (a *App) GetTxMessage(c echo.Context) error {
	recordID, err := a.resolveTxRouteID(c)
	if err != nil {
		return err
	}

	res, err := a.core.GetTransactionalMessage(recordID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{res})
}
