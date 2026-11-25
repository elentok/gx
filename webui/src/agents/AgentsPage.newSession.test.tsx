import { act, fireEvent, screen } from "@testing-library/react"
import { expect, test, vi } from "vitest"
import {
  createBodies,
  entryOptions,
  render,
  session,
  stubNewSession,
  installAgentsPageHooks,
} from "./agentsTestKit"

installAgentsPageHooks()

test("a create from the New chat form opens the new session", async () => {
  stubNewSession(session("q-9", "q"))
  const { router } = render([session("s-1", "q")], "/agents", true, entryOptions)
  fireEvent.click(await screen.findByRole("button", { name: "New chat" }))
  await screen.findByRole("combobox", { name: "Project" })
  fireEvent.click(screen.getByRole("button", { name: "Create chat" }))
  await vi.waitFor(() => expect(router.state.location.pathname).toBe("/agents/q-9"))
  expect(createBodies).toHaveLength(1)
  expect(screen.queryByRole("dialog", { name: "New chat" })).toBeNull()
})

test("closing the New chat form stays on the page", async () => {
  stubNewSession()
  const { router } = render([session("s-1", "q")], "/agents", true, entryOptions)
  fireEvent.click(await screen.findByRole("button", { name: "New chat" }))
  await screen.findByRole("combobox", { name: "Project" })
  fireEvent.click(screen.getByRole("button", { name: "Close" }))
  await vi.waitFor(() => expect(screen.queryByRole("dialog", { name: "New chat" })).toBeNull())
  expect(router.state.location.pathname).toBe("/agents")
  expect(createBodies).toHaveLength(0)
})

test.each([
  ["Pick a chat", [session("s-1", "p")]],
  ["No chats yet", []],
])("the %s panel's New chat button opens the form", async (text, sessions) => {
  stubNewSession()
  render(sessions, "/agents", true, entryOptions)
  expect(await screen.findByText(text)).toBeTruthy()
  fireEvent.click(screen.getByRole("button", { name: "New chat" }))
  expect(await screen.findByRole("dialog", { name: "New chat" })).toBeTruthy()
})

test("a just-created chat shows Starting chat… until it is listed, never Agent not found", async () => {
  const { setSessions } = render([], { pathname: "/agents/q-9", state: { creating: true } }, true)
  expect(await screen.findByRole("status", { name: "Starting chat…" })).toBeTruthy()
  expect(screen.queryByText("Agent not found")).toBeNull()
  setSessions([session("q-9", "q")])
  await vi.waitFor(() =>
    expect(screen.queryByRole("status", { name: "Starting chat…" })).toBeNull(),
  )
  expect(screen.queryByText("Agent not found")).toBeNull()
})

test("a just-created chat still missing after 10 s shows Agent not found", async () => {
  vi.useFakeTimers()
  try {
    render([], { pathname: "/agents/q-9", state: { creating: true } }, true)
    await act(async () => vi.advanceTimersByTimeAsync(9_000))
    expect(screen.queryByText("Agent not found")).toBeNull()
    await act(async () => vi.advanceTimersByTimeAsync(1_500))
    expect(screen.getByText("Agent not found")).toBeTruthy()
  } finally {
    vi.useRealTimers()
  }
})

test("a gone id without creating state shows Agent not found at once", async () => {
  render([], "/agents/q-9", true)
  expect(await screen.findByText("Agent not found")).toBeTruthy()
})

test("the Agents page has no session list, switcher or Queue/Design toggles", () => {
  render([session("s-1", "p", { name: "plain" })], "/agents/s-1", true, entryOptions)
  expect(screen.queryByRole("button", { name: "New chat in p" })).toBeNull()
  expect(screen.queryByRole("button", { name: /plain/ })).toBeNull()
  expect(screen.queryByRole("button", { name: "Queue" })).toBeNull()
  expect(screen.queryByRole("button", { name: "Design" })).toBeNull()
})
