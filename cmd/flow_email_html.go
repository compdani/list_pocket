package main

import (
	"errors"
	"regexp"
	"strings"

	"github.com/compdani/list_pocket/internal/apperr"
	"github.com/compdani/list_pocket/models"
	"github.com/microcosm-cc/bluemonday"
	pbcore "github.com/pocketbase/pocketbase/core"
	"golang.org/x/net/html"
)

var flowEmailFormats = []string{
	models.CampaignContentTypePlain, models.CampaignContentTypeRichtext, models.CampaignContentTypeHTML,
	models.CampaignContentTypeMarkdown, models.CampaignContentTypeVisual, models.CampaignContentTypeGrapes,
}

func flowEmailFormatEnum() []any {
	out := make([]any, len(flowEmailFormats))
	for i, format := range flowEmailFormats {
		out[i] = format
	}
	return out
}

// flowEmailPolicy keeps what email needs and drops anything that can run or load
// other documents: scripts, event handlers, frames, forms, style blocks and script URLs.
var flowEmailPolicy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowStyles(
		"color", "background-color", "text-align", "vertical-align", "text-decoration",
		"font-family", "font-size", "font-style", "font-weight", "line-height",
		"margin", "margin-top", "margin-right", "margin-bottom", "margin-left",
		"padding", "padding-top", "padding-right", "padding-bottom", "padding-left",
		"border", "border-top", "border-right", "border-bottom", "border-left", "border-radius", "border-collapse", "border-spacing",
		"width", "min-width", "max-width", "height", "display", "letter-spacing", "text-transform", "word-break",
	).Globally()
	p.AllowAttrs("bgcolor", "cellpadding", "cellspacing", "border", "width", "height", "align", "valign").OnElements("table", "tr", "td", "th", "tbody")
	return p
}()

// flowEmailHumanWritten names the field that makes these send_email arguments unsafe
// without a person's check. Plain text can't hide anything; HTML can.
func flowEmailHumanWritten(args map[string]any) string {
	contentType := argStr(args, "content_type")
	if argStr(args, "template_id") != "" || contentType == "" || contentType == models.CampaignContentTypePlain {
		return ""
	}
	return "body"
}

// flowEmailHTML turns a workflow email body into sanitized HTML and its plain text version.
func flowEmailHTML(contentType, body string) ([]byte, string, error) {
	// The rich text editor can mark links for campaign tracking, which these emails don't have.
	src := []byte(strings.ReplaceAll(body, "@TrackLink", ""))
	switch contentType {
	// The rich text and design editors all produce HTML in body.
	case models.CampaignContentTypeHTML, models.CampaignContentTypeRichtext, models.CampaignContentTypeVisual, models.CampaignContentTypeGrapes:
	case models.CampaignContentTypeMarkdown:
		rendered, err := models.MarkdownToHTML(src)
		if err != nil {
			return nil, "", err
		}
		src = rendered
	default:
		return nil, "", errors.New("content_type must be one of " + strings.Join(flowEmailFormats, ", "))
	}
	clean := flowEmailPolicy.SanitizeBytes(src)
	text := htmlToText(string(clean))
	if text == "" {
		return nil, "", errors.New("the email body has no content left after sanitizing")
	}
	return clean, text, nil
}

var (
	textSpacesRE = regexp.MustCompile(`[ \t\r\n]+`)
	textBlanksRE = regexp.MustCompile(`\n{3,}`)
	textRunsRE   = regexp.MustCompile(`[ \t]{2,}`)
	textLineRE   = regexp.MustCompile(`[ \t]*\n[ \t]*`)
)

// PreviewFlowEmail shows what messages.send_email would send for a body, so the
// person writing it sees the result of sanitizing.
func (a *App) PreviewFlowEmail(re *pbcore.RequestEvent) error {
	var in struct {
		ContentType string `json:"content_type"`
		Body        string `json:"body"`
	}
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	if in.ContentType == "" || in.ContentType == models.CampaignContentTypePlain {
		return okJSON(re, map[string]string{"text": in.Body})
	}
	body, text, err := flowEmailHTML(in.ContentType, in.Body)
	if err != nil {
		return apperr.BadRequest(err.Error())
	}
	return okJSON(re, map[string]string{"html": string(body), "text": text})
}

// htmlToText writes HTML as readable text, keeping link targets.
func htmlToText(src string) string {
	var (
		b     strings.Builder
		links []string
		z     = html.NewTokenizer(strings.NewReader(src))
	)
	for {
		kind := z.Next()
		if kind == html.ErrorToken {
			out := textLineRE.ReplaceAllString(textRunsRE.ReplaceAllString(b.String(), " "), "\n")
			return strings.TrimSpace(textBlanksRE.ReplaceAllString(out, "\n\n"))
		}
		tok := z.Token()
		switch kind {
		case html.TextToken:
			b.WriteString(textSpacesRE.ReplaceAllString(tok.Data, " "))
		case html.StartTagToken, html.SelfClosingTagToken:
			switch tok.Data {
			case "br", "tr", "hr":
				b.WriteString("\n")
			case "li":
				b.WriteString("\n- ")
			case "td", "th":
				b.WriteString(" ")
			case "a":
				links = append(links, tokenAttr(tok, "href"))
			case "img":
				b.WriteString(tokenAttr(tok, "alt"))
			}
		case html.EndTagToken:
			switch tok.Data {
			case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "table", "blockquote", "pre":
				b.WriteString("\n\n")
			case "a":
				if n := len(links); n > 0 {
					if href := links[n-1]; href != "" {
						b.WriteString(" (" + href + ")")
					}
					links = links[:n-1]
				}
			}
		}
	}
}

func tokenAttr(tok html.Token, name string) string {
	for _, attr := range tok.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}
