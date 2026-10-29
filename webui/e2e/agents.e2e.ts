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
  await expect(header.getByRole("img", { name: "status" })).toBeVisible();
  await expect(header.getByText(idle.status.state, { exact: true })).toBeVisible();
  await expect(header.getByText(idle.dir)).toBeVisible();
  await expect(header.getByText(idle.harness, { exact: true })).toHaveCount(0);
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

  // Live updates: a removed Exited session leaves, a new one arrives.
  const exited = list.find((s) => s.status.state === "exited")!;
  await fixture.cli("agents", "remove", exited.id);
  await expect(row(page, exited)).toHaveCount(0);
  const before = new Set(list.map((s) => s.id));
  await fixture.cli("agents", "create", "--project", "demo", "--prompt", "hello");
  await expect.poll(async () => (await sessions(fixture)).filter((s) => !before.has(s.id)).length).toBe(1);
  const added = (await sessions(fixture)).find((s) => !before.has(s.id))!;
  await expect(row(page, added)).toBeVisible();
  await expect(side.getByText(`${live + 1} live`)).toBeVisible();
});

test("74. Design sessions toggle", async ({ page, start }) => {
  const fixture = await start("agents");
  const list = await sessions(fixture);
  const designs = list.filter((s) => s.design);
  expect(designs.length).toBe(2);
  await go(page, "#/agents");
  const side = sidebar(page);
  const toggle = side.getByRole("checkbox", { name: /^Design sessions/ });

  // Hidden by default, with the count.
  await expect(toggle).not.toBeChecked();
  await expect(side.getByText(`· ${designs.length} hidden`)).toBeVisible();
  for (const s of designs) await expect(row(page, s)).toHaveCount(0);

  // Remembered in a fresh page of the same browser.
  await toggle.check();
  for (const s of designs) await expect(row(page, s)).toBeVisible();
  await expect(side.getByText("hidden")).toHaveCount(0);
  const second = await freshVisit(page, "/#/agents");
  await expect(sidebar(second).getByRole("checkbox", { name: /^Design sessions/ })).toBeChecked();
  for (const s of designs) await expect(row(second, s)).toBeVisible();
  await second.close();

  // A selected design session keeps its row while the rest hide.
  await toggle.uncheck();
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
