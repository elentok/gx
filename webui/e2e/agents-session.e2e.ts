import type { Page } from "@playwright/test";
import type { SessionInfo } from "../src/generated/server";
import { expect, test } from "./fixture";
import { ask, chatOf, composer, go, messageBox, REPLY, row, sendButton, sessions, titleButton } from "./agentsKit";

/** The chat's scroll box, found from a message in it. */
const scrollBox = (page: Page, text: string) => page.getByText(text, { exact: true }).first();
const scrollTo = (page: Page, text: string, top: boolean) =>
  scrollBox(page, text).evaluate((el, toTop) => {
    let box = el.parentElement;
    while (box && getComputedStyle(box).overflowY !== "auto") box = box.parentElement;
    box!.scrollTop = toTop ? 0 : box!.scrollHeight;
  }, top);

test("75. Read a session", async ({ page, start }) => {
  const fixture = await start("agents");
  const list = await sessions(fixture);
  const idle = list[0];
  const working = list[1];
  const design = list[2];
  const unread = list.find((s) => s.unread.unread && s.harness === "claude")!;
  await page.setViewportSize({ width: 1280, height: 560 });

  // Header fields and the chat.
  await go(page, `#/agents/${idle.id}`);
  const header = page.getByRole("banner").or(page.locator("header")).last();
  await expect(header.getByRole("img", { name: idle.status.state, exact: true })).toBeVisible();
  await expect(header.getByText(new RegExp(`^${idle.status.state}$`, "i"))).toBeVisible();
  await expect(header.getByText(idle.dir)).toBeVisible();
  await expect(header.getByText(idle.harness, { exact: true })).toHaveCount(0);
  await expect(titleButton(page)).toContainText(idle.name || idle.id);
  const ticketed = list.find((s) => s.ticket && s.harness === "claude")!;
  await go(page, `#/agents/${ticketed.id}`);
  await expect(header.getByText(ticketed.ticket!, { exact: true })).toBeVisible();
  await go(page, `#/agents/${idle.id}`);
  await expect(page.getByText("hello", { exact: true })).toBeVisible();
  await expect(page.getByText(REPLY)).toBeVisible();
  await go(page, `#/agents/${design.id}`);
  await expect(page.getByRole("link", { name: "Open design page →" })).toBeVisible();

  // Unread clears when the session is viewed.
  expect(unread.unread.unread).toBe(true);
  await expect(row(page, unread).getByRole("img", { name: "Unread" })).toBeVisible();
  await go(page, `#/agents/${unread.id}`);
  await expect(row(page, unread).getByRole("img", { name: "Unread" })).toHaveCount(0);
  await expect.poll(async () => !!(await sessions(fixture)).find((s) => s.id === unread.id)!.unread.unread).toBe(false);

  // A Working session shows a plain Working row.
  await go(page, `#/agents/${working.id}`);
  await expect(page.getByRole("status").filter({ hasText: /^Working$/ })).toBeVisible();

  // The chat opens at the newest message and follows new ones until the reader scrolls up.
  await go(page, `#/agents/${idle.id}`);
  for (const n of [1, 2, 3, 4, 5, 6, 7, 8]) await ask(fixture, idle.id, `turn ${n}`);
  const newest = page.getByText(REPLY).last();
  await expect(page.getByText(REPLY)).toHaveCount(9);
  await expect(newest).toBeInViewport();
  await expect(page.getByText("hello", { exact: true })).not.toBeInViewport();
  await ask(fixture, idle.id, "turn 9");
  await expect(page.getByText(REPLY)).toHaveCount(10);
  await expect(newest).toBeInViewport();
  await scrollTo(page, "hello", true);
  await expect(page.getByText("hello", { exact: true })).toBeInViewport();
  await ask(fixture, idle.id, "turn 10");
  await expect(page.getByText(REPLY)).toHaveCount(11);
  await expect(newest).not.toBeInViewport();
  await scrollTo(page, "hello", false);

  // A failed chat read keeps the chat; the next event refetches it.
  let failing = true;
  await page.route("**/v1/sessions/chat*", (route) => (failing ? route.fulfill({ status: 500, body: "boom" }) : route.continue()));
  await ask(fixture, idle.id, "while failing");
  await expect(page.getByText("turn 10", { exact: true })).toBeVisible();
  await expect(page.getByText("while failing", { exact: true })).toHaveCount(0);
  failing = false;
  await ask(fixture, idle.id, "after recovery");
  await expect(page.getByText("while failing", { exact: true })).toHaveCount(1);
  await expect(page.getByText("after recovery", { exact: true })).toBeVisible();
});

test("77. Send while busy", async ({ page, start }) => {
  const fixture = await start("agents");
  const working = (await sessions(fixture))[1];
  expect(working.status.state).toBe("working");
  await go(page, `#/agents/${working.id}`);

  await expect(composer(page).getByText("Working. Send turns on when the session is idle.")).toBeVisible();
  const box = composer(page).getByRole("textbox", { name: "Message" });
  await box.fill("a draft for later");
  await expect(box).toHaveValue("a draft for later");
  await expect(composer(page).getByRole("button", { name: "Send" })).toBeDisabled();
});

test("81. Session without chat", async ({ page, start }) => {
  const fixture = await start("agents");
  const other = (await sessions(fixture)).find((s) => s.harness !== "claude")!;
  await go(page, `#/agents/${other.id}`);

  await expect(page.getByText("Chat works for Claude sessions only.")).toBeVisible();
  await expect(composer(page).getByText("Sending works for Claude sessions only")).toBeVisible();
  await expect(composer(page).getByRole("button", { name: "Send" })).toBeDisabled();

  await page.getByRole("button", { name: "Show screen" }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

test("82. No messages yet", async ({ page, start }) => {
  const fixture = await start("agents");
  const claude = (await sessions(fixture)).filter((s) => s.harness === "claude");
  const empty: SessionInfo[] = [];
  for (const s of claude) if ((await chatOf(fixture, s.id)).messages.length === 0) empty.push(s);
  expect(empty.length).toBeGreaterThan(0);
  await go(page, `#/agents/${empty[0].id}`);

  await expect(page.getByText("No messages yet")).toBeVisible();
  await expect(composer(page).getByRole("textbox", { name: "Message" })).toBeVisible();
});

const failure = (page: Page) => page.getByRole("region", { name: "Session failure" });

test("80. Failure notices", async ({ page, start }) => {
  const fixture = await start("agents-failures");
  await page.setViewportSize({ width: 1280, height: 800 });
  const byKind = async (kind: string) => (await sessions(fixture)).find((s) => s.failure?.kind === kind)!;
  const open = async (kind: string) => {
    const s = await byKind(kind);
    await go(page, `#/agents/${s.id}`);
    return s;
  };
  const buttons = (kind: string) => failure(page).getByRole("button").evaluateAll((els) => els.map((el) => el.textContent));

  // Stalled: the fixture's threshold is 3s.
  await open("stalled");
  await expect(failure(page).getByText("No progress for 15 min")).toBeVisible();
  expect(await buttons("stalled")).toEqual(["Screen"]);
  await failure(page).getByRole("button", { name: "Screen" }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.keyboard.press("Escape");

  // Blocked: Screen only.
  await open("blocked");
  await expect(failure(page).getByText("Waiting at a prompt in the terminal")).toBeVisible();
  expect(await buttons("blocked")).toEqual(["Screen"]);

  // Auth: Retry, and Send stays on.
  await open("auth");
  await expect(failure(page).getByText("Claude login needed")).toBeVisible();
  expect(await buttons("auth")).toEqual(["Retry"]);
  await messageBox(page).fill("hi");
  await expect(sendButton(page)).toBeEnabled();
  const auth = await byKind("auth");
  const texts = async () => (await chatOf(fixture, auth.id)).messages.map((m) => (m as { text: string }).text);
  expect(await texts()).not.toContain("continue");
  await failure(page).getByRole("button", { name: "Retry" }).click();
  await expect.poll(texts).toContain("continue");

  // Rate limit: label and text, no buttons, and the reason Send is off.
  const limited = await open("rate-limit");
  await expect(failure(page).getByText("Rate limited", { exact: true })).toBeVisible();
  await expect(failure(page).getByText(limited.failure!.text!)).toBeVisible();
  expect(await buttons("rate-limit")).toEqual([]);
  await expect(composer(page).getByText("Rate limited. Send turns on when the session is idle.")).toBeVisible();
  await expect(sendButton(page)).toBeDisabled();
});

test("80. Failure notices, crashed", async ({ page, start }) => {
  const fixture = await start("agents");
  await page.setViewportSize({ width: 1280, height: 800 });
  const crashed = (await sessions(fixture)).find((s) => s.exit === "crashed")!;
  await go(page, `#/agents/${crashed.id}`);

  await expect(failure(page).getByText("Agent crashed")).toBeVisible();
  await expect(page.getByText("hello", { exact: true })).toBeVisible();
  await expect(composer(page).getByText("Exited. Restore it in gx, Agents tab.")).toBeVisible();
  await expect(page.getByRole("button", { name: "Restore" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "New session", exact: true })).toHaveCount(0);
  await expect(sendButton(page)).toBeDisabled();
});

test("83. Design session on the Agents page", async ({ page, start }) => {
  const fixture = await start("agents");
  await page.setViewportSize({ width: 1280, height: 800 });
  const design = (await sessions(fixture)).find((s) => s.design)!;
  await go(page, `#/agents/${design.id}`);

  await expect(page.getByText(REPLY)).toBeVisible();
  await expect(page.getByRole("alertdialog")).toHaveCount(0);
  await expect(page.getByText("Unanswered questions")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Continue" })).toHaveCount(0);
  await expect(messageBox(page)).toBeVisible();

  await page.getByRole("link", { name: "Open design page →" }).click();
  await expect(page).toHaveURL(/#\/d\/demo:design-epic$/);
});

// The hold ends at the reset clock plus the 60 s buffer.
const holdEnds = { timeout: 180_000 };

test("103. Rate-limit hold", async ({ page, start }) => {
  test.setTimeout(240_000);
  const fixture = await start("rate-limited");
  const verdict = (epic: string) => page.locator(`a[href$="/t/demo:${epic}/01"]`).filter({ hasNotText: "→" }).locator("[data-verdict]");

  await page.getByRole("link", { name: "limited-epic", exact: true }).click();
  await expect(page.getByText(/rate-limited: resets /).first()).toBeVisible();
  await expect(page.getByText("Paused", { exact: false })).toHaveCount(0);

  await go(page, "#/e/demo:waiting-epic");
  await expect(verdict("waiting-epic")).toContainText("rate-limited");

  await go(page, "#/agents");
  await page.getByRole("complementary").getByText(/^Queue agents/).click();
  const [queueAgent] = (await sessions(fixture)).filter((s) => s.id.startsWith("queue-limited-epic"));
  await expect(row(page, queueAgent).getByRole("img", { name: "rate-limited" })).toBeVisible();

  await expect(row(page, queueAgent).getByRole("img", { name: "rate-limited" })).toHaveCount(0, holdEnds);
  await go(page, "#/e/demo:waiting-epic");
  await expect(verdict("waiting-epic")).toHaveCount(0, holdEnds);
});

test("104. Usage gate and override", async ({ page, start }) => {
  await start("usage-gated");
  const verdict = page.locator('a[href$="/t/demo:gated-epic/01"]').filter({ hasNotText: "→" }).locator("[data-verdict]");

  await go(page, "#/e/demo:gated-epic");
  await expect(verdict).toContainText(/usage at 99% of the weekly limit, resets /);
  await verdict.locator("..").getByRole("button", { name: "Override" }).click();
  await expect(verdict).toHaveCount(0);
});
