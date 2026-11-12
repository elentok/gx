import { cleanup, fireEvent, screen } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { hasLayer } from "../ui/modal/layers"
import { renderThemed } from "../ui/render"
import { ChordBox } from "./ChordBox"
import type { Chord } from "./chords"

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

test("defaults to the real chord table", () => {
  renderThemed(<ChordBox leader="cmd+e" ctx={ctx} onClose={() => {}} />)
  expect(screen.getAllByRole("menuitem").map((i) => i.textContent)).toEqual([
    "cNew chat",
    "dNew design",
  ])
})

test("is on the layer stack while mounted", () => {
  const { unmount } = renderThemed(<ChordBox leader="cmd+e" ctx={ctx} onClose={() => {}} />)
  expect(hasLayer()).toBe(true)
  unmount()
  expect(hasLayer()).toBe(false)
})
