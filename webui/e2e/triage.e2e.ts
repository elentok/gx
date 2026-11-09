import { card, expect, nav, section, test } from "./fixture";

const step = (page: import("@playwright/test").Page, name: string, state: string) =>
  page.locator(`li[data-step="${name}"][data-state="${state}"]`);

test.describe("199. Triage journey", () => {
  test("start, Verify chain, Plan, the card and the sidebar popover", async ({ page, start }) => {
    const fixture = await start("triage", { shape: "journey", autoAdvanceDelay: "3s" });
    const followUps = card(page, "follow-ups");

    // Ready to design: count words and a Triage button.
    const ready = await section(page, "Ready to design");
    await expect(ready.getByText("demo · 3 bugs, 2 ideas")).toBeVisible();
    await ready.getByRole("button", nav("Triage")).click();
    await expect(page).toHaveURL(/#\/triage\/demo$/);
    await expect(step(page, "verify", "current")).toBeVisible();

    // The card is Designing now; a second Triage click opens the same page and starts no second record.
    await page.evaluate(() => (location.hash = "#/epics"));
    const designing = await section(page, "Designing");
    await expect(designing.getByRole("link", nav("follow-ups"))).toBeVisible();
    const startedAt = async () => (await fixture.cli("tickets", "triage", "show", "--project", "demo", "--json")).match(/"startedAt":\s*"[^"]+"/)?.[0];
    const first = await startedAt();
    expect(first).toBeTruthy();
    await followUps.getByRole("button", nav("Open")).click();
    await expect(page).toHaveURL(/#\/triage\/demo$/);
    // `triage show` reads one live record: the second click started no other Triage.
    expect(await startedAt()).toBe(first);

    // Verify: the strip counts verdicts, the first session ends, a countdown chains the second.
    await expect(page.getByText(/Triage session running · verdicts saved \d of 5/)).toBeVisible();
    await expect(page.getByText(/Continuing in \ds/)).toBeVisible();
    await expect(step(page, "verify", "done")).toBeVisible();

    // Plan ends with no strip, no Continue and no countdown; the page waits at Approve.
    await expect(step(page, "approve", "current")).toBeVisible();
    await expect(page.getByText(/Triage session running/)).toHaveCount(0);
    await expect(page.getByRole("button", { name: /^Continue/ })).toHaveCount(0);
    await expect(page.getByText(/Continuing in/)).toHaveCount(0);

    // The card is in Needs you, waiting, with Review plan.
    await page.evaluate(() => (location.hash = "#/epics"));
    const needsYou = await section(page, "Needs you");
    await expect(needsYou.getByText("Waiting for you")).toBeVisible();
    await expect(needsYou.getByRole("button", nav("Review plan"))).toBeVisible();

    // The sidebar badge shows the count and is amber at Approve; its popover line opens the Triage page.
    const inbox = page.getByRole("button", { name: /^Follow-ups · \d+ open$/ });
    await expect(inbox).toBeVisible();
    const badge = inbox.locator("span");
    await expect(badge).toHaveText("5");
    await expect(badge).toHaveCSS("background-color", await page.evaluate(() => {
      const probe = document.createElement("div");
      probe.style.color = "var(--chakra-colors-tone-waiting-fg)";
      document.body.append(probe);
      const color = getComputedStyle(probe).color;
      probe.remove();
      return color;
    }));
    await inbox.click();
    await page.getByRole("menuitem", { name: /demo/ }).click();
    await expect(page).toHaveURL(/#\/triage\/demo$/);
  });
});
