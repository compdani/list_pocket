// Package paginator parses page/per_page query params into offset/limit values.
package paginator

import (
	"bytes"
	"fmt"
	"math"
	"net/url"
	"strconv"
)

// Opt configures a Paginator.
type Opt struct {
	DefaultPerPage int
	MaxPerPage     int
	NumPageNums    int
	AllowAll       bool
	AllowAllParam  string
}

// Paginator is a factory for per-request Sets.
type Paginator struct {
	o Opt
}

// Set is the pagination state for one request.
type Set struct {
	Page       int
	PerPage    int
	TotalPages int
	Total      int
	Offset     int
	Limit      int

	PinFirstPage bool
	PinLastPage  bool
	Pages        []int
	pg           *Paginator
}

// New returns a Paginator. Query params are always "page" and "per_page".
func New(o Opt) *Paginator {
	if o.AllowAllParam == "" {
		o.AllowAllParam = "all"
	}
	if o.DefaultPerPage < 1 {
		o.DefaultPerPage = 20
	}
	if o.MaxPerPage < 1 {
		o.MaxPerPage = 50
	}
	if o.NumPageNums < 1 {
		o.NumPageNums = 10
	}
	return &Paginator{o: o}
}

// NewFromURL builds a Set from URL query values.
func (p *Paginator) NewFromURL(q url.Values) Set {
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	page, _ := strconv.Atoi(q.Get("page"))
	if q.Get("per_page") == p.o.AllowAllParam {
		perPage = -1
	}
	return p.New(page, perPage)
}

// New returns a Set for the given page and per-page size.
func (p *Paginator) New(page, perPage int) Set {
	if perPage < 0 && p.o.AllowAll {
		perPage = 0
	} else if perPage < 1 {
		perPage = p.o.DefaultPerPage
	} else if !p.o.AllowAll && perPage > p.o.MaxPerPage {
		perPage = p.o.MaxPerPage
	}
	if page < 1 {
		page = 1
	}
	return Set{
		Page:    page,
		PerPage: perPage,
		Offset:  (page - 1) * perPage,
		Limit:   perPage,
		pg:      p,
	}
}

// SetTotal records the total result count and computes page numbers.
func (s *Set) SetTotal(t int) {
	s.Total = t
	s.generateNumbers()
}

func (s *Set) generateNumbers() {
	if s.PerPage <= 0 || s.Total <= s.PerPage {
		return
	}
	numPages := int(math.Ceil(float64(s.Total) / float64(s.PerPage)))
	s.TotalPages = numPages
	half := s.pg.o.NumPageNums / 2

	first := s.Page - half
	last := s.Page + half
	if first < 1 {
		first = 1
	}
	if last > numPages {
		last = numPages
	}
	if numPages > s.pg.o.NumPageNums {
		if last < numPages && s.Page <= half {
			last = first + s.pg.o.NumPageNums - 1
		}
		if s.Page > numPages-half {
			first = last - s.pg.o.NumPageNums
		}
	}
	if first != 1 {
		s.PinFirstPage = true
	}
	if last != numPages {
		s.PinLastPage = true
	}
	s.Pages = make([]int, 0, last-first+1)
	for i := first; i <= last; i++ {
		s.Pages = append(s.Pages, i)
	}
}

// HTML prints page links using uri as a fmt pattern, e.g. "?page=%d".
func (s *Set) HTML(uri string) string {
	var b bytes.Buffer
	if s.PinFirstPage {
		b.WriteString(`<a class="pg-page-first" href="`)
		b.WriteString(fmt.Sprintf(uri, 1))
		b.WriteString(`">1</a> `)
		b.WriteString(`<span class="pg-page-ellipsis-first">...</span> `)
	}
	for _, p := range s.Pages {
		c := ""
		if s.Page == p {
			c = " pg-selected"
		}
		b.WriteString(`<a class="pg-page`)
		b.WriteString(c)
		b.WriteString(`" href="`)
		b.WriteString(fmt.Sprintf(uri, p))
		b.WriteString(`">`)
		b.WriteString(fmt.Sprintf("%d", p))
		b.WriteString(`</a> `)
	}
	if s.PinLastPage {
		b.WriteString(`<span class="pg-page-ellipsis-last">...</span> `)
		b.WriteString(`<a class="pg-page-last" href="`)
		b.WriteString(fmt.Sprintf(uri, s.TotalPages))
		b.WriteString(`">`)
		b.WriteString(fmt.Sprintf("%d", s.TotalPages))
		b.WriteString(`</a> `)
	}
	return b.String()
}
