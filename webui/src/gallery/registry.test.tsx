// @vitest-environment node
// Importing vite.config pulls in esbuild, which breaks under jsdom.
/// <reference types="vite/client" />
import { existsSync } from "node:fs";
import { join } from "node:path";
import { expect, it } from "vitest";
import config from "../../vite.config";
import { TONES } from "../ui/tone";
import { galleryEntries } from "./registry";

// Every primitive is src/ui/<folder>/<Name>.tsx; tests and non-component files don't match.
const primitiveFiles = Object.keys(import.meta.glob("../ui/*/[A-Z]*.tsx")).filter((f) => !f.endsWith(".test.tsx"));
const primitiveNames = primitiveFiles.map((f) => f.split("/").pop()?.replace(".tsx", "") ?? f);

it("finds the primitives on disk", () => {
  expect(primitiveNames.length).toBeGreaterThan(0);
});

it("shows every exported primitive in the gallery", () => {
  const shown = galleryEntries.map((e) => e.name);
  for (const name of primitiveNames) expect(shown, name).toContain(name);
});

it("shows every primitive in every tone", () => {
  for (const name of primitiveNames) {
    const labels = galleryEntries.find((e) => e.name === name)?.cases.map((c) => c.label) ?? [];
    for (const tone of TONES) expect(labels, `${name} ${tone}`).toContain(tone);
  }
});

it("keeps gallery.html out of the production build input", () => {
  expect(existsSync(join(__dirname, "../../gallery.html"))).toBe(true);
  expect(config.build?.rollupOptions?.input).toBe("index.html");
});
