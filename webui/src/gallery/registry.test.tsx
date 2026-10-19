// @vitest-environment node
// Importing vite.config pulls in esbuild, which breaks under jsdom.
import { existsSync } from "node:fs";
import { join } from "node:path";
import { expect, it } from "vitest";
import config from "../../vite.config";
import { TONES } from "../ui/tone";
import { galleryEntries } from "./registry";

it("shows every primitive in every tone", () => {
  for (const name of ["Badge", "ProgressBar", "TicketLine", "Card"]) {
    const labels = galleryEntries.find((e) => e.name === name)?.cases.map((c) => c.label) ?? [];
    for (const tone of TONES) expect(labels, `${name} ${tone}`).toContain(tone);
  }
});

it("keeps gallery.html out of the production build input", () => {
  expect(existsSync(join(__dirname, "../../gallery.html"))).toBe(true);
  expect(config.build?.rollupOptions?.input).toBe("index.html");
});
