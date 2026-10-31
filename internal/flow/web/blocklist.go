// Package web holds web search wiring (provider-side) and web.fetch.
package web

import (
	"net/url"
	"strings"
)

// Blocklist matches hosts against domain patterns. "example.com" and
// "*.example.com" both block example.com and every subdomain.
type Blocklist struct {
	domains []string
}

func NewBlocklist(patterns []string) *Blocklist {
	b := &Blocklist{}
	seen := map[string]bool{}
	for _, p := range patterns {
		d := normalizeDomain(p)
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		b.domains = append(b.domains, d)
	}
	return b
}

func normalizeDomain(p string) string {
	p = strings.ToLower(strings.TrimSpace(p))
	p = strings.TrimPrefix(p, "*.")
	p = strings.TrimPrefix(p, ".")
	return strings.TrimSuffix(p, ".")
}

// Domains returns the normalized domains (for provider excluded_domains).
func (b *Blocklist) Domains() []string {
	if b == nil {
		return nil
	}
	return append([]string(nil), b.domains...)
}

// BlockedHost reports whether host (no port) is blocked.
func (b *Blocklist) BlockedHost(host string) bool {
	if b == nil {
		return false
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	for _, d := range b.domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

// BlockedURL reports whether raw's host is blocked. Unparseable URLs are
// treated as blocked so they can't slip through.
func (b *Blocklist) BlockedURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" {
		return true
	}
	return b.BlockedHost(u.Hostname())
}
