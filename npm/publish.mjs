// Publishes npm/dist: platform packages first, then the main package, so the
// main package never points at versions that are not on the registry yet.
// Extra arguments go to `npm publish` (e.g. --dry-run, --provenance).
// Versions already on the registry are skipped and failed publishes are
// retried, so a release that broke halfway can simply be run again.
import { execFileSync } from "node:child_process";
import { readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const dist = join(dirname(fileURLToPath(import.meta.url)), "dist");
const dirs = readdirSync(dist).sort((a, b) => (a === "substack-mcp") - (b === "substack-mcp"));

function published(name, version) {
  try {
    return execFileSync("npm", ["view", `${name}@${version}`, "version"], { encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] }).trim() === version;
  } catch {
    return false;
  }
}

for (const d of dirs) {
  const { name, version } = JSON.parse(readFileSync(join(dist, d, "package.json"), "utf8"));
  for (let attempt = 1; ; attempt++) {
    if (published(name, version)) {
      console.log(`\n${name}@${version} is on the registry`);
      break;
    }
    console.log(`\npublish ${name}@${version} (attempt ${attempt})`);
    try {
      execFileSync("npm", ["publish", "--access", "public", ...process.argv.slice(2)], { cwd: join(dist, d), stdio: "inherit" });
      break;
    } catch (err) {
      // A timed-out publish may still have gone through: the check above
      // sees it on the next attempt.
      if (attempt === 3) throw err;
      execFileSync("sleep", [String(15 * attempt)]);
    }
  }
}
