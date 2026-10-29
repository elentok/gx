import type { Page } from "@playwright/test";
import type { SessionInfo } from "../src/generated/server";
import { card, expect, nav, test, type Fixture } from "./fixture";

const sessions = async (fixture: Fixture) => JSON.parse(await fixture.cli("agents", "list", "--json")) as SessionInfo[];

const isActionable = (s: SessionInfo) => !!s.actionable.reason || s.status.state === "blocked";

const row = (page: Page, s: SessionInfo) => page.locator(`a[href$="/agents/${s.id}"]`);

const sidebar = (page: Page) => page.getByRole("complementary");

// A hash change keeps the page, since the fixture fails a test whose page reloaded.
const go = (page: Page, hash: string) => page.evaluate((h) => (location.hash = h), hash);

/** A second visit is a fresh page on the same server and context, since a reload would fail the test. */
async function freshVisit(page: Page, hash: string) {
  const other = await page.context().newPage();
  await other.goto(new URL(page.url()).origin + hash);
  return other;
}

const REPLY = "Sure, here is a short answer.";

const chatOf = async (fixture: Fixture, id: string) =>
  JSON.parse(await fixture.cli("agents", "chat", id, "--json")) as { messages: unknown[] };

/** Prompts a session and waits for the reply. A prompt is refused until the last turn ends, so it retries. */
async function ask(fixture: Fixture, id: string, text: string) {
  const before = (await chatOf(fixture, id)).messages.length;
  await expect
    .poll(() => fixture.cli("agents", "prompt", id, text).then(() => true, () => false))
    .toBe(true);
  await expect.poll(async () => (await chatOf(fixture, id)).messages.length).toBe(before + 2);
  // The server's state follows the fake a moment late, so a prompt sent now could still land on the old turn.
  await expect.poll(async () => (await sessions(fixture)).find((s) => s.id === id)!.status.state).toBe("working");
  await expect.poll(async () => (await sessions(fixture)).find((s) => s.id === id)!.status.state).toBe("idle");
}

/** The chat's scroll box, found from a message in it. */
const scrollBox = (page: Page, text: string) => page.getByText(text, { exact: true }).first();
const scrollTo = (page: Page, text: string, top: boolean) =>
  scrollBox(page, text).evaluate((el, toTop) => {
    let box = el.parentElement;
    while (box && getComputedStyle(box).overflowY !== "auto") box = box.parentElement;
    box!.scrollTop = toTop ? 0 : box!.scrollHeight;
  }, top);

const composer = (page: Page) => page.getByRole("form", { name: "Composer" });

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
  await expect(header.getByText(idle.status.state, { exact: true })).toBeVisible();
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

const messageBox = (page: Page) => composer(page).getByRole("textbox", { name: "Message" });
const sendButton = (page: Page) => composer(page).getByRole("button", { name: "Send", exact: true });

/** Counts the prompts the page posts, so a key that does nothing can be told from one that sent. */
function countPrompts(page: Page) {
  const seen = { count: 0 };
  page.on("request", (r) => {
    if (r.method() === "POST" && r.url().includes("/v1/sessions/prompt")) seen.count++;
  });
  return seen;
}

const waitIdle = (fixture: Fixture, id: string) =>
  expect.poll(async () => (await sessions(fixture)).find((s) => s.id === id)!.status.state).toBe("idle");

test("76. Send and draft", async ({ page, start }) => {
  const fixture = await start("agents");
  const idle = (await sessions(fixture))[0];
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, `#/agents/${idle.id}`);

  // The draft survives a new tab of the same browser.
  await messageBox(page).fill("a draft to keep");
  await expect(composer(page).getByText("Draft saved in this browser")).toBeVisible();
  const second = await freshVisit(page, `/#/agents/${idle.id}`);
  await expect(messageBox(second)).toHaveValue("a draft to keep");
  await second.close();

  await sendButton(page).click();
  await expect(messageBox(page)).toHaveValue("");
  await expect(page.getByText("a draft to keep", { exact: true })).toBeVisible();
  await expect.poll(() => fixture.cli("agents", "chat", idle.id)).toContain("a draft to keep");
  await expect(composer(page).getByText("Draft saved in this browser")).toHaveCount(0);
});

test("78. Send refused", async ({ page, start }) => {
  const fixture = await start("agents");
  const idle = (await sessions(fixture))[0];
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, `#/agents/${idle.id}`);
  await expect(page.getByText(REPLY)).toBeVisible();

  // The page keeps its old chat while the chat read fails, so the CLI prompt moves the chat under it.
  let failing = true;
  await page.route("**/v1/sessions/chat*", (route) => (failing ? route.fulfill({ status: 500, body: "boom" }) : route.continue()));
  await ask(fixture, idle.id, "moved from the CLI");
  await expect(page.getByText("moved from the CLI", { exact: true })).toHaveCount(0);

  await messageBox(page).fill("my draft");
  const dim = await composer(page).getByText("Draft saved in this browser").evaluate((el) => getComputedStyle(el).color);
  failing = false;
  await sendButton(page).click();
  const reason = composer(page).getByText("the conversation moved; review it first");
  await expect(reason).toBeVisible();
  expect(await reason.evaluate((el) => getComputedStyle(el).color)).not.toBe(dim);
  await expect(page.getByText("moved from the CLI", { exact: true })).toBeVisible();
  await expect(messageBox(page)).toHaveValue("my draft");
  expect(await fixture.cli("agents", "chat", idle.id)).not.toContain("my draft");

  // A send that starts with / is refused by prefix and keeps the draft.
  await messageBox(page).fill("/clear");
  await sendButton(page).click();
  await expect(composer(page).getByText("text may not start with / or !")).toBeVisible();
  await expect(messageBox(page)).toHaveValue("/clear");
});

test("79. Keyboard send", async ({ page, start }) => {
  const fixture = await start("agents");
  const idle = (await sessions(fixture))[0];
  const prompts = countPrompts(page);

  // At 390px only the button sends.
  await page.setViewportSize({ width: 390, height: 800 });
  await go(page, `#/agents/${idle.id}`);
  await messageBox(page).fill("phone text");
  await messageBox(page).press("Control+Enter");
  await expect(messageBox(page)).toHaveValue("phone text");
  expect(prompts.count).toBe(0);
  await sendButton(page).click();
  await expect(messageBox(page)).toHaveValue("");
  await expect(page.getByText("phone text", { exact: true })).toBeVisible();
  await waitIdle(fixture, idle.id);

  // Wide, Enter adds a newline and Ctrl+Enter sends.
  await page.setViewportSize({ width: 1280, height: 800 });
  await messageBox(page).fill("line one");
  await messageBox(page).press("Enter");
  await messageBox(page).pressSequentially("line two");
  await expect(messageBox(page)).toHaveValue("line one\nline two");
  expect(prompts.count).toBe(1);
  await messageBox(page).press("Control+Enter");
  await expect(messageBox(page)).toHaveValue("");
  expect(prompts.count).toBe(2);
  await expect.poll(() => fixture.cli("agents", "chat", idle.id)).toContain("line two");
});

test("79. Keyboard send on the design page", async ({ page, start }) => {
  await start("design", { autoAdvanceDelay: "1h" });
  await page.setViewportSize({ width: 1280, height: 800 });
  await expect(card(page, "design-epic")).toBeVisible();
  await go(page, "#/d/demo:design-epic");
  await page.getByRole("button", nav("Start design session")).click();
  const prompts = countPrompts(page);
  const questions = page.getByRole("group", nav("Questions"));
  const popover = page.getByRole("alertdialog", nav("Unanswered questions"));
  const box = page.getByRole("textbox", { name: "Message" });
  await expect(questions.getByText("Q8 - Which store?")).toBeVisible();

  // Send answers with gaps opens the popover and posts nothing.
  await page.getByRole("button", nav("Note")).click();
  await expect(box).toBeEnabled();
  await box.press("Control+Enter");
  await expect(popover.getByText("2 questions are unanswered")).toBeVisible();
  expect(prompts.count).toBe(0);
  await page.keyboard.press("Escape");
  await expect(popover).toHaveCount(0);

  // With every question answered the key sends the round.
  await questions.getByRole("button", { name: /SQLite/ }).click();
  await questions.getByRole("button", { name: /Yes/ }).click();
  await box.press("Control+Enter");
  await expect.poll(() => prompts.count).toBe(1);
  const cont = page.getByRole("button", { name: /^Continue/ });
  await expect(cont).toBeVisible();

  // The main button is Continue: the key does nothing. The sent round closed the Note box, so it opens again.
  await page.getByRole("button", nav("Note")).click();
  await box.press("Control+Enter");
  await expect(popover).toHaveCount(0);
  await expect(cont).toBeVisible();
  expect(prompts.count).toBe(1);

  // With text the main button is Send, and the key sends it.
  await box.fill("one more thing");
  await expect(page.getByRole("button", nav("Send"))).toBeVisible();
  await box.press("Control+Enter");
  await expect.poll(() => prompts.count).toBe(2);
  await expect(box).toHaveValue("");
});

test("79. Keyboard send while the design works", async ({ page, start }) => {
  await start("design-working");
  await page.setViewportSize({ width: 1280, height: 800 });
  await expect(card(page, "design-epic")).toBeVisible();
  await go(page, "#/d/demo:design-epic");
  await page.getByRole("button", nav("Start design session")).click();
  const prompts = countPrompts(page);
  await expect(page.getByRole("button", nav("Interrupt"))).toBeVisible();

  // The box is off while the agent works, so the key is sent to it directly.
  await page.getByRole("button", nav("Note")).click();
  const box = page.getByRole("textbox", { name: "Message" });
  await expect(box).toBeDisabled();
  await box.dispatchEvent("keydown", { key: "Enter", ctrlKey: true, bubbles: true });
  await expect(page.getByRole("button", nav("Interrupt"))).toBeVisible();
  await expect(page.getByRole("alertdialog")).toHaveCount(0);
  expect(prompts.count).toBe(0);
});

test("72. Agents tab", async ({ page, start }) => {
  const fixture = await start("agents");
  const list = await sessions(fixture);
  const actionable = list.filter(isActionable).length;
  expect(actionable).toBeGreaterThan(0);

  const tabs = page.getByRole("navigation").first().getByRole("link");
  await expect(tabs.nth(0)).toHaveText("Epics");
  await expect(tabs.nth(1)).toHaveText(`Agents${actionable}`);
  await expect(tabs.nth(1)).toHaveAccessibleName(`Agents · ${actionable} ${actionable === 1 ? "needs" : "need"} you`);
  await expect(tabs.nth(2)).toHaveText("Stats");

  await tabs.nth(1).click();
  await expect(tabs.nth(1)).toHaveAttribute("aria-current", "page");
  await expect(page).toHaveURL(/#\/agents/);

  // The deep link selects the session, in this page and in a fresh one.
  const target = list[1];
  await go(page, `#/agents/${target.id}`);
  await expect(row(page, target)).toHaveAttribute("aria-current", "page");
  const second = await freshVisit(page, `/#/agents/${target.id}`);
  await expect(row(second, target)).toHaveAttribute("aria-current", "page");
  await expect(second.getByRole("navigation").first().getByRole("link").nth(1)).toHaveText(`Agents${actionable}`);

  // The count follows gx agents list and hides when no session is left.
  await second.close();
  const empty = await start("empty");
  expect(await sessions(empty)).toHaveLength(0);
  await expect(page.getByRole("navigation").first().getByRole("link").nth(1)).toHaveText("Agents");
});

test("73. Agents sidebar", async ({ page, start }) => {
  const fixture = await start("agents");
  const list = await sessions(fixture);
  await go(page, "#/agents");
  const side = sidebar(page);

  // Projects in order of their first session; the design sessions start hidden.
  const projects = [...new Set(list.map((s) => s.project))];
  await expect(side.locator("p, span, div").filter({ hasText: /^(demo|other|site)$/ })).toHaveText(projects);
  for (const s of list) {
    if (s.design) await expect(row(page, s)).toHaveCount(0);
    else await expect(row(page, s)).toHaveCount(1);
  }
  const visible = list.filter((s) => !s.design);
  const order = await side.locator('a[href*="/agents/"]').evaluateAll((els) => els.map((el) => el.getAttribute("href")!.split("/agents/")[1]));
  expect(order).toEqual(visible.map((s) => s.id));

  // Marks.
  for (const s of visible) {
    const mark = s.exit === "crashed" ? "crashed" : s.status.state;
    await expect(row(page, s).getByRole("img", { name: mark, exact: true })).toBeVisible();
    await expect(row(page, s).getByRole("img", { name: "Unread" })).toHaveCount(s.unread.unread ? 1 : 0);
    await expect(row(page, s).getByRole("img", { name: "Actionable" })).toHaveCount(isActionable(s) ? 1 : 0);
  }
  const waiting = list.find((s) => s.actionable.reason)!;
  await expect(row(page, waiting).getByRole("img", { name: "Actionable" })).toHaveAttribute("title", waiting.actionable.reason!);

  const live = list.filter((s) => s.status.state !== "exited").length;
  await expect(side.getByText(`${live} live`)).toBeVisible();

  // Live updates: a stopped idle session changes its mark and the count with no reload.
  const idle = visible.find((s) => s.status.state === "idle" && s.harness === "claude")!;
  await fixture.cli("agents", "stop", idle.id);
  await expect(row(page, idle).getByRole("img", { name: "exited", exact: true })).toBeVisible();
  await expect(side.getByText(`${live - 1} live`)).toBeVisible();

  // A removed Exited session leaves, a new one arrives.
  const exited = list.find((s) => s.status.state === "exited")!;
  await fixture.cli("agents", "remove", exited.id);
  await expect(row(page, exited)).toHaveCount(0);
  const before = new Set(list.map((s) => s.id));
  await fixture.cli("agents", "create", "--project", "demo", "--prompt", "hello");
  await expect.poll(async () => (await sessions(fixture)).filter((s) => !before.has(s.id)).length).toBe(1);
  const added = (await sessions(fixture)).find((s) => !before.has(s.id))!;
  await expect(row(page, added)).toBeVisible();
  await expect(side.getByText(`${live} live`)).toBeVisible();
});

test("74. Design sessions toggle", async ({ page, start }) => {
  const fixture = await start("agents");
  const list = await sessions(fixture);
  const designs = list.filter((s) => s.design);
  expect(designs.length).toBe(2);
  await go(page, "#/agents");
  const side = sidebar(page);
  const toggle = side.getByRole("checkbox", { name: /^Design sessions/ });
  const toggleLabel = side.getByText(/^Design sessions/);

  // Hidden by default, with the count.
  await expect(toggle).not.toBeChecked();
  await expect(side.getByText(`· ${designs.length} hidden`)).toBeVisible();
  for (const s of designs) await expect(row(page, s)).toHaveCount(0);

  // Remembered in a fresh page of the same browser.
  await toggleLabel.click();
  for (const s of designs) await expect(row(page, s)).toBeVisible();
  await expect(side.getByText("hidden")).toHaveCount(0);
  const second = await freshVisit(page, "/#/agents");
  await expect(sidebar(second).getByRole("checkbox", { name: /^Design sessions/ })).toBeChecked();
  for (const s of designs) await expect(row(second, s)).toBeVisible();
  await second.close();

  // A selected design session keeps its row while the rest hide.
  await toggleLabel.click();
  for (const s of designs) await expect(row(page, s)).toHaveCount(0);
  await go(page, `#/agents/${designs[0].id}`);
  await expect(row(page, designs[0])).toHaveAttribute("aria-current", "page");
  await expect(row(page, designs[1])).toHaveCount(0);
  await expect(side.getByText("· 1 hidden")).toBeVisible();
});

test("92. Last viewed session", async ({ page, start }) => {
  const fixture = await start("agents");
  const list = await sessions(fixture);
  const viewed = list[1];

  // With none viewed, the wide page asks for a pick.
  await go(page, "#/agents");
  await expect(page.getByRole("heading", { name: "Pick a session" })).toBeVisible();

  await go(page, `#/agents/${viewed.id}`);
  await expect(row(page, viewed)).toHaveAttribute("aria-current", "page");

  // Opening #/agents in a fresh page reopens it.
  const second = await freshVisit(page, "/#/agents");
  await expect(row(second, viewed)).toHaveAttribute("aria-current", "page");
  await expect(second).toHaveURL(new RegExp(`#/agents/${viewed.id}$`));
  await expect(second.getByRole("heading", { name: "Pick a session" })).toHaveCount(0);
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

test("89. Session not found", async ({ page, start }) => {
  const fixture = await start("agents");
  const list = await sessions(fixture);
  const exited = list.find((s) => s.status.state === "exited")!;
  await page.setViewportSize({ width: 1280, height: 800 });

  // A deep link to a live session never flashes the panel while the list loads.
  const live = list.find((s) => s.status.state === "idle" && s.harness === "claude")!;
  const deep = await page.context().newPage();
  await deep.addInitScript(() => {
    const w = window as unknown as { flashed: boolean };
    w.flashed = false;
    new MutationObserver(() => {
      if (document.body?.textContent?.includes("Session not found")) w.flashed = true;
    }).observe(document, { childList: true, subtree: true, characterData: true });
  });
  await deep.goto(new URL(page.url()).origin + `#/agents/${live.id}`);
  await expect(deep.getByText("hello", { exact: true })).toBeVisible();
  expect(await deep.evaluate(() => (window as unknown as { flashed: boolean }).flashed)).toBe(false);
  await deep.close();

  // A made-up label: the panel names it and the sidebar stays.
  await go(page, "#/agents/no-such-session");
  await expect(page.getByText("Session not found")).toBeVisible();
  await expect(page.getByText("no-such-session no longer exists.")).toBeVisible();
  await expect(row(page, list[0])).toBeVisible();
  await expect(page.getByRole("link", { name: "Back to sessions" })).toHaveCount(0);

  // A viewed session removed under the reader: the selection stays and the panel replaces the view.
  await go(page, `#/agents/${exited.id}`);
  await expect(row(page, exited)).toHaveAttribute("aria-current", "page");
  await fixture.cli("agents", "remove", exited.id);
  await expect(page.getByText("Session not found")).toBeVisible();
  await expect(page.getByText(`${exited.id} no longer exists.`)).toBeVisible();
  await expect(row(page, exited)).toHaveCount(0);
  await expect(row(page, list[0])).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`#/agents/${exited.id}$`));

  // No other session's turns were read by any of this.
  const unreadBefore = list.filter((s) => s.unread.unread && s.id !== exited.id).map((s) => s.id);
  expect(unreadBefore.length).toBeGreaterThan(0);
  const unreadNow = (await sessions(fixture)).filter((s) => s.unread.unread).map((s) => s.id);
  expect(unreadNow.sort()).toEqual(unreadBefore.sort());

  // At 390px the panel offers the way back.
  await page.setViewportSize({ width: 390, height: 800 });
  await page.getByRole("link", { name: "Back to sessions" }).click();
  await expect(page).toHaveURL(/#\/agents$/);
});

const switcher = (page: Page) => page.getByRole("dialog", { name: "Switch session" });
const switcherFilter = (page: Page) => switcher(page).getByRole("combobox");
const switcherRows = (page: Page) => switcher(page).getByRole("option");

/** The switcher's order: Actionable, then Unread, then the rest, each in list order. */
const switcherOrder = (list: SessionInfo[]) => {
  const bucket = (s: SessionInfo) => (isActionable(s) ? 0 : s.unread.unread ? 1 : 2);
  return [0, 1, 2].flatMap((b) => list.filter((s) => bucket(s) === b));
};
const titleButton = (page: Page) => page.getByRole("heading", { level: 1 }).getByRole("button");

test("84. Session switcher", async ({ page, start }) => {
  const fixture = await start("agents");
  const list = await sessions(fixture);
  const idle = list[0];
  const design = list.find((s) => s.design)!;
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, `#/agents/${idle.id}`);

  // The title button and Cmd-K both open it, on the filter.
  await titleButton(page).click();
  await expect(switcherFilter(page)).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(switcher(page)).toHaveCount(0);
  await page.keyboard.press("Control+k");
  await expect(switcherFilter(page)).toBeFocused();

  // Sections, the current tag, and design sessions listed while the sidebar hides them.
  for (const title of ["Actionable", "Unread", "Other"]) await expect(switcher(page).getByRole("group", { name: title })).toBeVisible();
  const order = switcherOrder(list);
  await expect(switcherRows(page)).toHaveCount(list.length + 1);
  const at = order.indexOf(idle);
  await expect(switcherRows(page).nth(at)).toHaveAttribute("aria-current", "true");
  await expect(switcherRows(page).nth(at).getByText("current")).toBeVisible();
  await expect(switcherRows(page).filter({ has: page.getByText("current", { exact: true }) })).toHaveCount(1);
  await expect(row(page, design)).toHaveCount(0);
  await expect(switcherRows(page).nth(order.indexOf(design)).getByRole("img", { name: "Design" })).toBeVisible();

  // The filter matches the ticket and the dir; the New session row stays.
  const withTicket = list.find((s) => s.ticket)!;
  const ticketCount = list.filter((s) => s.ticket === withTicket.ticket).length;
  await switcherFilter(page).fill(withTicket.ticket!);
  await expect(switcherRows(page)).toHaveCount(ticketCount + 1);
  await expect(switcherRows(page).last()).toContainText("+ New session");
  const other = switcherOrder(list.filter((s) => s.project === "other"))[0];
  await switcherFilter(page).fill(other.dir);
  await expect(switcherRows(page)).toHaveCount(list.filter((s) => s.dir === other.dir).length + 1);

  // Picking a row opens the session and closes the switcher.
  await switcherRows(page).first().click();
  await expect(switcher(page)).toHaveCount(0);
  await expect(page).toHaveURL(new RegExp(`#/agents/${other.id}$`));
  await expect(row(page, other)).toHaveAttribute("aria-current", "page");

  // Escape closes it.
  await page.keyboard.press("Control+k");
  await expect(switcher(page)).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(switcher(page)).toHaveCount(0);

  // An epic's Design page keeps its own jump menu on the same key.
  await go(page, "#/d/demo:design-epic");
  // The jump menu's key listener mounts after the route changes, so press again until it opens.
  await expect(async () => {
    await page.keyboard.press("Control+k");
    await expect(page.getByRole("dialog", nav("Jump to epic"))).toBeVisible({ timeout: 500 });
  }).toPass();
  await expect(switcher(page)).toHaveCount(0);
});

const newForm = (page: Page) => page.getByRole("dialog", { name: "New session" });
const projectSelect = (page: Page) => newForm(page).getByRole("combobox", { name: "Project" });
const worktreeBox = (page: Page) => newForm(page).getByRole("combobox", { name: "Worktree" });
const promptBox = (page: Page) => newForm(page).getByRole("textbox", { name: "First prompt" });
const createButton = (page: Page) => newForm(page).getByRole("button", { name: "Create session" });

const created = async (fixture: Fixture, before: SessionInfo[]) => {
  const known = new Set(before.map((s) => s.id));
  await expect.poll(async () => (await sessions(fixture)).filter((s) => !known.has(s.id)).length).toBe(1);
  return (await sessions(fixture)).find((s) => !known.has(s.id))!;
};

test("85. Create session", async ({ page, start }) => {
  const fixture = await start("agents");
  const before = await sessions(fixture);
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, "#/agents");

  await sidebar(page).getByRole("button", { name: "New session in demo" }).click();
  await expect(projectSelect(page)).toHaveValue("demo");
  await expect(worktreeBox(page)).toHaveValue("Project root");
  await promptBox(page).fill("first words");
  await createButton(page).click();

  const made = await created(fixture, before);
  expect(made.project).toBe("demo");
  await expect(page).toHaveURL(new RegExp(`#/agents/${made.id}$`));
  await expect(newForm(page)).toHaveCount(0);
  await expect(row(page, made)).toHaveAttribute("aria-current", "page");
  await expect(page.getByText("first words", { exact: true })).toBeVisible();
  expect(await fixture.cli("agents", "list")).toContain(made.id);
});

test("86. Create in a new worktree", async ({ page, start }) => {
  const fixture = await start("agents");
  const before = await sessions(fixture);
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, "#/agents");

  await sidebar(page).getByRole("button", { name: "New session in demo" }).click();
  await expect(newForm(page)).toBeVisible();

  // A bad name shows the refusal in the form and keeps the input.
  await worktreeBox(page).fill("bad name");
  await page.getByRole("option", { name: /Create worktree/ }).click();
  await createButton(page).click();
  await expect(newForm(page).getByRole("alert")).toBeVisible();
  await expect(worktreeBox(page)).toHaveValue("bad name");
  expect(await sessions(fixture)).toHaveLength(before.length);

  // A first prompt that starts with / is refused too.
  await worktreeBox(page).fill("fresh-tree");
  await page.getByRole("option", { name: /Create worktree/ }).click();
  await promptBox(page).fill("/clear");
  await createButton(page).click();
  await expect(newForm(page).getByRole("alert")).toBeVisible();
  await expect(promptBox(page)).toHaveValue("/clear");
  await expect(worktreeBox(page)).toHaveValue("fresh-tree");
  expect(await sessions(fixture)).toHaveLength(before.length);

  // A good one creates the worktree and runs the session in it.
  await promptBox(page).fill("hello");
  await createButton(page).click();
  const made = await created(fixture, before);
  expect(made.dir).toMatch(/fresh-tree$/);
  await expect(page).toHaveURL(new RegExp(`#/agents/${made.id}$`));
  await expect(newForm(page)).toHaveCount(0);
});

test("87. Create entry points", async ({ page, start }) => {
  const fixture = await start("agents");
  const list = await sessions(fixture);
  const inSite = list.find((s) => s.project === "site" && !s.design)!;
  await page.setViewportSize({ width: 1280, height: 800 });
  const closeForm = async () => {
    await newForm(page).getByRole("button", { name: "Close" }).click();
    await expect(newForm(page)).toHaveCount(0);
  };

  // No session selected: a group's "+" prefills its project.
  await go(page, "#/agents");
  await sidebar(page).getByRole("button", { name: "New session in other" }).click();
  await expect(projectSelect(page)).toHaveValue("other");
  await closeForm();

  // With a session selected, the sidebar button and the switcher row prefill its project.
  await go(page, `#/agents/${inSite.id}`);
  await sidebar(page).getByRole("button", { name: "+ New session" }).click();
  await expect(projectSelect(page)).toHaveValue("site");
  await closeForm();
  await titleButton(page).click();
  await switcher(page).getByRole("option", { name: /\+ New session/ }).click();
  await expect(switcher(page)).toHaveCount(0);
  await expect(projectSelect(page)).toHaveValue("site");
  await closeForm();

  // Cmd-K, then the keyboard to the last row.
  await page.keyboard.press("Control+k");
  await switcherFilter(page).press("ArrowUp");
  await switcherFilter(page).press("Enter");
  await expect(projectSelect(page)).toHaveValue("site");

  // The draft survives closing and a fresh page of the same browser.
  await promptBox(page).fill("a draft to keep");
  await closeForm();
  const second = await freshVisit(page, "/#/agents");
  await second.getByRole("button", { name: "+ New session" }).click();
  await expect(promptBox(second)).toHaveValue("a draft to keep");
  await expect(second.getByRole("dialog", { name: "New session" }).getByRole("combobox", { name: "Project" })).toHaveValue("site");
  await second.close();
  expect(await sessions(fixture)).toHaveLength(list.length);
});

test("88. Phone, Agents page", async ({ page, start }) => {
  const fixture = await start("agents");
  const list = await sessions(fixture);
  const target = list[1];
  await page.setViewportSize({ width: 375, height: 812 });
  const { width, height } = page.viewportSize()!;

  // The list alone, with no session panel beside it.
  await go(page, "#/agents");
  await expect(row(page, target)).toBeVisible();
  await expect(page.getByRole("heading", { name: "Pick a session" })).toHaveCount(0);

  // A row opens the session on its own; "‹" returns to the list.
  await row(page, target).click();
  await expect(page).toHaveURL(new RegExp(`#/agents/${target.id}$`));
  await expect(row(page, target)).toHaveCount(0);
  await expect(composer(page)).toBeVisible();
  await page.getByRole("link", { name: "All sessions" }).click();
  await expect(page).toHaveURL(/#\/agents$/);
  await expect(row(page, target)).toBeVisible();

  // The switcher covers the screen and Close closes it.
  await row(page, target).click();
  await titleButton(page).click();
  await expect(switcher(page)).toBeVisible();
  await expect
    .poll(async () => {
      const box = (await switcher(page).boundingBox())!;
      return [box.x, box.y, box.width, box.height];
    })
    .toEqual([0, 0, width, height]);
  await switcher(page).getByRole("button", { name: "Close" }).click();
  await expect(switcher(page)).toHaveCount(0);

  // Create opens as a sheet at the bottom of the screen.
  await page.getByRole("link", { name: "All sessions" }).click();
  await page.getByRole("button", { name: "+ New session", exact: true }).click();
  await expect(newForm(page)).toBeVisible();
  await expect(page.getByTestId("grab-handle")).toBeVisible();
  await expect
    .poll(async () => {
      const box = (await newForm(page).boundingBox())!;
      return [box.x, Math.round(box.y + box.height), box.width];
    })
    .toEqual([0, height, width]);
  await newForm(page).getByRole("button", { name: "Close" }).click();
  await expect(newForm(page)).toHaveCount(0);
});

test("90. Agents offline", async ({ page, start }) => {
  const fixture = await start("agents");
  const idle = (await sessions(fixture))[0];
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, `#/agents/${idle.id}`);
  await expect(page.getByText(REPLY)).toBeVisible();
  await messageBox(page).fill("typed before the drop");
  await sidebar(page).getByRole("button", { name: "New session in demo" }).click();
  await promptBox(page).fill("a first prompt");
  await expect(createButton(page)).toBeEnabled();

  await fixture.stopServer();
  await expect(page.getByText("Offline, reconnecting…")).toBeVisible({ timeout: 15_000 });
  await expect(createButton(page)).toBeDisabled();
  await expect(promptBox(page)).toHaveValue("a first prompt");
  await newForm(page).getByRole("button", { name: "Close" }).click();

  // The last data stays; Send is off with its reason and the draft is kept.
  await expect(row(page, idle)).toBeVisible();
  await expect(page.getByText(REPLY)).toBeVisible();
  await expect(composer(page).getByText("Offline. Send turns on when gx reconnects.")).toBeVisible();
  await expect(sendButton(page)).toBeDisabled();
  await expect(messageBox(page)).toHaveValue("typed before the drop");
});

test("91. Removed session draft", async ({ page, start }) => {
  const fixture = await start("agents");
  const exited = (await sessions(fixture)).find((s) => s.status.state === "exited")!;
  await page.setViewportSize({ width: 1280, height: 800 });
  const draftKey = `gx.draft.composer.${exited.id}`;
  const stored = () => page.evaluate((k) => localStorage.getItem(k), draftKey);

  // The draft is written in a tab that is then closed.
  const tab = await freshVisit(page, `/#/agents/${exited.id}`);
  await messageBox(tab).fill("a draft for a dead session");
  await expect(composer(tab).getByText("Exited. Restore it in gx, Agents tab.")).toBeVisible();
  await expect.poll(stored).toBe("a draft for a dead session");
  await tab.close();

  // Removing the session and opening the Epics page drops the draft.
  await fixture.cli("agents", "remove", exited.id);
  await go(page, "#/");
  await expect.poll(stored).toBeNull();

  // A new session starts empty. Ids are random, so it cannot reuse the removed one's label.
  const before = await sessions(fixture);
  await fixture.cli("agents", "create", "--project", "demo", "--name", exited.id, "--prompt", "hello");
  const made = await created(fixture, before);
  await go(page, `#/agents/${made.id}`);
  await expect(messageBox(page)).toHaveValue("");
  await expect(composer(page).getByText("Draft saved in this browser")).toHaveCount(0);
});

test("93. Shared draft", async ({ page, start }) => {
  const fixture = await start("design", { autoAdvanceDelay: "1h" });
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, "#/d/demo:design-epic");
  await page.getByRole("button", nav("Start design session")).click();
  const questions = page.getByRole("group", nav("Questions"));
  await expect(questions.getByText("Q8 - Which store?")).toBeVisible();
  const design = (await sessions(fixture)).find((s) => s.design)!;

  // A half-answered round survives opening the design page in a new tab.
  await questions.getByRole("button", { name: /SQLite/ }).click();
  const answered = (p: Page) => p.getByRole("button", { name: /^Question .*, answered$/ });
  await expect(answered(page)).toHaveCount(1);
  const fresh = await freshVisit(page, "/#/d/demo:design-epic");
  await expect(fresh.getByRole("group", nav("Questions"))).toBeVisible();
  await expect(answered(fresh)).toHaveCount(1);
  await fresh.close();

  // Text typed on either page shows live on the other.
  const agentsPage = await freshVisit(page, `/#/agents/${design.id}`);
  await page.getByRole("button", nav("Note")).click();
  const box = page.getByRole("textbox", { name: "Message" });
  await expect(box).toBeEnabled();
  await messageBox(agentsPage).fill("typed on Agents");
  await expect(box).toHaveValue("typed on Agents");
  await box.fill("typed on Design");
  await expect(messageBox(agentsPage)).toHaveValue("typed on Design");

  // Send answers sends it as the note, and both boxes are empty after.
  await questions.getByRole("button", { name: /Yes/ }).click();
  await page.getByRole("button", nav("Send answers")).click();
  await expect.poll(() => fixture.cli("agents", "chat", design.id)).toContain("typed on Design");
  await expect(messageBox(agentsPage)).toHaveValue("");
  await page.getByRole("button", nav("Note")).click();
  await expect(box).toHaveValue("");
  await agentsPage.close();
});
