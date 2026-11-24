import { cleanup, fireEvent, screen } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { chatEntity } from "../actions/entity"
import { menuActions } from "../actions/registry"
import { session } from "../agents/agentsTestKit"
import { setBigPrompt } from "../ui/expandable-text/bigPromptStore"
import { hasLayer } from "../ui/modal/layers"
import { renderThemed } from "../ui/render"
import { ChordBox } from "./ChordBox"
import { chordTable, type Chord } from "./chords"

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

const chord = (key: string, label: string, over: Partial<Chord> = {}): Chord => ({
  key,
  label: () => label,
  run: () => {},
  available: () => true,
  ...over,
})

const ctx = { path: "/" }

test("shows the leader chip and one row per available chord", () => {
  vi.spyOn(navigator, "platform", "get").mockReturnValue("MacIntel")
  const table = [chord("a", "Alpha"), chord("b", "Beta", { available: () => false })]
  renderThemed(<ChordBox leader="cmd+e" ctx={ctx} chords={table} onClose={() => {}} />)
  expect(screen.getByRole("menu").textContent).toContain("⌘E")
  const items = screen.getAllByRole("menuitem")
  expect(items.map((i) => i.textContent)).toEqual(["aAlpha"])
  expect(screen.getByText("esc closes")).toBeTruthy()
})

test("the danger row is present only when available", () => {
  const rm = (available: boolean) => [
    chord("x", "Remove chat", { danger: true, available: () => available }),
  ]
  renderThemed(<ChordBox leader="cmd+e" ctx={ctx} chords={rm(false)} onClose={() => {}} />)
  expect(screen.queryByText("Remove chat")).toBeNull()
  cleanup()
  renderThemed(<ChordBox leader="cmd+e" ctx={ctx} chords={rm(true)} onClose={() => {}} />)
  expect(screen.getByText("Remove chat")).toBeTruthy()
})

test("a row click runs its action, then closes", () => {
  const calls: string[] = []
  const table = [chord("a", "Alpha", { run: () => calls.push("run") })]
  renderThemed(
    <ChordBox leader="cmd+e" ctx={ctx} chords={table} onClose={() => calls.push("close")} />,
  )
  fireEvent.click(screen.getByRole("menuitem"))
  expect(calls).toEqual(["run", "close"])
})

const noop = () => {}
const chatActions = menuActions(chatEntity(session("s-a", "gamma"))).map((d) => ({
  id: d.id,
  label: d.label,
  chord: d.chord,
  danger: !!d.danger,
  disabled: false,
  run: noop,
}))
const realTable = () => chordTable({ actions: chatActions, history: noop })

test("lists the real chord table: the entity group, a separator, then the app group", () => {
  renderThemed(<ChordBox leader="cmd+e" ctx={ctx} chords={realTable()} onClose={() => {}} />)
  const menu = screen.getByRole("menu")
  const sequence = [...menu.querySelectorAll('[role="menuitem"], [role="separator"]')].map((i) =>
    i.getAttribute("role") === "separator" ? "---" : i.textContent,
  )
  expect(sequence).toEqual([
    "rRename",
    "wMove to workspace",
    "mSwitch model",
    "yCopy ID",
    "xRemove",
    "---",
    "cNew chat draft",
    "CNew chat",
    "dNew design",
    "sFocus sidebar",
    "bBig prompt box",
  ])
})

test("shows no separator when only one group has rows", () => {
  renderThemed(
    <ChordBox
      leader="cmd+e"
      ctx={ctx}
      chords={chordTable({ actions: [], history: noop })}
      onClose={noop}
    />,
  )
  expect(screen.queryByRole("separator")).toBeNull()
})

test("Remove is in the danger color and the rest are not", () => {
  renderThemed(<ChordBox leader="cmd+e" ctx={ctx} chords={realTable()} onClose={() => {}} />)
  const colorOf = (key: string) => {
    const el = screen.getAllByRole("menuitem").find((i) => i.textContent?.startsWith(key))!
    return getComputedStyle(el.querySelector("b")!).color
  }
  expect(colorOf("x")).not.toBe(colorOf("r"))
  expect(colorOf("r")).toBe(colorOf("c"))
})

test("labels the b row Normal prompt box after big mode is on, and has no Gear control", () => {
  const table = chordTable({ actions: [], history: noop })
  const open = () => {
    renderThemed(<ChordBox leader="cmd+e" ctx={ctx} chords={table} onClose={() => {}} />)
    const out = screen.getAllByRole("menuitem").map((i) => i.textContent)
    cleanup()
    return out
  }
  try {
    setBigPrompt(true)
    expect(open()).toContain("bNormal prompt box")
    expect(screen.queryByRole("button")).toBeNull()
  } finally {
    setBigPrompt(false)
  }
})

test("is on the layer stack while mounted", () => {
  const { unmount } = renderThemed(
    <ChordBox leader="cmd+e" ctx={ctx} chords={[]} onClose={() => {}} />,
  )
  expect(hasLayer()).toBe(true)
  unmount()
  expect(hasLayer()).toBe(false)
})
