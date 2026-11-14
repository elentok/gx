import { describe, expect, it } from "vitest"
import { viewerHref, viewerLink } from "./viewerUrl"

describe("viewerHref", () => {
  it("encodes each segment and keeps the slashes", () => {
    expect(viewerHref("/home/me/my notes/a#b.md")).toBe("#/files/home/me/my%20notes/a%23b.md")
  })

  it("adds line and heading", () => {
    expect(viewerHref("/a.md", { line: 42 })).toBe("#/files/a.md?line=42")
    expect(viewerHref("/a.md", { line: 42, heading: "intro" })).toBe(
      "#/files/a.md?line=42&heading=intro",
    )
  })

  it("starts the query with heading when there is no line", () => {
    expect(viewerHref("/a.md", { heading: "intro" })).toBe("#/files/a.md?heading=intro")
  })
})

describe("viewerLink", () => {
  it("starts with the page origin", () => {
    expect(viewerLink("/a b.md", { line: 3 }, "http://host:1")).toBe(
      "http://host:1/#/files/a%20b.md?line=3",
    )
    expect(viewerLink("/a.md").startsWith(`${window.location.origin}/#/files/a.md`)).toBe(true)
  })
})
