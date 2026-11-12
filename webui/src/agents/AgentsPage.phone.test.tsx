import { fireEvent, screen, within } from "@testing-library/react"
import { expect, test } from "vitest"
import { roundCards } from "../design/roundFixture"
import type { SessionFailure, SessionRoundCard } from "../generated/server"
import {
  chat,
  chatOf,
  entryOptions,
  render,
  session,
  stubNewSession,
  useAgentsPageHooks,
} from "./agentsTestKit"

useAgentsPageHooks()

test("phone: #/agents never redirects to the last session", () => {
  localStorage.setItem("gx.agents.last", "s-1")
  const r = render([session("s-1", "p")], "/agents", true, {}, false)
  expect(r.router.state.location.pathname).toBe("/agents")
})

test("phone: a session shows its title and project · status", () => {
  render([session("s-1", "gx", { name: "plain" })], "/agents/s-1", true, {}, false)
  expect(screen.getByRole("heading", { name: "plain" })).toBeTruthy()
  expect(screen.getByText("Chat · gx · Idle")).toBeTruthy()
})

test("phone: a design session's header links to its design page", () => {
  render(
    [session("s-1", "p", { design: true, ticket: "gx:map/01" })],
    "/agents/s-1",
    true,
    {},
    false,
  )
  expect(screen.getByRole("link", { name: "Open design page →" }).getAttribute("href")).toBe(
    "#/d/gx:map",
  )
})

test("phone: a plain session's header has no design link", () => {
  render([session("s-2", "p")], "/agents/s-2", true, {}, false)
  expect(screen.queryByRole("link", { name: "Open design page →" })).toBeNull()
})

test("phone: a round shows the sheet beside the banner, with no sideways scroll", async () => {
  const round = [
    {
      label: "1",
      title: "Which?",
      body: "Which?",
      kind: "options",
      options: [{ id: "a", label: "A" }],
    },
  ] as unknown as SessionRoundCard[]
  const failure = { kind: "stalled", label: "Failure stalled", text: "" } as SessionFailure
  render(
    [session("s-1", "p", { failure })],
    "/agents/s-1",
    true,
    chatOf(chat({ questions: roundCards(round) })),
    false,
  )
  expect(await screen.findByRole("group", { name: "Questions" })).toBeTruthy()
  expect(screen.getByText("Failure stalled")).toBeTruthy()
  expect(screen.getByRole("button", { name: "Screen" })).toBeTruthy()
  expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
    document.documentElement.clientWidth,
  )
})

test("phone: the empty panel's New session button opens the form as a sheet", async () => {
  stubNewSession()
  render([session("s-1", "gx", { name: "plain" })], "/agents", true, entryOptions, false)
  fireEvent.click(screen.getByRole("button", { name: "New session" }))
  const form = await screen.findByRole("dialog", { name: "New session" })
  expect(within(form).getByTestId("grab-handle")).toBeTruthy()
})
