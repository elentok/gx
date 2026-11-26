// @vitest-environment node
import { spawnSync } from "node:child_process"
import { cpSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { dirname, join } from "node:path"
import { afterAll, beforeAll, describe, expect, it } from "vitest"

const webuiDir = join(__dirname, "../..")
const biome = join(webuiDir, "node_modules/.bin/biome")
const fixture = readFileSync(join(__dirname, "raw-text-box.fixture.tsx.txt"), "utf8")

interface Example {
  source: string
  first: number
  last: number
}

/** The fixture's `const bad…` and `const ok…` blocks, with the lines each one covers. */
function examples(prefix: "bad" | "ok"): Example[] {
  const lines = fixture.trimEnd().split("\n")
  const starts = lines.flatMap((line, i) => (line.startsWith("const ") ? [i] : []))
  return starts.flatMap((start, n) => {
    const block = lines
      .slice(start, starts[n + 1])
      .join("\n")
      .trimEnd()
    if (!block.startsWith(`const ${prefix}`)) return []
    return [{ source: block, first: start + 1, last: start + block.split("\n").length }]
  })
}

// A raw box in each kind of place, linted under the real config.
const placed = {
  "src/ui/text-box/Box.tsx": false,
  "src/shell/Host.test.tsx": false,
  "src/shell/Host.tsx": true,
}

let dir = ""
/** The plugin's hits, as `file:line`. */
let hits: { file: string; line: number }[] = []

beforeAll(() => {
  dir = mkdtempSync(join(tmpdir(), "raw-text-box-"))
  cpSync(join(webuiDir, "biome-plugins"), join(dir, "biome-plugins"), { recursive: true })
  // The real config, without the parts that need its own folder.
  const config = JSON.parse(
    readFileSync(join(webuiDir, "biome.jsonc"), "utf8").replace(/^\s*\/\/.*$/gm, ""),
  )
  delete config.$schema
  delete config.vcs
  writeFileSync(join(dir, "biome.json"), JSON.stringify(config))
  const files: Record<string, string> = { "src/examples.tsx": fixture }
  for (const path of Object.keys(placed)) files[path] = "export const box = <textarea />\n"
  for (const [path, source] of Object.entries(files)) {
    mkdirSync(dirname(join(dir, path)), { recursive: true })
    writeFileSync(join(dir, path), source)
  }
  const run = spawnSync(biome, ["lint", "--reporter=github", "--max-diagnostics=500", "."], {
    cwd: dir,
    encoding: "utf8",
  })
  hits = [...run.stdout.matchAll(/^::error title=plugin,file=([^,]+),line=(\d+)/gm)].map((m) => ({
    file: m[1].split("\\").join("/"),
    line: Number(m[2]),
  }))
})

afterAll(() => rmSync(dir, { recursive: true, force: true }))

function hitsOn(file: string, first = 1, last = Number.MAX_SAFE_INTEGER) {
  return hits.filter((h) => h.file.endsWith(file) && h.line >= first && h.line <= last)
}

describe("raw text box lint plugin", () => {
  it.each(examples("bad"))("reports $source", ({ first, last }) => {
    expect(hitsOn("src/examples.tsx", first, last)).toHaveLength(1)
  })

  it.each(examples("ok"))("allows $source", ({ first, last }) => {
    expect(hitsOn("src/examples.tsx", first, last)).toEqual([])
  })

  it.each(Object.entries(placed))("under the real config, %s fails: %s", (path, fails) => {
    expect(hitsOn(path).length > 0).toBe(fails)
  })
})
