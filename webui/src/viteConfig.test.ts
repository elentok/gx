// @vitest-environment node
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import config from "../vite.config";

describe("vite config", () => {
  it("keeps /@fs/ reads inside webui/", () => {
    const webuiDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
    expect(config.server?.fs?.strict).toBe(true);
    expect(config.server?.fs?.allow).toEqual([webuiDir]);
  });
});
