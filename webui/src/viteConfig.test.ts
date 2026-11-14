// @vitest-environment node
import { dirname, resolve } from "node:path"
import { fileURLToPath } from "node:url"
import { describe, expect, it } from "vitest"
import config from "../vite.config"

describe("vite config", () => {
  it("keeps /@fs/ reads inside webui/", () => {
    const webuiDir = resolve(dirname(fileURLToPath(import.meta.url)), "..")
    expect(config.server?.fs?.strict).toBe(true)
    expect(config.server?.fs?.allow).toEqual([webuiDir])
  })

  it("defines __WEB_VERSION__ as a JSON string, dev by default", () => {
    const value = config.define?.__WEB_VERSION__
    expect(typeof value).toBe("string")
    expect(JSON.parse(value as string)).toBe(process.env.GX_WEB_VERSION || "dev")
  })
})
