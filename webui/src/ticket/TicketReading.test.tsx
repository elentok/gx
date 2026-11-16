import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, expect, test } from "vitest"
import canned from "../canned-detail.json"
import { FileLinkProvider } from "../files/FileLinkContext"
import { stubResolver } from "../files/stubResolver.testutil"
import type { TicketDetail } from "../generated/server"
import { renderThemed } from "../ui/render"
import { TicketReading } from "./TicketReading"

afterEach(cleanup)

const base = canned as TicketDetail
const html = () => document.body.textContent ?? ""

test("sections appear in order with the agent label", () => {
  renderThemed(<TicketReading detail={base} />)
  const text = html()
  const order = ["THE AGENT ASKS", "Agent's notes", "Earlier rounds (2)"].map((s) =>
    text.indexOf(s),
  )
  expect(order.every((i) => i >= 0)).toBe(true)
  expect(order).toEqual([...order].sort((a, b) => a - b))
})

test("a gx question is labelled GX ASKS", () => {
  renderThemed(<TicketReading detail={{ ...base, latest: { ...base.latest!, by: "gx" } }} />)
  expect(screen.getByText("GX ASKS")).toBeTruthy()
  expect(screen.queryByText("THE AGENT ASKS")).toBeNull()
})

test("notes and earlier rounds are collapsed until tapped", () => {
  renderThemed(<TicketReading detail={base} />)
  expect(screen.queryByText("I compared both; Redis survives restarts.")).toBeNull()
  expect(screen.queryByText(/Which database\?/)).toBeNull()
  fireEvent.click(screen.getByRole("button", { name: /Agent's notes/ }))
  expect(screen.getByText("I compared both; Redis survives restarts.")).toBeTruthy()
  fireEvent.click(screen.getByRole("button", { name: /Earlier rounds/ }))
  expect(screen.getByText(/Which database\?/)).toBeTruthy()
})

test("the asker's text links a found path and leaves a missing one plain", async () => {
  const { resolver } = stubResolver(["docs/plans/host-files.md"])
  const detail: TicketDetail = {
    ...base,
    latest: { ...base.latest!, text: "Read docs/plans/host-files.md and gone/x.md first" },
  }
  const { container } = renderThemed(
    <FileLinkProvider resolver={resolver}>
      <TicketReading detail={detail} />
    </FileLinkProvider>,
  )
  await waitFor(() => expect(container.querySelectorAll("a")).toHaveLength(1))
  expect(container.querySelector("a")?.textContent).toBe("docs/plans/host-files.md")
  expect(container.textContent).toContain("gone/x.md")
})

test("an answered round shows its thread read-only", () => {
  const detail: TicketDetail = {
    ...base,
    status: "open",
    answered: true,
    latest: { ...base.latest!, outcome: "answered", answer: { source: "web", text: "Redis" } },
    chat: [...base.chat, { from: "web", text: "Redis" }],
    actions: [],
  }
  renderThemed(<TicketReading detail={detail} />)
  expect(screen.getByText("Redis")).toBeTruthy()
  expect(screen.queryByRole("textbox")).toBeNull()
})

test("a closed round shows its reason read-only", () => {
  const detail: TicketDetail = {
    ...base,
    latest: { ...base.latest!, outcome: "closed", closed_reason: "superseded" },
    actions: [],
  }
  renderThemed(<TicketReading detail={detail} />)
  expect(screen.getByText("Closed: superseded")).toBeTruthy()
  expect(screen.queryByRole("textbox")).toBeNull()
})

test("the agent's notes render as markdown", () => {
  const handoff = "- Committed **abc**\n- Removed the row"
  renderThemed(<TicketReading detail={{ ...base, latest: { ...base.latest!, handoff } }} />)
  fireEvent.click(screen.getByRole("button", { name: /Agent's notes/ }))
  expect(screen.getAllByRole("listitem").map((li) => li.textContent)).toEqual([
    "Committed abc",
    "Removed the row",
  ])
  expect(screen.getByText("abc").tagName).toBe("STRONG")
})
