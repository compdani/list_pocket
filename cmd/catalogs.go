package main

import (
	"errors"
	validation "ganigeorgiev.com/pocketbase/ozzo-validation/v4"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"ganigeorgiev.com/pocketbase/dbx"
	pbcore "ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/search"
	"github.com/compdani/list_pocket/internal/apperr"
)

func (a *App) listPictures(re *pbcore.RequestEvent) error {
	page, _ := strconv.Atoi(queryParam(re, "page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(queryParam(re, "perPage"))
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	filter := ""
	params := dbx.Params{}
	if q := strings.TrimSpace(queryParam(re, "query")); q != "" {
		filter = "original_name ~ {:q} || file ~ {:q}"
		params["q"] = q
	}
	records, err := a.pb.FindRecordsByFilter("pictures", filter, "-created", size, (page-1)*size, params)
	if err != nil {
		return err
	}
	var expression dbx.Expression = dbx.NewExp("1=1")
	if filter != "" {
		expression, err = search.FilterData(filter).BuildExpr(search.NewSimpleFieldResolver("original_name", "file"), params)
		if err != nil {
			return err
		}
	}
	total, err := a.pb.CountRecords("pictures", expression)
	if err != nil {
		return err
	}
	return okJSON(re, map[string]any{"items": records, "page": page, "perPage": size, "totalItems": total, "totalPages": (total + int64(size) - 1) / int64(size)})
}

func (a *App) uploadPicture(re *pbcore.RequestEvent) error {
	re.Request.Body = http.MaxBytesReader(re.Response, re.Request.Body, 21<<20)
	files, err := re.FindUploadedFiles("file")
	if err != nil || len(files) != 1 {
		return apperr.BadRequest("one picture is required")
	}
	col, err := a.pb.FindCachedCollectionByNameOrId("pictures")
	if err != nil {
		return err
	}
	rec := pbcore.NewRecord(col)
	rec.Set("file", files[0])
	rec.Set("original_name", files[0].OriginalName)
	if err := a.pb.Save(rec); err != nil {
		return err
	}
	return okJSON(re, rec)
}

func (a *App) deletePicture(re *pbcore.RequestEvent) error {
	rec, err := a.pb.FindRecordById("pictures", pathParam(re, "id"))
	if err != nil {
		return err
	}
	if err := a.pb.Delete(rec); err != nil {
		return err
	}
	return okJSON(re, true)
}

func (a *App) tagCatalog(collection string, create bool) func(*pbcore.RequestEvent) error {
	return func(re *pbcore.RequestEvent) error {
		if !create {
			records, err := a.pb.FindAllRecords(collection)
			if err != nil {
				return err
			}
			tags := make([]string, 0, len(records))
			for _, rec := range records {
				tags = append(tags, rec.GetString("tag"))
			}
			sort.Strings(tags)
			return okJSON(re, tags)
		}
		var body struct {
			Tag string `json:"tag"`
		}
		if err := re.BindBody(&body); err != nil {
			return err
		}
		body.Tag = strings.TrimSpace(body.Tag)
		if body.Tag == "" {
			return apperr.BadRequest("tag is required")
		}
		col, err := a.pb.FindCachedCollectionByNameOrId(collection)
		if err != nil {
			return err
		}
		rec := pbcore.NewRecord(col)
		rec.Set("tag", body.Tag)
		if err := a.pb.Save(rec); err != nil {
			var fields validation.Errors
			if !errors.As(err, &fields) {
				return err
			}
			fieldErr, ok := fields["tag"].(validation.Error)
			if !ok || fieldErr.Code() != "validation_not_unique" {
				return err
			}
			if _, found := a.pb.FindFirstRecordByFilter(collection, "tag = {:tag}", dbx.Params{"tag": body.Tag}); found != nil {
				return err
			}
		}
		return okJSON(re, true)
	}
}
