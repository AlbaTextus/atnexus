import { spawnSync } from "node:child_process"
import { mkdtempSync, readdirSync, rmdirSync, rmSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import path from "node:path"
import { fileURLToPath } from "node:url"

const repo = fileURLToPath(new URL("../", import.meta.url))
const testRoot = path.join(repo, "test", "_go")
const replace = {}

// Compile each relocated test as part of its original Go package.
function collect(dir) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const file = path.join(dir, entry.name)
    if (entry.isDirectory()) collect(file)
    else if (entry.isFile() && entry.name.endsWith("_test.go")) {
      replace[path.join(repo, path.relative(testRoot, file))] = file
    }
  }
}

collect(testRoot)

const temp = mkdtempSync(path.join(tmpdir(), "atnexus-go-test-"))
const overlay = path.join(temp, "overlay.json")
try {
  writeFileSync(overlay, JSON.stringify({ Replace: replace }))

  const args = process.argv.slice(2)
  const result = spawnSync("go", ["test", `-overlay=${overlay}`, ...(args.length ? args : ["./..."])], {
    cwd: repo, stdio: "inherit", windowsHide: true,
  })
  if (result.error) throw result.error
  if (result.signal) console.error(`go test terminated by ${result.signal}`)
  process.exitCode = result.status ?? 1
} finally {
  rmSync(overlay, { force: true })
  rmdirSync(temp)
}
