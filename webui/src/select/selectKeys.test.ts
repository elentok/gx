// @vitest-environment node
import { describe, expect, it } from "vitest"
import { dispatchSelectKey, selectIntent, type SelectKeyHandlers } from "./selectKeys"

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

describe("dispatchSelectKey", () => {
  const run = (key: string, selecting: boolean, over: Partial<SelectKeyHandlers> = {}) => {
    const calls: string[] = []
    const e = {
      key,
      preventDefault: () => calls.push("prevent"),
      stopPropagation: () => calls.push("stop"),
    }
    dispatchSelectKey(e, selecting, false, {
      enter: () => void calls.push("enter"),
      itemKey: "a",
      toggle: (k) => calls.push(`toggle:${k}`),
      open: (k) => calls.push(`open:${k}`),
      ...over,
    })
    return calls
  }

  it("enters select mode on s", () => {
    expect(run("s", false)).toEqual(["enter", "prevent"])
  })

  it("leaves s alone when enter reports nothing to select", () => {
    expect(run("s", false, { enter: () => false })).toEqual([])
  })

  it("toggles in select mode and opens outside it, claiming the event", () => {
    expect(run("Enter", true)).toEqual(["prevent", "stop", "toggle:a"])
    expect(run("Enter", false)).toEqual(["prevent", "stop", "open:a"])
  })

  it("does nothing without a focused item, or on open without an open handler", () => {
    expect(run("Enter", true, { itemKey: null })).toEqual([])
    expect(run("Enter", false, { open: undefined })).toEqual([])
  })
})
