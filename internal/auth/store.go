// Package auth stores the Substack session on disk and resolves it from the
// environment.
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
)

// SessionCookies are the cookie names Substack has used for the login session.
var SessionCookies = []string{"substack.sid", "connect.sid"}

// Credentials is the persisted login state.
type Credentials struct {
	Version     int       `json:"version"`
	Cookies     []Cookie  `json:"cookies"`
	UserID      int64     `json:"user_id,omitempty"`
	Name        string    `json:"name,omitempty"`
	Email       string    `json:"email,omitempty"`
	Publication string    `json:"publication,omitempty"`
	SavedAt     time.Time `json:"saved_at"`
}

// Cookie is a stored session cookie, sent verbatim.
type Cookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ErrNoCredentials means neither the environment nor the config dir has a session.
var ErrNoCredentials = errors.New("not logged in to Substack: run `substack-mcp login` or set SUBSTACK_SID")

// Dir returns the config directory, honoring SUBSTACK_MCP_CONFIG_DIR.
func Dir() (string, error) {
	if d := os.Getenv("SUBSTACK_MCP_CONFIG_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "substack-mcp"), nil
}

// Path returns the credentials file path.
func Path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "credentials.json"), nil
}

// Load returns credentials from the environment first, then from disk.
func Load() (*Credentials, error) {
	if c := FromEnv(); c != nil {
		return c, nil
	}
	p, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNoCredentials
	}
	if err != nil {
		return nil, err
	}
	if runtime.GOOS != "windows" {
		if fi, err := os.Stat(p); err == nil && fi.Mode().Perm()&0o077 != 0 {
			return nil, fmt.Errorf("%s is readable by other users; run `chmod 600 %q`", p, p)
		}
	}
	var c Credentials
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	if len(c.Cookies) == 0 {
		return nil, ErrNoCredentials
	}
	return &c, nil
}

// FromEnv builds credentials from SUBSTACK_COOKIE, SUBSTACK_SID or the legacy
// SUBSTACK_SESSION_TOKEN. It returns nil when none is set.
func FromEnv() *Credentials {
	var cookies []Cookie
	if h := os.Getenv("SUBSTACK_COOKIE"); h != "" {
		cookies = ParseCookies(h)
	} else if v := firstEnv("SUBSTACK_SID", "SUBSTACK_SESSION_TOKEN"); v != "" {
		cookies = ParseCookies(v)
	}
	if len(cookies) == 0 {
		return nil
	}
	return &Credentials{Version: 1, Cookies: cookies}
}

// Save writes credentials atomically with 0600 permissions.
func Save(c *Credentials) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	c.Version = 1
	c.SavedAt = time.Now().UTC()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".credentials-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil && runtime.GOOS != "windows" {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}

// Delete removes the stored credentials. A missing file is not an error.
func Delete() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// ParseCookies accepts a bare session value, "name=value" pairs or a whole
// "Cookie:" header, and keeps only the session cookies. A bare value is sent
// under every known session cookie name, since Substack has used both.
func ParseCookies(s string) []Cookie {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "Cookie:")
	s = strings.TrimPrefix(s, "cookie:")
	s = strings.Trim(strings.TrimSpace(s), `"'`)
	if s == "" {
		return nil
	}
	if !strings.Contains(s, "=") {
		out := make([]Cookie, 0, len(SessionCookies))
		for _, name := range SessionCookies {
			out = append(out, Cookie{Name: name, Value: s})
		}
		return out
	}
	// Split by hand: http.ParseCookie rejects the whole header when any
	// unrelated cookie is malformed.
	var out []Cookie
	for pair := range strings.SplitSeq(s, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(pair), "=")
		value = strings.Trim(strings.TrimSpace(value), `"`)
		if ok && slices.Contains(SessionCookies, strings.TrimSpace(name)) && value != "" {
			out = append(out, Cookie{Name: strings.TrimSpace(name), Value: value})
		}
	}
	return out
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}
