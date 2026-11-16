import { act, cleanup, fireEvent, screen } from "@testing-library/react"
import { afterEach, describe, expect, it } from "vitest"
import { FileLinkProvider } from "../files/FileLinkContext"
import { stubResolver } from "../files/stubResolver.testutil"
import type { SessionChatMessage, SessionChatResponse } from "../generated/server"
import { renderThemed } from "../ui/render"
import { ChatMessages } from "./ChatCore"

afterEach(cleanup)

const msg = (n: number): SessionChatMessage => ({
  id: `m${n}`,
  role: "agent",
  kind: "text",
  text: `message number ${n}`,
  at: new Date().toISOString(),
})

const chatOf = (count: number): SessionChatResponse => ({
  conversation_id: "c1",
  messages: Array.from({ length: count }, (_, i) => msg(i + 1)),
  questions: [],
  background: [],
})

const shown = (c: HTMLElement) => c.textContent?.match(/message number \d+/g) ?? []

describe("the chat window", () => {
  it("mounts only the newest messages and grows on request", () => {
    const { container } = renderThemed(
      <ChatMessages chat={chatOf(150)} busy={false} sessionId="s1" />,
    )
    expect(shown(container)).toHaveLength(60)
    expect(container.textContent).toContain("message number 150")
    expect(container.textContent).not.toContain("message number 90 ")
    fireEvent.click(screen.getByRole("button", { name: /earlier messages \(90 hidden\)/ }))
    expect(shown(container)).toHaveLength(120)
    fireEvent.click(screen.getByRole("button", { name: /earlier messages \(30 hidden\)/ }))
    expect(shown(container)).toHaveLength(150)
    expect(screen.queryByRole("button", { name: /earlier messages/ })).toBeNull()
  })

  it("shows a short chat whole, with no button", () => {
    const { container } = renderThemed(
      <ChatMessages chat={chatOf(60)} busy={false} sessionId="s1" />,
    )
    expect(shown(container)).toHaveLength(60)
    expect(screen.queryByRole("button", { name: /earlier messages/ })).toBeNull()
  })

  it("still links a path once its verdict arrives (Markdown reads the stable on-flag)", async () => {
    const { resolver } = stubResolver(["a/b.go"])
    const { container } = renderThemed(
      <FileLinkProvider resolver={resolver}>
        <ChatMessages
          chat={{
            ...chatOf(0),
            messages: [{ ...msg(1), text: "see a/b.go" }],
          }}
          busy={false}
          sessionId="s1"
        />
      </FileLinkProvider>,
    )
    await act(async () => {})
    expect(container.querySelectorAll("a")).toHaveLength(1)
  })
})
