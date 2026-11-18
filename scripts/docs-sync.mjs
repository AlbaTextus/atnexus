import { spawnSync } from "node:child_process"
import { fileURLToPath } from "node:url"

// Keeps the documentation submodule in step with this repository:
// checks out only the Markdown, follows the tracked branch, and records the
// submodule commit here. Pass -m "<message>" to commit pending doc edits first.
// Nothing is pushed: pushing the docs branch publishes the site.

const repo = fileURLToPath(new URL("../", import.meta.url))
const submodule = "docs"
const markdownDir = "content"

function git(args, { cwd = repo, check = true } = {}) {
  const result = spawnSync("git", args, { cwd, encoding: "utf8", windowsHide: true })
  if (result.error) throw result.error
  if (check && result.status !== 0) {
    throw new Error(`git ${args.join(" ")} 失败：\n${(result.stderr || result.stdout).trim()}`)
  }
  return { ok: result.status === 0, out: result.stdout.trim() }
}

function setting(key) {
  const value = git(["config", "-f", ".gitmodules", `submodule.${submodule}.${key}`], { check: false }).out
  if (!value) throw new Error(`.gitmodules 缺少 submodule.${submodule}.${key}`)
  return value
}

function messageArg() {
  const args = process.argv.slice(2)
  if (!args.length) return ""
  if (args.length !== 2 || args[0] !== "-m" || !args[1].trim()) {
    throw new Error('用法：pnpm docs:sync [-m "文档提交说明"]')
  }
  return args[1]
}

function main() {
  const message = messageArg()
  const dir = fileURLToPath(new URL(`../${setting("path")}/`, import.meta.url))
  const branch = setting("branch")
  const remote = `origin/${branch}`

  // A fresh clone or worktree has an empty directory until the submodule is initialised.
  if (git(["submodule", "status", "--", submodule]).out.startsWith("-")) {
    git(["submodule", "update", "--init", "--", submodule])
  }
  git(["sparse-checkout", "set", "--no-cone", `/${markdownDir}/`], { cwd: dir })
  git(["switch", branch], { cwd: dir })

  const dirty = () => git(["status", "--porcelain"], { cwd: dir }).out
  if (message && dirty()) {
    git(["add", "-A"], { cwd: dir })
    git(["commit", "-m", message], { cwd: dir })
    console.log(`已在 ${submodule}/ 提交文档改动。`)
  }

  const fetched = git(["fetch", "origin", branch], { cwd: dir, check: false })
  if (!fetched.ok) {
    throw new Error("拉取文档仓库失败。连不上 GitHub 时先设置 HTTPS_PROXY 再运行。")
  }
  if (!git(["merge", "--ff-only", remote], { cwd: dir, check: false }).ok) {
    throw new Error(`${submodule}/ 的 ${branch} 与 ${remote} 已分叉，请在 ${submodule}/ 内手动 rebase 后重试。`)
  }

  const head = git(["rev-parse", "HEAD"], { cwd: dir }).out
  const recorded = git(["rev-parse", `HEAD:${submodule}`], { check: false }).out
  if (head === recorded) {
    console.log(`${submodule} 指针已是最新（${head.slice(0, 7)}）。`)
  } else {
    git(["add", "--", submodule])
    git(["commit", "-m", `文档：docs 子模块同步到 ${head.slice(0, 7)}`, "--", submodule])
    console.log(`已提交 ${submodule} 指针：${recorded.slice(0, 7) || "无"} → ${head.slice(0, 7)}。`)
  }

  if (dirty()) console.log(`${submodule}/ 还有未提交的改动；加 -m "说明" 重新运行即可一并提交。`)
  const ahead = Number(git(["rev-list", "--count", `${remote}..HEAD`], { cwd: dir }).out)
  if (ahead) {
    console.log(`${submodule}/ 有 ${ahead} 个提交尚未推送。推送 ${branch} 会发布文档站；推送本仓库前先推送它。`)
  }
}

try {
  main()
} catch (error) {
  console.error(error.message)
  process.exitCode = 1
}
