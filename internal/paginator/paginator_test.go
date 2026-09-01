package paginator

import (
	"net/url"
	"testing"
)

func TestNewFromURLDefaults(t *testing.T) {
	pg := New(Opt{DefaultPerPage: 20, MaxPerPage: 50, AllowAll: true, NumPageNums: 10})
	s := pg.NewFromURL(url.Values{})
	if s.Page != 1 || s.PerPage != 20 || s.Offset != 0 || s.Limit != 20 {
		t.Fatalf("got page=%d per=%d off=%d lim=%d", s.Page, s.PerPage, s.Offset, s.Limit)
	}
}

func TestAllowAllAndMax(t *testing.T) {
	pg := New(Opt{DefaultPerPage: 20, MaxPerPage: 50, AllowAll: true})
	all := pg.NewFromURL(url.Values{"per_page": []string{"all"}})
	if all.Limit != 0 || all.PerPage != 0 {
		t.Fatalf("all: per=%d lim=%d", all.PerPage, all.Limit)
	}

	capped := pg.NewFromURL(url.Values{"page": []string{"3"}, "per_page": []string{"100"}})
	if capped.Page != 3 || capped.PerPage != 100 || capped.Offset != 200 {
		t.Fatalf("allow-all cap: %+v", capped)
	}

	strict := New(Opt{DefaultPerPage: 20, MaxPerPage: 50, AllowAll: false})
	s := strict.NewFromURL(url.Values{"per_page": []string{"100"}})
	if s.PerPage != 50 {
		t.Fatalf("max per_page=%d", s.PerPage)
	}
}

func TestHTML(t *testing.T) {
	pg := New(Opt{DefaultPerPage: 20, MaxPerPage: 50, NumPageNums: 10, AllowAll: true})
	s := pg.NewFromURL(url.Values{"page": []string{"2"}})
	s.SetTotal(100)
	html := s.HTML("?page=%d")
	if html == "" || s.TotalPages != 5 {
		t.Fatalf("pages=%d html=%q", s.TotalPages, html)
	}
}
