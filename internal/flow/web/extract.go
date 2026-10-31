package web

import (
	"bytes"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var (
	skipTags = map[atom.Atom]bool{
		atom.Script: true, atom.Style: true, atom.Noscript: true, atom.Template: true,
		atom.Svg: true, atom.Iframe: true, atom.Nav: true, atom.Footer: true,
		atom.Form: true, atom.Button: true, atom.Select: true, atom.Head: true,
	}
	blockTags = map[atom.Atom]bool{
		atom.P: true, atom.Div: true, atom.Section: true, atom.Article: true, atom.Main: true,
		atom.Header: true, atom.Aside: true, atom.Li: true, atom.Ul: true, atom.Ol: true,
		atom.Table: true, atom.Tr: true, atom.Br: true, atom.Hr: true, atom.Blockquote: true,
		atom.Pre: true, atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true,
		atom.H5: true, atom.H6: true, atom.Dl: true, atom.Dt: true, atom.Dd: true, atom.Address: true,
	}
	spaceRun   = regexp.MustCompile(`[ \t\f\r\x{00a0}]+`)
	blankLines = regexp.MustCompile(`\n{3,}`)
)

// ExtractText converts HTML into readable text: drops scripts, styles, and
// navigation; keeps headings, paragraphs, list items; renders links as
// "text (url)" and mailto links as the address, so published contact
// details survive extraction.
func ExtractText(body []byte, baseURL string) (title, text string) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return "", ""
	}
	base, _ := url.Parse(baseURL)

	title = findTitle(doc)

	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if skipTags[n.DataAtom] {
				return
			}
			if blockTags[n.DataAtom] {
				sb.WriteString("\n")
			}
			if n.DataAtom == atom.Li {
				sb.WriteString("- ")
			}
			if strings.HasPrefix(n.Data, "h") && len(n.Data) == 2 && n.Data[1] >= '1' && n.Data[1] <= '6' {
				sb.WriteString(strings.Repeat("#", int(n.Data[1]-'0')) + " ")
			}
			if n.DataAtom == atom.A {
				writeLink(&sb, n, base)
				return
			}
		}
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode && blockTags[n.DataAtom] {
			sb.WriteString("\n")
		}
	}
	walk(doc)

	lines := strings.Split(sb.String(), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(spaceRun.ReplaceAllString(l, " "))
	}
	text = blankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")
	return title, strings.TrimSpace(text)
}

func findTitle(n *html.Node) string {
	if n.Type == html.ElementNode && n.DataAtom == atom.Title && n.FirstChild != nil {
		return strings.TrimSpace(n.FirstChild.Data)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if t := findTitle(c); t != "" {
			return t
		}
	}
	return ""
}

func writeLink(sb *strings.Builder, n *html.Node, base *url.URL) {
	var inner strings.Builder
	var collect func(*html.Node)
	collect = func(c *html.Node) {
		if c.Type == html.TextNode {
			inner.WriteString(c.Data)
		}
		for cc := c.FirstChild; cc != nil; cc = cc.NextSibling {
			collect(cc)
		}
	}
	collect(n)
	label := strings.TrimSpace(spaceRun.ReplaceAllString(inner.String(), " "))

	href := ""
	for _, a := range n.Attr {
		if a.Key == "href" {
			href = strings.TrimSpace(a.Val)
		}
	}
	switch {
	case strings.HasPrefix(strings.ToLower(href), "mailto:"):
		addr := strings.SplitN(href[len("mailto:"):], "?", 2)[0]
		if label == "" || label == addr {
			sb.WriteString(" " + addr + " ")
		} else {
			sb.WriteString(" " + label + " (" + addr + ") ")
		}
	case href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(strings.ToLower(href), "javascript:"):
		sb.WriteString(" " + label + " ")
	default:
		if base != nil {
			if u, err := base.Parse(href); err == nil {
				href = u.String()
			}
		}
		if label == "" {
			sb.WriteString(" " + href + " ")
		} else {
			sb.WriteString(" " + label + " (" + href + ") ")
		}
	}
}
