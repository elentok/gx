import { describe, expect, it } from "vitest"
import { detectPaths } from "./paths"

const summary = (text: string, wholeText = false) =>
  detectPaths(text, { wholeText }).map((c) => [c.path, c.line, text.slice(c.start, c.end)])

describe("detectPaths", () => {
  it.each([
    ["see /etc/hosts now", [["/etc/hosts", undefined, "/etc/hosts"]]],
    ["open ~/dev/gx/a.md", [["~/dev/gx/a.md", undefined, "~/dev/gx/a.md"]]],
    ["server/files/handler", [["server/files/handler", undefined, "server/files/handler"]]],
    ["edit README.md please", [["README.md", undefined, "README.md"]]],
    ["at handler.go:42 here", [["handler.go", 42, "handler.go:42"]]],
    ["at path/x.go:42:7", [["path/x.go", 42, "path/x.go:42:7"]]],
    ["see a/b.go.", [["a/b.go", undefined, "a/b.go"]]],
    ["see a/b.go,", [["a/b.go", undefined, "a/b.go"]]],
    ["(a/b.go)", [["a/b.go", undefined, "a/b.go"]]],
    ["did a/b.go:3?!", [["a/b.go", 3, "a/b.go:3"]]],
    [
      "Read(server/files/handler.go)",
      [["server/files/handler.go", undefined, "server/files/handler.go"]],
    ],
    [
      "Read: server/files/handler.go",
      [["server/files/handler.go", undefined, "server/files/handler.go"]],
    ],
    [
      '"a/b.go" and `c/d.go`',
      [
        ["a/b.go", undefined, "a/b.go"],
        ["c/d.go", undefined, "c/d.go"],
      ],
    ],
    ["https://example.com/a/b", []],
    ["see (https://example.com/a/b)", []],
    ["plain words only", []],
  ])("%s", (text, want) => {
    expect(summary(text)).toEqual(want)
  })

  it("skips fenced blocks", () => {
    const text = "before a/b.go\n```go\nc/d.go\n```\nafter e/f.go"
    expect(summary(text).map((c) => c[0])).toEqual(["a/b.go", "e/f.go"])
  })

  it("skips an unclosed fence to the end", () => {
    expect(summary("```\na/b.go")).toEqual([])
  })

  it("links a path with spaces only as a whole code span or link target", () => {
    const text = "~/dev/gx/my notes.md"
    expect(summary(text, true)).toEqual([[text, undefined, text]])
    expect(summary(text).map((c) => c[0])).not.toContain(text)
  })

  it("keeps false hits for the server check to remove", () => {
    expect(summary("e.g.").map((c) => c[0])).toEqual(["e.g"])
  })
})
