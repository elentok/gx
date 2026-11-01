import type { Page } from "@playwright/test";
import { expect, nav, test, type Fixture } from "./fixture";
import { chatOf, claudeIdle, codexIdle, composer, go, messageBox, sendButton, sessions } from "./agentsKit";

const conversationOf = async (fixture: Fixture, id: string) => (await chatOf(fixture, id)).conversation_id;
const infoOf = async (fixture: Fixture, id: string) => (await sessions(fixture)).find((s) => s.id === id)!;

const output = (page: Page) => page.getByTestId("command-output");

/** A command that starts no turn changes no status, so no event reaches the open chat: leave and come back. */
async function reopen(page: Page, hash: string) {
  await go(page, "#/agents");
  await go(page, hash);
}

/** The Note box of a design page, opened when closed. */
async function designBox(page: Page) {
  const box = messageBox(page);
  if (!(await box.isVisible())) await page.getByRole("button", nav("Note")).click();
  await expect(box).toBeEnabled();
  return box;
}

/** Lets the design agent end a run, which leaves a Continue and puts the note's Send on the page. */
async function endRun(fixture: Fixture, id: string) {
  await expect.poll(async () => (await infoOf(fixture, id)).status.state).toBe("idle");
  await expect.poll(() => fixture.cli("agents", "prompt", id, "carry on").then(() => true, () => false)).toBe(true);
  await expect.poll(async () => (await infoOf(fixture, id)).actionable.reason ?? "").not.toBe("");
}

const ESC =String.fromCharCode(27);

test("155. Command output in the chat, cut at 12 lines", async ({ page, start }) => {
  const fixture = await start("agents-commands");
  await page.setViewportSize({ width: 1280, height: 800 });
  const idle = await claudeIdle(fixture);
  const conv = await conversationOf(fixture, idle.id);
  await go(page, `#/agents/${idle.id}`);

  const rows = Array.from({ length: 15 }, (_, i) => `cost-row-${i + 1}`);
  await fixture.controlClaude(conv, { action: "commandoutput", message: rows.map((r) => `${ESC}[1m${r}${ESC}[0m`).join("\n") });
  await messageBox(page).fill("/cost");
  await sendButton(page).click();
  await expect.poll(async () => (await chatOf(fixture, idle.id)).messages.length).toBe(4);
  await reopen(page, `#/agents/${idle.id}`);

  // The pill, then the block cut at 12 lines with a way to the rest.
  await expect(page.getByText("/cost", { exact: true })).toBeVisible();
  await expect(output(page)).toContainText("cost-row-12");
  await expect(output(page)).not.toContainText("cost-row-13");
  await page.getByRole("button", { name: "Show all (15 lines)" }).click();
  await expect(output(page)).toContainText("cost-row-15");
  await expect(page.getByRole("button", { name: /^Show all/ })).toHaveCount(0);

  // No ANSI code, no caveat line, nothing sent back to the agent, and the session stays read.
  expect(await output(page).innerText()).not.toMatch(/\[1m|\[0m|\u001b/);
  await expect(page.getByText(/Caveat/)).toHaveCount(0);
  expect(await fixture.input(conv)).not.toContain("cost-row");
  expect((await infoOf(fixture, idle.id)).actionable.reason ?? "").toBe("");
  expect(await fixture.cli("agents", "chat", idle.id)).not.toContain("Caveat");
});

test("155. Command output in the chat, hidden output", async ({ page, start }) => {
  const fixture = await start("agents-commands");
  await page.setViewportSize({ width: 1280, height: 800 });

  // `/model` and `/compact` leave the pill alone: empty output and the Compacted line are dropped.
  for (const [n, command, message] of [[0, "/model", ""], [1, "/compact", "Compacted (ctrl+o to see full summary)"]] as const) {
    const idle = await claudeIdle(fixture, n);
    const conv = await conversationOf(fixture, idle.id);
    await go(page, `#/agents/${idle.id}`);
    await fixture.controlClaude(conv, { action: "commandoutput", message });
    await messageBox(page).fill(command);
    await sendButton(page).click();
    await expect.poll(() => fixture.cli("agents", "chat", idle.id)).toContain(command);
    await reopen(page, `#/agents/${idle.id}`);
    await expect(page.getByText(command, { exact: true })).toBeVisible();
    await expect(output(page)).toHaveCount(0);
    await expect(page.getByText(/Caveat|Compacted/)).toHaveCount(0);
  }

  // The Codex `/status` writes no output: the pill alone.
  const codex = await codexIdle(fixture);
  await go(page, `#/agents/${codex.id}`);
  await messageBox(page).fill("/status");
  await sendButton(page).click();
  await expect.poll(() => fixture.cli("agents", "chat", codex.id)).toContain("/status");
  await reopen(page, `#/agents/${codex.id}`);
  await expect(page.getByText("/status", { exact: true })).toBeVisible();
  await expect(output(page)).toHaveCount(0);
});

test("155. Command output in the chat, design page", async ({ page, start }) => {
  const fixture = await start("agents-commands", { autoAdvanceDelay: "1h" });
  await page.setViewportSize({ width: 1280, height: 800 });
  const design = (await sessions(fixture)).find((s) => s.ticket === "demo:design-auto")!;
  const conv = await conversationOf(fixture, design.id);
  await endRun(fixture, design.id);
  await go(page, "#/d/demo:design-auto");

  const rows = Array.from({ length: 14 }, (_, i) => `design-row-${i + 1}`);
  await fixture.controlClaude(conv, { action: "commandoutput", message: rows.map((r) => `${ESC}[2m${r}${ESC}[0m`).join("\n") });
  const before = (await chatOf(fixture, design.id)).messages.length;
  await (await designBox(page)).fill("/cost");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect.poll(async () => (await chatOf(fixture, design.id)).messages.length).toBe(before + 2);
  await go(page, "#/agents");
  await go(page, "#/d/demo:design-auto");

  await expect(page.getByText("/cost", { exact: true })).toBeVisible();
  await expect(output(page)).toContainText("design-row-12");
  await expect(output(page)).not.toContainText("design-row-13");
  await page.getByRole("button", { name: "Show all (14 lines)" }).click();
  await expect(output(page)).toContainText("design-row-14");
  expect(await output(page).innerText()).not.toMatch(/\[2m|\[0m|\u001b/);
  await expect(page.getByText(/Caveat/)).toHaveCount(0);
  expect(await fixture.input(conv)).not.toContain("design-row");
});

test("159. Exit with /exit, Agents page", async ({ page, start }) => {
  const fixture = await start("agents-commands");
  await page.setViewportSize({ width: 1280, height: 800 });
  const idle = await claudeIdle(fixture);
  await go(page, `#/agents/${idle.id}`);
  await expect(page.getByText("hello", { exact: true })).toBeVisible();

  await messageBox(page).fill("/exit");
  await sendButton(page).click();
  await expect.poll(async () => (await infoOf(fixture, idle.id)).status.state).toBe("exited");

  // The chat stays, the composer is off and says how to get the session back.
  await expect(page.getByText("hello", { exact: true })).toBeVisible();
  await expect(composer(page).getByText("Exited. Restore it in gx, Agents tab.")).toBeVisible();
  await expect(sendButton(page)).toBeDisabled();
});

test("159. Exit with /exit, design page", async ({ page, start }) => {
  const fixture = await start("agents-commands", { autoAdvanceDelay: "1h" });
  await page.setViewportSize({ width: 1280, height: 800 });
  const design = (await sessions(fixture)).find((s) => s.ticket === "demo:design-auto")!;
  await endRun(fixture, design.id);
  await go(page, "#/d/demo:design-auto");

  await (await designBox(page)).fill("/exit");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect.poll(async () => (await infoOf(fixture, design.id)).status.state).toBe("exited");

  await expect(page.getByRole("button", { name: "Restore", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "New session", exact: true })).toBeVisible();
  await expect(page.getByText("carry on", { exact: true })).toBeVisible();
});
