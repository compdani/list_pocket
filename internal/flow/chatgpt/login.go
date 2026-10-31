package chatgpt

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Loopback is a temporary 127.0.0.1 listener that receives the OAuth
// callback in the browser running on this machine.
type Loopback struct {
	RedirectURI string
	ln          net.Listener
	srv         *http.Server
	result      chan url.Values
}

// Listen starts the callback listener on port (0 = DefaultPort, falling back
// to any free port). Start it before opening the browser.
func Listen(port int) (*Loopback, error) {
	if port == 0 {
		port = DefaultPort
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		if ln, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
			return nil, err
		}
	}
	lb := &Loopback{
		RedirectURI: fmt.Sprintf("http://127.0.0.1:%d%s", ln.Addr().(*net.TCPAddr).Port, CallbackPath),
		ln:          ln,
		result:      make(chan url.Values, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc(CallbackPath, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		msg := "Signed in. You can close this tab and return to List Pocket."
		if e := q.Get("error"); e != "" {
			msg = "Sign-in didn't complete: " + e
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>List Pocket</title><body style="font-family:system-ui;padding:40px">%s</body>`, html.EscapeString(msg))
		select {
		case lb.result <- q:
		default:
		}
	})
	lb.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = lb.srv.Serve(ln) }()
	return lb, nil
}

// Deliver hands a callback query received elsewhere (pasted URL, or the
// this app's own /auth/callback route) to Wait. False if one already arrived.
func (lb *Loopback) Deliver(q url.Values) bool {
	select {
	case lb.result <- q:
		return true
	default:
		return false
	}
}

// Wait returns the callback query, or an error on timeout/cancel.
func (lb *Loopback) Wait(ctx context.Context) (url.Values, error) {
	defer lb.Close()
	select {
	case q := <-lb.result:
		return q, nil
	case <-ctx.Done():
		return nil, errors.New("sign-in timed out or was cancelled")
	}
}

func (lb *Loopback) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = lb.srv.Shutdown(ctx)
}
