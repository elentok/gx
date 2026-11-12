// @vitest-environment node
import { describe, expect, it } from "vitest"
import { selectIntent } from "./selectKeys"

describe("selectIntent", () => {
  it.each([
    ["s", false, "enter-select"],
    ["s", true, "none"],
    [" ", false, "none"],
    [" ", true, "toggle"],
    ["Enter", false, "open"],
    ["Enter", true, "toggle"],
    ["Escape", false, "none"],
    ["Escape", true, "leave"],
    ["x", true, "none"],
  ] as const)("%j with selecting=%s gives %s", (key, selecting, want) => {
    expect(selectIntent({ key }, selecting, false)).toBe(want)
  })

  it("ignores every key while a text field, menu or dialog has focus", () => {
    for (const key of ["s", " ", "Enter", "Escape"]) {
      for (const selecting of [false, true])
        expect(selectIntent({ key }, selecting, true)).toBe("none")
    }
  })

  it("ignores chords", () => {
    expect(selectIntent({ key: "s", metaKey: true }, false, false)).toBe("none")
    expect(selectIntent({ key: "s", altKey: true }, false, false)).toBe("none")
  })
})
