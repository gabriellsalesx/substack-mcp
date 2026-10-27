package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/gabriellsalesx/substack-mcp/internal/auth"
	"github.com/gabriellsalesx/substack-mcp/internal/browser"
	"github.com/gabriellsalesx/substack-mcp/internal/server"
	"github.com/gabriellsalesx/substack-mcp/internal/substack"
)

const loginHelp = `Usage: substack-mcp login [flags]

Opens Substack in your Chrome, Chromium, Brave or Edge (a clean window, not
your everyday profile). Sign in as usual; the session is captured and the
window closes by itself. Without a usable browser, or with --cookie, you can
paste the session cookie instead.

`

const cookieSteps = `
To copy your session cookie:
  1. Sign in at https://substack.com in your browser.
  2. Open DevTools (F12) > Application (Chrome) or Storage (Firefox) > Cookies > https://substack.com
  3. Copy the Value of "substack.sid" (or "connect.sid"). It usually starts with s%3A.
     You can also paste a whole "Cookie:" request header from the Network tab.
`

func login(ctx context.Context, args []string) error {
	fs := newFlagSet("login", loginHelp)
	useCookie := fs.Bool("cookie", false, "paste a session cookie instead of signing in through the browser")
	pub := fs.String("publication", server.EnvPublication(), "default publication (URL, host or subdomain)")
	_ = fs.Parse(args) // ExitOnError: never returns an error

	in := bufio.NewReader(os.Stdin)
	interactive := term.IsTerminal(int(os.Stdin.Fd()))
	ua := os.Getenv("SUBSTACK_USER_AGENT")

	var cookies []auth.Cookie
	if !*useCookie && interactive {
		var err error
		cookies, err = browserLogin(ctx, ua)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			fmt.Fprintf(os.Stderr, "\nBrowser sign-in did not work: %v\n", err)
		}
	}
	if len(cookies) == 0 {
		var err error
		if cookies, err = cookieLogin(in, interactive); err != nil {
			return err
		}
	}

	c := substack.New(substack.Options{Cookies: cookies, UserAgent: ua})
	prof, err := c.Profile(ctx)
	if err != nil {
		return fmt.Errorf("the session was not accepted: %w", err)
	}
	pubs, err := c.Publications(ctx)
	if err != nil {
		return err
	}
	creds := &auth.Credentials{Cookies: cookies, UserID: prof.ID, Name: prof.Name, Email: prof.Email}
	switch {
	case *pub != "":
		p, err := c.Publication(ctx, *pub)
		if err != nil {
			return err
		}
		creds.Publication = p.PublicURL()
	case len(pubs) > 1 && interactive:
		creds.Publication = choosePublication(in, pubs)
	case len(pubs) > 0:
		creds.Publication = pubs[0].PublicURL()
	}
	if err := auth.Save(creds); err != nil {
		return err
	}
	path, _ := auth.Path()
	fmt.Printf("\nSigned in as %s (@%s). Default publication: %s\nSaved to %s\n", prof.Name, prof.Handle, orNone(creds.Publication), path)
	return nil
}

func browserLogin(ctx context.Context, ua string) ([]auth.Cookie, error) {
	path, err := browser.Find()
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "Opening Substack in %s.\nSign in as usual (email code or password); this window closes by itself when you are in.\n", filepath.Base(path))
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	return browser.Login(ctx, path, func(ctx context.Context, cookies []auth.Cookie) bool {
		_, err := substack.New(substack.Options{Cookies: cookies, UserAgent: ua}).Profile(ctx)
		return err == nil
	})
}

func cookieLogin(in *bufio.Reader, interactive bool) ([]auth.Cookie, error) {
	var raw string
	if interactive {
		fmt.Fprint(os.Stderr, cookieSteps)
		fmt.Fprint(os.Stderr, "\nPaste the cookie (input is hidden): ")
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return nil, err
		}
		raw = string(b)
	} else {
		// Piped input, e.g. from a password manager.
		b, err := io.ReadAll(io.LimitReader(in, 64<<10))
		if err != nil {
			return nil, err
		}
		raw = string(b)
	}
	cookies := auth.ParseCookies(raw)
	if len(cookies) == 0 {
		return nil, fmt.Errorf("no substack.sid or connect.sid value found in the input")
	}
	return cookies, nil
}

func choosePublication(in *bufio.Reader, pubs []substack.Publication) string {
	fmt.Println("\nPublications on this account:")
	for i, p := range pubs {
		fmt.Printf("  %d. %s (%s)\n", i+1, p.Name, p.PublicURL())
	}
	n, err := strconv.Atoi(prompt(in, "Default publication [1]: "))
	if err != nil || n < 1 || n > len(pubs) {
		n = 1
	}
	return pubs[n-1].PublicURL()
}

func status(ctx context.Context) error {
	creds, err := auth.Load()
	if err != nil {
		return err
	}
	source := "environment"
	if auth.FromEnv() == nil {
		source, _ = auth.Path()
	}
	c := substack.New(substack.Options{Cookies: creds.Cookies, UserAgent: os.Getenv("SUBSTACK_USER_AGENT")})
	prof, err := c.Profile(ctx)
	if err != nil {
		return err
	}
	pubs, err := c.Publications(ctx)
	if err != nil {
		return err
	}
	def := server.EnvPublication()
	if def == "" {
		def = creds.Publication
	}
	fmt.Printf("Signed in as %s (@%s), user id %d\nSession from: %s\nDefault publication: %s\n\nPublications:\n", prof.Name, prof.Handle, prof.ID, source, orNone(def))
	for _, p := range pubs {
		fmt.Printf("  - %s  %s  (%s)\n", p.Name, p.PublicURL(), p.Role)
	}
	return nil
}

func logout() error {
	if err := auth.Delete(); err != nil {
		return err
	}
	fmt.Println("Stored session removed. Your browser session is untouched.")
	return nil
}

func prompt(in *bufio.Reader, label string) string {
	fmt.Fprint(os.Stderr, label)
	s, _ := in.ReadString('\n')
	return strings.TrimSpace(s)
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
