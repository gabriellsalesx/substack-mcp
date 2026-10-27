// Package browser signs in through the user's installed Chromium-based browser
// and reads the Substack session cookie over the DevTools protocol, so nobody
// has to copy cookies by hand. No browser is bundled.
package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/websocket"

	"github.com/gabriellsalesx/substack-mcp/internal/auth"
)

// ErrNotFound means no Chromium-based browser was found.
var ErrNotFound = errors.New("no Chrome, Chromium, Brave, Edge or Vivaldi found; set SUBSTACK_BROWSER to its path, or use `substack-mcp login --cookie`")

// Find returns the path of an installed Chromium-based browser.
func Find() (string, error) {
	if p := os.Getenv("SUBSTACK_BROWSER"); p != "" {
		return p, nil
	}
	for _, name := range []string{
		"google-chrome", "google-chrome-stable", "chromium", "chromium-browser",
		"brave", "brave-browser", "microsoft-edge", "microsoft-edge-stable", "vivaldi",
	} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		for _, app := range []string{"Google Chrome", "Chromium", "Brave Browser", "Microsoft Edge", "Vivaldi"} {
			candidates = append(candidates, "/Applications/"+app+".app/Contents/MacOS/"+app)
		}
	case "windows":
		for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LocalAppData")} {
			if base == "" {
				continue
			}
			candidates = append(candidates,
				filepath.Join(base, `Google\Chrome\Application\chrome.exe`),
				filepath.Join(base, `BraveSoftware\Brave-Browser\Application\brave.exe`),
				filepath.Join(base, `Microsoft\Edge\Application\msedge.exe`),
				filepath.Join(base, `Chromium\Application\chrome.exe`))
		}
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", ErrNotFound
}

// Validator reports whether captured cookies are a working, signed-in session.
type Validator func(ctx context.Context, cookies []auth.Cookie) bool

// Login opens Substack's sign-in page in a fresh browser profile and waits
// until the user has signed in, then returns the session cookies and closes
// the browser. The temporary profile is deleted afterwards.
func Login(ctx context.Context, path string, valid Validator) ([]auth.Cookie, error) {
	dir, err := os.MkdirTemp("", "substack-mcp-login-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	cmd := exec.Command(path,
		"--user-data-dir="+dir,
		"--remote-debugging-port=0",
		"--remote-allow-origins=http://127.0.0.1",
		"--no-first-run",
		"--no-default-browser-check",
		"--new-window",
		"https://substack.com/sign-in",
	)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", path, err)
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	defer func() {
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-exited
		}
	}()

	ws, err := connect(ctx, dir, exited)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}
	defer ws.Close()
	cdp := &client{ws: ws}

	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	last := ""
	for {
		select {
		case <-ctx.Done():
			_, _ = cdp.call("Browser.close")
			return nil, ctx.Err()
		case <-exited:
			return nil, errors.New("the browser was closed before the sign-in finished")
		case <-tick.C:
		}
		cookies, err := cdp.sessionCookies()
		if err != nil {
			return nil, err
		}
		// Anonymous visitors get a session cookie too, so only a cookie that
		// Substack accepts as signed in ends the wait.
		key := fmt.Sprint(cookies)
		if len(cookies) == 0 || key == last {
			continue
		}
		last = key
		if valid(ctx, cookies) {
			_, _ = cdp.call("Browser.close")
			return cookies, nil
		}
	}
}

// connect waits for the browser to write its DevTools endpoint and dials it.
func connect(ctx context.Context, dir string, exited <-chan struct{}) (*websocket.Conn, error) {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(filepath.Join(dir, "DevToolsActivePort"))
		if lines := strings.Split(strings.TrimSpace(string(data)), "\n"); err == nil && len(lines) == 2 {
			url := "ws://127.0.0.1:" + strings.TrimSpace(lines[0]) + strings.TrimSpace(lines[1])
			ws, err := websocket.Dial(url, "", "http://127.0.0.1")
			if err != nil {
				return nil, fmt.Errorf("connect to the browser: %w", err)
			}
			ws.MaxPayloadBytes = 32 << 20
			return ws, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-exited:
			return nil, errors.New("the browser exited right away; if it was already open, it may have reused that window. Set SUBSTACK_BROWSER or use --cookie")
		case <-time.After(200 * time.Millisecond):
		}
	}
	return nil, errors.New("the browser did not open its DevTools port; try `substack-mcp login --cookie`")
}

// client is the few DevTools protocol calls the login needs.
type client struct {
	ws *websocket.Conn
	id atomic.Int64
}

func (c *client) call(method string) (json.RawMessage, error) {
	id := c.id.Add(1)
	if err := websocket.JSON.Send(c.ws, map[string]any{"id": id, "method": method}); err != nil {
		return nil, err
	}
	for {
		var msg struct {
			ID     int64           `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := c.ws.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
			return nil, err
		}
		if err := websocket.JSON.Receive(c.ws, &msg); err != nil {
			return nil, err
		}
		if msg.ID != id {
			continue // an event, not our answer
		}
		if msg.Error != nil {
			return nil, fmt.Errorf("%s: %s", method, msg.Error.Message)
		}
		return msg.Result, nil
	}
}

// sessionCookies reads HttpOnly cookies too, which page scripts cannot see.
func (c *client) sessionCookies() ([]auth.Cookie, error) {
	raw, err := c.call("Storage.getCookies")
	if err != nil {
		return nil, err
	}
	var res struct {
		Cookies []struct {
			Name   string `json:"name"`
			Value  string `json:"value"`
			Domain string `json:"domain"`
		} `json:"cookies"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	var out []auth.Cookie
	for _, ck := range res.Cookies {
		d := strings.TrimPrefix(ck.Domain, ".")
		if d != "substack.com" || ck.Value == "" {
			continue
		}
		for _, name := range auth.SessionCookies {
			if ck.Name == name {
				out = append(out, auth.Cookie{Name: ck.Name, Value: ck.Value})
			}
		}
	}
	return out, nil
}
