import { describe, expect, it } from "vitest";
import { system, themeConfig } from "./theme";
import { KINDS, TONES } from "./tone";

const tokens = themeConfig.theme!.tokens!;

describe("theme", () => {
  it.each(TONES)("has fg and bg tokens for tone %s", (tone) => {
    const entry = (tokens.colors as Record<string, any>).tone[tone];
    expect(entry.fg.value).toBeTruthy();
    expect(entry.bg.value).toBeTruthy();
    expect(system.token(`colors.tone.${tone}.fg`)).toBeTruthy();
    expect(system.token(`colors.tone.${tone}.bg`)).toBeTruthy();
  });

  it.each(KINDS)("has fg and bg tokens for kind %s", (kind) => {
    expect(system.token(`colors.kind.${kind}.fg`)).toBeTruthy();
    expect(system.token(`colors.kind.${kind}.bg`)).toBeTruthy();
  });

  it("defines every token group", () => {
    for (const group of ["colors", "fonts", "fontSizes", "spacing", "radii", "sizes"] as const) {
      expect(Object.keys(tokens[group] ?? {}), group).not.toHaveLength(0);
    }
    const colors = tokens.colors as Record<string, any>;
    for (const name of ["surface", "text"]) expect(colors[name], name).toBeDefined();
expect(themeConfig.theme!.semanticTokens!.colors!.border).toBeDefined();
    expect(colors.text.muted).toBeDefined();
    expect(colors.text.dim).toBeDefined();
  });

  it("has font size tokens for the Design page text", () => {
    expect(system.token("fontSizes.base")).toBe("16px");
    expect(system.token("fontSizes.subtitle")).toBe("18px");
    expect(system.token("fontSizes.title")).toBe("24px");
  });

  it("uses the condensed and text font families with a 6px bar", () => {
    expect(system.token("fonts.condensed")).toContain("Saira Condensed");
    expect(system.token("fonts.text")).toContain("Saira");
    expect(system.token("sizes.bar")).toBe("6px");
  });
});
