package substack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gabriellsalesx/substack-mcp/internal/auth"
)

func TestRetryAndErrors(t *testing.T) {
	var hits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/flaky" && hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(503)
			return
		}
		if r.URL.Path == "/denied" {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"errors":[{"msg":"Please sign in"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()
	c := New(Options{Cookies: []auth.Cookie{{Name: "substack.sid", Value: "x"}}})
	ctx := context.Background()

	var out struct{ OK bool }
	if err := c.do(ctx, request{Method: "GET", URL: ts.URL + "/flaky"}, &out); err != nil || !out.OK {
		t.Fatalf("GET should retry: %v %v", err, out)
	}
	err := c.do(ctx, request{Method: "GET", URL: ts.URL + "/denied"}, nil)
	if !IsStatus(err, 401) || !strings.Contains(err.Error(), "substack-mcp login") {
		t.Errorf("want auth hint, got %v", err)
	}
}

func TestPublicationMatching(t *testing.T) {
	c := New(Options{})
	c.profile = &Profile{PublicationUsers: []PublicationUser{
		{Publication: Publication{ID: 1, Subdomain: "main"}},
		{IsPrimary: true, Publication: Publication{ID: 2, Subdomain: "side", CustomDomain: "www.side.dev"}},
	}}
	ctx := context.Background()
	for sel, want := range map[string]int64{"": 2, "main": 1, "https://main.substack.com/p/x": 1, "side.dev": 2, "https://www.side.dev": 2} {
		p, err := c.Publication(ctx, sel)
		if err != nil || p.ID != want {
			t.Errorf("Publication(%q) = %d, %v; want %d", sel, p.ID, err, want)
		}
	}
	if _, err := c.Publication(ctx, "other"); err == nil {
		t.Error("unknown publication should fail")
	}
}
