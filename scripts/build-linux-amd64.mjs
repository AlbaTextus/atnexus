#!/usr/bin/env node
import { spawnSync } from "node:child_process"
import { createHash } from "node:crypto"
import { createReadStream } from "node:fs"
import { access, cp, mkdir, mkdtemp, readdir, rename, rm, writeFile } from "node:fs/promises"
import path from "node:path"
import { fileURLToPath } from "node:url"

const repo = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..")
const args = process.argv.slice(2)
if (args.includes("--help") || args.includes("-h")) {
  console.log(`Usage: node scripts/build-linux-amd64.mjs [release-name]

Native Windows or Linux build; WSL and Docker are not required.
Requires Go (see go.mod), Node.js >= 20.9, Corepack, and tar.
Output: release/<name>/, <name>.tar.gz, and <name>.tar.gz.sha256.
Target server: Linux AMD64 with glibc, Node.js >= 20.9, PostgreSQL.

Optional build environment:
  OAUTH_BACKEND_URL          http://127.0.0.1:8010
  NEXT_PUBLIC_ATHUB_API_URL https://api.albatextus.com
  NEXT_PUBLIC_CREATPLAT_API_URL  public creator API origin
  CREATPLAT_BACKEND_URL     http://127.0.0.1:8010
  NEXT_PUBLIC_WIKI_URL      http://localhost:3200
  NEXT_PUBLIC_CREDIT_URL    http://localhost:3300

Local .env files are excluded. Public URLs must be set before building.`)
  process.exit(0)
}

const stamp = new Date().toISOString().replace(/[-:]/g, "").replace("T", "-").slice(0, 15)
const name = args[0] || `atnexus-linux-amd64-${stamp}`
if (args.length > 1 || !/^[a-zA-Z0-9][a-zA-Z0-9._-]*$/.test(name)) {
  throw new Error("Expected one release name containing only letters, digits, dots, underscores, or hyphens.")
}
const [major, minor] = process.versions.node.split(".").map(Number)
if (major < 20 || (major === 20 && minor < 9)) throw new Error("Node.js >= 20.9 is required.")

// Run package managers through their JS entry, avoiding cmd.exe quoting and .cmd shims.
function run(command, argv, cwd = repo, extraEnv = {}) {
  const result = spawnSync(command, argv, {
    cwd, env: { ...process.env, ...extraEnv }, stdio: "inherit", windowsHide: true,
  })
  if (result.error) throw result.error
  if (result.status !== 0) throw new Error(`${path.basename(command)} ${argv[0] || ""} failed (${result.status ?? result.signal}).`)
}
const corepack = spawnSync(process.execPath, ["-p", 'require.resolve("corepack/dist/corepack.js")'], {
  encoding: "utf8", windowsHide: true,
})
let corepackFile = corepack.status === 0 ? corepack.stdout.trim() : ""
if (!corepackFile) {
  const nodeDirectory = path.dirname(process.execPath)
  const candidates = [
    path.join(nodeDirectory, "node_modules/corepack/dist/corepack.js"),
    path.resolve(nodeDirectory, "../lib/node_modules/corepack/dist/corepack.js"),
    "/usr/local/lib/node_modules/corepack/dist/corepack.js",
    "/usr/lib/node_modules/corepack/dist/corepack.js",
  ]
  for (const candidate of candidates) {
    try { await access(candidate); corepackFile = candidate; break } catch { /* try the next installation layout */ }
  }
}
if (!corepackFile) throw new Error("Corepack is required. Install it with: npm install --global corepack")
function pnpm(argv, cwd, extraEnv = {}) {
  run(process.execPath, [corepackFile, "pnpm", ...argv], cwd, extraEnv)
}
run("go", ["version"])
run("tar", ["--version"])

const releaseRoot = path.join(repo, "release")
const tempRoot = path.join(repo, "temp")
await mkdir(releaseRoot, { recursive: true })
await mkdir(tempRoot, { recursive: true })
for (const suffix of ["", ".tar.gz", ".tar.gz.sha256"]) {
  let exists = false
  try { await access(path.join(releaseRoot, name + suffix)); exists = true } catch (error) {
    if (error.code !== "ENOENT") throw error
  }
  if (exists) throw new Error(`Output already exists: release/${name + suffix}`)
}
const urls = {
  OAUTH_BACKEND_URL: process.env.OAUTH_BACKEND_URL || "http://127.0.0.1:8010",
  CREATPLAT_BACKEND_URL: process.env.CREATPLAT_BACKEND_URL || "http://127.0.0.1:8010",
  NEXT_PUBLIC_ATHUB_API_URL: process.env.NEXT_PUBLIC_ATHUB_API_URL || "http://localhost:8010",
  NEXT_PUBLIC_CREATPLAT_API_URL: process.env.NEXT_PUBLIC_CREATPLAT_API_URL || "http://localhost:8010",
  NEXT_PUBLIC_WIKI_URL: process.env.NEXT_PUBLIC_WIKI_URL || "http://localhost:3200",
  NEXT_PUBLIC_CREDIT_URL: process.env.NEXT_PUBLIC_CREDIT_URL || "http://localhost:3300",
}
for (const [key, value] of Object.entries(urls)) {
  const parsed = new URL(value)
  if (!["http:", "https:"].includes(parsed.protocol) || parsed.username || parsed.password || /[\r\n]/.test(value)) {
    throw new Error(`${key} must be an HTTP(S) URL without credentials or newlines.`)
  }
}
const buildEnv = {
  ...urls, NEXT_TELEMETRY_DISABLED: "1", NEXT_OUTPUT_DIR: ".next",
}
const ignored = new Set(["node_modules", ".next", ".git", ".codegraph", "temp", "temps", "release"])
async function copySource(from, to) {
  await cp(from, to, {
    recursive: true,
    filter: file => {
      const base = path.basename(file)
      return !ignored.has(base) && base !== ".env" && !base.startsWith(".env.") && !base.endsWith(".tsbuildinfo")
    },
  })
}
async function hash(file) {
  const digest = createHash("sha256")
  for await (const chunk of createReadStream(file)) digest.update(chunk)
  return digest.digest("hex")
}
async function checksumFiles(directory, prefix = "") {
  const lines = []
  const entries = await readdir(directory, { withFileTypes: true })
  entries.sort((a, b) => a.name < b.name ? -1 : a.name > b.name ? 1 : 0)
  for (const entry of entries) {
    const relative = prefix + entry.name
    const absolute = path.join(directory, entry.name)
    if (entry.isDirectory()) lines.push(...await checksumFiles(absolute, relative + "/"))
    else if (entry.isFile()) lines.push(`${await hash(absolute)}  ${relative}`)
    else throw new Error(`Unexpected symlink in deployment tree: ${relative}`)
  }
  return lines
}
const work = await mkdtemp(path.join(tempRoot, "build-linux-amd64-"))
let staging
try {
  staging = await mkdtemp(path.join(releaseRoot, ".build-linux-amd64-"))
  const source = path.join(work, "source")
  const bundle = path.join(staging, name)
  const frontend = path.join(bundle, "frontend")
  console.log("[1/5] Copying current sources without local .env or build outputs")
  await mkdir(source, { recursive: true })
  await mkdir(frontend, { recursive: true })
  for (const item of ["go.mod", "go.sum", "cmd", "internal", "modules", "pkg", "prisma", "web",
    "package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml"]) {
    await copySource(path.join(repo, item), path.join(source, item))
  }
  await cp(path.join(repo, ".env.example"), path.join(bundle, ".env.example"))
  await cp(path.join(source, "prisma/oauth"), path.join(bundle, "database/account"), { recursive: true })
  await cp(path.join(source, "prisma/athub"), path.join(bundle, "database/content"), { recursive: true })
  await cp(path.join(source, "prisma/creatplat"), path.join(bundle, "database/creator"), { recursive: true })
  await cp(path.join(source, "prisma/image"), path.join(bundle, "database/image"), { recursive: true })

  console.log("[2/5] Cross-compiling Go server for linux/amd64")
  run("go", ["build", "-trimpath", "-ldflags=-s -w", "-o", path.join(bundle, "atnexus"), "./cmd/server"],
    source, { CGO_ENABLED: "0", GOOS: "linux", GOARCH: "amd64", GOAMD64: "v1" })
  run("go", ["build", "-trimpath", "-ldflags=-s -w", "-o", path.join(bundle, "atnexus-migrate-creatplat"), "./cmd/migrate-creatplat"],
    source, { CGO_ENABLED: "0", GOOS: "linux", GOARCH: "amd64", GOAMD64: "v1" })
  run("go", ["build", "-trimpath", "-ldflags=-s -w", "-o", path.join(bundle, "atnexus-migrate-image"), "./cmd/migrate-image"],
    source, { CGO_ENABLED: "0", GOOS: "linux", GOARCH: "amd64", GOAMD64: "v1" })

  console.log("[3/5] Building Next.js using the host toolchain")
  pnpm(["install", "--frozen-lockfile", "--prod=false"], source, buildEnv)
  pnpm(["--filter", "@albatextus/atnexus-web", "exec", "next", "build", "--webpack"], source, buildEnv)
  for (const item of ["package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml"]) {
    await cp(path.join(source, item), path.join(frontend, item))
  }
  await mkdir(path.join(frontend, "web"), { recursive: true })
  for (const item of ["package.json", "next.config.mjs"]) {
    await cp(path.join(source, "web", item), path.join(frontend, "web", item))
  }
  await cp(path.join(source, "web/.next"), path.join(frontend, "web/.next"), {
    recursive: true,
    filter: file => path.basename(file) !== "cache",
  })
  try { await access(path.join(source, "web/public")); await cp(path.join(source, "web/public"), path.join(frontend, "web/public"), { recursive: true }) }
  catch (error) { if (error.code !== "ENOENT") throw error }

  console.log("[4/5] Installing production dependencies for Linux AMD64 (glibc)")
  // A fresh hoisted tree avoids Windows junctions and installs Linux native
  // packages (including sharp and SWC), rather than copying host node_modules.
  pnpm(["install", "--prod", "--frozen-lockfile", "--ignore-scripts",
    "--os=linux", "--cpu=x64", "--libc=glibc", "--config.node-linker=hoisted"], frontend,
  { ...buildEnv, CI: "true" })
  // Deployment uses node directly; pnpm's generated Windows shims are unnecessary.
  for (const location of ["node_modules/.bin", "web/node_modules/.bin"]) {
    await rm(path.join(frontend, location), { recursive: true, force: true })
  }
  for (const native of ["@img/sharp-linux-x64", "@img/sharp-libvips-linux-x64"]) {
    await access(path.join(frontend, "node_modules", native))
  }
  await writeFile(path.join(bundle, "start-backend.sh"), `#!/bin/sh
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
exec ./atnexus
`)
  const shellQuote = value => "'" + value.replaceAll("'", "'\\''") + "'"
  await writeFile(path.join(bundle, "start-frontend.sh"), `#!/bin/sh
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/frontend/web"
export NEXT_TELEMETRY_DISABLED=1
export NEXT_OUTPUT_DIR=.next
if [ -z "\${OAUTH_BACKEND_URL:-}" ]; then export OAUTH_BACKEND_URL=${shellQuote(urls.OAUTH_BACKEND_URL)}; fi
if [ -z "\${CREATPLAT_BACKEND_URL:-}" ]; then export CREATPLAT_BACKEND_URL=${shellQuote(urls.CREATPLAT_BACKEND_URL)}; fi
exec node ../node_modules/next/dist/bin/next start --hostname "\${HOSTNAME_OVERRIDE:-0.0.0.0}" --port "\${PORT:-3288}"
`)
  await writeFile(path.join(bundle, "README.txt"), `ATNexus Linux AMD64

Backend: static Go binary, CGO_ENABLED=0, GOAMD64=v1.
Frontend: Next.js built on ${process.platform}/${process.arch}, Linux x64 glibc production dependencies.
Target requires Linux AMD64 with glibc and Node.js >= 20.9 (recommended: Node.js 24).
Node.js is not bundled.

Verify archive: sha256sum -c ${name}.tar.gz.sha256
Verify extracted files: sha256sum -c SHA256SUMS
Preserve the existing .env, encryption keys, databases and user data.
For a new installation, copy .env.example to .env and configure it; chmod 600 .env.
chmod +x atnexus atnexus-migrate-creatplat atnexus-migrate-image start-backend.sh start-frontend.sh
Run ./start-backend.sh and ./start-frontend.sh as separate services.
The backend serves all five APIs on port 8010 (GATEWAY_HTTP_ADDR); the frontend uses 3288.
When upgrading, point reverse proxies and API clients to the gateway instead of the old module ports.
HOSTNAME_OVERRIDE and PORT configure the frontend listener.

Build-time configuration:
${Object.entries(urls).map(([key, value]) => `  ${key}=${value}`).join("\n")}
Public URLs and Next.js rewrites require a rebuild to change.
OAUTH_BACKEND_URL also configures server-side API requests at runtime.
Frontend launch does not load the backend .env.

Database schemas and migrations: database/account, database/content, database/creator, database/image.
Apply pending migrations using your established Prisma deployment workflow.
CREATPLAT_DATABASE_URL must point to a separate PostgreSQL database.
After applying creator migrations, stop all backend writers and run
./atnexus-migrate-creatplat to import and verify legacy reviews/history.
Source data is retained; do not resume the old backend after cutover.
IMAGE_DATABASE_URL must point to its own PostgreSQL database. Apply account
and image migrations, stop legacy image writers, back up SQLite and file bytes,
then run ./atnexus-migrate-image -source /path/to/database.sqlite.
Set IMAGE_LOCAL_ROOT to the existing byte directory and IMAGE_PUBLIC_URL to the
public origin. Existing /file paths are served by the gateway; route any old
public image origin to the gateway if those links must remain reachable.
The original image frontend is not included or modified by this migration.
Startup does not migrate databases or seed data.
Local .env files are excluded from the build and package.
`)

  console.log("[5/5] Creating archive and SHA-256 checksums")
  await writeFile(path.join(bundle, "SHA256SUMS"), (await checksumFiles(bundle)).join("\n") + "\n")
  run("tar", ["-czf", path.join(staging, name + ".tar.gz"), "-C", staging, name])
  await writeFile(path.join(staging, name + ".tar.gz.sha256"),
    `${await hash(path.join(staging, name + ".tar.gz"))}  ${name}.tar.gz\n`)
  for (const suffix of ["", ".tar.gz", ".tar.gz.sha256"]) {
    await rename(path.join(staging, name + suffix), path.join(releaseRoot, name + suffix))
  }
  console.log(`\nBuild complete: ${path.join(releaseRoot, name + ".tar.gz")}`)
} finally {
  // Both paths are created by mkdtemp under this repository, never user input.
  await rm(work, { recursive: true, force: true })
  if (staging) await rm(staging, { recursive: true, force: true })
}
