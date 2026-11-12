import { cleanup, fireEvent, screen } from "@testing-library/react"
import { describe, expect, test, vi } from "vitest"
import {
  accepted,
  chat,
  chatOf,
  composerBox,
  prompts,
  refused,
  render,
  session,
  stubPrompt,
  type,
  useAgentsPageHooks,
} from "./agentsTestKit"

useAgentsPageHooks()

const noItem = () =>
  expect(screen.queryByRole("menuitem", { name: /Promote to Design/ })).toBeNull()
const noButton = () =>
  expect(screen.queryByRole("button", { name: /Promote to Design/ })).toBeNull()
const disabled = (el: HTMLElement) =>
  el.hasAttribute("data-disabled") || el.getAttribute("aria-disabled") === "true"

// The item lives in the "More actions" menu, which opens on click.
const findPromote = async () => {
  fireEvent.click(await screen.findByRole("button", { name: "More actions" }))
  return screen.findByRole("menuitem", { name: /Promote to Design/ })
}

// wide=true is the wide layout, false the phone one.
describe.each([true, false])("wide=%s", (wide) => {
  const open = (s = session("s-1", "p"), c = chat()) =>
    render([s], "/agents/s-1", true, chatOf(c), wide)

  test("a plain session with messages shows an enabled item and no header button", async () => {
    open()
    await screen.findByText("hello there")
    noButton()
    expect(disabled(await findPromote())).toBe(false)
  })

  test("a Design agent and a Queue agent show no item", async () => {
    open(session("s-1", "p", { design: true }))
    await composerBox()
    noButton()
    fireEvent.click(screen.queryByRole("button", { name: "More actions" }) ?? document.body)
    noItem()
    cleanup()
    open(session("s-1", "p", { queue: true }))
    await composerBox()
    noButton()
    fireEvent.click(screen.queryByRole("button", { name: "More actions" }) ?? document.body)
    noItem()
  })

  test("it is disabled with a reason while the chat has no message and while the composer cannot send", async () => {
    open(session("s-1", "p"), chat({ messages: [] }))
    expect(disabled(await findPromote())).toBe(true)
    expect(
      screen.getByText("No messages yet. Promote turns on after the first message."),
    ).toBeTruthy()
    cleanup()
    open(session("s-1", "p", { status: { state: "working" } }))
    await screen.findByText("hello there")
    expect(disabled(await findPromote())).toBe(true)
    expect(
      screen.getAllByText("Working. Send turns on when the session is idle.").length,
    ).toBeGreaterThan(0)
  })

  test("it is disabled with the exited reason for an exited session", async () => {
    open(session("s-1", "p", { status: { state: "exited" } }))
    expect(disabled(await findPromote())).toBe(true)
    expect(screen.getAllByText("Exited. Restore it in gx, Agents tab.").length).toBeGreaterThan(0)
  })

  test("a pick posts promote with the last message and leaves the draft alone", async () => {
    prompts.length = 0
    stubPrompt(accepted)
    open()
    await type("half written")
    fireEvent.click(await findPromote())
    await vi.waitFor(() =>
      expect(prompts).toEqual([
        expect.objectContaining({ id: "s-1", promote: true, last_message: expect.any(String) }),
      ]),
    )
    expect((await composerBox()).value).toBe("half written")
    expect(localStorage.getItem("gx.draft.composer.s-1")).toBe("half written")
    expect(screen.queryByRole("dialog")).toBeNull()
  })

  test("a refusal shows its reason", async () => {
    stubPrompt(refused("stale", "the conversation moved"))
    open()
    fireEvent.click(await findPromote())
    expect(await screen.findByText(/conversation moved|stale/i)).toBeTruthy()
  })
})
