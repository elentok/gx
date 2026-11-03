import type { Page } from "@playwright/test";
import { expect, nav, test, type Fixture } from "./fixture";
import { chatOf, claudeIdle, codexIdle, go, messageBox, sendButton, sessions } from "./agentsKit";

/** The conversation id the fake claude of a session is named by. */
const conversationOf = async (fixture: Fixture, id: string) => (await chatOf(fixture, id)).conversation_id;

const countOf = (text: string, part: string) => text.split(part).length - 1;

// 153 "Refused" was deleted as a duplicate of 78 Send refused; its checks live in AgentsPage.composer.test.tsx
// ("a refusal keeps the draft", "stale refetches") and NewSessionForm.test.tsx ("a refusal shows in the form").
// 153 "Working" is the vitest test "Send is off while Working" (AgentsPage.composer.test.tsx).
test("153. Send a command, typed once or nudged, and /clear", async ({ page, start }) => {
  const fixture = await start("agents-commands");
  await page.setViewportSize({ width: 1280, height: 800 });

  // `/cost` is typed once: one Enter in the fake, the pill and its output in the chat, no failure.
  const cost = await claudeIdle(fixture);
  const costConv = await conversationOf(fixture, cost.id);
  await go(page, `#/agents/${cost.id}`);
  await fixture.controlClaude(costConv, { action: "commandoutput", message: "total cost: $1.00" });
  await messageBox(page).fill("/cost");
  await sendButton(page).click();
  // A command that starts no turn changes no status, so no event reaches the open chat: leave and come back.
  await expect.poll(async () => (await chatOf(fixture, cost.id) as { messages: unknown[] }).messages.length).toBe(4);
  await go(page, "#/agents");
  await go(page, `#/agents/${cost.id}`);
  await expect(page.getByTestId("command-output")).toHaveText("total cost: $1.00");
  await expect(page.getByText("/cost", { exact: true })).toBeVisible();
  expect(countOf(await fixture.input(costConv), "/cost")).toBe(1);
  await expect(page.getByText("Send failed")).toHaveCount(0);
  expect((await sessions(fixture)).find((s) => s.id === cost.id)!.actionable.reason ?? "").toBe("");

  // `/compact foo` starts a turn, so it keeps the nudge path: the session goes Working.
  const compact = await claudeIdle(fixture, 1);
  const compactConv = await conversationOf(fixture, compact.id);
  await go(page, `#/agents/${compact.id}`);
  await messageBox(page).fill("/compact foo");
  await sendButton(page).click();
  await expect.poll(async () => (await sessions(fixture)).find((s) => s.id === compact.id)!.status.state).toBe("working");
  expect(countOf(await fixture.input(compactConv), "/compact foo")).toBe(1);

  // A Codex `$` skill is plain text for the agent, so it takes the nudge path too: the prompt reaches the chat.
  const codex = await codexIdle(fixture);
  await go(page, `#/agents/${codex.id}`);
  await messageBox(page).fill("$review-skill");
  await sendButton(page).click();
  await expect(page.getByText("$review-skill", { exact: true })).toBeVisible();
  await expect.poll(() => fixture.cli("agents", "chat", codex.id)).toContain("$review-skill");

  // `/clear` on the Idle session starts a new conversation: the open chat resets and a draft typed since stays.
  const withOutput = await findWithOutput(fixture);
  await go(page, `#/agents/${withOutput.id}`);
  await expect(page.getByTestId("command-output")).toBeVisible();
  await messageBox(page).fill("/clear");
  await sendButton(page).click();
  await expect(messageBox(page)).toHaveValue("");
  await messageBox(page).fill("a draft to keep");
  await expect(page.getByTestId("command-output")).toHaveCount(0);
  await expect(page.getByText("/cost", { exact: true })).toHaveCount(0);
  await expect(messageBox(page)).toHaveValue("a draft to keep");
});

/** The seeded session whose chat holds the `/cost` output. */
async function findWithOutput(fixture: Fixture) {
  for (const s of await sessions(fixture)) {
    if (s.harness === "codex" || s.design || s.queue) continue;
    const chat = (await chatOf(fixture, s.id)) as { messages: { kind: string }[] };
    if (chat.messages.some((m) => m.kind === "output")) return s;
  }
  throw new Error("no session holds command output");
}

// ---- 154

const designOf = async (fixture: Fixture, ticket: string) => (await sessions(fixture)).find((s) => s.ticket === ticket)!;
const designInfo = async (fixture: Fixture, id: string) => (await sessions(fixture)).find((s) => s.id === id)!;

const BASH_DIALOG = (command: string) => ["────────────────────", "Bash command", `  ${command}`, "Do you want to proceed?", "❯ 1. Yes", "  2. No", "Esc to cancel"];

/** Lets the design agent end a run, which leaves a Continue. Its first round must have ended. */
async function endRun(fixture: Fixture, id: string) {
  await expect.poll(async () => (await designInfo(fixture, id)).status.state).toBe("idle");
  await expect
    .poll(() => fixture.cli("agents", "prompt", id, "carry on").then(() => true, () => false))
    .toBe(true);
  await expect.poll(async () => (await designInfo(fixture, id)).actionable.reason ?? "").not.toBe("");
}

/** Ends a run with auto-advance on, which starts the countdown; one already running stays. */
async function startCountdown(fixture: Fixture, id: string) {
  if ((await designInfo(fixture, id)).auto_advance_at) return;
  await endRun(fixture, id);
  await expect.poll(async () => (await designInfo(fixture, id)).auto_advance_at).toBeTruthy();
}

const openNote = async (page: Page) => {
  const box = messageBox(page);
  // A note with text stays open across sends, and Note toggles it.
  if (!(await box.isVisible())) await page.getByRole("button", nav("Note")).click();
  await expect(box).toBeEnabled();
  return box;
};

const countdownText = (page: Page) => page.getByText(/^Continuing in \d+s$/);

test("154. Commands in a design session, /clear and /compact; keys and Type; Close dialog, Allow once and Deny", async ({ page, start }) => {
  const fixture = await start("agents-commands", { autoAdvanceDelay: "1h" });
  await page.setViewportSize({ width: 1280, height: 800 });
  const design = await designOf(fixture, "demo:design-auto");
  const conv = await conversationOf(fixture, design.id);

  for (const command of ["/clear", "/compact"]) {
    await startCountdown(fixture, design.id);
    const deadline = (await designInfo(fixture, design.id)).auto_advance_at;
    await go(page, "#/d/demo:design-auto");
    await expect(countdownText(page)).toBeVisible();
    const box = await openNote(page);
    await box.fill(command);
    await page.getByRole("button", { name: "Send", exact: true }).click();
    // The deadline moves on: /clear leaves none, and /compact's turn ends with a run-end that starts a new one.
    await expect.poll(async () => (await designInfo(fixture, design.id)).auto_advance_at).not.toBe(deadline);
    // As typed text does, a command stops only the countdown: the run-end Continue stays for the person to press.
    expect((await designInfo(fixture, design.id)).actionable.reason).toBeTruthy();
  }

  // The key bar and Type field send through the keys route; the Screen view that holds them opens on a design
  // session only while it is Blocked or stalled, and leaving that state starts a new countdown, so the route is
  // called while the session is Idle with the countdown running.
  for (const input of [["--key", "down"], ["--text", "x"]]) {
    await startCountdown(fixture, design.id);
    await go(page, "#/d/demo:design-auto");
    await expect(countdownText(page)).toBeVisible();
    await fixture.cli("agents", "keys", design.id, ...input);
    await expect(countdownText(page)).toHaveCount(0);
    await expect.poll(async () => (await designInfo(fixture, design.id)).auto_advance_at).toBeUndefined();
    await expect.poll(async () => (await designInfo(fixture, design.id)).actionable.reason ?? "").toBe("");
  }

  const dialogs: [string, string, string[]][] = [
    ["Close dialog", "input needed", ["Select a thing", "❯ 1. One", "  2. Two", "Esc to cancel"]],
    ["Deny", "permission prompt", BASH_DIALOG("rg design-deny")],
    ["Allow once", "permission prompt", BASH_DIALOG("rg design-allow")],
  ];
  for (const [button, waitingFor, dialog] of dialogs) {
    await startCountdown(fixture, design.id);
    const deadline = (await designInfo(fixture, design.id)).auto_advance_at;
    await fixture.controlClaude(conv, { action: "block", waiting_for: waitingFor, dialog });
    await go(page, "#/d/demo:design-auto");
    await page.getByRole("button", { name: button, exact: true }).click();
    await expect.poll(async () => (await designInfo(fixture, design.id)).status.state).not.toBe("blocked");
    // The same deadline: the countdown was neither stopped nor restarted.
    await expect(countdownText(page)).toBeVisible();
    expect((await designInfo(fixture, design.id)).auto_advance_at).toBe(deadline);
  }
});

test("154. Commands in a design session, Continue with the countdown off", async ({ page, start }) => {
  const fixture = await start("agents-commands");
  await page.setViewportSize({ width: 1280, height: 800 });
  const design = await designOf(fixture, "demo:design-auto");
  await go(page, "#/d/demo:design-auto");
  await page.getByRole("button", { name: /^Auto: Auto-advance on/ }).click();
  await page.getByRole("dialog", nav("Auto")).getByRole("switch", { name: "Auto-advance on" }).click();
  await expect.poll(async () => (await designInfo(fixture, design.id)).auto_advance).toBe(false);
  await page.keyboard.press("Escape");
  await endRun(fixture, design.id);
  expect((await designInfo(fixture, design.id)).auto_advance_at).toBeUndefined();
  await expect(countdownText(page)).toHaveCount(0);
  await page.getByRole("button", { name: /^Continue/ }).click();
  await expect.poll(async () => (await designInfo(fixture, design.id)).actionable.reason ?? "").toBe("");
});
