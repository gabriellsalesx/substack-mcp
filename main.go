// Command substack-mcp is an MCP server for Substack. Run it without
// arguments from an MCP client (stdio), or use `substack-mcp login` once to
// connect your account.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gabriellsalesx/substack-mcp/internal/server"
)

// version is set by the release build with -ldflags "-X main.version=...".
var version = ""

const usage = `substack-mcp: MCP server for Substack (unofficial)

Usage:
  substack-mcp            run the MCP server over stdio (what MCP clients call)
  substack-mcp login      connect your Substack account
  substack-mcp status     show the connected account and publications
  substack-mcp logout     forget the stored session
  substack-mcp version    print the version

Run "substack-mcp login -h" for login options.

Environment:
  SUBSTACK_PUBLICATION    default publication (URL, host or subdomain)
  SUBSTACK_SID            session cookie value; overrides the stored login
  SUBSTACK_COOKIE         full Cookie header; overrides SUBSTACK_SID
  SUBSTACK_USER_AGENT     User-Agent for Substack requests
  SUBSTACK_BROWSER        browser used by login (Chrome, Chromium, Brave, Edge)
  SUBSTACK_MCP_CONFIG_DIR where credentials.json lives
  SUBSTACK_DEBUG=1        log HTTP calls to stderr
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	args := []string{}
	if len(os.Args) > 2 {
		args = os.Args[2:]
	}

	var err error
	switch cmd {
	case "serve":
		err = serve(ctx)
	case "login":
		err = login(ctx, args)
	case "status":
		err = status(ctx)
	case "logout":
		err = logout()
	case "version", "--version", "-v":
		fmt.Println(appVersion())
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func serve(ctx context.Context) error {
	// Quiet by default: MCP clients surface stderr as server logs.
	level := slog.LevelWarn
	if os.Getenv("SUBSTACK_DEBUG") != "" && os.Getenv("SUBSTACK_DEBUG") != "0" && os.Getenv("SUBSTACK_DEBUG") != "false" {
		level = slog.LevelDebug
	}
	// stdout carries the protocol; everything else goes to stderr.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	s := server.New(server.Config{
		Version:     appVersion(),
		Logger:      logger,
		UserAgent:   os.Getenv("SUBSTACK_USER_AGENT"),
		Publication: server.EnvPublication(),
	})
	return s.Run(ctx, &mcp.StdioTransport{})
}

func appVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

func newFlagSet(name, help string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), help)
		fs.PrintDefaults()
	}
	return fs
}
