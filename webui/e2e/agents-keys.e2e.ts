import type { Page } from "@playwright/test";
import { expect, test, type Fixture } from "./fixture";
import { chatOf, claudeIdle, composer, go, sendButton, sessions } from "./agentsKit";

const conversationOf = async (fixture: Fixture, id: string) => (await chatOf(fixture, id)).conversation_id;
const stateOf = async (fixture: Fixture, id: string) => (await sessions(fixture)).find((s) => s.id === id)!.status.state;

const failure = (page: Page) => page.getByRole("region", { name: "Session failure" });
const screenDialog = (page: Page) => page.getByRole("dialog", { name: "Screen" });
const typeBox = (page: Page) => screenDialog(page).getByRole("textbox", { name: "Text to type" });
const typeButton = (page: Page) => screenDialog(page).getByRole("button", { name: "Type", exact: true });
const keyButton = (page: Page, name: string) => screenDialog(page).getByRole("group", { name: "Keys" }).getByRole("button", { name, exact: true });

// The sequences the keys route sends, in the key bar's order.
const KEYS: [name: string, cli: string, bytes: string][] = [
  ["Esc", "esc", "\x1b"],
  ["Enter", "enter", "\r"],
  ["Tab", "tab", "\t"],
  ["Up", "up", "\x1b[A"],
  ["Down", "down", "\x1b[B"],
  ["Left", "left", "\x1b[D"],
  ["Right", "right", "\x1b[C"],
  ["Ctrl+C", "ctrl+c", "\x03"],
];

/** What the fake read after `before`, the input log it had then. */
const readSince = async (fixture: Fixture, conv: string, before: string) => (await fixture.input(conv)).slice(before.length);

/** Counts the screen reads the page makes. */
function countScreenReads(page: Page) {
  let reads = 0;
  page.on("request", (r) => {
    if (r.url().includes("/v1/sessions/screen")) reads++;
  });
  return () => reads;
}

async function openScreen(page: Page, id: string) {
  await go(page, `#/agents/${id}`);
  await failure(page).getByRole("button", { name: "Screen", exact: true }).click();
  await expect(screenDialog(page)).toBeVisible();
  await expect(screenDialog(page).getByLabel("Screen rows")).toBeVisible();
}

test("156. Key bar and Type field in the screen view", async ({ page, start }) => {
  const fixture = await start("agents-failures");
  await page.setViewportSize({ width: 1280, height: 800 });
  // The stalled session is a Working one whose notice offers Screen.
  await expect.poll(async () => (await sessions(fixture)).some((s) => s.failure?.kind === "stalled")).toBe(true);
  const stalled = (await sessions(fixture)).find((s) => s.failure?.kind === "stalled")!;
  const conv = await conversationOf(fixture, stalled.id);
  const reads = countScreenReads(page);
  await openScreen(page, stalled.id);

  // Type sends the text and no Enter; Enter is a key of its own. Each send reads the screen again.
  let before = await fixture.input(conv);
  let seen = reads();
  await typeBox(page).fill("hello");
  await typeButton(page).click();
  await expect.poll(() => readSince(fixture, conv, before)).toBe("hello");
  await expect(typeBox(page)).toHaveValue("");
  await expect.poll(reads).toBeGreaterThan(seen);
  expect(await readSince(fixture, conv, before)).not.toContain("\r");

  // Ctrl+C ends the stalled turn and closes the notice, so it goes last.
  for (const [name, , bytes] of KEYS) {
    before = await fixture.input(conv);
    seen = reads();
    await keyButton(page, name).click();
    await expect.poll(() => readSince(fixture, conv, before)).toBe(bytes);
    if (name !== "Ctrl+C") await expect.poll(reads).toBeGreaterThan(seen);
  }

  await expect.poll(() => stateOf(fixture, stalled.id)).toBe("idle");
});

test("156. Type refuses !x in the screen view", async ({ page, start }) => {
  const fixture = await start("agents-failures");
  await page.setViewportSize({ width: 1280, height: 800 });
  await expect.poll(async () => (await sessions(fixture)).some((s) => s.failure?.kind === "stalled")).toBe(true);
  const stalled = (await sessions(fixture)).find((s) => s.failure?.kind === "stalled")!;
  const conv = await conversationOf(fixture, stalled.id);
  await openScreen(page, stalled.id);

  const before = await fixture.input(conv);
  await typeBox(page).fill("!x");
  await typeButton(page).click();
  await expect(screenDialog(page).getByRole("alert")).toContainText("text may not start with !");
  expect(await readSince(fixture, conv, before)).toBe("");
});

// 156 "Keys by session state" moved to the Go test TestSessionKeys_KeysAndTextReachALiveSessionInEveryReachableState;
// 156 "Queue agent session shows no Screen" moved to AgentsPage.sessionView.test.tsx.

test("156. Error-blocked session, Type and Enter get out", async ({ page, start }) => {
  const fixture = await start("agents-blocked");
  await page.setViewportSize({ width: 1280, height: 800 });
  const errored = (await sessions(fixture)).find((s) => s.failure?.label === "Agent hit an error")!;
  await openScreen(page, errored.id);

  // Screen is the notice's only button, and the composer stays off while the session is Blocked.
  await screenDialog(page).getByRole("button", { name: "Close", exact: true }).click();
  await expect(failure(page).getByRole("button")).toHaveText(["Screen"]);
  await expect(sendButton(page)).toBeDisabled();
  await expect(composer(page).getByText(/^Blocked\./)).toBeVisible();

  await page.getByRole("button", { name: "Screen", exact: true }).click();
  await typeBox(page).fill("go on");
  await typeButton(page).click();
  await keyButton(page, "Enter").click();
  await expect.poll(() => stateOf(fixture, errored.id)).not.toBe("blocked");
});

const BASH_DIALOG = ["────────────────────", "Bash command", "  rg phone-notice", "Do you want to proceed?", "❯ 1. Yes", "  2. No", "Esc to cancel"];
const LONG_ROW = `Select a thing ${"long-row ".repeat(30)}`;

/** Every button of the notice sits inside the viewport, and the notice itself does not overflow. */
async function expectNoticeFits(page: Page, buttons: string[]) {
  const notice = failure(page);
  await expect(notice.getByRole("button")).toHaveText(buttons);
  expect(await notice.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
  for (const button of await notice.getByRole("button").all()) {
    const box = (await button.boundingBox())!;
    expect(box.x).toBeGreaterThanOrEqual(0);
    expect(box.x + box.width).toBeLessThanOrEqual(390);
  }
}

test("160. Phone, key input and Blocked notices", async ({ page, start }) => {
  const fixture = await start("agents-blocked");
  await page.setViewportSize({ width: 390, height: 844 });
  const pageFits = () => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);

  // A dialog with a long row: the notice offers Close dialog, and the screen rows scroll sideways.
  const dialogSession = (await claudeIdle(fixture, 0))!;
  await fixture.controlClaude(await conversationOf(fixture, dialogSession.id), {
    action: "block", waiting_for: "input needed", dialog: [LONG_ROW, "❯ 1. One", "  2. Two", "Esc to cancel"],
  });
  await go(page, `#/agents/${dialogSession.id}`);
  await expectNoticeFits(page, ["Screen", "Close dialog"]);
  expect(await pageFits()).toBe(true);

  await failure(page).getByRole("button", { name: "Screen", exact: true }).click();
  const rows = screenDialog(page).getByLabel("Screen rows");
  await expect(rows).toContainText("long-row");
  expect(await rows.evaluate((el) => el.scrollWidth > el.clientWidth && getComputedStyle(el).overflowX !== "visible")).toBe(true);
  expect(await pageFits()).toBe(true);

  // The keys are at least 44px tall and wrap onto more than one line.
  const ys = new Set<number>();
  for (const [name] of KEYS) {
    const box = (await keyButton(page, name).boundingBox())!;
    expect(box.height, name).toBeGreaterThanOrEqual(44);
    expect(box.x + box.width, name).toBeLessThanOrEqual(390);
    ys.add(Math.round(box.y));
  }
  expect(ys.size).toBeGreaterThan(1);
  const type = (await typeButton(page).boundingBox())!;
  expect(type.height).toBeGreaterThanOrEqual(44);
  await screenDialog(page).getByRole("button", { name: "Close", exact: true }).click();

  // A permission prompt: Deny and Allow once fit beside Screen.
  const permissionSession = (await claudeIdle(fixture, 1))!;
  await fixture.controlClaude(await conversationOf(fixture, permissionSession.id), { action: "block", waiting_for: "permission prompt", dialog: BASH_DIALOG });
  await go(page, `#/agents/${permissionSession.id}`);
  await expectNoticeFits(page, ["Screen", "Deny", "Allow once"]);
  expect(await pageFits()).toBe(true);
});
