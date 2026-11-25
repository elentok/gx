import { afterEach, expect, test, vi } from "vitest"
import { fetchSubagentChat } from "./chat"

afterEach(() => {
  vi.restoreAllMocks()
})

test("fetchSubagentChat sends both parameters, encoded", async () => {
  const spy = vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValue(new Response(JSON.stringify({ conversation_id: "c1" })))
  const r = await fetchSubagentChat("s a", "call/1")
  expect(String(spy.mock.calls[0][0])).toContain(
    "/v1/sessions/chat?session=s%20a&subagent=call%2F1",
  )
  expect(r).toEqual({ kind: "ok", detail: { conversation_id: "c1" } })
})

test("fetchSubagentChat keeps the message of a 404", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    new Response("no such subagent\n", { status: 404 }),
  )
  expect(await fetchSubagentChat("s", "c")).toEqual({
    kind: "not-found",
    message: "no such subagent",
  })
})
