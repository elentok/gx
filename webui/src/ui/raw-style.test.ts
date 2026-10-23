import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const srcDir = join(__dirname, "..");
// theme.ts is the one place raw colors and font names belong.
const allowed = new Set(["ui/theme.ts"]);

const rawHex = /#[0-9a-fA-F]{3,8}\b/;
const fontTokens = new Set(["condensed", "text", "mono", "inherit"]);
const fontFamilyValue = /font-?family\s*[:=]?\s*\{?\s*["'`]([^"'`]*)/gi;
// Chakra's built-in bg.* tokens are light-mode colors; this dark-only theme defines its own surfaces.
const chakraBgToken = /["'`]bg\.[a-z]/;

function violations(source: string): string[] {
  const found: string[] = [];
  if (rawHex.test(source)) found.push("raw hex color");
  if (chakraBgToken.test(source)) found.push("chakra bg token");
  for (const m of source.matchAll(fontFamilyValue)) {
    if (!fontTokens.has(m[1])) found.push("font-family literal");
  }
  return found;
}

function componentSources(): string[] {
  return readdirSync(srcDir, { recursive: true, encoding: "utf8" })
    .map((p) => p.split("\\").join("/"))
    .filter((p) => /\.tsx?$/.test(p) && !/\.test\.tsx?$/.test(p) && !allowed.has(p));
}

describe("raw style guard", () => {
  it("finds no raw colors or font-family literals in component sources", () => {
    const offenders = componentSources().flatMap((p) =>
      violations(readFileSync(join(srcDir, p), "utf8")).map((v) => `${p}: ${v}`),
    );
    expect(offenders).toEqual([]);
  });

  it("scans the ui sources", () => {
    expect(componentSources()).toContain("ui/tone.ts");
  });

  it("fails on a planted raw hex color", () => {
    expect(violations('<Box bg="#ff00aa" />')).toEqual(["raw hex color"]);
    expect(violations("const c = '#fff';")).toEqual(["raw hex color"]);
  });

  it("fails on a planted Chakra bg token", () => {
    expect(violations('<Box bg="bg.subtle" />')).toEqual(["chakra bg token"]);
  });

  it("fails on a planted font-family literal", () => {
    expect(violations('<Box style={{ fontFamily: "Comic Sans" }} />')).toEqual(["font-family literal"]);
    expect(violations("const css = `font-family: 'Arial'`;")).toEqual(["font-family literal"]);
    expect(violations('<Box fontFamily="Arial" />')).toEqual(["font-family literal"]);
  });

  it("accepts token references", () => {
    expect(violations('<Box bg="surface" fontFamily="condensed" />')).toEqual([]);
  });
});
