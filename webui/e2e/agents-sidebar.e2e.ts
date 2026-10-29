import { expect, test } from "./fixture";
import { freshVisit, go, isActionable, row, sidebar, sessions } from "./agentsKit";

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
  await row(page, waiting).getByRole("img", { name: "Actionable" }).hover();
  await expect(row(page, waiting).getByRole("tooltip")).toHaveText(`★ Actionable: ${waiting.actionable.reason!}`);

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
