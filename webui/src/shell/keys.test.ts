import { afterEach, describe, expect, it, vi } from "vitest"
import { digitOf, isMod, isTextTarget } from "./keys"

afterEach(() => vi.restoreAllMocks())

describe("isTextTarget", () => {
  it.each(["input", "textarea", "select"])("is true for %s", (tag) => {
    expect(isTextTarget(document.createElement(tag))).toBe(true)
  })

  it("is true for a contenteditable element", () => {
    const el = document.createElement("div")
    Object.defineProperty(el, "isContentEditable", { value: true })
    expect(isTextTarget(el)).toBe(true)
  })

  it("is true inside a CodeMirror editor", () => {
    const editor = document.createElement("div")
    editor.className = "cm-editor"
    const content = document.createElement("div")
    editor.appendChild(content)
    expect(isTextTarget(content)).toBe(true)
  })

  it("is false for a button, plain div and null", () => {
    expect(isTextTarget(document.createElement("button"))).toBe(false)
    expect(isTextTarget(document.createElement("div"))).toBe(false)
    expect(isTextTarget(null)).toBe(false)
  })
})

describe("digitOf", () => {
  it("reads the digit from code when Option changed key", () => {
    expect(digitOf({ code: "Digit1" })).toBe(1)
  })
  it("covers 1 to 9 only", () => {
    expect(digitOf({ code: "Digit9" })).toBe(9)
    expect(digitOf({ code: "Digit0" })).toBeNull()
    expect(digitOf({ code: "KeyA" })).toBeNull()
  })
})

describe("isMod", () => {
  it("uses Cmd on Mac", () => {
    vi.spyOn(navigator, "platform", "get").mockReturnValue("MacIntel")
    expect(isMod({ metaKey: true, ctrlKey: false })).toBe(true)
    expect(isMod({ metaKey: false, ctrlKey: true })).toBe(false)
  })
  it("uses Ctrl elsewhere", () => {
    vi.spyOn(navigator, "platform", "get").mockReturnValue("Linux x86_64")
    expect(isMod({ metaKey: false, ctrlKey: true })).toBe(true)
    expect(isMod({ metaKey: true, ctrlKey: false })).toBe(false)
  })
})
