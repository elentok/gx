import { cleanup, fireEvent, screen, within } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import type { DraftHistoryEntry } from "../generated/ops"
import { renderThemed } from "../ui/render"
import { WideContext } from "../useWide"
import { HistoryPopup } from "./HistoryPopup"
import { historyTime } from "./historyTime"

afterEach(cleanup)

const now = new Date(2025, 10, 19, 15, 0)
const ago = (ms: number) => new Date(now.getTime() - ms).toISOString()

const entry = (over: Partial<DraftHistoryEntry>): DraftHistoryEntry => ({
  id: "x",
  savedAt: ago(120_000),
  text: "",
  imageCount: 0,
  imagesDropped: false,
  project: "",
  slug: "",
  name: "",
  harness: "",
  model: "",
  effort: "",
  folder: "",
  composer: "",
  ...over,
})

const designDrafts = [
  entry({ id: "a", text: "Rework the sidebar", slug: "sidebar", project: "gx", imageCount: 2 }),
  entry({ id: "b", text: "Add dark mode", slug: "dark", project: "demo" }),
  entry({ id: "c", slug: "shots", project: "gx", imageCount: 1 }),
  entry({ id: "d", text: "Old one", slug: "old", project: "gx", imagesDropped: true }),
]

function setup(over: Partial<Parameters<typeof HistoryPopup>[0]> = {}, wide = true) {
  const onRestore = vi.fn()
  const onClose = vi.fn()
  renderThemed(
    <WideContext.Provider value={wide}>
      <HistoryPopup
        list="design-drafts"
        entries={designDrafts}
        now={now}
        onRestore={onRestore}
        onClose={onClose}
        {...over}
      />
    </WideContext.Provider>,
  )
  return { onRestore, onClose, filter: screen.getByRole("combobox") }
}

const rows = () => screen.getAllByRole("option")
const selected = () => rows().findIndex((r) => r.getAttribute("aria-selected") === "true")
const restoreButton = () => screen.getByRole("button", { name: "Restore" }) as HTMLButtonElement

test("lists entries with the title and a focused filter", () => {
  const { filter } = setup()
  expect(screen.getByTestId("history-title").textContent).toBe("Design drafts · newest first")
  expect(document.activeElement).toBe(filter)
  expect(filter.getAttribute("placeholder")).toBe("Filter design drafts…")
  expect(rows()).toHaveLength(4)
  expect(screen.getByText("4 of 50")).toBeTruthy()
})

test("filters by text in any case, slug and project, and the count follows", () => {
  const { filter } = setup()
  fireEvent.change(filter, { target: { value: "SIDEBAR" } })
  expect(rows()).toHaveLength(1)
  expect(screen.getByText("1 of 50")).toBeTruthy()
  fireEvent.change(filter, { target: { value: "dark" } })
  expect(rows()).toHaveLength(1)
  fireEvent.change(filter, { target: { value: "demo" } })
  expect(rows()).toHaveLength(1)
  expect(within(rows()[0]!).getByText("Add dark mode")).toBeTruthy()
  fireEvent.change(filter, { target: { value: "nothing" } })
  expect(screen.getByText('No match for "nothing"')).toBeTruthy()
  expect(screen.queryAllByRole("option")).toHaveLength(0)
})

test("filters a chat draft by session name and a sent message by name", () => {
  const chat = entry({
    id: "c1",
    text: "Fix it",
    project: "gx",
    name: "wobbly",
    harness: "claude",
    model: "opus",
    effort: "high",
  })
  const { filter } = setup({ list: "chat-drafts", entries: [chat] })
  expect(screen.getByText("gx / wobbly")).toBeTruthy()
  expect(screen.getByText("claude · opus · high")).toBeTruthy()
  fireEvent.change(filter, { target: { value: "WOBB" } })
  expect(rows()).toHaveLength(1)
  fireEvent.change(filter, { target: { value: "zzz" } })
  expect(screen.queryAllByRole("option")).toHaveLength(0)
})

test("a sent message shows a Chat or Design pill", () => {
  setup({
    list: "sent",
    entries: [
      entry({ id: "s1", text: "hi", composer: "chat", name: "talk", project: "gx" }),
      entry({ id: "s2", text: "yo", composer: "design", slug: "blog", project: "gx" }),
    ],
  })
  expect(screen.getByText("Chat · talk")).toBeTruthy()
  expect(screen.getByText("Design · blog")).toBeTruthy()
})

test("shows images-only text, the image count and images not kept", () => {
  setup()
  expect(screen.getByText("(images only)")).toBeTruthy()
  expect(within(rows()[0]!).getByText("2")).toBeTruthy()
  expect(within(rows()[3]!).getByText("images not kept")).toBeTruthy()
  expect(within(rows()[1]!).queryByText("images not kept")).toBeNull()
})

test("arrows move the highlight, Enter restores it", () => {
  const { filter, onRestore } = setup()
  expect(selected()).toBe(0)
  fireEvent.keyDown(filter, { key: "ArrowDown" })
  expect(selected()).toBe(1)
  fireEvent.keyDown(filter, { key: "ArrowUp" })
  fireEvent.keyDown(filter, { key: "ArrowUp" })
  expect(selected()).toBe(3)
  fireEvent.keyDown(filter, { key: "Enter" })
  expect(onRestore).toHaveBeenCalledWith(designDrafts[3])
})

test("Esc closes and restores nothing; the hint line shows the keys", () => {
  const { onClose, onRestore } = setup()
  for (const hint of ["↑↓ pick", "⏎ restore", "esc close"]) {
    expect(screen.getByText(hint)).toBeTruthy()
  }
  fireEvent.keyDown(window, { key: "Escape" })
  expect(onClose).toHaveBeenCalledTimes(1)
  expect(onRestore).not.toHaveBeenCalled()
})

test("one click on a wide screen restores the row", () => {
  const { onRestore } = setup()
  fireEvent.click(rows()[1]!)
  expect(onRestore).toHaveBeenCalledWith(designDrafts[1])
})

test("on a phone a tap marks a row and Restore restores it", () => {
  const { onRestore, onClose } = setup({}, false)
  expect(screen.queryByText("esc close")).toBeNull()
  expect(restoreButton().disabled).toBe(true)
  fireEvent.click(rows()[2]!)
  expect(onRestore).not.toHaveBeenCalled()
  expect(selected()).toBe(2)
  fireEvent.click(restoreButton())
  expect(onRestore).toHaveBeenCalledWith(designDrafts[2])
  fireEvent.click(screen.getByRole("button", { name: "Close" }))
  expect(onClose).toHaveBeenCalledTimes(1)
})

test("on a phone the popup draws no grab handle", () => {
  setup({}, false)
  expect(screen.queryByTestId("grab-handle")).toBeNull()
})

test("while pending, rows show Saving… and nothing restores", () => {
  const { filter, onRestore, onClose } = setup({ restoreDisabled: true })
  expect(screen.getAllByText("Saving…")).toHaveLength(4)
  fireEvent.keyDown(filter, { key: "Enter" })
  fireEvent.click(rows()[0]!)
  expect(onRestore).not.toHaveBeenCalled()
  fireEvent.change(filter, { target: { value: "dark" } })
  expect(rows()).toHaveLength(1)
  fireEvent.keyDown(window, { key: "Escape" })
  expect(onClose).toHaveBeenCalledTimes(1)
})

test("while pending on a phone, Restore stays disabled for a marked row", () => {
  const { onRestore } = setup({ restoreDisabled: true }, false)
  fireEvent.click(rows()[0]!)
  expect(restoreButton().disabled).toBe(true)
  expect(onRestore).not.toHaveBeenCalled()
})

test("historyTime words", () => {
  const at = (d: Date) => historyTime(d, now)
  expect(at(new Date(2025, 10, 19, 14, 58))).toBe("2 min ago")
  expect(at(new Date(2025, 10, 19, 14, 0))).toBe("1 h ago")
  expect(at(new Date(2025, 10, 18, 9, 0))).toBe("yesterday")
  expect(at(new Date(2025, 10, 17, 14, 2))).toBe("Mon 14:02")
  expect(at(new Date(2025, 10, 12, 8, 0))).toBe("Nov 12")
})
