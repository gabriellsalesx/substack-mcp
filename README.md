# substack-mcp

An MCP server for Substack, written in Go. It is a single binary with no Python, Node or bundled browser, and it works with any stdio MCP client: Claude Code, Codex, OpenCode, Gemini CLI, Cursor, Claude Desktop and others.

Ask your agent to write drafts in Markdown (with real formatting and images), revise them, schedule, publish, read past posts and post Notes.

> Unofficial; not affiliated with Substack Inc. It uses the same internal API as the Substack website, which may change without notice.

[Versão em português](README.pt-BR.md)

## Install

Pick one; none needs Go.

```bash
# With Node.js: nothing to install, the first run fetches the right binary
npx -y @gabriellsales/substack-mcp login

# Without Node: download from GitHub Releases, verify the checksum, install to ~/.local/bin
curl -fsSL https://raw.githubusercontent.com/gabriellsalesx/substack-mcp/main/install.sh | sh
```

```powershell
# Windows, without Node
irm https://raw.githubusercontent.com/gabriellsalesx/substack-mcp/main/install.ps1 | iex
```

Also: `npm i -g @gabriellsales/substack-mcp`, `pnpm add -g @gabriellsales/substack-mcp`, `go install github.com/gabriellsalesx/substack-mcp@latest`, or a binary from [Releases](https://github.com/gabriellsalesx/substack-mcp/releases).

## Log in

```bash
npx -y @gabriellsales/substack-mcp login   # opens Substack in your Chrome/Chromium/Brave/Edge; sign in, done
substack-mcp login --cookie                # no browser (servers, Firefox-only): paste the substack.sid cookie
substack-mcp status                        # after npm i -g (or prefix with npx -y @gabriellsales/substack-mcp)
substack-mcp logout
```

`login` uses a temporary browser profile, captures the session as soon as you are signed in and closes the window by itself. Set `SUBSTACK_BROWSER` if your browser is not on `PATH`. The session is stored in your OS config dir (`credentials.json`, mode 600); your password is never stored. In CI, set `SUBSTACK_SID` instead.

## Client setup

```bash
claude mcp add -s user substack -- npx -y @gabriellsales/substack-mcp   # Claude Code
gemini mcp add -s user substack npx -y @gabriellsales/substack-mcp      # Gemini CLI
```

Codex (`~/.codex/config.toml`):

```toml
[mcp_servers.substack]
command = "npx"
args = ["-y", "@gabriellsales/substack-mcp"]
default_tools_approval_mode = "writes"
```

OpenCode (`opencode.json`):

```json
{ "mcp": { "substack": { "type": "local", "command": ["npx", "-y", "@gabriellsales/substack-mcp"], "enabled": true } } }
```

Cursor / Claude Desktop:

```json
{ "mcpServers": { "substack": { "command": "npx", "args": ["-y", "@gabriellsales/substack-mcp"] } } }
```

On Windows, if `npx` is not found, use `"command": "cmd", "args": ["/c", "npx", "-y", "@gabriellsales/substack-mcp"]`.

## Tools

`list_posts`, `get_post`, `create_draft`, `update_draft`, `duplicate_post`, `publish_post`, `delete_draft`, `upload_image`, `get_publication`, `create_note`, `list_scheduled_notes`, `delete_note`, `create_community_post`.

Tools that reach readers or delete data (`publish_post`, `delete_draft`, `create_note`, `delete_note`, `create_community_post`) return a preview unless called with `confirm: true`, so the agent has to show you what will happen first. `send_email` defaults to `false`. Every tool takes an optional `publication` for accounts with several newsletters.

Markdown support: headings, bold, italic, strikethrough, inline code, links, nested lists, quotes, fenced code, rules, images with captions (local files and URLs are uploaded to Substack's CDN), `<!-- paywall -->` to mark the paywall, and `<!-- subscribe -->` for the subscribe button (copied from your latest post that has one). Tables become code blocks.

## Environment

`SUBSTACK_PUBLICATION` (default publication), `SUBSTACK_SID` / `SUBSTACK_COOKIE` (session override), `SUBSTACK_USER_AGENT`, `SUBSTACK_BROWSER`, `SUBSTACK_MCP_CONFIG_DIR`, `SUBSTACK_DEBUG=1`.

## Author

Created and maintained by **Gabriell Sales**: [GitHub](https://github.com/gabriellsalesx) · [Substack](https://gabriellsales.substack.com). Issues and pull requests are welcome.

## License

MIT. See [LICENSE](LICENSE).
