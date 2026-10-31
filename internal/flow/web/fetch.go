package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

const (
	UserAgent       = "ListPocketBot/1.0"
	defaultMaxBytes = 5 << 20
	defaultMaxChars = 8_000
	maxMaxChars     = 30_000
	maxRedirects    = 5
)

var (
	ErrBlocked       = errors.New("domain is on the blocklist")
	ErrRobots        = errors.New("disallowed by robots.txt")
	ErrPrivate       = errors.New("refusing to fetch a private or local address")
	ErrUnsupportedCT = errors.New("unsupported content type")
)

type FetchResult struct {
	URL         string `json:"url"`
	FinalURL    string `json:"final_url"`
	Status      int    `json:"status"`
	ContentType string `json:"content_type"`
	Title       string `json:"title,omitempty"`
	Text        string `json:"text"`
	Truncated   bool   `json:"truncated"`
}

type Fetcher struct {
	Client    *http.Client
	Blocklist func() *Blocklist // read per call so settings changes apply
	MaxBytes  int64
	// AllowPrivate disables the private-address guard (tests only).
	AllowPrivate bool

	robotsMu sync.Mutex
	robots   map[string]robotsEntry
}

type robotsEntry struct {
	rules   []string // disallowed path prefixes for our agent
	fetched time.Time
}

func NewFetcher(blocklist func() *Blocklist) *Fetcher {
	f := &Fetcher{Blocklist: blocklist, MaxBytes: defaultMaxBytes, robots: map[string]robotsEntry{}}
	f.Client = f.newClient()
	return f
}

func (f *Fetcher) newClient() *http.Client {
	return &http.Client{
		Transport: GuardedTransport(func() bool { return f.AllowPrivate }),
		Timeout:   45 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("too many redirects")
			}
			if f.blocklist().BlockedHost(req.URL.Hostname()) {
				return fmt.Errorf("redirect to %s: %w", req.URL.Hostname(), ErrBlocked)
			}
			return nil
		},
	}
}

// GuardedTransport is an HTTP transport that refuses to connect to
// loopback, private, link-local, and CGNAT addresses unless allowPrivate()
// is true. The check runs after DNS resolution, so it also stops DNS
// rebinding to internal addresses.
func GuardedTransport(allowPrivate func() bool) *http.Transport {
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: PublicOnly(allowPrivate)}
	return &http.Transport{
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		MaxIdleConns:          20,
		IdleConnTimeout:       60 * time.Second,
	}
}

// PublicOnly is a net.Dialer Control that refuses loopback, private,
// link-local, and CGNAT addresses unless allowPrivate() is true.
func PublicOnly(allowPrivate func() bool) func(network, address string, _ syscall.RawConn) error {
	return func(network, address string, _ syscall.RawConn) error {
		if allowPrivate != nil && allowPrivate() {
			return nil
		}
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		ip := net.ParseIP(host)
		if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() || isCGNAT(ip) {
			return ErrPrivate
		}
		return nil
	}
}

func isCGNAT(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 64
}

func (f *Fetcher) blocklist() *Blocklist {
	if f.Blocklist == nil {
		return nil
	}
	return f.Blocklist()
}

// Fetch downloads rawURL and returns readable text, capped at maxChars.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string, maxChars int) (*FetchResult, error) {
	if maxChars <= 0 {
		maxChars = defaultMaxChars
	}
	if maxChars > maxMaxChars {
		maxChars = maxMaxChars
	}
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return nil, fmt.Errorf("invalid URL %q: only http(s) URLs are allowed", rawURL)
	}
	if f.blocklist().BlockedHost(u.Hostname()) {
		return nil, fmt.Errorf("%s: %w", u.Hostname(), ErrBlocked)
	}
	if !f.robotsAllowed(ctx, u) {
		return nil, fmt.Errorf("%s: %w", u.String(), ErrRobots)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain,application/json;q=0.9,*/*;q=0.1")

	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	result := &FetchResult{URL: rawURL, FinalURL: resp.Request.URL.String(), Status: resp.StatusCode}
	ct, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	result.ContentType = ct

	maxBytes := f.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return nil, err
	}

	switch {
	case ct == "text/html" || ct == "application/xhtml+xml" || ct == "":
		result.Title, result.Text = ExtractText(body, result.FinalURL)
	case strings.HasPrefix(ct, "text/") || ct == "application/json" || strings.HasSuffix(ct, "+json"):
		result.Text = strings.ToValidUTF8(string(body), "")
	default:
		return nil, fmt.Errorf("%s: %w", ct, ErrUnsupportedCT)
	}

	if utf8.RuneCountInString(result.Text) > maxChars {
		result.Text = string([]rune(result.Text)[:maxChars])
		result.Truncated = true
	}
	return result, nil
}

// robotsAllowed applies the site's robots.txt rules for "*" and our agent.
// Unreachable or missing robots.txt allows the fetch.
func (f *Fetcher) robotsAllowed(ctx context.Context, u *url.URL) bool {
	origin := u.Scheme + "://" + u.Host
	f.robotsMu.Lock()
	entry, ok := f.robots[origin]
	f.robotsMu.Unlock()

	if !ok || time.Since(entry.fetched) > time.Hour {
		rules, definitive := f.loadRobots(ctx, origin)
		entry = robotsEntry{rules: rules, fetched: time.Now()}
		// Only cache real answers (a robots.txt or a clean 4xx); network
		// failures are retried on the next fetch.
		if definitive {
			f.robotsMu.Lock()
			f.robots[origin] = entry
			f.robotsMu.Unlock()
		}
	}

	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	for _, prefix := range entry.rules {
		if strings.HasPrefix(path, prefix) {
			return false
		}
	}
	return true
}

// loadRobots returns the rules and whether the answer is definitive.
func (f *Fetcher) loadRobots(ctx context.Context, origin string) ([]string, bool) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/robots.txt", nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode >= 400 && resp.StatusCode < 500
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	if err != nil {
		return nil, false
	}
	return ParseRobots(string(raw), "ListPocketBot"), true
}

// ParseRobots returns Disallow prefixes that apply to agent (its own group
// if present, else the "*" group). Allow lines and wildcards are ignored —
// this errs on the side of not fetching.
func ParseRobots(body, agent string) []string {
	agent = strings.ToLower(agent)
	groups := map[string][]string{}
	var current []string
	inAgents := false
	for _, line := range strings.Split(body, "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "user-agent":
			if !inAgents {
				current = nil
			}
			inAgents = true
			current = append(current, strings.ToLower(value))
			if _, ok := groups[strings.ToLower(value)]; !ok {
				groups[strings.ToLower(value)] = []string{}
			}
		case "disallow":
			inAgents = false
			if value == "" {
				continue
			}
			if i := strings.IndexAny(value, "*$"); i >= 0 {
				value = value[:i]
			}
			if value == "" {
				value = "/"
			}
			for _, a := range current {
				groups[a] = append(groups[a], value)
			}
		default:
			inAgents = false
		}
	}
	for name, rules := range groups {
		if name != "*" && strings.Contains(agent, name) {
			return rules
		}
	}
	return groups["*"]
}
