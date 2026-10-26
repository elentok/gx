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

it("shows the Button in every role, tone and size", () => {
  const labels = galleryEntries.find((e) => e.name === "Button")?.cases.map((c) => c.label) ?? [];
  for (const role of ["main", "secondary", "ghost"]) {
    for (const tone of TONES) {
      for (const size of ["sm", "md"]) expect(labels, `${role} ${tone} ${size}`).toContain(`${role} ${tone} ${size}`);
    }
  }
  expect(labels).toContain("disabled");
  for (const old of ["solid", "tinted", "outline"]) expect(labels).not.toContain(old);
});

it("shows the TopBar in every state the spec lists", () => {
  const labels = galleryEntries.find((e) => e.name === "TopBar")?.cases.map((c) => c.label) ?? [];
  for (const label of ["Epics active", "Stats active", "none active", "Paused", "Offline", "no build string", "menu closed", "menu open, AFK off", "menu open, AFK on"]) {
    expect(labels, label).toContain(label);
  }
});

it("shows the EpicCard in every state the spec lists", () => {
  const labels = galleryEntries.find((e) => e.name === "EpicCard")?.cases.map((c) => c.label) ?? [];
  for (const label of [
    "running",
    "queued",
    "merging",
    "finished",
    "draft",
    "ready with Queue",
    "Needs you with buttons",
    "merge blocked",
    "design not started",
    "joined card",
    "one-off",
    "follow-ups",
    "two in a row, one footer",
  ]) {
    expect(labels, label).toContain(label);
  }
});

it("shows the epic page in every state the spec lists", () => {
  const labels = galleryEntries.find((e) => e.name === "EpicPage")?.cases.map((c) => c.label) ?? [];
  for (const label of ["running", "finished", "Design", "draft -impl", "merge-blocked", "empty", "needs-repair", "needs-repair dialog"]) {
    expect(labels, label).toContain(label);
  }
});

it("shows the jump menu rows in every case the spec lists", () => {
  const labels = galleryEntries.find((e) => e.name === "JumpRow")?.cases.map((c) => c.label) ?? [];
  for (const label of ["design", "implementation", "one-off", "follow-ups", "Needs you", "current", "cursor", "finished"]) {
    expect(labels, label).toContain(label);
  }
});

it("keeps gallery.html out of the production build input", () => {
  expect(existsSync(join(__dirname, "../../gallery.html"))).toBe(true);
  expect(config.build?.rollupOptions?.input).toBe("index.html");
});
