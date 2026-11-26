import { createRef } from "react"
import { fireEvent, screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"
import { renderThemed } from "../render"
import { TextBox, type TextBoxProps } from "./TextBox"

const base = { label: "Note", name: "note" } as const

const element = () => screen.getByLabelText("Note")

// jsdom skips @layer and var() shorthands in computed styles, so read the emitted rules.
const cssOf = (el: Element) => {
  const rules = [...document.querySelectorAll("style")].map((s) => s.textContent).join("\n")
  return rules
    .split("\n")
    .filter((line) => [...el.classList].some((c) => line.includes(`.${c}{`)))
    .join("\n")
}

type Row = [
  string,
  TextBoxProps,
  { tag: string; type: string | null; autocomplete: string; typingHelp: boolean },
]

const plain = { tag: "INPUT", type: "text", autocomplete: "off", typingHelp: true }
const credential = (
  autoComplete: "username" | "current-password" | "new-password" | "one-time-code",
  type: string,
): Row => [
  `credential ${autoComplete}`,
  { ...base, kind: "credential", autoComplete },
  { tag: "INPUT", type, autocomplete: autoComplete, typingHelp: false },
]

const rows: Row[] = [
  [
    "prose",
    { ...base, kind: "prose" },
    { tag: "TEXTAREA", type: null, autocomplete: "off", typingHelp: true },
  ],
  ["line", { ...base, kind: "line" }, plain],
  ["line verbatim", { ...base, kind: "line", verbatim: true }, { ...plain, typingHelp: false }],
  [
    "line secret",
    { ...base, kind: "line", secret: true },
    { ...plain, type: "password", typingHelp: false },
  ],
  ["search", { ...base, kind: "search" }, { ...plain, type: "search", typingHelp: false }],
  credential("username", "text"),
  credential("current-password", "password"),
  credential("new-password", "password"),
  credential("one-time-code", "text"),
]

describe("TextBox kinds", () => {
  it.each(rows)("%s sets its element and attributes", (_, props, want) => {
    renderThemed(<TextBox {...props} />)
    const box = element()
    expect(box.tagName).toBe(want.tag)
    expect(box.getAttribute("type")).toBe(want.type)
    expect(box.getAttribute("autocomplete")).toBe(want.autocomplete)
    expect(box.getAttribute("autocapitalize")).toBe(want.typingHelp ? null : "off")
    expect(box.getAttribute("autocorrect")).toBe(want.typingHelp ? null : "off")
    expect(box.getAttribute("spellcheck")).toBe(want.typingHelp ? null : "false")
    expect(box.getAttribute("enterkeyhint")).toBe(props.kind === "search" ? "search" : null)
    expect(box.getAttribute("data-focus-ring")).toBe("none")
    expect(box.getAttribute("name")).toBe("note")
    expect(box.getAttribute("aria-label")).toBeNull()
    expect(box.getAttribute("aria-invalid")).toBeNull()
  })
})

describe("TextBox label", () => {
  it("links a real label to an id of its own", () => {
    renderThemed(
      <>
        <TextBox {...base} kind="line" />
        <TextBox kind="line" label="Other note" name="other" />
      </>,
    )
    const first = screen.getByRole("textbox", { name: "Note" })
    const second = screen.getByRole("textbox", { name: "Other note" })
    expect(first.id).not.toBe("")
    expect(second.id).not.toBe(first.id)
    expect(screen.getByText("Note").closest("label")?.htmlFor).toBe(first.id)
  })

  it("hides the label from sight only", () => {
    renderThemed(<TextBox {...base} kind="search" hideLabel />)
    expect(screen.getByRole("searchbox", { name: "Note" })).toBeTruthy()
    const label = screen.getByText("Note")
    expect(label.closest("[aria-hidden]")).toBeNull()
    const css = cssOf(label.parentElement as HTMLElement)
    expect(css).toContain("position:absolute")
    expect(css).not.toContain("display:none")
  })

  it("keeps the optional marker out of the accessible name", () => {
    renderThemed(<TextBox {...base} kind="line" optional />)
    expect(screen.getByRole("textbox", { name: "Note" })).toBeTruthy()
    expect(screen.getByText("· optional").getAttribute("aria-hidden")).toBe("true")
    expect(screen.getByText("· optional").closest("label")).not.toBeNull()
  })
})

describe("TextBox hint and invalid", () => {
  it("draws the hint outside the label and does not link it", () => {
    renderThemed(<TextBox {...base} kind="line" hint="Keep it short" />)
    expect(screen.getByText("Keep it short").closest("label")).toBeNull()
    expect(element().getAttribute("aria-describedby")).toBeNull()
  })

  it("links the hint on a credential box", () => {
    renderThemed(
      <TextBox {...base} kind="credential" autoComplete="new-password" hint="Keep it short" />,
    )
    const hint = screen.getByText("Keep it short")
    expect(hint.closest("label")).toBeNull()
    expect(element().getAttribute("aria-describedby")).toBe(hint.id)
  })

  it("merges the hint with the caller's description", () => {
    renderThemed(
      <TextBox
        {...base}
        kind="credential"
        autoComplete="username"
        hint="Keep it short"
        aria-describedby="outside"
      />,
    )
    const hint = screen.getByText("Keep it short")
    expect(element().getAttribute("aria-describedby")).toBe(`outside ${hint.id}`)
  })

  it("keeps the caller's description on other kinds", () => {
    renderThemed(<TextBox {...base} kind="prose" hint="Short" aria-describedby="outside" />)
    expect(element().getAttribute("aria-describedby")).toBe("outside")
  })

  it("marks an invalid box", () => {
    renderThemed(
      <>
        <TextBox {...base} kind="line" invalid />
        <TextBox kind="line" label="Fine" name="fine" />
      </>,
    )
    expect(element().getAttribute("aria-invalid")).toBe("true")
    const red = "border-color:var(--chakra-colors-tone-deadlocked-fg)"
    expect(cssOf(element())).toContain(red)
    expect(cssOf(screen.getByLabelText("Fine"))).not.toContain(red)
  })
})

describe("TextBox pass-through", () => {
  it("passes the native props to an input", () => {
    const ref = createRef<HTMLInputElement>()
    const onChange = vi.fn()
    const onFocus = vi.fn()
    renderThemed(
      <TextBox
        {...base}
        kind="line"
        ref={ref}
        defaultValue="start"
        placeholder="Type here"
        required
        minLength={3}
        autoFocus
        role="combobox"
        list="choices"
        inputMode="numeric"
        data-mock-part="box"
        aria-expanded={false}
        onChange={onChange}
        onFocus={onFocus}
      />,
    )
    const box = screen.getByRole<HTMLInputElement>("combobox", { name: "Note" })
    expect(ref.current).toBe(box)
    expect(document.activeElement).toBe(box)
    expect(onFocus).toHaveBeenCalled()
    expect(box.value).toBe("start")
    expect(box.placeholder).toBe("Type here")
    expect(box.required).toBe(true)
    expect(box.minLength).toBe(3)
    expect(box.getAttribute("list")).toBe("choices")
    expect(box.getAttribute("inputmode")).toBe("numeric")
    expect(box.getAttribute("data-mock-part")).toBe("box")
    expect(box.getAttribute("aria-expanded")).toBe("false")
    fireEvent.change(box, { target: { value: "next" } })
    expect(onChange).toHaveBeenCalled()
  })

  it("passes the native props to a textarea", () => {
    const ref = createRef<HTMLTextAreaElement>()
    renderThemed(<TextBox {...base} kind="prose" ref={ref} rows={4} value="text" disabled />)
    const box = element() as HTMLTextAreaElement
    expect(ref.current).toBe(box)
    expect(box.rows).toBe(4)
    expect(box.value).toBe("text")
    expect(box.disabled).toBe(true)
  })
})

describe("TextBox Escape", () => {
  it("prevents the default on a search box after the caller's handler", () => {
    const seen: boolean[] = []
    const above = vi.fn()
    renderThemed(
      <TextBox {...base} kind="search" onKeyDown={(e) => seen.push(e.defaultPrevented)} />,
    )
    document.addEventListener("keydown", above)
    expect(fireEvent.keyDown(element(), { key: "Escape" })).toBe(false)
    document.removeEventListener("keydown", above)
    expect(seen).toEqual([false])
    expect(above).toHaveBeenCalled()
    expect(fireEvent.keyDown(element(), { key: "Enter" })).toBe(true)
  })

  it("prevents Escape's default on a search box with no handler", () => {
    renderThemed(<TextBox {...base} kind="search" />)
    expect(fireEvent.keyDown(element(), { key: "Escape" })).toBe(false)
  })

  it.each(rows.filter(([, props]) => props.kind !== "search"))("leaves %s alone", (_, props) => {
    renderThemed(<TextBox {...props} />)
    expect(fireEvent.keyDown(element(), { key: "Escape" })).toBe(true)
  })
})
