import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");

// Builds the web UI and the binaries once so each flow's gxfixture starts
// without compiling. The workers inherit the paths through the environment.
export default function globalSetup() {
  // The gx binary embeds webui/dist, so a stale or empty dist only shows up as
  // a locator timeout. Vite empties dist, which removes the committed .gitkeep.
  const webuiDir = resolve(repoRoot, "webui");
  execFileSync("pnpm", ["build"], { cwd: webuiDir, stdio: "inherit" });
  const keep = join(webuiDir, "dist", ".gitkeep");
  if (!existsSync(keep)) writeFileSync(keep, "");

  // One fixed dir per worktree, overwritten each run, so nothing leaks into $TMPDIR.
  const dir = join(webuiDir, ".e2e-bin");
  mkdirSync(dir, { recursive: true });
  const build = (out: string, pkg: string) => {
    const path = join(dir, out);
    // Without cgo fsnotify watches with kqueue: the cgo FSEvents watcher misses events on some test machines.
    execFileSync("go", ["build", "-o", path, pkg], { cwd: repoRoot, stdio: "inherit", env: { ...process.env, CGO_ENABLED: "0" } });
    return path;
  };
  process.env.GX_E2E_GX = build("gx", "github.com/elentok/gx");
  process.env.GX_E2E_FAKECLAUDE = build("claude", "github.com/elentok/gx/testutil/agentfake/cmd/fakeclaude");
  process.env.GX_E2E_GXFIXTURE = build("gxfixture", "github.com/elentok/gx/testutil/gxfixture");
}
