import { act, cleanup, screen, waitFor } from "@testing-library/react"
import { expect, test, vi } from "vitest"
import { chat, chatOf, focusPosts, installAgentsPageHooks, render, session } from "./agentsTestKit"

installAgentsPageHooks()

const subChat = (callId: string, text = "from the subagent", reason?: string) => ({
  ...chat({
    messages: [{ id: "m1", role: "agent", kind: "text", text, at: "2026-01-01T00:00:00Z" }],
  }),
  subagent: {
    call_id: callId,
    root_session_id: "s-1",
    title: `Title ${callId}`,
    status: reason ? "running" : "done",
    unavailable: reason,
    prompt: "do it",
  },
})

const noWatch = async () => {}

test("the subagent route shows the subagent page and the focus report names the session", async () => {
  const subagentChat = vi.fn(async () => ({ kind: "ok" as const, detail: subChat("call-1") }))
  render([session("s-1", "p", { name: "one" })], "/agents/s-1/subagents/call-1", true, {
    subagentChat,
    subagentWatch: noWatch,
  })
  await screen.findByText("from the subagent")
  expect(screen.queryByText(/Session not found/)).toBeNull()
  expect(subagentChat).toHaveBeenCalledWith("s-1", "call-1")
  await waitFor(() => expect(focusPosts.some((p) => p.session === "s-1")).toBe(true))
  expect(focusPosts.every((p) => !p.session.includes("subagents"))).toBe(true)
})

test("an unknown session at a subagent URL shows the gone notice", async () => {
  render([session("s-2", "p")], "/agents/s-1/subagents/call-1", true, {
    subagentChat: async () => ({ kind: "ok" as const, detail: subChat("call-1") }),
    subagentWatch: noWatch,
  })
  await screen.findByText("Agent not found")
})

test("one call's detail never shows under another call id", async () => {
  let release: (v: unknown) => void = () => {}
  const subagentChat = vi.fn((_s: string, callId: string) =>
    callId === "call-1"
      ? Promise.resolve({ kind: "ok" as const, detail: subChat("call-1", "first call") })
      : new Promise((r) => {
          release = r
        }),
  )
  const { router } = render([session("s-1", "p")], "/agents/s-1/subagents/call-1", true, {
    subagentChat: subagentChat as never,
    subagentWatch: noWatch,
  })
  await screen.findByText("first call")
  await act(() => router.navigate("/agents/s-1/subagents/call-2"))
  expect(screen.queryByText("first call")).toBeNull()
  await act(async () => release({ kind: "ok", detail: subChat("call-2", "second call") }))
  await screen.findByText("second call")
})

test("a session event and the call's own subagent event refetch, another call's does not", async () => {
  let text = "v1"
  const subagentChat = vi.fn(async () => ({ kind: "ok" as const, detail: subChat("call-1", text) }))
  const { emit } = render([session("s-1", "p")], "/agents/s-1/subagents/call-1", true, {
    subagentChat,
    subagentWatch: noWatch,
  })
  await screen.findByText("v1")
  text = "v2"
  emit("s-1", "session-subagent-changed", { call: "call-9" })
  expect(subagentChat).toHaveBeenCalledTimes(1)
  emit("s-1", "session-subagent-changed", { call: "call-1" })
  await screen.findByText("v2")
  text = "v3"
  emit("s-1", "session-chat-changed")
  await screen.findByText("v3")
})

test("a Starting page fills in on the subagent event alone", async () => {
  let current = subChat("call-1", "x", "starting")
  const { emit } = render([session("s-1", "p")], "/agents/s-1/subagents/call-1", true, {
    subagentChat: async () => ({ kind: "ok" as const, detail: current }),
    subagentWatch: noWatch,
  })
  await screen.findByText("Starting…")
  current = subChat("call-1", "now it talks")
  emit("s-1", "session-subagent-changed", { call: "call-1" })
  await screen.findByText("now it talks")
})

test("the watch starts on mount, repeats when streams rise and ends on unmount", async () => {
  const posts: { call_id: string; closed?: boolean }[] = []
  const { update } = render([session("s-1", "p")], "/agents/s-1/subagents/call-1", true, {
    subagentChat: async () => ({ kind: "ok" as const, detail: subChat("call-1") }),
    subagentWatch: async (r) => {
      posts.push(r)
    },
  })
  await waitFor(() => expect(posts).toEqual([{ client: "c1", session: "s-1", call_id: "call-1" }]))
  update({ streams: 2 })
  await waitFor(() => expect(posts).toHaveLength(2))
  cleanup()
  expect(posts.at(-1)).toMatchObject({ call_id: "call-1", closed: true })
})

test("Back returns to the page the subagent page was opened from", async () => {
  const { router } = render([session("s-1", "p")], "/agents/s-1", true, {
    ...chatOf(chat()),
    subagentChat: async () => ({ kind: "ok" as const, detail: subChat("call-1") }),
    subagentWatch: noWatch,
  })
  await act(() => router.navigate("/agents/s-1/subagents/call-1"))
  await screen.findByText("from the subagent")
  await act(() => router.navigate(-1))
  expect(router.state.location.pathname).toBe("/agents/s-1")
})
