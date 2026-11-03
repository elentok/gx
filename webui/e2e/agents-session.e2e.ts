import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { ask, go, REPLY, row, sessions, titleButton } from "./agentsKit";

/** The chat's scroll box, found from a message in it. */
const scrollBox = (page: Page, text: string) => page.getByText(text, { exact: true }).first();
const scrollTo = async (page: Page, text: string, top: boolean) => {
  const message = scrollBox(page, text);
  await expect(message).toBeVisible();
  await message.evaluate((el, toTop) => {
    let box = el.parentElement;
    while (box && getComputedStyle(box).overflowY !== "auto") box = box.parentElement;
    box!.scrollTop = toTop ? 0 : box!.scrollHeight;
  }, top);
};

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

// Moved to vitest: 77 and 81 (AgentsPage.composer.test.tsx), 82 (AgentsPage.sessionView.test.tsx),
// 80 and 80 crashed (FailureBanner.test.tsx, AgentsPage.composer.test.tsx failureCases),
// 83 (AgentsPage.composer.test.tsx).

// The hold ends about 8 s after the limit in this scenario.
const holdEnds = { timeout: 60_000 };

test("103. Rate-limit hold", async ({ page, start }) => {
  const fixture = await start("rate-limited");
  const verdict = (epic: string) => page.locator(`a[href$="/t/demo:${epic}/01"]`).filter({ hasNotText: "→" }).locator("[data-verdict]");
  // The pages learn of a verdict change on the connection's 60 s tick, so each try moves the page clock by one.
  await page.clock.install();
  const afterTick = (assertion: () => Promise<void>) =>
    expect(async () => {
      await page.clock.fastForward(60_000);
      await assertion();
    }).toPass({ timeout: 40_000, intervals: [500] });

  await page.getByRole("link", { name: "limited-epic", exact: true }).click();
  await afterTick(() => expect(page.getByText(/rate-limited: resets /).first()).toBeVisible({ timeout: 1_000 }));
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
  await expect(verdict).toContainText(/Claude usage: 99% of the weekly limit, resets /);
  await verdict.locator("..").getByRole("button", { name: "Override" }).click();
  await expect(verdict).toHaveCount(0);
});
