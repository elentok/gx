import { useState } from "react"
import { act, cleanup } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"
import { FileLinkProvider } from "../files/FileLinkContext"
import type { ResolveBase } from "../files/resolve"
import { stubResolver } from "../files/stubResolver.testutil"
import type { SessionChatMessage, SessionChatResponse } from "../generated/server"
import { renderThemed } from "../ui/render"
import { ChatMessages } from "./ChatCore"

afterEach(cleanup)

const bases: ResolveBase[] = []
vi.mock("../files/resolve", async (orig) => {
  const real = await orig<typeof import("../files/resolve")>()
  return {
    ...real,
    createResolver: (base: ResolveBase) => {
      bases.push(base)
      return real.createResolver(base, (() => {
        throw new Error("no network in this test")
      }) as unknown as typeof fetch)
    },
  }
})

const msg = (id: string, over: Partial<SessionChatMessage>): SessionChatMessage => ({
  id,
  role: "agent",
  kind: "text",
  text: "",
  at: new Date().toISOString(),
  ...over,
})

const chatOf = (messages: SessionChatMessage[]): SessionChatResponse => ({
  conversation_id: "c1",
  messages,
  questions: [],
})

const settle = () => act(async () => {})
const hrefs = (c: HTMLElement) =>
  Array.from(c.querySelectorAll("a")).map((a) => a.getAttribute("href"))

const base = [
  msg("m1", { text: "Edit `server/files/handler.go` and nope/x.go" }),
  msg("m2", {
    kind: "tools",
    calls: ["Read: server/files/handler.go", "Read(server/files/handler.go)"],
    tool_count: 2,
  }),
  msg("m3", { role: "user", kind: "prompt", text: "look at server/files/handler.go\nthen stop" }),
  msg("m4", { role: "user", kind: "command", text: "/open server/files/handler.go" }),
  msg("m5", { kind: "output", text: "server/files/handler.go:3: oops" }),
]

describe("file links in the chat", () => {
  it("links found paths in text, tool lines and prompts, and nothing in commands or output", async () => {
    const { resolver } = stubResolver(["server/files/handler.go"])
    const { container } = renderThemed(
      <FileLinkProvider resolver={resolver}>
        <ChatMessages chat={chatOf(base)} busy={false} sessionId="s1" showTools />
      </FileLinkProvider>,
    )
    await settle()
    // The tool lines mount when their run is opened.
    expect(hrefs(container)).toHaveLength(2)
    const details = container.querySelector("[data-testid=tool-calls]") as HTMLDetailsElement
    act(() => {
      details.open = true
      details.dispatchEvent(new Event("toggle"))
    })
    await settle()
    const wanted = "#/files/abs/server/files/handler.go"
    expect(hrefs(container)).toEqual([wanted, wanted, wanted, wanted])
    expect(container.textContent).toContain("nope/x.go")
    expect(container.querySelector("[data-mock-part=chat-text]")?.textContent).toBe(
      "look at server/files/handler.go\nthen stop",
    )
  })

  it("shows no tool lines, so no tool links, without showTools", async () => {
    const { resolver } = stubResolver(["server/files/handler.go"])
    const { container } = renderThemed(
      <FileLinkProvider resolver={resolver}>
        <ChatMessages chat={chatOf(base)} busy={false} sessionId="s1" />
      </FileLinkProvider>,
    )
    await settle()
    expect(hrefs(container)).toHaveLength(2)
    expect(container.querySelector("[data-testid=tool-calls]")).toBeNull()
  })

  it("makes one call per load and one per new message, rechecking the missing ones", async () => {
    const { resolver, calls } = stubResolver(["server/files/handler.go"])
    let append: () => void = () => {}
    function Page() {
      const [msgs, set] = useState(base)
      append = () => set([...msgs, msg("m6", { text: "also new/file.go" })])
      return (
        <FileLinkProvider resolver={resolver} recheckKey={msgs.at(-1)?.id}>
          <ChatMessages chat={chatOf(msgs)} busy={false} sessionId="s1" showTools />
        </FileLinkProvider>
      )
    }
    renderThemed(<Page />)
    await settle()
    expect(calls).toHaveLength(1)
    act(() => append())
    await settle()
    expect(calls).toHaveLength(2)
    const missingBefore = calls[0].candidates.filter((c) => c !== "server/files/handler.go")
    expect(missingBefore.length).toBeGreaterThan(0)
    expect(calls[1].candidates).toEqual(expect.arrayContaining([...missingBefore, "new/file.go"]))
    expect([...(calls[1].recheckMissing ?? [])].sort()).toEqual([...missingBefore].sort())
  })

  it("keeps the session id as the resolve base on the Design and Agents pages", async () => {
    bases.length = 0
    const { AgentChat } = await import("../agents/AgentChat")
    renderThemed(
      <AgentChat
        session={
          {
            id: "s-a",
            dir: "/x",
            status: { state: "idle" },
          } as unknown as Parameters<typeof AgentChat>[0]["session"]
        }
        chat={chatOf(base)}
        refetch={() => {}}
      />,
    )
    await settle()
    expect(bases.at(-1)).toEqual({ session: "s-a", project: undefined, dir: undefined })
  })
})
