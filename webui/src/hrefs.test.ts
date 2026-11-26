import { expect, test } from "vitest"
import { backHref, parseAgentSplat, subagentHref, subagentOrigin } from "./hrefs"

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
  expect(subagentHref("s-1", "c", "/d/demo:x")).toBe(
    "#/agents/s-1/subagents/c?from=%2Fd%2Fdemo%3Ax",
  )
  expect(subagentOrigin("?from=%2Fd%2Fdemo%3Ax")).toBe("/d/demo:x")
  expect(subagentOrigin("")).toBeUndefined()
})

test.each(["/agents/s-2", "/d/demo:x", "/triage/demo"])("back goes to the origin %s", (origin) => {
  expect(backHref("s-1", origin)).toBe(`#${origin}`)
})

test.each([
  undefined,
  "",
  "https://example.com/d/x",
  "//example.com/d/x",
  "javascript:alert(1)",
  "/agents/s-2/subagents/c",
  "/agents",
  "/epics",
  "/d/x/y",
  "/triage/demo?x=1",
])("back goes to the chat page for the origin %s", (origin) => {
  expect(backHref("s-1", origin)).toBe("#/agents/s-1")
})
