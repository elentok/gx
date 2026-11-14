import { expect, test } from "vitest"
import { rawUrl } from "./rawUrl"

test("encodes each segment and keeps the slashes", () => {
  expect(rawUrl("/Users/me/my notes.md")).toBe("/v1/files/Users/me/my%20notes.md")
  expect(rawUrl("/tmp/100%.md")).toBe("/v1/files/tmp/100%25.md")
  expect(rawUrl("/tmp/a?b#c.md")).toBe("/v1/files/tmp/a%3Fb%23c.md")
})

test("download adds ?download=1", () => {
  expect(rawUrl("/tmp/a b.txt", { download: true })).toBe("/v1/files/tmp/a%20b.txt?download=1")
})
