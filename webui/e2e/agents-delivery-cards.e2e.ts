import type { Page } from "@playwright/test"
import { expect, nav, test, type Fixture } from "./fixture"
import { chatOf, freshVisit, go, messageBox, sendButton, sessions } from "./agentsKit"

// The first prompts of the three chats of deliver-cards (catalog.DeliverCardChats).
const CHATS = ["not-sent chat", "send-failed chat", "unknown-command chat"]
const KINDS = ["maybe-sent", "send-failed", "unknown-command"]

const countOf = (text: string, part: string) => text.split(part).length - 1

/** The session and conversation id of the chat whose first prompt is `name`. */
async function chatNamed(fixture: Fixture, name: string) {
  for (const s of await sessions(fixture)) {
    const chat = (await chatOf(fixture, s.id)) as {
      messages: { text?: string }[]
      conversation_id: string
    }
    if (chat.messages.some((m) => m.text?.includes(name)))
      return { id: s.id, conv: chat.conversation_id }
  }
  throw new Error(`no chat holds ${name}`)
}

const kindOf = async (fixture: Fixture, id: string) =>
  (await sessions(fixture)).find((s) => s.id === id)!.actionable.kind ?? ""

const card = (page: Page, title: string) => page.getByRole("region", nav(title))
const button = (page: Page, name: string) => page.getByRole("button", { name, exact: true })

// The clipboard, the second tab and the reload need a real browser; the card components are covered by vitest.
test("212. Delivery cards: Copy, Show screen, Dismiss, Restart agent and a later good send", async ({
  page,
  context,
  start,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"])
  const fixture = await start("deliver-cards")
  await page.setViewportSize({ width: 1280, height: 800 })
  const chats = await Promise.all(CHATS.map((name) => chatNamed(fixture, name)))
  const [maybe, failed, unknown] = chats

  // Each chat takes its control, then a send that ends in its card.
  const sends: [string, string, string][] = [
    ["undecided", "/model opus", maybe.conv],
    ["stall", "do the thing", failed.conv],
    ["unknownskill", "/my-skill", unknown.conv],
  ]
  for (const [i, [action, text, conv]] of sends.entries()) {
    await fixture.controlClaude(conv, { action })
    void fixture.cli("agents", "prompt", chats[i].id, text).catch(() => {})
  }
  for (const [i, kind] of KINDS.entries())
    await expect.poll(() => kindOf(fixture, chats[i].id), { timeout: 30_000 }).toBe(kind)

  await test.step("may not have been sent: Copy, Show screen, no Resend, Dismiss", async () => {
    await go(page, `#/agents/${maybe.id}`)
    const c = card(page, "May not have been sent")
    await expect(c).toBeVisible()
    await expect(c.getByRole("button", { name: "Resend", exact: true })).toHaveCount(0)
    await c.getByRole("button", { name: "Copy", exact: true }).click()
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe("/model opus")
    await c.getByRole("button", { name: "Show screen", exact: true }).click()
    await expect(page.getByRole("dialog", nav("Screen"))).toBeVisible()
    await page.keyboard.press("Escape")
    await expect(page.getByRole("dialog", nav("Screen"))).toHaveCount(0)

    await c.getByRole("button", { name: "Dismiss", exact: true }).click()
    await expect(c).toHaveCount(0)
    // The harness allows no reload of the open page: a fresh tab is a second load of the same server state.
    const second = await freshVisit(page, `/#/agents/${maybe.id}`)
    await expect(messageBox(second)).toBeVisible()
    await expect(card(second, "May not have been sent")).toHaveCount(0)
    await second.close()
    // The session has no card now: the CLI verb refuses it.
    await expect(fixture.cli("agents", "dismiss", maybe.id)).rejects.toThrow()
  })

  await test.step("send failed: a later good send clears it", async () => {
    await go(page, `#/agents/${failed.id}`)
    const c = card(page, "Send failed")
    await expect(c).toBeVisible()
    await expect(c.getByRole("button", { name: "Resend", exact: true })).toHaveCount(0)
    // The fake's stall lasts until its process ends, so a new process stands for the agent that works again.
    await fixture.cli("agents", "stop", failed.id)
    await fixture.cli("agents", "resume", failed.id)
    await expect
      .poll(async () => (await sessions(fixture)).find((s) => s.id === failed.id)!.status.state)
      .toBe("idle")
    await messageBox(page).fill("a good send")
    await sendButton(page).click()
    await expect(c).toHaveCount(0)
    await expect.poll(() => kindOf(fixture, failed.id)).toBe("")
  })

  await test.step("unknown command: Restart agent keeps the card, Resend lands the command", async () => {
    await go(page, `#/agents/${unknown.id}`)
    const c = card(page, "Send failed")
    await expect(c).toBeVisible()
    await c.getByRole("button", { name: "Restart agent", exact: true }).click()
    await expect(c.getByText("Restart the agent?")).toBeVisible()
    await c.getByRole("button", { name: "Keep", exact: true }).click()
    await expect(c.getByText("Restart the agent?")).toHaveCount(0)
    // The card's own Restart stops the agent, but its resume is refused as stale (follow-up in Spec deviations), so
    // the CLI stands in for the resume: a new fake process takes the command the old one rejected.
    await fixture.cli("agents", "stop", unknown.id)
    await fixture.cli("agents", "resume", unknown.id)
    await expect
      .poll(async () => (await sessions(fixture)).find((s) => s.id === unknown.id)!.status.state)
      .toBe("idle")
    await expect(c).toBeVisible()
    expect(await kindOf(fixture, unknown.id)).toBe("unknown-command")
    await button(page, "Resend").click()
    await expect(c).toHaveCount(0)
    await expect
      .poll(async () => countOf(await fixture.cli("agents", "chat", unknown.id), "/my-skill"))
      .toBe(1)
  })
})
