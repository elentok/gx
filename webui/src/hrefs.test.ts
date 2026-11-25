import { expect, test } from "vitest"
import { parseAgentSplat, subagentHref } from "./hrefs"

test("a plain session splat has no call id", () => {
  expect(parseAgentSplat("s-1")).toEqual({ session: "s-1" })
  expect(parseAgentSplat("s-1/subagents/")).toEqual({ session: "s-1" })
})

test("a subagent splat splits into session id and call id", () => {
  expect(parseAgentSplat("s-1/subagents/call-1")).toEqual({ session: "s-1", callId: "call-1" })
})

test("a call id the router decoded stays whole, slashes included", () => {
  expect(parseAgentSplat("s-1/subagents/a b/c")).toEqual({ session: "s-1", callId: "a b/c" })
  expect(subagentHref("s-1", "a b")).toBe("#/agents/s-1/subagents/a%20b")
})
