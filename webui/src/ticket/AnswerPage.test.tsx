import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { createMemoryRouter, RouterProvider, useParams, type InitialEntry } from "react-router"
import { afterEach, beforeEach, expect, test, vi } from "vitest"
import canned from "../canned-detail.json"
import { ConnectionContext, type Connection } from "../connection"
import type { TicketDetail } from "../generated/server"
import { renderThemed } from "../ui/render"
import { AnswerPage } from "./AnswerPage"
import type { AnswerBody } from "./answer"
import type { FetchDetail } from "./detail"
import { clearDrafts } from "./draft"
import { clearSents, useSent } from "./sent"
import { writePageCache } from "../pageCache"
import { DeviceContext, type Device } from "../useWide"

const post = vi.fn<(b: AnswerBody) => Promise<unknown>>()
vi.mock("./answer", async (orig) => ({
  ...(await orig<typeof import("./answer")>()),
  postAnswer: (b: AnswerBody) => post(b),
}))

beforeEach(() => {
  post.mockReset()
  post.mockResolvedValue({ outcome: "delivered" })
  clearSents()
  clearDrafts()
  handlers.clear()
  sessionStorage.clear()
})
afterEach(cleanup)

const detail = canned as TicketDetail
const ADDRESS = detail.address
const connState = { snapshot: null, connected: true, loadedOnce: true, liveSnapshot: true }
const fakeConn = {
  getState: () => connState,
  subscribe: () => () => {},
  onEvent: () => () => {},
  refresh: () => {},
} as unknown as Connection

function TicketStub() {
  const sent = useSent(useParams()["*"] ?? "")
  return <p>ticket page {sent ? `sent:${sent.outcome}` : "unsent"}</p>
}

function AnswerRoute({ fetchDetail }: { fetchDetail: FetchDetail }) {
  return <AnswerPage address={useParams()["*"] ?? ""} fetchDetail={fetchDetail} />
}

function renderAnswer(
  entries: InitialEntry[],
  fetchDetail: FetchDetail = async () => ({ kind: "ok", detail }),
  device?: Device,
) {
  const router = createMemoryRouter(
    [
      { path: "/t/*", element: <TicketStub /> },
      { path: "/answer/*", element: <AnswerRoute fetchDetail={fetchDetail} /> },
    ],
    { initialEntries: entries, initialIndex: entries.length - 1 },
  )
  renderThemed(
    <DeviceContext.Provider value={device}>
      <ConnectionContext.Provider value={fakeConn}>
        <RouterProvider router={router} />
      </ConnectionContext.Provider>
    </DeviceContext.Provider>,
  )
  return router
}

const answerUrl = `/answer/${ADDRESS}`
const ticketUrl = `/t/${ADDRESS}`
// The entry the ticket card's "Answer question" button pushes.
const answerFromTicket = { pathname: answerUrl, state: { fromTicket: true } }
const sendButton = () => screen.getByRole("button", { name: "Send answer" }) as HTMLButtonElement

test("renders the header, the question once, earlier rounds collapsed and only Send in the footer", async () => {
  renderAnswer([answerUrl])
  expect(await screen.findByText("THE AGENT ASKS")).toBeTruthy()
  expect(screen.getByRole("link", { name: "← Ticket" })).toBeTruthy()
  expect(screen.getByRole("heading", { name: "Answer question" })).toBeTruthy()
  expect(screen.getAllByText(new RegExp(detail.latest!.summary))).toHaveLength(1)
  expect(screen.getByRole("form", { name: "Answer" })).toBeTruthy()
  expect(screen.getByRole("button", { name: /Earlier rounds \(2\)/ })).toBeTruthy()
  expect(screen.queryByText(/#1 · agent/)).toBeNull()
  expect(screen.getAllByRole("button", { name: /^(Send answer|Resume)/ })).toHaveLength(1)
  expect(document.title).toContain("Answer question")
})

test("Send is disabled until there is a pick or text, then posts the form's body", async () => {
  renderAnswer([answerUrl, answerUrl])
  await screen.findByText("THE AGENT ASKS")
  expect(sendButton().disabled).toBe(true)
  const field = screen.getByLabelText(`Answer to question ${detail.latest!.seq}`)
  fireEvent.change(field, { target: { value: "use postgres" } })
  expect(sendButton().disabled).toBe(false)
  fireEvent.click(sendButton())
  await waitFor(() => expect(post).toHaveBeenCalled())
  expect(post.mock.calls[0][0]).toMatchObject({ address: ADDRESS, state: detail.state })
})

test("from [ticket, answer] a send goes back one entry and leaves a sent record", async () => {
  const router = renderAnswer([ticketUrl, answerFromTicket])
  await screen.findByText("THE AGENT ASKS")
  fireEvent.change(screen.getByLabelText(`Answer to question ${detail.latest!.seq}`), {
    target: { value: "yes" },
  })
  fireEvent.click(sendButton())
  expect(await screen.findByText("ticket page sent:delivered")).toBeTruthy()
  expect(router.state.location.pathname).toBe(ticketUrl)
  expect(router.state.historyAction).toBe("POP")
})

test("from [ticket, answer] ← Ticket goes back one entry", async () => {
  const router = renderAnswer([ticketUrl, answerFromTicket])
  fireEvent.click(await screen.findByRole("link", { name: "← Ticket" }))
  expect(await screen.findByText(/ticket page/)).toBeTruthy()
  expect(router.state.historyAction).toBe("POP")
})

test("a push tap while the app is open replaces the entry on ← Ticket and on a send", async () => {
  const router = renderAnswer([ticketUrl])
  await act(() => router.navigate(answerUrl))
  fireEvent.click(await screen.findByRole("link", { name: "← Ticket" }))
  expect(await screen.findByText(/ticket page/)).toBeTruthy()
  expect(router.state.location.pathname).toBe(ticketUrl)
  expect(router.state.historyAction).toBe("REPLACE")
  cleanup()

  const second = renderAnswer([ticketUrl])
  await act(() => second.navigate(answerUrl))
  await screen.findByText("THE AGENT ASKS")
  fireEvent.change(screen.getByLabelText(`Answer to question ${detail.latest!.seq}`), {
    target: { value: "yes" },
  })
  fireEvent.click(sendButton())
  expect(await screen.findByText("ticket page sent:delivered")).toBeTruthy()
  expect(second.state.historyAction).toBe("REPLACE")
})

test("on a deep link ← Ticket and a send replace the entry with the ticket page", async () => {
  const router = renderAnswer([answerUrl])
  fireEvent.click(await screen.findByRole("link", { name: "← Ticket" }))
  expect(await screen.findByText(/ticket page/)).toBeTruthy()
  expect(router.state.location.pathname).toBe(ticketUrl)
  expect(router.state.historyAction).toBe("REPLACE")
  cleanup()

  const second = renderAnswer([answerUrl])
  await screen.findByText("THE AGENT ASKS")
  fireEvent.change(screen.getByLabelText(`Answer to question ${detail.latest!.seq}`), {
    target: { value: "yes" },
  })
  fireEvent.click(sendButton())
  expect(await screen.findByText("ticket page sent:delivered")).toBeTruthy()
  expect(second.state.historyAction).toBe("REPLACE")
})

// A detail the test can swap, and a connection event that makes the page refetch it.
let served: TicketDetail = detail
const handlers = new Set<(a: string | undefined, type: string) => void>()
const changeTicket = (next: TicketDetail) => {
  served = next
  for (const h of handlers) h(ADDRESS, "changed")
}
const liveConn = {
  ...fakeConn,
  onEvent: (h: never) => {
    handlers.add(h)
    return () => handlers.delete(h)
  },
}
const fetchServed: FetchDetail = async () => ({ kind: "ok", detail: served })
const round = (seq: number, outcome: string, state: string): TicketDetail => ({
  ...detail,
  state,
  latest: { ...detail.latest!, seq, outcome },
  actions: outcome === "open" ? detail.actions : [],
})
const typeAnswer = (v: string) =>
  fireEvent.change(screen.getByLabelText(`Answer to question ${detail.latest!.seq}`), {
    target: { value: v },
  })
const backButton = () => screen.getByRole("button", { name: "Back to ticket" })

function renderLive(entries: string[], initial: TicketDetail = detail) {
  served = initial
  const router = createMemoryRouter(
    [
      { path: "/t/*", element: <TicketStub /> },
      { path: "/answer/*", element: <AnswerRoute fetchDetail={fetchServed} /> },
    ],
    { initialEntries: entries, initialIndex: entries.length - 1 },
  )
  renderThemed(
    <ConnectionContext.Provider value={liveConn as unknown as Connection}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )
  return router
}

test("leaving and coming back keeps the text and the note", async () => {
  renderLive([ticketUrl, answerUrl])
  await screen.findByText("THE AGENT ASKS")
  typeAnswer("use postgres")
  fireEvent.click(screen.getByRole("button", { name: `Note on question ${detail.latest!.seq}` }))
  fireEvent.change(screen.getByLabelText(`Note text for question ${detail.latest!.seq}`), {
    target: { value: "mind the lock" },
  })
  fireEvent.click(screen.getByRole("link", { name: "← Ticket" }))
  await screen.findByText(/ticket page/)
  cleanup()
  renderLive([ticketUrl, answerUrl])
  await screen.findByText("THE AGENT ASKS")
  expect(
    (screen.getByLabelText(`Answer to question ${detail.latest!.seq}`) as HTMLTextAreaElement)
      .value,
  ).toBe("use postgres")
  expect(
    (screen.getByLabelText(`Note text for question ${detail.latest!.seq}`) as HTMLTextAreaElement)
      .value,
  ).toBe("mind the lock")
})

test("a send clears the draft", async () => {
  renderLive([ticketUrl, answerUrl])
  await screen.findByText("THE AGENT ASKS")
  typeAnswer("yes")
  fireEvent.click(sendButton())
  await screen.findByText("ticket page sent:delivered")
  cleanup()
  renderLive([answerUrl])
  await screen.findByText("THE AGENT ASKS")
  expect(
    (screen.getByLabelText(`Answer to question ${detail.latest!.seq}`) as HTMLTextAreaElement)
      .value,
  ).toBe("")
})

test("a page opened on a replaced question is a fresh form with the note kept and no banner", async () => {
  renderLive([ticketUrl, answerUrl])
  await screen.findByText("THE AGENT ASKS")
  typeAnswer("old text")
  fireEvent.click(screen.getByRole("button", { name: `Note on question ${detail.latest!.seq}` }))
  fireEvent.change(screen.getByLabelText(`Note text for question ${detail.latest!.seq}`), {
    target: { value: "keep me" },
  })
  fireEvent.click(screen.getByRole("link", { name: "← Ticket" }))
  await screen.findByText(/ticket page/)
  cleanup()
  const next = round(detail.latest!.seq + 1, "open", "tok-2")
  renderLive([ticketUrl, answerUrl], next)
  await screen.findByText("THE AGENT ASKS")
  expect(screen.queryByRole("status")).toBeNull()
  const seq = next.latest!.seq
  expect((screen.getByLabelText(`Answer to question ${seq}`) as HTMLTextAreaElement).value).toBe("")
  expect(
    (screen.getByLabelText(`Note text for question ${seq}`) as HTMLTextAreaElement).value,
  ).toBe("keep me")
})

test("answered elsewhere while open: banner, typed text stays, only Back to ticket", async () => {
  renderLive([ticketUrl, answerUrl])
  await screen.findByText("THE AGENT ASKS")
  typeAnswer("half typed")
  changeTicket(round(detail.latest!.seq, "answered", "tok-9"))
  expect(await screen.findByText("This question was answered elsewhere")).toBeTruthy()
  expect(
    (screen.getByLabelText(`Answer to question ${detail.latest!.seq}`) as HTMLTextAreaElement)
      .value,
  ).toBe("half typed")
  expect(screen.queryByRole("button", { name: "Send answer" })).toBeNull()
  fireEvent.click(backButton())
  expect(await screen.findByText(/ticket page/)).toBeTruthy()
})

test("a new question while open shows the changed banner and keeps the old question", async () => {
  renderLive([ticketUrl, answerUrl])
  await screen.findByText("THE AGENT ASKS")
  typeAnswer("half typed")
  changeTicket(round(detail.latest!.seq + 1, "open", "tok-2"))
  expect(
    await screen.findByText("This question changed. Back to ticket to see the new one."),
  ).toBeTruthy()
  expect(
    (screen.getByLabelText(`Answer to question ${detail.latest!.seq}`) as HTMLTextAreaElement)
      .value,
  ).toBe("half typed")
  expect(screen.queryByRole("button", { name: "Send answer" })).toBeNull()
})

test("a stale refusal refetches and shows the changed banner", async () => {
  renderLive([ticketUrl, answerUrl])
  await screen.findByText("THE AGENT ASKS")
  typeAnswer("x")
  post.mockResolvedValue({ refused: true, reason: "stale", message: "Question changed." })
  fireEvent.click(sendButton())
  expect(
    await screen.findByText("This question changed. Back to ticket to see the new one."),
  ).toBeTruthy()
  expect(screen.queryByRole("button", { name: "Send answer" })).toBeNull()
})

test("a stale refusal after the question was answered elsewhere shows answered elsewhere", async () => {
  renderLive([ticketUrl, answerUrl])
  await screen.findByText("THE AGENT ASKS")
  typeAnswer("x")
  served = round(detail.latest!.seq, "answered", "tok-9")
  post.mockResolvedValue({ refused: true, reason: "stale", message: "Question changed." })
  fireEvent.click(sendButton())
  expect(await screen.findByText("This question was answered elsewhere")).toBeTruthy()
  expect(screen.queryByText(/This question changed/)).toBeNull()
})

test("a stale refusal after the question was replaced shows the changed banner", async () => {
  renderLive([ticketUrl, answerUrl])
  await screen.findByText("THE AGENT ASKS")
  typeAnswer("x")
  served = round(detail.latest!.seq + 1, "open", "tok-2")
  post.mockResolvedValue({ refused: true, reason: "stale", message: "Question changed." })
  fireEvent.click(sendButton())
  expect(
    await screen.findByText("This question changed. Back to ticket to see the new one."),
  ).toBeTruthy()
})

test("a ticket with no open question shows answered elsewhere and Back to ticket, no form", async () => {
  const router = renderLive([answerUrl], round(detail.latest!.seq, "answered", "tok-9"))
  expect(await screen.findByText("This question was answered elsewhere")).toBeTruthy()
  expect(screen.queryByRole("form", { name: "Answer" })).toBeNull()
  fireEvent.click(backButton())
  await screen.findByText(/ticket page/)
  expect(router.state.historyAction).toBe("REPLACE")
})

test("an open gx-asked question says it is answered on the ticket page", async () => {
  renderLive([answerUrl], { ...detail, actions: [] })
  expect(await screen.findByText("This question is answered on the ticket page.")).toBeTruthy()
  expect(screen.queryByRole("button", { name: "Send answer" })).toBeNull()
  expect(backButton()).toBeTruthy()
})

test("a cached first detail never makes a changed banner", async () => {
  writePageCache("ticket", detail, ADDRESS)
  const next = round(detail.latest!.seq + 1, "open", "tok-2")
  renderAnswer([answerUrl], async () => ({ kind: "ok", detail: next }))
  await screen.findByLabelText(`Answer to question ${next.latest!.seq}`)
  expect(screen.queryByRole("status")).toBeNull()
})

test("agent-not-ready keeps the answer, shows the hint and stays on the page", async () => {
  const router = renderLive([ticketUrl, answerUrl])
  await screen.findByText("THE AGENT ASKS")
  typeAnswer("go")
  post.mockResolvedValue({ refused: true, reason: "agent-not-ready", message: "Agent waits." })
  fireEvent.click(sendButton())
  expect(
    await screen.findByText(/Approve the agent's prompt first, then send again\./),
  ).toBeTruthy()
  expect(
    (screen.getByLabelText(`Answer to question ${detail.latest!.seq}`) as HTMLTextAreaElement)
      .value,
  ).toBe("go")
  expect(router.state.location.pathname).toBe(answerUrl)
})

test("a ticket that does not exist shows Ticket not found", async () => {
  renderAnswer([answerUrl], async () => ({ kind: "not-found" }))
  expect(await screen.findByText("Ticket not found")).toBeTruthy()
})

class FakeViewport extends EventTarget {
  height = 800
  scale = 1
  offsetTop = 0
  resize(height: number) {
    this.height = height
    this.dispatchEvent(new Event("resize"))
  }
}

function withKeyboardViewport() {
  const vv = new FakeViewport()
  Object.defineProperty(window, "visualViewport", { value: vv, configurable: true })
  Object.defineProperty(window, "innerHeight", { value: 800, configurable: true })
  return vv
}

afterEach(() => {
  Object.defineProperty(window, "visualViewport", { value: undefined, configurable: true })
})

const pageHeader = () =>
  screen.getByRole("heading", { name: "Answer question", hidden: true }).parentElement!

test("on a phone the header hides while the keyboard is open and the footer stays", async () => {
  const vv = withKeyboardViewport()
  renderAnswer([answerUrl], undefined, "phone")
  const field = await screen.findByLabelText(`Answer to question ${detail.latest!.seq}`)
  expect(getComputedStyle(pageHeader()).display).not.toBe("none")
  act(() => {
    field.focus()
    vv.resize(500)
  })
  await waitFor(() => expect(getComputedStyle(pageHeader()).display).toBe("none"))
  expect(sendButton()).toBeTruthy()
  expect(document.documentElement.style.getPropertyValue("--fill-height")).toBe("500px")
  act(() => {
    field.blur()
    vv.resize(800)
  })
  await waitFor(() => expect(getComputedStyle(pageHeader()).display).not.toBe("none"))
})

test("on a desktop the same shrink hides nothing", async () => {
  const vv = withKeyboardViewport()
  renderAnswer([answerUrl])
  const field = await screen.findByLabelText(`Answer to question ${detail.latest!.seq}`)
  act(() => {
    field.focus()
    vv.resize(500)
  })
  await waitFor(() =>
    expect(document.documentElement.style.getPropertyValue("--fill-height")).toBe("500px"),
  )
  expect(getComputedStyle(pageHeader()).display).not.toBe("none")
})
