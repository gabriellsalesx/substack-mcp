// Builds the npm packages: one per platform with the native binary, plus the
// main package that depends on them. Usage: node npm/build.mjs 1.2.3
import { execFileSync } from "node:child_process";
import { cpSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const version = (process.argv[2] || "").replace(/^v/, "");
if (!/^\d+\.\d+\.\d+(-[\w.]+)?$/.test(version)) {
  console.error("usage: node npm/build.mjs <version>");
  process.exit(1);
}

const here = dirname(fileURLToPath(import.meta.url));
const root = dirname(here);
const dist = join(here, "dist");
const main = JSON.parse(readFileSync(join(here, "substack-mcp", "package.json"), "utf8"));

// npm platform/arch -> Go GOOS/GOARCH
const targets = [
  ["linux", "x64", "linux", "amd64"],
  ["linux", "arm64", "linux", "arm64"],
  ["darwin", "x64", "darwin", "amd64"],
  ["darwin", "arm64", "darwin", "arm64"],
  ["win32", "x64", "windows", "amd64"],
  ["win32", "arm64", "windows", "arm64"],
];

rmSync(dist, { recursive: true, force: true });
main.version = version;
main.optionalDependencies = {};

for (const [os, cpu, goos, goarch] of targets) {
  const name = `${main.name}-${os}-${cpu}`;
  const dir = join(dist, name.split("/")[1]);
  const exe = os === "win32" ? "substack-mcp.exe" : "substack-mcp";
  mkdirSync(join(dir, "bin"), { recursive: true });
  console.log(`build ${name}`);
  execFileSync("go", [
    "build", "-trimpath", "-buildvcs=false",
    "-ldflags", `-s -w -X main.version=v${version}`,
    "-o", join(dir, "bin", exe), ".",
  ], { cwd: root, stdio: "inherit", env: { ...process.env, CGO_ENABLED: "0", GOOS: goos, GOARCH: goarch } });
  writeFileSync(join(dir, "package.json"), JSON.stringify({
    name,
    version,
    description: `${main.name} binary for ${os}-${cpu}`,
    repository: main.repository,
    license: main.license,
    author: main.author,
    os: [os],
    cpu: [cpu],
    files: ["bin"],
    preferUnplugged: true,
  }, null, 2) + "\n");
  main.optionalDependencies[name] = version;
}

const mainDir = join(dist, "substack-mcp");
cpSync(join(here, "substack-mcp", "bin"), join(mainDir, "bin"), { recursive: true });
cpSync(join(root, "README.md"), join(mainDir, "README.md"));
cpSync(join(root, "LICENSE"), join(mainDir, "LICENSE"));
writeFileSync(join(mainDir, "package.json"), JSON.stringify(main, null, 2) + "\n");
console.log(`\nnpm/dist ready for ${main.name}@${version}`);
