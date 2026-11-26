import { describe, expect, it } from "vitest"
import { system, themeConfig } from "./theme"
import { KINDS, TONES } from "./tone"

const tokens = themeConfig.theme!.tokens!
// The token trees are walked by path, so tests read them as JSON.parse's untyped result.
const untyped = (v: unknown): ReturnType<typeof JSON.parse> => v

describe("theme", () => {
  it("defines the surface.hover token used by task rows", () => {
    expect(system.token("colors.surface.hover")).toBeTruthy()
  })

  it.each(TONES)("has fg and bg tokens for tone %s", (tone) => {
    const entry = untyped(tokens.colors).tone[tone]
    expect(entry.fg.value).toBeTruthy()
    expect(entry.bg.value).toBeTruthy()
    expect(system.token(`colors.tone.${tone}.fg`)).toBeTruthy()
    expect(system.token(`colors.tone.${tone}.bg`)).toBeTruthy()
  })

  it("defines one global focus-visible ring plus the inset sidebar form", () => {
    const css = untyped(themeConfig.globalCss)
    expect(css[":focus-visible"].boxShadow).toContain("{colors.tone.running.fg}")
    expect(css[":focus-visible"].outline).toContain("none")
    expect(css["[data-focus-ring='inset']:focus-visible"].boxShadow).toContain("inset 0 0 0 2px")
  })

  it("lets a title box keep 20px on a coarse pointer", () => {
    const css = untyped(themeConfig.globalCss)
    const forced = Object.keys(css).filter((k) => css[k]["@media (pointer: coarse)"]?.fontSize)
    expect(forced).toEqual(["input:not([data-text-box-title]), textarea, select"])
  })

  it("draws a text box's inset ring over the rule that turns the global ring off", () => {
    const css = untyped(themeConfig.globalCss)
    const rule = css["[data-focus-ring='none'][data-text-box-ring]:focus-visible"]
    expect(rule.boxShadow).toBe("inset 0 0 0 2px {colors.tone.running.fg} !important")
  })

  it.each(KINDS)("has fg and bg tokens for kind %s", (kind) => {
    expect(system.token(`colors.kind.${kind}.fg`)).toBeTruthy()
    expect(system.token(`colors.kind.${kind}.bg`)).toBeTruthy()
  })

  it.each([
    ["claude", "#6cb4ff"],
    ["codex", "#b48cff"],
    ["blue", "#6cb4ff"],
    ["purple", "#b48cff"],
    ["amber", "#e8c25a"],
    ["green", "#5fd39a"],
    ["muted", "#6b7584"],
  ])("has chart color %s", (name, value) => {
    expect(system.token(`colors.chart.${name}`)).toBe(value)
  })

  it("defines every token group", () => {
    for (const group of ["colors", "fonts", "fontSizes", "spacing", "radii", "sizes"] as const) {
      expect(Object.keys(tokens[group] ?? {}), group).not.toHaveLength(0)
    }
    const colors = untyped(tokens.colors)
    for (const name of ["surface", "text"]) expect(colors[name], name).toBeDefined()
    expect(themeConfig.theme!.semanticTokens!.colors!.border).toBeDefined()
    expect(colors.text.muted).toBeDefined()
    expect(colors.text.dim).toBeDefined()
  })

  it("has font size tokens for the Design page text", () => {
    expect(system.token("fontSizes.base")).toBe("16px")
    expect(system.token("fontSizes.subtitle")).toBe("18px")
    expect(system.token("fontSizes.title")).toBe("24px")
  })

  it("uses the condensed and text font families with a 6px bar", () => {
    expect(system.token("fonts.condensed")).toContain("Saira Condensed")
    expect(system.token("fonts.text")).toContain("Saira")
    expect(system.token("sizes.bar")).toBe("6px")
  })
})
