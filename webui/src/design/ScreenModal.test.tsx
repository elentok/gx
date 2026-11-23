import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { renderThemed } from "../ui/render"
import type { SessionAttachEvent } from "../generated/server"
import type { SubscribeOwner, TakeOver } from "./inputOwner"
import { ATTACH_KEYS, type PostInput } from "./keys"
import { tabClientId } from "./inputOwner"
import { ScreenModal } from "./ScreenModal"
import type { FetchScreen } from "./screen"

afterEach(cleanup)

const fetcher = (): FetchScreen =>
  vi.fn(
    async () => ({ kind: "ok", detail: { rows: ["row one"] } }) as Awaited<ReturnType<FetchScreen>>,
  )

type Emit = (ev: SessionAttachEvent) => void

function open(opts: { keys?: boolean; poster?: PostInput; hintMs?: number } = {}) {
  const f = fetcher()
  const poster = opts.poster ?? vi.fn<PostInput>(async () => ({}))
  let emit: Emit = () => {}
  const subscribe: SubscribeOwner = (_t, _c, onEvent) => {
    emit = onEvent
    return () => {}
  }
  const takeOver = vi.fn<TakeOver>(async () => ({}))
  renderThemed(
    <ScreenModal
      target="s-a"
      fetcher={f}
      keys={opts.keys ?? true}
      poster={poster}
      subscribe={subscribe}
      takeOver={takeOver}
      blockedHintMs={opts.hintMs}
      onClose={() => {}}
    />,
  )
  return { f, poster, takeOver, emit: (ev: SessionAttachEvent) => act(() => emit(ev)) }
}

test.each([
  ["self", { type: "owner", self: true }, "Input: this client", false],
  ["another client", { type: "owner" }, "Input: another client", true],
  ["gx", { type: "owner", gx: true }, "Input: gx is typing", true],
])("the indicator shows %s as owner; Take over only for non-owners", (_n, ev, text, canTake) => {
  const { emit } = open()
  emit(ev)
  expect(screen.getByText(text)).toBeTruthy()
  expect(!!screen.queryByRole("button", { name: "Take over" })).toBe(canTake)
})

test("with three clients the owner sees no Take over, a non-owner sees one", () => {
  const { emit } = open()
  emit({ type: "owner", self: true })
  expect(screen.queryByRole("button", { name: "Take over" })).toBeNull()
  cleanup()
  const other = open()
  other.emit({ type: "owner" })
  expect(screen.getAllByRole("button", { name: "Take over" })).toHaveLength(1)
})

test("Take over calls the route and clears a blocked-key hint", async () => {
  const { emit, takeOver } = open()
  emit({ type: "owner" })
  emit({ type: "blocked" })
  expect(screen.getByText(/Your key was not sent/)).toBeTruthy()
  fireEvent.click(screen.getByRole("button", { name: "Take over" }))
  expect(takeOver).toHaveBeenCalledWith("s-a", expect.any(String))
  expect(screen.queryByText(/Your key was not sent/)).toBeNull()
})

test("the blocked-key hint goes away after the timeout", async () => {
  const { emit } = open({ hintMs: 20 })
  emit({ type: "owner" })
  emit({ type: "blocked" })
  expect(screen.getByText(/Your key was not sent/)).toBeTruthy()
  await waitFor(() => expect(screen.queryByText(/Your key was not sent/)).toBeNull())
})

test.each([
  ["Esc", "esc"],
  ["Enter", "enter"],
  ["Tab", "tab"],
  ["Up", "up"],
  ["Down", "down"],
  ["Left", "left"],
  ["Right", "right"],
  ["Ctrl+C", "ctrl+c"],
])(
  "the %s button sends %s through attach input with the tab's client id and refetches",
  async (name, key) => {
    const { f, poster } = open()
    await waitFor(() => expect(f).toHaveBeenCalledTimes(1))
    fireEvent.click(screen.getByRole("button", { name }))
    await waitFor(() =>
      expect(poster).toHaveBeenCalledWith({
        id: "s-a",
        client: tabClientId(),
        key: ATTACH_KEYS[key],
      }),
    )
    await waitFor(() => expect(f).toHaveBeenCalledTimes(2))
  },
)

test("Type pastes the text through attach input with the client id and clears the field", async () => {
  const { f, poster } = open()
  const field = screen.getByLabelText("Text to type") as HTMLInputElement
  fireEvent.change(field, { target: { value: "yes please" } })
  fireEvent.click(screen.getByRole("button", { name: "Type" }))
  await waitFor(() =>
    expect(poster).toHaveBeenCalledWith({
      id: "s-a",
      client: tabClientId(),
      paste: "yes please",
    }),
  )
  await waitFor(() => expect(field.value).toBe(""))
  await waitFor(() => expect(f).toHaveBeenCalledTimes(2))
})

const rowsPoster = (rows: string[]) => vi.fn<PostInput>(async () => ({ rows }))
const rowOf = (text: string) => screen.getByText(text) as HTMLElement

test("settled rows show without a second read and the changed row is marked", async () => {
  const { f } = open({ poster: rowsPoster(["row one", "row two"]) })
  await screen.findByText("row one")
  fireEvent.click(screen.getByRole("button", { name: "Enter" }))
  await screen.findByText("row two")
  expect(f).toHaveBeenCalledTimes(1)
  expect(getComputedStyle(rowOf("row two")).fontWeight).toBe("700")
  expect(getComputedStyle(rowOf("row one")).fontWeight).not.toBe("700")
})

test("equal rows say the screen is unchanged, and the mark and the line fade", async () => {
  open({ poster: rowsPoster(["row one"]) })
  await screen.findByText("row one")
  vi.useFakeTimers()
  try {
    fireEvent.click(screen.getByRole("button", { name: "Enter" }))
    await act(async () => {})
    expect(screen.getByRole("status").textContent).toBe("Screen unchanged")
    await act(async () => {
      vi.advanceTimersByTime(5000)
    })
    expect(screen.getByRole("status").textContent).toBe("")
  } finally {
    vi.useRealTimers()
  }
})

test("marks fade", async () => {
  open({ poster: rowsPoster(["row one", "changed"]) })
  await screen.findByText("row one")
  vi.useFakeTimers()
  try {
    fireEvent.click(screen.getByRole("button", { name: "Enter" }))
    await act(async () => {})
    expect(getComputedStyle(rowOf("changed")).fontWeight).toBe("700")
    await act(async () => {
      vi.advanceTimersByTime(5000)
    })
    expect(getComputedStyle(rowOf("changed")).fontWeight).not.toBe("700")
  } finally {
    vi.useRealTimers()
  }
})

test("Refresh does one plain read and sends no key", async () => {
  const { f, poster } = open()
  await screen.findByText("row one")
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }))
  await waitFor(() => expect(f).toHaveBeenCalledTimes(2))
  expect(poster).not.toHaveBeenCalled()
})

test("a refusal shows its message and keeps the text", async () => {
  const poster = vi.fn<PostInput>(async () => ({ refused: true, message: "q-1 is a Queue agent" }))
  open({ poster })
  const field = screen.getByLabelText("Text to type") as HTMLInputElement
  fireEvent.change(field, { target: { value: "hi" } })
  fireEvent.click(screen.getByRole("button", { name: "Type" }))
  expect(await screen.findByText("q-1 is a Queue agent")).toBeTruthy()
  expect(field.value).toBe("hi")
})

test("no key bar or Type field when keys are not allowed", async () => {
  const { f } = open({ keys: false })
  await waitFor(() => expect(f).toHaveBeenCalled())
  expect(screen.queryByRole("button", { name: "Esc" })).toBeNull()
  expect(screen.queryByLabelText("Text to type")).toBeNull()
  expect(screen.getByRole("button", { name: "Refresh" })).toBeTruthy()
})
