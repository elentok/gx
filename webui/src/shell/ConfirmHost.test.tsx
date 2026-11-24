import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest"
import {
  closeConfirm,
  openConfirm,
  type ConfirmBody,
  type ConfirmRequest,
} from "../actions/confirmRequest"
import { ConnectionContext } from "../connection"
import { expectViEditor, findLargeEditor, viConnection } from "../ui/expandable-text/testing"
import { renderThemed } from "../ui/render"
import { WideContext } from "../useWide"
import { ConfirmHost } from "./ConfirmHost"
import { MAIN_HEADING_ATTR } from "./focusMainHeading"

const { isMac, showToast } = vi.hoisted(() => ({ isMac: vi.fn(() => true), showToast: vi.fn() }))
vi.mock("../platform", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../platform")>()),
  isMac,
}))
vi.mock("../ui/toaster", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../ui/toaster")>()),
  showToast,
}))

let anchor: HTMLButtonElement
beforeEach(() => {
  anchor = document.createElement("button")
  anchor.textContent = "⋯"
  document.body.append(anchor)
})
afterEach(() => {
  act(() => closeConfirm())
  cleanup()
  anchor.remove()
  document.body.replaceChildren()
  isMac.mockReturnValue(true)
  showToast.mockReset()
})

const body: ConfirmBody = { lines: ["Its session is deleted."] }

function request(over: Partial<ConfirmRequest> = {}): ConfirmRequest {
  return {
    actionId: "remove",
    entityName: "gx bugs",
    group: "danger",
    danger: true,
    confirmLabel: "Remove",
    dismissLabel: "Cancel",
    title: "Remove gx bugs",
    body,
    surface: { mode: "anchor", anchor },
    onConfirm: vi.fn(),
    ...over,
  }
}

function open(over: Partial<ConfirmRequest> = {}) {
  const req = request(over)
  renderThemed(<ConfirmHost />)
  act(() => openConfirm(req))
  return req
}

const cancel = () => screen.getByRole("button", { name: /^Cancel/ })
const confirm = (name: RegExp | string) => screen.getByRole("button", { name })

test("opens as a popover from a button and as a modal from a chord", () => {
  open()
  expect(screen.getByRole("alertdialog")).toBeTruthy()
  expect(screen.queryByRole("dialog")).toBeNull()
  act(() => closeConfirm())
  act(() => openConfirm(request({ surface: { mode: "modal" } })))
  expect(screen.getByRole("dialog").getAttribute("aria-modal")).toBe("true")
  expect(screen.queryByRole("alertdialog")).toBeNull()
})

test.each(["anchor", "modal"] as const)(
  "%s: title, body, Cancel and the action's own label",
  (mode) => {
    open({ surface: mode === "anchor" ? { mode, anchor } : { mode } })
    expect(screen.getByText("Remove gx bugs")).toBeTruthy()
    expect(screen.getByText("Its session is deleted.")).toBeTruthy()
    expect(cancel()).toBeTruthy()
    expect(confirm(/^Remove/)).toBeTruthy()
    expect(screen.queryByRole("button", { name: /^(OK|Yes|Keep)/ })).toBeNull()
  },
)

test.each([
  ["danger", "paused"],
  ["lifecycle", "running"],
  ["edit", "running"],
] as const)("a %s action's confirm button is tone %s", (group, tone) => {
  open({ group, danger: group === "danger", confirmLabel: "Hold" })
  expect(confirm(/^Hold/).getAttribute("data-tone")).toBe(tone)
})

test.each(["anchor", "modal"] as const)(
  "%s: Cancel has focus; Enter on it runs nothing",
  (mode) => {
    const req = open({ surface: mode === "anchor" ? { mode, anchor } : { mode } })
    expect(document.activeElement).toBe(cancel())
    fireEvent.click(cancel())
    expect(screen.queryByText("Remove gx bugs")).toBeNull()
    expect(req.onConfirm).not.toHaveBeenCalled()
  },
)

test("Esc cancels and focus returns to the anchor", async () => {
  const req = open()
  fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" })
  await waitFor(() => expect(screen.queryByText("Remove gx bugs")).toBeNull())
  await waitFor(() => expect(document.activeElement).toBe(anchor))
  expect(req.onConfirm).not.toHaveBeenCalled()
})

test("with the anchor gone, focus goes to the page's ⋯ button", async () => {
  const pageMenu = document.createElement("button")
  pageMenu.setAttribute("aria-label", "More actions")
  document.body.append(pageMenu)
  open()
  anchor.remove()
  fireEvent.click(cancel())
  await waitFor(() => expect(document.activeElement).toBe(pageMenu))
})

test("after an action that removed the open page, focus goes to the main heading", async () => {
  const h1 = document.createElement("h1")
  h1.setAttribute(MAIN_HEADING_ATTR, "")
  h1.tabIndex = -1
  document.body.append(h1)
  const req = open({ removesOpenPage: true })
  window.history.pushState({}, "", "/somewhere-else")
  fireEvent.click(confirm(/^Remove/))
  expect(req.onConfirm).toHaveBeenCalledTimes(1)
  await waitFor(() => expect(document.activeElement).toBe(h1))
  window.history.pushState({}, "", "/")
})

describe.each(["anchor", "modal"] as const)("keys in %s mode", (mode) => {
  const surface = () => (mode === "anchor" ? { mode, anchor } : { mode })
  test("y confirms and the buttons show their keys", () => {
    const req = open({ surface: surface() })
    expect(confirm("Remove y")).toBeTruthy()
    expect(confirm("Cancel n")).toBeTruthy()
    fireEvent.keyDown(window, { key: "y" })
    expect(req.onConfirm).toHaveBeenCalledTimes(1)
    expect(screen.queryByText("Remove gx bugs")).toBeNull()
  })
  test("n cancels", () => {
    const req = open({ surface: surface() })
    fireEvent.keyDown(window, { key: "n" })
    expect(req.onConfirm).not.toHaveBeenCalled()
    expect(screen.queryByText("Remove gx bugs")).toBeNull()
  })
})

test("with no confirm open, y is not caught by the host", () => {
  renderThemed(<ConfirmHost />)
  const seen = vi.fn((e: KeyboardEvent) => e.defaultPrevented)
  window.addEventListener("keydown", seen)
  fireEvent.keyDown(window, { key: "y" })
  window.removeEventListener("keydown", seen)
  expect(seen).toHaveReturnedWith(false)
})

describe("an input body", () => {
  const input = { lines: ["Starts the ticket again."], input: { label: "Reason", required: true } }
  const field = () => screen.getByRole("textbox", { name: "Reason" })

  test("y and n are typing; the confirm button is off until there is text", () => {
    const req = open({ body: input, confirmLabel: "Start over" })
    expect(confirm(/^Start over/).hasAttribute("disabled")).toBe(true)
    fireEvent.keyDown(field(), { key: "y" })
    fireEvent.keyDown(field(), { key: "n" })
    expect(req.onConfirm).not.toHaveBeenCalled()
    expect(screen.getByText("Starts the ticket again.")).toBeTruthy()
    fireEvent.change(field(), { target: { value: "stale" } })
    expect(confirm(/^Start over/).hasAttribute("disabled")).toBe(false)
  })

  test.each([
    ["Cmd+Enter on Mac", true, { metaKey: true }],
    ["Ctrl+Enter off Mac", false, { ctrlKey: true }],
  ])("%s confirms with the typed text", (_, mac, mods) => {
    isMac.mockReturnValue(mac)
    const req = open({ body: input })
    fireEvent.change(field(), { target: { value: "stale" } })
    fireEvent.keyDown(field(), { key: "Enter", ...mods })
    expect(req.onConfirm).toHaveBeenCalledExactlyOnceWith("stale")
  })

  test("Cmd+Enter with an empty reason does nothing", () => {
    const req = open({ body: input })
    fireEvent.keyDown(field(), { key: "Enter", metaKey: true })
    expect(req.onConfirm).not.toHaveBeenCalled()
  })

  test("the field is a PromptBox named recovery-reason with the body's placeholder", () => {
    open({
      body: {
        ...input,
        input: { ...input.input, name: "recovery-reason", placeholder: "Why start over?" },
      },
    })
    const area = field() as HTMLTextAreaElement
    expect(area.name).toBe("recovery-reason")
    expect(area.placeholder).toBe("Why start over?")
    expect(area.getAttribute("autocomplete")).toBe("off")
    expect(screen.getByLabelText("Open the note in a large editor")).toBeTruthy()
  })

  test("with vi mode on the field is an editor, and y and n typed in it do nothing", async () => {
    const req = request({ body: input })
    renderThemed(
      <ConnectionContext.Provider value={viConnection()}>
        <ConfirmHost />
      </ConnectionContext.Provider>,
    )
    act(() => openConfirm(req))
    await expectViEditor("Reason")
    const editor = field()
    fireEvent.keyDown(editor, { key: "y" })
    fireEvent.keyDown(editor, { key: "n" })
    expect(req.onConfirm).not.toHaveBeenCalled()
    expect(screen.getByRole("alertdialog")).toBeTruthy()
  })

  test("Esc in the large editor leaves the confirm open; Done closes the editor; Esc in the field cancels", async () => {
    const req = open({ body: input })
    fireEvent.click(screen.getByLabelText("Open the note in a large editor"))
    const large = await findLargeEditor(`${req.title} reason`)
    fireEvent.keyDown(large, { key: "Escape" })
    expect(screen.getByRole("alertdialog")).toBeTruthy()
    fireEvent.click(screen.getByRole("button", { name: "Done" }))
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull())
    expect(screen.getByRole("alertdialog")).toBeTruthy()
    fireEvent.keyDown(field(), { key: "Escape" })
    expect(screen.queryByRole("alertdialog")).toBeNull()
  })
})

describe("an async confirm", () => {
  const confirmNow = () => fireEvent.click(confirm(/^Remove/))

  test("keeps the popover open on a refusal and shows it as an alert", async () => {
    const onConfirm = vi.fn(async () => ({ refused: true, message: "The pause moved on." }))
    open({ onConfirm })
    confirmNow()
    expect((await screen.findByRole("alert")).textContent).toBe("The pause moved on.")
    expect(screen.getByRole("alertdialog")).toBeTruthy()
    expect(confirm(/^Remove/).hasAttribute("disabled")).toBe(false)
  })

  test("disables the confirm button while it runs, then closes on success", async () => {
    let finish: () => void = () => {}
    const onConfirm = vi.fn(() => new Promise<object>((r) => (finish = () => r({}))))
    open({ onConfirm })
    confirmNow()
    expect(confirm(/^Remove/).hasAttribute("disabled")).toBe(true)
    expect(screen.getByRole("alertdialog")).toBeTruthy()
    await act(async () => finish())
    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull())
  })

  test("a thrown write shows a reach error and stays open", async () => {
    open({ onConfirm: vi.fn(async () => Promise.reject(new Error("net"))) })
    confirmNow()
    expect((await screen.findByRole("alert")).textContent).toBe("Could not reach gx. Try again.")
  })
})

describe("a prepare step", () => {
  test("shows a spinner with the confirm off, then the result", async () => {
    let finish!: (b: ConfirmBody) => void
    const prepare = vi.fn(() => new Promise<ConfirmBody>((r) => (finish = r)))
    const req = open({ prepare })
    expect(screen.getByRole("status")).toBeTruthy()
    expect(confirm(/^Remove/).hasAttribute("disabled")).toBe(true)
    fireEvent.keyDown(window, { key: "y" })
    expect(req.onConfirm).not.toHaveBeenCalled()
    await act(async () => finish({ lines: [], warnings: ["Not merged to trunk."] }))
    expect(screen.getByText("Not merged to trunk.")).toBeTruthy()
    expect(confirm(/^Remove/).hasAttribute("disabled")).toBe(false)
    expect(req.onConfirm).not.toHaveBeenCalled()
  })

  test("a refusal closes the confirm and reports it", async () => {
    const req = open({ prepare: async () => ({ refused: true as const, message: "Not now." }) })
    await waitFor(() => expect(screen.queryByText("Remove gx bugs")).toBeNull())
    expect(showToast).toHaveBeenCalledWith(expect.objectContaining({ title: "Not now." }))
    expect(req.onConfirm).not.toHaveBeenCalled()
  })
})

test("a bulk request lists every row and counts them on the button", () => {
  const req = open({
    group: "lifecycle",
    danger: false,
    confirmLabel: "Stop 3",
    title: "Stop 3 sessions",
    body: { lines: [], rows: ["one", "two", "three"] },
  })
  expect(screen.getAllByRole("listitem").map((li) => li.textContent)).toEqual([
    "one",
    "two",
    "three",
  ])
  expect(screen.getAllByRole("alertdialog")).toHaveLength(1)
  fireEvent.click(confirm(/^Stop 3/))
  expect(req.onConfirm).toHaveBeenCalledTimes(1)
})

describe("on a phone", () => {
  const rects = { anchor: { top: 100, bottom: 130 }, card: 200 }
  const inputBody = { lines: ["Again."], input: { label: "Reason", required: true } }

  beforeEach(() => {
    rects.anchor = { top: 100, bottom: 130 }
    rects.card = 200
    vi.stubGlobal("visualViewport", {
      offsetTop: 0,
      height: 800,
      addEventListener() {},
      removeEventListener() {},
    })
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (
      this: HTMLElement,
    ) {
      if (this === anchor)
        return { ...rects.anchor, left: 300, right: 340, width: 40, height: 30 } as DOMRect
      return {
        top: 0,
        bottom: rects.card,
        left: 0,
        right: 0,
        width: 0,
        height: rects.card,
      } as DOMRect
    })
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  function openPhone(over: Partial<ConfirmRequest> = {}, wide = false) {
    const req = request(over)
    renderThemed(
      <WideContext.Provider value={wide}>
        <ConfirmHost />
      </WideContext.Provider>,
    )
    act(() => openConfirm(req))
    return req
  }
  const placement = () =>
    document.querySelector('[data-part="confirm-popover"]')?.getAttribute("data-side")

  test("a fitting confirm is a popover below a top anchor and above a bottom one", () => {
    openPhone()
    expect(screen.getByRole("alertdialog")).toBeTruthy()
    expect(placement()).toBe("below")
    act(() => closeConfirm())
    rects.anchor = { top: 700, bottom: 730 }
    act(() => openConfirm(request()))
    expect(placement()).toBe("above")
  })

  test("a confirm taller than both sides is the bottom sheet, with Cancel focused", () => {
    rects.card = 700
    openPhone()
    expect(screen.getByRole("dialog", { name: "Remove gx bugs" })).toBeTruthy()
    expect(screen.queryByRole("alertdialog")).toBeNull()
    expect(screen.getByTestId("grab-handle")).toBeTruthy()
    expect(document.activeElement).toBe(cancel())
  })

  test("the choice is kept when the body grows after the first full render", () => {
    openPhone({ body: inputBody })
    rects.card = 700
    fireEvent.change(screen.getByRole("textbox", { name: "Reason" }), { target: { value: "x" } })
    expect(screen.getByRole("alertdialog")).toBeTruthy()
  })

  test("a prepare step's spinner does not decide; the first full body does", async () => {
    rects.card = 60
    let finish!: (b: ConfirmBody) => void
    openPhone({ prepare: () => new Promise((r) => (finish = r)) })
    expect(screen.getByRole("alertdialog")).toBeTruthy()
    rects.card = 700
    await act(async () => finish({ lines: ["Long."] }))
    expect(screen.getByRole("dialog")).toBeTruthy()
  })

  test("the sheet keeps the keys: the reason field types, Cmd+Enter confirms", () => {
    rects.card = 700
    const req = openPhone({ body: inputBody })
    const field = screen.getByRole("textbox", { name: "Reason" })
    fireEvent.keyDown(field, { key: "y" })
    fireEvent.keyDown(field, { key: "n" })
    expect(req.onConfirm).not.toHaveBeenCalled()
    expect(confirm(/^Remove/).hasAttribute("disabled")).toBe(true)
    fireEvent.change(field, { target: { value: "why" } })
    fireEvent.keyDown(field, { key: "Enter", metaKey: true })
    expect(req.onConfirm).toHaveBeenCalledExactlyOnceWith("why")
  })

  test("Esc and n close the sheet, y confirms", () => {
    rects.card = 700
    const req = openPhone()
    fireEvent.keyDown(document.activeElement ?? window, { key: "Escape" })
    expect(screen.queryByRole("dialog")).toBeNull()
    act(() => openConfirm(request()))
    fireEvent.keyDown(window, { key: "n" })
    expect(screen.queryByRole("dialog")).toBeNull()
    expect(req.onConfirm).not.toHaveBeenCalled()
    act(() => openConfirm(request()))
    fireEvent.keyDown(window, { key: "y" })
    expect(screen.queryByRole("dialog")).toBeNull()
  })

  test("a wide screen never uses the sheet", () => {
    rects.card = 700
    openPhone({}, true)
    expect(screen.getByRole("alertdialog")).toBeTruthy()
  })
})
