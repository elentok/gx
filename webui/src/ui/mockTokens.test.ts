import { readFileSync } from "node:fs"
import { resolve } from "node:path"
import { describe, expect, it } from "vitest"
import { baseCss, outDir, tokensCss } from "../../scripts/mock-tokens"

const committed = (name: string): string =>
  readFileSync(resolve(__dirname, "../..", outDir, name), "utf8")

describe("mock token files", () => {
  it("tokens.css matches a fresh run (run `pnpm run mock:tokens` in webui)", () => {
    expect(committed("tokens.css")).toBe(tokensCss())
  })

  it("base.css matches a fresh run (run `pnpm run mock:tokens` in webui)", () => {
    expect(committed("base.css")).toBe(baseCss())
  })

  it("tokens.css carries the layer order, light, dark and base blocks", () => {
    const css = tokensCss()
    expect(css.startsWith("@layer reset, base, tokens, recipes;")).toBe(true)
    expect(css).toMatch(/^\s+:root, \.light \{/m)
    expect(css).toMatch(/^\s+\.dark, \.dark \.chakra-theme:not\(\.light\) \{/m)
    expect(css).toMatch(/^\s+:where\(html, \.chakra-theme\) \{/m)
  })

  it("base.css carries the body line height, font and background", () => {
    const css = baseCss()
    expect(css).toContain("line-height: 1.5")
    expect(css).toContain("font-family: var(--chakra-fonts-text)")
    expect(css).toContain("background: var(--chakra-colors-surface)")
  })
})
