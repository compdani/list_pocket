// Package chatgpt lets List Pocket use a person's ChatGPT plan for model
// requests ("Sign in with ChatGPT", open-source / locally hosted route):
// OAuth with PKCE and first-time dynamic registration, ID-token validation,
// rotating-refresh-token renewal, revocation, and the model list.
// https://developers.openai.com/siwc/token-sharing-open-source
package chatgpt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	DefaultAuthBase = "https://auth.openai.com"
	Resource        = "https://api.openai.com/v1"
	Scopes          = "openid profile email offline_access resource.invoke chatgpt.tokens.use.direct"
	PlanScope       = "chatgpt.tokens.use.direct"
	DynamicClient   = "dynamic_agent_client"
	CallbackPath    = "/auth/callback"
	DefaultPort     = 1455
	AgentName       = "List Pocket"
)

var ErrPlanNotGranted = errors.New("ChatGPT plan use wasn't granted (chatgpt.tokens.use.direct missing); sign in again and allow plan usage")

// Credentials are one signed-in ChatGPT account on this host.
type Credentials struct {
	ClientID     string    `json:"client_id"` // issued (oaiapp_…), never dynamic_agent_client
	Sub          string    `json:"sub"`
	Email        string    `json:"email,omitempty"`
	IDToken      string    `json:"id_token"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Scopes       []string  `json:"scopes"`
	ExpiresAt    time.Time `json:"expires_at"`
	HostID       string    `json:"host_id"`
}

func (c *Credentials) HasPlan() bool { return slices.Contains(c.Scopes, PlanScope) }

// OAuth talks to the ChatGPT authorization server.
type OAuth struct {
	AuthBase string // default https://auth.openai.com (tests point it at a mock)
	HTTP     *http.Client
	Now      func() time.Time

	mu   sync.Mutex
	disc *discovery
	keys map[string]*rsa.PublicKey
}

func (o *OAuth) base() string {
	if o.AuthBase != "" {
		return strings.TrimRight(o.AuthBase, "/")
	}
	return DefaultAuthBase
}
func (o *OAuth) client() *http.Client {
	if o.HTTP != nil {
		return o.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}
func (o *OAuth) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}
func (o *OAuth) issuer() string       { return o.base() }
func (o *OAuth) authorizeURL() string { return o.base() + "/api/accounts/authorize" }
func (o *OAuth) tokenURL() string     { return o.base() + "/api/accounts/oauth/token" }

// Pending is one authorization attempt, kept until the callback arrives.
type Pending struct {
	State       string `json:"state"`
	Nonce       string `json:"nonce"`
	Verifier    string `json:"verifier"`
	RedirectURI string `json:"redirect_uri"`
	ClientID    string `json:"client_id"` // "" for first registration
	HostID      string `json:"host_id"`
	AuthURL     string `json:"auth_url"`
}

func random(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Begin prepares an authorization request. existing (optional) reuses a
// previously issued client ID for the same account.
func (o *OAuth) Begin(hostID, redirectURI string, existing *Credentials) *Pending {
	p := &Pending{State: random(24), Nonce: random(24), Verifier: random(48), RedirectURI: redirectURI, HostID: hostID}
	sum := sha256.Sum256([]byte(p.Verifier))
	q := url.Values{
		"response_type":         {"code"},
		"redirect_uri":          {redirectURI},
		"scope":                 {Scopes},
		"resource":              {Resource},
		"state":                 {p.State},
		"nonce":                 {p.Nonce},
		"code_challenge_method": {"S256"},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"ext_agent_host_id":     {hostID},
	}
	if existing != nil && existing.ClientID != "" {
		p.ClientID = existing.ClientID
		q.Set("client_id", existing.ClientID)
		if existing.IDToken != "" {
			q.Set("id_token_hint", existing.IDToken)
		}
	} else {
		q.Set("client_id", DynamicClient)
		q.Set("agent_name_hint", AgentName)
	}
	p.AuthURL = o.authorizeURL() + "?" + q.Encode()
	return p
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// Complete handles the callback query: validates state, takes the issued
// client ID, exchanges the code, validates the ID token, and requires the
// plan scope.
func (o *OAuth) Complete(ctx context.Context, p *Pending, cb url.Values) (*Credentials, error) {
	if cb.Get("state") != p.State {
		return nil, errors.New("state mismatch; start sign-in again")
	}
	if e := cb.Get("error"); e != "" {
		if e == "access_denied" {
			return nil, errors.New("sign-in was declined")
		}
		return nil, fmt.Errorf("authorization failed: %s %s", e, cb.Get("error_description"))
	}
	clientID := p.ClientID
	if issued := cb.Get("client_id"); issued != "" {
		if clientID != "" && issued != clientID {
			return nil, errors.New("callback returned a different client ID than requested")
		}
		clientID = issued
	}
	if clientID == "" || clientID == DynamicClient {
		return nil, errors.New("registration incomplete: no issued client ID in the callback")
	}
	code := cb.Get("code")
	if code == "" {
		return nil, errors.New("callback has no authorization code")
	}
	tr, err := o.token(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"code_verifier": {p.Verifier},
		"redirect_uri":  {p.RedirectURI},
		"resource":      {Resource},
	})
	if err != nil {
		return nil, err
	}
	claims, err := o.ValidateIDToken(ctx, tr.IDToken, clientID, p.Nonce)
	if err != nil {
		return nil, err
	}
	cred := &Credentials{
		ClientID: clientID, Sub: claims.Subject, Email: claims.Email, IDToken: tr.IDToken,
		AccessToken: tr.AccessToken, RefreshToken: tr.RefreshToken, Scopes: strings.Fields(tr.Scope),
		ExpiresAt: o.now().Add(time.Duration(tr.ExpiresIn) * time.Second), HostID: p.HostID,
	}
	if !cred.HasPlan() {
		return cred, ErrPlanNotGranted
	}
	return cred, nil
}

// Refresh renews the access token. Refresh tokens rotate: the returned
// credentials carry the replacement and must be saved together.
func (o *OAuth) Refresh(ctx context.Context, c *Credentials) (*Credentials, error) {
	tr, err := o.token(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {c.ClientID},
		"refresh_token": {c.RefreshToken},
		"resource":      {Resource},
	})
	if err != nil {
		return nil, err
	}
	next := *c
	next.AccessToken = tr.AccessToken
	if tr.RefreshToken != "" {
		next.RefreshToken = tr.RefreshToken
	}
	if tr.IDToken != "" {
		next.IDToken = tr.IDToken
	}
	if tr.Scope != "" {
		next.Scopes = strings.Fields(tr.Scope)
	}
	next.ExpiresAt = o.now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	return &next, nil
}

func (o *OAuth) token(ctx context.Context, form url.Values) (*tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.tokenURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := o.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var tr tokenResponse
	_ = json.Unmarshal(raw, &tr)
	if resp.StatusCode != http.StatusOK || tr.Error != "" {
		if tr.Error != "" {
			return nil, fmt.Errorf("token endpoint: %s %s", tr.Error, tr.ErrorDesc)
		}
		return nil, fmt.Errorf("token endpoint returned HTTP %d", resp.StatusCode)
	}
	if tr.AccessToken == "" {
		return nil, errors.New("token endpoint returned no access token")
	}
	return &tr, nil
}

// --- discovery, JWKS, ID token ---

type discovery struct {
	Issuer             string `json:"issuer"`
	JWKSURI            string `json:"jwks_uri"`
	RevocationEndpoint string `json:"revocation_endpoint"`
}

func (o *OAuth) discover(ctx context.Context) (*discovery, error) {
	o.mu.Lock()
	if o.disc != nil {
		d := o.disc
		o.mu.Unlock()
		return d, nil
	}
	o.mu.Unlock()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, o.base()+"/.well-known/openid-configuration", nil)
	resp, err := o.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var d discovery
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil || d.JWKSURI == "" {
		return nil, fmt.Errorf("openid configuration unavailable: %v", err)
	}
	o.mu.Lock()
	o.disc = &d
	o.mu.Unlock()
	return &d, nil
}

func (o *OAuth) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	o.mu.Lock()
	if k := o.keys[kid]; k != nil {
		o.mu.Unlock()
		return k, nil
	}
	o.mu.Unlock()
	d, err := o.discover(ctx)
	if err != nil {
		return nil, err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, d.JWKSURI, nil)
	resp, err := o.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var set struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return nil, err
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "RSA" {
			continue
		}
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	o.mu.Lock()
	o.keys = keys
	o.mu.Unlock()
	if k := keys[kid]; k != nil {
		return k, nil
	}
	return nil, fmt.Errorf("signing key %q not found", kid)
}

type IDClaims struct {
	Email string `json:"email"`
	Nonce string `json:"nonce"`
	jwt.RegisteredClaims
}

// ValidateIDToken checks signature (published JWKS), issuer, audience
// (the issued client ID), expiry, and nonce.
func (o *OAuth) ValidateIDToken(ctx context.Context, raw, clientID, nonce string) (*IDClaims, error) {
	if raw == "" {
		return nil, errors.New("no ID token returned")
	}
	claims := &IDClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		kid, _ := t.Header["kid"].(string)
		return o.key(ctx, kid)
	}, jwt.WithIssuer(o.issuer()), jwt.WithAudience(clientID), jwt.WithExpirationRequired(), jwt.WithTimeFunc(o.now))
	if err != nil {
		return nil, fmt.Errorf("invalid ID token: %w", err)
	}
	if nonce != "" && claims.Nonce != nonce {
		return nil, errors.New("invalid ID token: nonce mismatch")
	}
	if claims.Subject == "" {
		return nil, errors.New("invalid ID token: no subject")
	}
	return claims, nil
}

// Revoke signs out remotely (revocation endpoint from discovery).
func (o *OAuth) Revoke(ctx context.Context, c *Credentials) error {
	d, err := o.discover(ctx)
	if err != nil {
		return err
	}
	if d.RevocationEndpoint == "" {
		return errors.New("no revocation endpoint advertised")
	}
	form := url.Values{"token": {c.RefreshToken}, "token_type_hint": {"refresh_token"}, "client_id": {c.ClientID}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, d.RevocationEndpoint, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := o.client().Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("revocation returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// --- models ---

type Model struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
}

// ListModels returns models the account may use (visibility == "list").
func ListModels(ctx context.Context, hc *http.Client, base, token string) ([]Model, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/models", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(raw[:min(len(raw), 300)])))
	}
	var body struct {
		Data   []map[string]any `json:"data"`
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	list := append(body.Data, body.Models...)
	var out []Model
	for _, m := range list {
		if v, ok := m["visibility"].(string); ok && v != "list" {
			continue
		}
		slug, _ := m["slug"].(string)
		if slug == "" {
			slug, _ = m["id"].(string)
		}
		if slug == "" {
			continue
		}
		name, _ := m["display_name"].(string)
		if name == "" {
			name = slug
		}
		out = append(out, Model{Slug: slug, DisplayName: name})
	}
	return out, nil
}
