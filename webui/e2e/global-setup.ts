import { execFileSync } from "node:child_process";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");

// Builds the binaries once so each flow's gxfixture starts without compiling.
// The workers inherit the paths through the environment.
export default function globalSetup() {
  const dir = mkdtempSync(join(tmpdir(), "gx-e2e-bin-"));
  const build = (out: string, pkg: string) => {
    const path = join(dir, out);
    execFileSync("go", ["build", "-o", path, pkg], { cwd: repoRoot, stdio: "inherit" });
    return path;
  };
  process.env.GX_E2E_GX = build("gx", "github.com/elentok/gx");
  process.env.GX_E2E_FAKECLAUDE = build("claude", "github.com/elentok/gx/testutil/agentfake/cmd/fakeclaude");
  process.env.GX_E2E_GXFIXTURE = build("gxfixture", "github.com/elentok/gx/testutil/gxfixture");
}
