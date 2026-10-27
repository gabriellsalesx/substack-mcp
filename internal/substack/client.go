// Package substack is a small client for Substack's unofficial web API.
package substack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/publicsuffix"

	"github.com/gabriellsalesx/substack-mcp/internal/auth"
)

// DefaultUserAgent is a browser-like UA. Substack's edge rejects some
// non-browser agents, especially on custom domains.
const DefaultUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"

// Client talks to substack.com and to one or more publication hosts.
type Client struct {
	http      *http.Client
	cookies   []auth.Cookie
	userAgent string
	timeout   time.Duration
	log       *slog.Logger

	// Root is the substack.com API base. Tests override it, along with PubBase.
	Root string
	// PubBase, when set, replaces https://<subdomain>.substack.com for every publication.
	PubBase string

	mu      sync.Mutex
	seeded  map[string]bool
	profile *Profile
}

// Options configures a Client.
type Options struct {
	Cookies   []auth.Cookie
	UserAgent string
	Logger    *slog.Logger
	Timeout   time.Duration
}

// New returns a Client that sends the given session cookies.
func New(opt Options) *Client {
	jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if opt.UserAgent == "" {
		opt.UserAgent = DefaultUserAgent
	}
	if opt.Timeout == 0 {
		opt.Timeout = 30 * time.Second
	}
	if opt.Logger == nil {
		opt.Logger = slog.New(slog.DiscardHandler)
	}
	c := &Client{
		cookies:   opt.Cookies,
		userAgent: opt.UserAgent,
		timeout:   opt.Timeout,
		log:       opt.Logger,
		Root:      "https://substack.com/api/v1",
		seeded:    map[string]bool{},
	}
	c.http = &http.Client{Jar: jar, CheckRedirect: c.checkRedirect}
	return c
}

// checkRedirect keeps the session on redirects to other Substack hosts (such
// as a custom domain) and refuses to follow writes, which Go would replay as
// a bodyless GET.
func (c *Client) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("too many redirects")
	}
	if m := via[0].Method; m != http.MethodGet && m != http.MethodHead {
		return fmt.Errorf("refusing to follow the redirect of %s %s to %s: writes are not replayed", m, via[0].URL.Path, req.URL.Host)
	}
	c.seed(req.URL)
	return nil
}

// Jar exposes the cookie jar, used by the login flow to harvest the session.
func (c *Client) Jar() http.CookieJar { return c.http.Jar }

// Error is a non-2xx answer from Substack.
type Error struct {
	Status  int
	Method  string
	URL     string
	Message string
}

func (e *Error) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	switch e.Status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Sprintf("Substack rejected the session (%d %s). Run `substack-mcp login` again, or check that this account can edit the publication.", e.Status, msg)
	case http.StatusNotFound:
		return fmt.Sprintf("not found on Substack (%s %s): %s", e.Method, e.URL, msg)
	case http.StatusTooManyRequests:
		return "Substack is rate limiting requests; wait a minute and try again"
	}
	return fmt.Sprintf("Substack API error %d on %s %s: %s", e.Status, e.Method, e.URL, msg)
}

// IsStatus reports whether err is a Substack error with the given status.
func IsStatus(err error, status int) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == status
}

// seed attaches the session cookies to host the first time it is contacted.
func (c *Client) seed(u *url.URL) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.seeded[u.Host] {
		return
	}
	c.seeded[u.Host] = true
	base := &url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/"}
	var cs []*http.Cookie
	for _, ck := range c.cookies {
		cs = append(cs, &http.Cookie{Name: ck.Name, Value: ck.Value, Path: "/", Secure: u.Scheme == "https", Quoted: false})
	}
	c.http.Jar.SetCookies(base, cs)
}

// request is one API call. Body is JSON-encoded unless Form is set.
type request struct {
	Method string
	URL    string
	Query  url.Values
	Body   any
	Form   url.Values
	// Timeout overrides the default per-request timeout.
	Timeout time.Duration
}

func (c *Client) do(ctx context.Context, r request, out any) error {
	u, err := url.Parse(r.URL)
	if err != nil {
		return err
	}
	if len(r.Query) > 0 {
		u.RawQuery = r.Query.Encode()
	}
	c.seed(u)
	timeout := c.timeout
	if r.Timeout > 0 {
		timeout = r.Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var payload []byte
	contentType := ""
	switch {
	case r.Form != nil:
		payload = []byte(r.Form.Encode())
		contentType = "application/x-www-form-urlencoded"
	case r.Body != nil:
		if payload, err = json.Marshal(r.Body); err != nil {
			return err
		}
		contentType = "application/json"
	}

	// Only retry requests that are safe to repeat.
	attempts := 1
	if r.Method == http.MethodGet || r.Method == http.MethodPut || r.Method == http.MethodDelete {
		attempts = 3
	}
	for i := 0; ; i++ {
		req, err := http.NewRequestWithContext(ctx, r.Method, u.String(), bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.userAgent)
		req.Header.Set("Origin", u.Scheme+"://"+u.Host)
		req.Header.Set("Referer", u.Scheme+"://"+u.Host+"/")
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		start := time.Now()
		resp, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("%s %s: %w", r.Method, u.Redacted(), err)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		resp.Body.Close()
		c.log.Debug("substack", "method", r.Method, "url", u.Path, "status", resp.StatusCode, "took", time.Since(start))
		if err != nil {
			return err
		}
		if retryable(resp.StatusCode) && i+1 < attempts {
			if err := sleep(ctx, retryAfter(resp, i)); err != nil {
				return err
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return &Error{Status: resp.StatusCode, Method: r.Method, URL: u.Path, Message: errorMessage(body)}
		}
		if out == nil || len(bytes.TrimSpace(body)) == 0 {
			return nil
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("unexpected response from %s %s: %w", r.Method, u.Path, err)
		}
		return nil
	}
}

func retryable(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

func retryAfter(resp *http.Response, attempt int) time.Duration {
	if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
		return min(time.Duration(s)*time.Second, 10*time.Second)
	}
	return time.Duration(attempt+1) * 750 * time.Millisecond
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// errorMessage pulls a human message out of Substack's error shapes:
// {"error":"..."}, {"errors":[{"msg":"..."}]} or plain text.
func errorMessage(body []byte) string {
	var e struct {
		Error  any `json:"error"`
		Errors []struct {
			Msg     string `json:"msg"`
			Message string `json:"message"`
		} `json:"errors"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &e) == nil {
		var parts []string
		if s, ok := e.Error.(string); ok && s != "" {
			parts = append(parts, s)
		}
		for _, x := range e.Errors {
			parts = append(parts, x.Msg+x.Message)
		}
		if e.Message != "" {
			parts = append(parts, e.Message)
		}
		if len(parts) > 0 {
			return strings.Join(parts, "; ")
		}
	}
	s := strings.TrimSpace(string(body))
	if strings.HasPrefix(s, "<") {
		return "non-JSON response"
	}
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}
