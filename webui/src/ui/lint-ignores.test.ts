// @vitest-environment node
import { execFileSync } from "node:child_process"
import { existsSync, readFileSync } from "node:fs"
import { join } from "node:path"
import { describe, expect, it } from "vitest"

const webuiDir = join(__dirname, "../..")

// The category list ends at the reason's colon.
const ignoreComment = /biome-ignore(?:-all|-start)?[ \t]+([^:\n]*)/g

/** The comments that would silence a lint plugin. */
function silencers(source: string): string[] {
  return [...source.matchAll(ignoreComment)]
    .filter((m) => m[1].split(/\s+/).some((c) => /^lint(?:\/plugin(?:\/\S*)?)?(?:\(|$)/.test(c)))
    .map((m) => m[0].trim())
}

/** The scripts Biome lints: what git does not ignore, less the `files.includes` exclusions. */
function lintedSources(): string[] {
  const config = JSON.parse(
    readFileSync(join(webuiDir, "biome.jsonc"), "utf8").replace(/^\s*\/\/.*$/gm, ""),
  )
  const excluded = (config.files.includes as string[])
    .filter((glob) => glob.startsWith("!**/"))
    .map((glob) => glob.slice(4).replaceAll(".", "\\.").replaceAll("*", "[^/]*"))
    .map((pattern) => new RegExp(`(^|/)${pattern}(/|$)`))
  return execFileSync("git", ["ls-files", "-z", "--cached", "--others", "--exclude-standard"], {
    cwd: webuiDir,
    encoding: "utf8",
  })
    .split("\0")
    .filter((p) => /\.[cm]?[jt]sx?$/.test(p) && !excluded.some((re) => re.test(p)))
    .filter((p) => existsSync(join(webuiDir, p)))
}

// Built from parts, so the scan does not report this file's examples.
const ignore = ["biome", "ignore"].join("-")
const forms = [ignore, `${ignore}-all`, `${ignore}-start`]

describe("lint ignore comment guard", () => {
  it("finds no comment that silences a lint plugin", () => {
    const offenders = lintedSources().flatMap((p) =>
      silencers(readFileSync(join(webuiDir, p), "utf8")).map((c) => `${p}: ${c}`),
    )
    expect(offenders).toEqual([])
  })

  it("scans the sources and the e2e flows", () => {
    expect(lintedSources()).toEqual(
      expect.arrayContaining(["src/ui/tone.ts", "e2e/fixture.ts", "vite.config.ts"]),
    )
  })

  const silencing = forms.flatMap((form) =>
    ["lint/plugin", "lint/plugin/raw-text-box", "lint", "lint/a11y/noAutofocus lint/plugin"].map(
      (category) => `// ${form} ${category}: reason`,
    ),
  )

  it.each(silencing)("fails on %s", (comment) => {
    expect(silencers(`const a = 1\n${comment}\n`)).toHaveLength(1)
    expect(silencers(`{/* ${comment.slice(3)} */}`)).toHaveLength(1)
  })

  it.each(
    forms.flatMap((form) =>
      ["lint/a11y/useSemanticElements", "lint/correctness/useExhaustiveDependencies"].map(
        (category) => `// ${form} ${category}: a lint/plugin reason`,
      ),
    ),
  )("allows %s", (comment) => {
    expect(silencers(comment)).toEqual([])
  })
})
