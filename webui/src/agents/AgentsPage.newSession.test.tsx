import { fireEvent, screen } from "@testing-library/react"
import { expect, test, vi } from "vitest"
import {
  createBodies,
  entryOptions,
  render,
  session,
  stubNewSession,
  useAgentsPageHooks,
} from "./agentsTestKit"

useAgentsPageHooks()

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
  ["Pick a session", [session("s-1", "p")]],
  ["No sessions yet", []],
])("the %s panel's New chat button opens the form", async (text, sessions) => {
  stubNewSession()
  render(sessions, "/agents", true, entryOptions)
  expect(await screen.findByText(text)).toBeTruthy()
  fireEvent.click(screen.getByRole("button", { name: "New chat" }))
  expect(await screen.findByRole("dialog", { name: "New chat" })).toBeTruthy()
})

test("the Agents page has no session list, switcher or Queue/Design toggles", () => {
  render([session("s-1", "p", { name: "plain" })], "/agents/s-1", true, entryOptions)
  expect(screen.queryByRole("button", { name: "New chat in p" })).toBeNull()
  expect(screen.queryByRole("button", { name: /plain/ })).toBeNull()
  expect(screen.queryByRole("button", { name: "Queue" })).toBeNull()
  expect(screen.queryByRole("button", { name: "Design" })).toBeNull()
})
