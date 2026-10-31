package chatgpt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

const (
	collection   = "oauth_credentials"
	providerName = "chatgpt"
	refreshAhead = 10 * time.Minute // tokens last 1h; keep ≥10 min for a long agent run
)

var ErrNotSignedIn = errors.New("no ChatGPT account is signed in (Settings → AI)")

// Store keeps the signed-in account in a superuser-only collection and
// hands out fresh access tokens.
type Store struct {
	App   core.App
	OAuth *OAuth

	mu sync.Mutex // serializes refreshes: refresh tokens rotate
}

func (s *Store) record() (*core.Record, error) {
	return s.App.FindFirstRecordByData(collection, "provider", providerName)
}

// Load returns the stored credentials.
func (s *Store) Load() (*Credentials, error) {
	rec, err := s.record()
	if err != nil {
		return nil, ErrNotSignedIn
	}
	c := &Credentials{
		ClientID: rec.GetString("client_id"), Sub: rec.GetString("sub"), Email: rec.GetString("email"),
		IDToken: rec.GetString("id_token"), AccessToken: rec.GetString("access_token"), RefreshToken: rec.GetString("refresh_token"),
		ExpiresAt: rec.GetDateTime("expires_at").Time(), HostID: rec.GetString("host_id"),
	}
	_ = rec.UnmarshalJSONField("scopes", &c.Scopes)
	if c.AccessToken == "" && c.RefreshToken == "" {
		return nil, ErrNotSignedIn
	}
	return c, nil
}

// Save replaces the stored credentials (one account per install).
func (s *Store) Save(c *Credentials) error {
	rec, err := s.record()
	if err != nil {
		col, err := s.App.FindCollectionByNameOrId(collection)
		if err != nil {
			return err
		}
		rec = core.NewRecord(col)
		rec.Set("provider", providerName)
	}
	if c.HostID == "" {
		c.HostID = s.HostID()
	}
	rec.Set("client_id", c.ClientID)
	rec.Set("sub", c.Sub)
	rec.Set("email", c.Email)
	rec.Set("id_token", c.IDToken)
	rec.Set("access_token", c.AccessToken)
	rec.Set("refresh_token", c.RefreshToken)
	rec.Set("scopes", c.Scopes)
	if dt, err := types.ParseDateTime(c.ExpiresAt.UTC()); err == nil {
		rec.Set("expires_at", dt)
	}
	rec.Set("host_id", c.HostID)
	return s.App.Save(rec)
}

// Clear forgets the account locally.
func (s *Store) Clear() error {
	rec, err := s.record()
	if err != nil {
		return nil
	}
	return s.App.Delete(rec)
}

// Token returns an access token valid for at least refreshAhead, refreshing
// (and saving the rotated refresh token) when needed.
func (s *Store) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.Load()
	if err != nil {
		return "", err
	}
	if !c.HasPlan() {
		return "", ErrPlanNotGranted
	}
	if c.AccessToken != "" && c.ExpiresAt.Sub(s.OAuth.now()) > refreshAhead {
		return c.AccessToken, nil
	}
	if c.RefreshToken == "" {
		return "", errors.New("ChatGPT session expired; sign in again")
	}
	next, err := s.OAuth.Refresh(ctx, c)
	if err != nil {
		return "", errors.New("refreshing the ChatGPT session failed (" + err.Error() + "); sign in again")
	}
	if err := s.Save(next); err != nil {
		return "", err
	}
	return next.AccessToken, nil
}

// HostID is this install's stable ext_agent_host_id: an opaque urn:uuid
// (one of the formats the authorize endpoint accepts), created once.
func (s *Store) HostID() string {
	rec, err := s.App.FindFirstRecordByData("ai_settings", "key", "chatgpt_host_id")
	if err == nil {
		var id string
		raw, _ := json.Marshal(rec.Get("value"))
		if json.Unmarshal(raw, &id) == nil && validHostID(id) {
			return id
		}
	}
	id := newHostID()
	if rec == nil {
		col, err := s.App.FindCollectionByNameOrId("ai_settings")
		if err != nil {
			return id
		}
		rec = core.NewRecord(col)
		rec.Set("key", "chatgpt_host_id")
	}
	rec.Set("value", id)
	rec.Set("description", "Stable host identifier sent to ChatGPT sign-in (ext_agent_host_id). Don't copy between installs.")
	_ = s.App.Save(rec)
	return id
}

func validHostID(id string) bool {
	return strings.HasPrefix(id, "urn:uuid:") || strings.HasPrefix(id, "urn:ietf:params:oauth:jwk-thumbprint:") || strings.HasPrefix(id, "did:key:")
}

// newHostID returns urn:uuid:<UUIDv4>.
func newHostID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return "urn:uuid:" + h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// Status summarizes the session for the UI (never the tokens).
type Status struct {
	SignedIn  bool      `json:"signed_in"`
	Email     string    `json:"email,omitempty"`
	Plan      bool      `json:"plan"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	ClientID  string    `json:"client_id,omitempty"`
	HostID    string    `json:"host_id"`
}

func (s *Store) Status() Status {
	st := Status{HostID: s.HostID()}
	c, err := s.Load()
	if err != nil {
		return st
	}
	st.SignedIn, st.Email, st.Plan, st.ExpiresAt, st.ClientID = true, c.Email, c.HasPlan(), c.ExpiresAt, c.ClientID
	return st
}

// Import stores credentials exported from another computer (self-hosted
// VM flow). This install keeps its own host ID for later authorizations.
func (s *Store) Import(c *Credentials) error {
	if c.ClientID == "" || c.ClientID == DynamicClient || c.RefreshToken == "" {
		return errors.New("credentials need an issued client_id and a refresh_token")
	}
	if !c.HasPlan() {
		return ErrPlanNotGranted
	}
	c.HostID = s.HostID()
	return s.Save(c)
}
