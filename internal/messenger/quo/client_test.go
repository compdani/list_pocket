package quo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_GetMessageText(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/v1/messages/ACabc123" {
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			}
			if r.Header.Get("Authorization") != "key123" {
				t.Errorf("missing or wrong Authorization header")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"id":"ACabc123","text":"STOP please","direction":"incoming"}}`))
		}))
		t.Cleanup(srv.Close)

		c := &Client{
			APIKey:  "key123",
			BaseURL: srv.URL,
			HTTP:    srv.Client(),
		}
		got, err := c.GetMessageText(context.Background(), "ACabc123")
		if err != nil {
			t.Fatal(err)
		}
		if got != "STOP please" {
			t.Fatalf("text: got %q want %q", got, "STOP please")
		}
	})

	t.Run("empty_text", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"id":"ACx","text":"","direction":"incoming"}}`))
		}))
		t.Cleanup(srv.Close)

		c := &Client{APIKey: "k", BaseURL: srv.URL, HTTP: srv.Client()}
		got, err := c.GetMessageText(context.Background(), "ACx")
		if err != nil {
			t.Fatal(err)
		}
		if got != "" {
			t.Fatalf("expected empty text, got %q", got)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"not found"}`))
		}))
		t.Cleanup(srv.Close)

		c := &Client{APIKey: "k", BaseURL: srv.URL, HTTP: srv.Client()}
		_, err := c.GetMessageText(context.Background(), "ACmissing")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestClient_GetMessageText_validation(t *testing.T) {
	t.Parallel()
	c := &Client{APIKey: "k", BaseURL: "http://example.com", HTTP: http.DefaultClient}
	if _, err := c.GetMessageText(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty id")
	}
	if _, err := (*Client)(nil).GetMessageText(context.Background(), "AC1"); err == nil {
		t.Fatal("expected error for nil client")
	}
}
