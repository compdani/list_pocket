package models

import (
	"bytes"
	"html"
	"regexp"
	"strings"
)

var bodyTagRE = regexp.MustCompile(`(?i)<body[^>]*>`)

const preheaderMarker = `data-listpocket-preheader="true"`

func ApplyPreheaderToHTML(body []byte, contentType, preheader string) []byte {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if ct == "plain" || ct == "text/plain" {
		return body
	}

	preheader = strings.TrimSpace(preheader)
	if preheader == "" || bytes.Contains(body, []byte(preheaderMarker)) {
		return body
	}

	snippet := []byte(`<div ` + preheaderMarker + ` style="display:none!important;visibility:hidden;opacity:0;color:transparent;height:0;width:0;overflow:hidden;mso-hide:all;">` +
		html.EscapeString(preheader) +
		`</div>`)

	idx := bodyTagRE.FindIndex(body)
	if idx == nil {
		out := make([]byte, 0, len(snippet)+len(body))
		out = append(out, snippet...)
		out = append(out, body...)
		return out
	}

	out := make([]byte, 0, len(snippet)+len(body))
	out = append(out, body[:idx[1]]...)
	out = append(out, snippet...)
	out = append(out, body[idx[1]:]...)
	return out
}
