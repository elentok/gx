import { expect, test } from "vitest"
import { languageOf } from "./language"

test.each([
  ["/a/handler.go", "go"],
  ["/a/x.ts", "typescript"],
  ["/a/x.tsx", "typescript"],
  ["/a/x.py", "python"],
  ["/a/x.sh", "bash"],
  ["/a/x.json", "json"],
  ["/a/x.yaml", "yaml"],
  ["/a/x.css", "css"],
  ["/a/X.GO", "go"],
])("%s maps to %s", (path, language) => {
  expect(languageOf(path)).toBe(language)
})

test.each(["/a/x.unknownext", "/a/Makefile", "/a.d/Makefile", "/a/.gitignore"])(
  "%s maps to none",
  (path) => {
    expect(languageOf(path)).toBeUndefined()
  },
)
