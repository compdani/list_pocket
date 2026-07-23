package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/compdani/list_pocket/internal/apperr"
	"github.com/compdani/list_pocket/internal/auth"
	"github.com/compdani/list_pocket/models"
	pbcore "github.com/pocketbase/pocketbase/core"
)

const appContextKey = "app"

// asHandler wraps a native handler so *apperr.Error and plain errors are
// written in the listmonk JSON shape {"message":"..."} instead of PocketBase ApiError.
func asHandler(h func(*pbcore.RequestEvent) error) func(*pbcore.RequestEvent) error {
	return func(re *pbcore.RequestEvent) error {
		if err := h(re); err != nil {
			return writeAppError(re, err)
		}
		return nil
	}
}

func getApp(re *pbcore.RequestEvent) *App {
	if re == nil {
		return nil
	}
	if a, ok := re.Get(appContextKey).(*App); ok {
		return a
	}
	return nil
}

func pathParam(re *pbcore.RequestEvent, name string) string {
	if re == nil {
		return ""
	}
	if name == "id" {
		if v, ok := re.Get("override_id").(string); ok && v != "" {
			return v
		}
	}
	if name == "type" {
		if v, ok := re.Get("override_type").(string); ok && v != "" {
			return v
		}
	}
	if re.Request == nil {
		return ""
	}
	return re.Request.PathValue(name)
}

func queryParam(re *pbcore.RequestEvent, name string) string {
	if re == nil || re.Request == nil {
		return ""
	}
	return re.Request.URL.Query().Get(name)
}

func queryParams(re *pbcore.RequestEvent, name string) []string {
	if re == nil || re.Request == nil {
		return nil
	}
	return re.Request.URL.Query()[name]
}

func okJSON(re *pbcore.RequestEvent, data any) error {
	return re.JSON(http.StatusOK, okResp{Data: data})
}

func bindJSON(re *pbcore.RequestEvent, dst any) error {
	if err := re.BindBody(dst); err != nil {
		return apperr.BadRequest(fmt.Sprintf("invalid JSON: %v", err))
	}
	return nil
}

func writeAppError(re *pbcore.RequestEvent, err error) error {
	if err == nil {
		return nil
	}
	status, msg := extractHTTPError(err)
	if writeErr := re.JSON(status, map[string]any{"message": msg}); writeErr != nil {
		return writeErr
	}
	return nil
}

// isHTTPStatus reports whether err is an app HTTP error with the given status.
func isHTTPStatus(err error, status int) bool {
	s, _, ok := asHTTPError(err)
	return ok && s == status
}

func asHTTPError(err error) (int, string, bool) {
	if err == nil {
		return 0, "", false
	}
	var ae *apperr.Error
	if errors.As(err, &ae) && ae != nil {
		status := ae.Status
		if status <= 0 {
			status = http.StatusInternalServerError
		}
		msg := ae.Message
		if msg == "" {
			msg = http.StatusText(status)
		}
		return status, msg, true
	}
	return 0, "", false
}

func extractHTTPError(err error) (int, string) {
	if status, msg, ok := asHTTPError(err); ok {
		return status, msg
	}
	return http.StatusInternalServerError, err.Error()
}

// hydrateAuthUser loads auth.User onto the RequestEvent when PB auth is present.
func hydrateAuthUser(re *pbcore.RequestEvent) error {
	if re.Auth == nil {
		return re.Next()
	}

	re.Set(auth.AuthRecordHTTPCtxKey, re.Auth)

	if strings.TrimSpace(re.Auth.Id) == "" {
		return writeAppError(re, apperr.Forbidden("invalid auth user"))
	}

	app := getApp(re)
	if app == nil || app.core == nil {
		return writeAppError(re, apperr.Forbidden("invalid auth user"))
	}

	user, err := app.core.GetUser(re.Auth.Id, "", "")
	if err != nil {
		return writeAppError(re, apperr.Forbidden("invalid auth user"))
	}

	if roleID := auth.ExtractRoleIDFromRecord(re.Auth); roleID > 0 {
		user.UserRoleID = roleID
	}
	re.Set(auth.UserHTTPCtxKey, user)
	return re.Next()
}

// renderTpl renders a public HTML template using the shared tplRenderer data envelope.
func renderTpl(re *pbcore.RequestEvent, status int, name string, data any) error {
	app := getApp(re)
	if app == nil || app.renderer == nil {
		return apperr.Internal("templates unavailable")
	}

	var buf bytes.Buffer
	if err := app.renderer.Render(&buf, name, data); err != nil {
		return apperr.Internal(err.Error())
	}
	re.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	re.Response.WriteHeader(status)
	_, err := re.Response.Write(buf.Bytes())
	return err
}

func (a *App) hasRecordIDRE(next func(*pbcore.RequestEvent) error, params ...string) func(*pbcore.RequestEvent) error {
	return asHandler(func(re *pbcore.RequestEvent) error {
		for _, p := range params {
			if !models.IsPublicTrackingPathID(pathParam(re, p)) {
				return renderTpl(re, http.StatusBadRequest, tplMessage, makeMsgTpl(a.i18n.T("public.errorTitle"), "",
					a.i18n.T("globals.messages.invalidUUID")))
			}
		}
		return next(re)
	})
}

func (a *App) hasUUIDRE(next func(*pbcore.RequestEvent) error, params ...string) func(*pbcore.RequestEvent) error {
	return asHandler(func(re *pbcore.RequestEvent) error {
		for _, p := range params {
			if !reUUID.MatchString(pathParam(re, p)) {
				return renderTpl(re, http.StatusBadRequest, tplMessage, makeMsgTpl(a.i18n.T("public.errorTitle"), "",
					a.i18n.T("globals.messages.invalidUUID")))
			}
		}
		return next(re)
	})
}

func (a *App) hasSubRE(next func(*pbcore.RequestEvent) error) func(*pbcore.RequestEvent) error {
	return asHandler(func(re *pbcore.RequestEvent) error {
		subUUID := strings.TrimSpace(pathParam(re, "subUUID"))
		if subUUID == "" {
			subUUID = strings.TrimSpace(pathParam(re, "subID"))
		}

		if _, err := a.core.GetSubscriber(0, subUUID, ""); err != nil {
			if isHTTPStatus(err, http.StatusBadRequest) {
				_, msg, _ := asHTTPError(err)
				return renderTpl(re, http.StatusNotFound, tplMessage,
					makeMsgTpl(a.i18n.T("public.notFoundTitle"), "", msg))
			}
			a.log.Printf("error checking subscriber existence: %v", err)
			return renderTpl(re, http.StatusInternalServerError, tplMessage,
				makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.T("public.errorProcessingRequest")))
		}
		return next(re)
	})
}

func noIndexRE(next func(*pbcore.RequestEvent) error) func(*pbcore.RequestEvent) error {
	return func(re *pbcore.RequestEvent) error {
		re.Response.Header().Set("X-Robots-Tag", "noindex")
		return next(re)
	}
}

func clientIP(re *pbcore.RequestEvent) string {
	ipAddress := strings.TrimSpace(re.Request.Header.Get("X-Forwarded-For"))
	if ipAddress != "" {
		ipAddress = strings.TrimSpace(strings.Split(ipAddress, ",")[0])
	}
	if ipAddress == "" {
		ipAddress = strings.TrimSpace(re.Request.RemoteAddr)
		if i := strings.LastIndex(ipAddress, ":"); i >= 0 {
			ipAddress = ipAddress[:i]
		}
		ipAddress = strings.Trim(ipAddress, "[]")
	}
	return ipAddress
}

func writeBlob(re *pbcore.RequestEvent, status int, contentType string, b []byte) error {
	if contentType != "" {
		re.Response.Header().Set("Content-Type", contentType)
	}
	re.Response.WriteHeader(status)
	_, err := re.Response.Write(b)
	return err
}

func writeStream(re *pbcore.RequestEvent, status int, contentType string, reader io.Reader) error {
	if contentType != "" {
		re.Response.Header().Set("Content-Type", contentType)
	}
	re.Response.WriteHeader(status)
	_, err := io.Copy(re.Response, reader)
	return err
}

func flushResponse(w http.ResponseWriter) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
