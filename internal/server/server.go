// Package server exposes Substack as MCP tools.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gabriellsalesx/substack-mcp/internal/auth"
	"github.com/gabriellsalesx/substack-mcp/internal/substack"
)

const instructions = `Tools for the user's own Substack publication. Write content in Markdown; images may be local paths or URLs and are uploaded automatically. Drafts are private. publish_post, delete_draft, create_note, delete_note and create_community_post reach readers or delete data: call them first without confirm to get a preview, show it to the user, and only repeat with confirm=true after the user agrees. Use get_publication to see the account's publications and sections.`

// Config wires the server to its environment.
type Config struct {
	Version   string
	Logger    *slog.Logger
	UserAgent string
	// Publication is the default publication selector (URL, host or subdomain).
	Publication string
	// NewClient overrides client construction, for tests.
	NewClient func() (*substack.Client, error)
}

// Server holds the lazily created Substack client.
type Server struct {
	cfg Config

	mu     sync.Mutex
	client *substack.Client
	pub    string
	loaded time.Time // mtime of the credentials file behind client
}

// New builds the MCP server with every tool registered.
func New(cfg Config) *mcp.Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.DiscardHandler)
	}
	s := &Server{cfg: cfg}
	ms := mcp.NewServer(&mcp.Implementation{
		Name:       "substack-mcp",
		Title:      "Substack",
		Version:    cfg.Version,
		WebsiteURL: "https://github.com/gabriellsalesx/substack-mcp",
	}, &mcp.ServerOptions{Instructions: instructions, Logger: cfg.Logger})
	s.register(ms)
	ms.AddReceivingMiddleware(stringIDs)
	return ms
}

// stringIDs lets clients send ids as JSON numbers. list_posts returns numeric
// ids and models often pass them back unquoted, while the schema says string
// (a string-or-integer union is not portable across clients).
func stringIDs(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if p, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok && method == "tools/call" && len(p.Arguments) > 0 {
			var args map[string]json.RawMessage
			if json.Unmarshal(p.Arguments, &args) == nil {
				changed := false
				for _, k := range []string{"post_id", "note_id"} {
					if v, ok := args[k]; ok && len(v) > 0 && v[0] >= '0' && v[0] <= '9' {
						args[k], _ = json.Marshal(string(v))
						changed = true
					}
				}
				if changed {
					p.Arguments, _ = json.Marshal(args)
				}
			}
		}
		return next(ctx, method, req)
	}
}

// session returns the client, creating it on first use so that a login made
// after the MCP client started is picked up without a restart.
func (s *Server) session() (*substack.Client, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil && !s.credentialsChanged() {
		return s.client, s.pub, nil
	}
	if s.cfg.NewClient != nil {
		c, err := s.cfg.NewClient()
		if err != nil {
			return nil, "", err
		}
		s.client, s.pub = c, s.cfg.Publication
		return c, s.pub, nil
	}
	creds, err := auth.Load()
	if err != nil {
		return nil, "", err
	}
	s.loaded = credentialsMtime()
	s.client = substack.New(substack.Options{Cookies: creds.Cookies, UserAgent: s.cfg.UserAgent, Logger: s.cfg.Logger})
	s.pub = firstNonEmpty(s.cfg.Publication, creds.Publication)
	return s.client, s.pub, nil
}

// credentialsChanged reports whether `substack-mcp login` wrote a new session
// since the client was built. Environment sessions never change.
func (s *Server) credentialsChanged() bool {
	if s.cfg.NewClient != nil || auth.FromEnv() != nil {
		return false
	}
	return !credentialsMtime().Equal(s.loaded)
}

func credentialsMtime() time.Time {
	p, err := auth.Path()
	if err != nil {
		return time.Time{}
	}
	fi, err := os.Stat(p)
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}

// resolve returns the client and the publication a tool call targets.
func (s *Server) resolve(ctx context.Context, selector string) (*substack.Client, substack.Publication, error) {
	c, def, err := s.session()
	if err != nil {
		return nil, substack.Publication{}, err
	}
	pub, err := c.Publication(ctx, firstNonEmpty(selector, def))
	return c, pub, err
}

// idPath finds the id in editor, post and note URLs, such as
// /publish/post/123456 or /@ana/note/c-555.
var idPath = regexp.MustCompile(`/(?:post|posts|drafts)/(\d+)(?:/|$)|/note/c-(\d+)(?:/|$)|/comment/(\d+)(?:/|$)`)

// parseID accepts a numeric id, "c-123", or a Substack URL with the id in its
// path. Query strings are ignored, so ?r=2 can never be taken for an id.
func parseID(s, field string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("%s is required", field)
	}
	if n, err := strconv.ParseInt(strings.TrimPrefix(s, "c-"), 10, 64); err == nil && n > 0 {
		return n, nil
	}
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		if m := idPath.FindStringSubmatch(u.Path); m != nil {
			return strconv.ParseInt(m[1]+m[2]+m[3], 10, 64)
		}
	}
	return 0, fmt.Errorf("%s %q is not a Substack id; pass the numeric id from list_posts (public /p/slug links do not contain it)", field, s)
}

// parseTime reads RFC 3339, or "YYYY-MM-DD HH:MM" in the server's local time,
// and requires a moment in the future.
func parseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		for _, layout := range []string{"2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02T15:04:05"} {
			if t, err = time.ParseInLocation(layout, s, time.Local); err == nil {
				break
			}
		}
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("scheduled_for %q is not a date; use RFC 3339 like 2026-11-03T09:00:00-03:00", s)
	}
	if !t.After(time.Now()) {
		return time.Time{}, fmt.Errorf("scheduled_for %s is in the past", t.Format(time.RFC3339))
	}
	return t, nil
}

var audiences = []string{"everyone", "only_paid", "founding", "only_free"}

func checkAudience(a string) error {
	if a == "" {
		return nil
	}
	for _, v := range audiences {
		if a == v {
			return nil
		}
	}
	return fmt.Errorf("audience %q is invalid; use one of %s", a, strings.Join(audiences, ", "))
}

func clamp(v, def, lo, hi int) int {
	if v == 0 {
		return def
	}
	return max(lo, min(v, hi))
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// truncate keeps tool output inside what MCP clients accept without spilling
// to files (Claude Code warns at ~10k tokens).
func truncate(s string, limit int) (string, bool) {
	if len(s) <= limit {
		return s, false
	}
	cut := strings.LastIndex(s[:limit], "\n")
	if cut < limit/2 {
		cut = limit
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
	}
	return s[:cut], true
}

// EnvPublication reads the default publication from the environment.
func EnvPublication() string {
	for _, k := range []string{"SUBSTACK_PUBLICATION", "SUBSTACK_PUBLICATION_URL"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}
