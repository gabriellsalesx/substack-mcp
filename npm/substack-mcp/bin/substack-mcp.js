#!/usr/bin/env node
// Launcher: runs the native binary that npm installed for this platform.
"use strict";

const { spawn } = require("node:child_process");

const pkg = `@gabriellsales/substack-mcp-${process.platform}-${process.arch}`;
const exe = process.platform === "win32" ? "substack-mcp.exe" : "substack-mcp";

let bin;
try {
  bin = require.resolve(`${pkg}/bin/${exe}`);
} catch {
  console.error(
    `substack-mcp: no binary for ${process.platform}-${process.arch}.\n` +
      `Reinstall without --no-optional / --omit=optional, or download it from\n` +
      `https://github.com/gabriellsalesx/substack-mcp/releases`
  );
  process.exit(1);
}

const child = spawn(bin, process.argv.slice(2), { stdio: "inherit", windowsHide: true });
for (const sig of ["SIGINT", "SIGTERM", "SIGHUP"]) {
  process.on(sig, () => child.kill(sig));
}
child.on("error", (err) => {
  console.error(`substack-mcp: ${err.message}`);
  process.exit(1);
});
child.on("exit", (code, signal) => {
  if (signal) process.kill(process.pid, signal);
  else process.exit(code ?? 1);
});
