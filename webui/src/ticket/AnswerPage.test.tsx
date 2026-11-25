import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { createMemoryRouter, RouterProvider, useParams } from "react-router"
import { afterEach, beforeEach, expect, test, vi } from "vitest"
import canned from "../canned-detail.json"
import { ConnectionContext, type Connection } from "../connection"
import type { TicketDetail } from "../generated/server"
import { renderThemed } from "../ui/render"
import { AnswerPage } from "./AnswerPage"
import type { AnswerBody } from "./answer"
import type { FetchDetail } from "./detail"
import { clearSents, useSent } from "./sent"

const post = vi.fn<(b: AnswerBody) => Promise<unknown>>()
vi.mock("./answer", async (orig) => ({
  ...(await orig<typeof import("./answer")>()),
  postAnswer: (b: AnswerBody) => post(b),
}))

beforeEach(() => {
  post.mockReset()
  post.mockResolvedValue({ outcome: "delivered" })
  clearSents()
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
  entries: string[],
  fetchDetail: FetchDetail = async () => ({ kind: "ok", detail }),
) {
  const router = createMemoryRouter(
    [
      { path: "/t/*", element: <TicketStub /> },
      { path: "/answer/*", element: <AnswerRoute fetchDetail={fetchDetail} /> },
    ],
    { initialEntries: entries, initialIndex: entries.length - 1 },
  )
  renderThemed(
    <ConnectionContext.Provider value={fakeConn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )
  return router
}

const answerUrl = `/answer/${ADDRESS}`
const ticketUrl = `/t/${ADDRESS}`
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
  const router = renderAnswer([ticketUrl, answerUrl])
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
  const router = renderAnswer([ticketUrl, answerUrl])
  fireEvent.click(await screen.findByRole("link", { name: "← Ticket" }))
  expect(await screen.findByText(/ticket page/)).toBeTruthy()
  expect(router.state.historyAction).toBe("POP")
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

test("a ticket that does not exist shows Ticket not found", async () => {
  renderAnswer([answerUrl], async () => ({ kind: "not-found" }))
  expect(await screen.findByText("Ticket not found")).toBeTruthy()
})
