// Publishes npm/dist: platform packages first, then the main package, so the
// main package never points at versions that are not on the registry yet.
// Extra arguments go to `npm publish` (e.g. --dry-run, --provenance).
import { execFileSync } from "node:child_process";
import { readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const dist = join(dirname(fileURLToPath(import.meta.url)), "dist");
const dirs = readdirSync(dist).sort((a, b) => (a === "substack-mcp") - (b === "substack-mcp"));
for (const d of dirs) {
  console.log(`\npublish ${d}`);
  execFileSync("npm", ["publish", "--access", "public", ...process.argv.slice(2)], { cwd: join(dist, d), stdio: "inherit" });
}
