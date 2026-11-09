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

    // Approve opens on Questions: three open, the bar disabled and linking back to Questions.
    const approveBar = page.getByRole("group", nav("Approve"));
    const approve = approveBar.getByRole("button", nav("Approve and write"));
    await expect(page.getByRole("button", { name: "Questions 3" })).toBeVisible();
    await expect(approveBar.getByText("3 questions left.")).toBeVisible();
    await expect(approve).toBeDisabled();
    await expect(approveBar.getByRole("button", nav("Answer them"))).toBeVisible();

    // Answering in the question component enables the bar and moves the page to Review.
    const questions = page.getByRole("group", nav("Questions"));
    await expect(questions).toBeVisible();
    for (const [id, answer] of [["q1", "parser"], ["q2", "no"], ["d1", "the sqlite store"]]) {
      await questions.getByRole("textbox", nav(`Answer to question ${id}`)).fill(answer);
      if (id !== "d1") await questions.getByRole("button", nav("Next →")).click();
    }

    // Review: the bar enables, nothing is changed yet.
    await expect(page.getByRole("button", { name: /^Review 5 rows/ })).toHaveAttribute("aria-pressed", "true");
    await expect(approve).toBeEnabled();
    await expect(approveBar.getByText(/changed by you/)).toHaveCount(0);

    // Changing a row's action outlines it and counts it.
    const collapsed = page.locator('[data-part="action-group"]').getByRole("button", { expanded: false }).filter({ hasText: "▸" });
    for (let n = await collapsed.count(); n > 0; n--) await collapsed.first().click();
    await expect(collapsed).toHaveCount(0);
    const rowOf = (title: string) => page.getByRole("button", nav(`Action for ${title}`)).locator("xpath=..");
    const pick = async (menu: string, item: string) => {
      await page.getByRole("button", nav(menu)).click();
      await page.getByRole("menuitem", { name: item, exact: true }).click();
    };
    const toMapEpic = async (title: string) => {
      await pick(`Action for ${title}`, "Map epic");
      await pick(`Destination for ${title}`, "New map epic…");
      await page.getByRole("textbox", nav("New map epic name")).fill("triage-map");
      await page.keyboard.press("Enter");
    };
    // The fifth row leaves Keep for the map epic, so every follow-up leaves follow-ups.
    await toMapEpic("idea-05");
    await expect(approveBar.getByText("1 changed by you")).toBeVisible();
    await expect(rowOf("idea-05")).toHaveAttribute("data-changed", "true");

    // Setting the merge target away from a ready or map epic outlines its merged rows and blocks the bar.
    await pick("Destination for bug-03", "idea-05");
    await pick("Action for idea-05", "Close");
    await expect(rowOf("bug-03")).toHaveAttribute("data-orphan", "true");
    await expect(approveBar.getByText("A merge target is gone. Pick a new one.")).toBeVisible();
    await expect(approve).toBeDisabled();
    await pick("Destination for bug-03", "bug-01");
    await expect(rowOf("bug-03")).not.toHaveAttribute("data-orphan", "true");
    await expect(approve).toBeEnabled();
    await toMapEpic("idea-05");
    await expect(approve).toBeEnabled();

    // Two tabs on the same page. An edit in the second makes the first tab's save stale: it is refused and Review reloads.
    const other = await page.context().browser()!.newContext({ ignoreHTTPSErrors: true, storageState: await page.context().storageState() });
    try {
      const second = await other.newPage();
      await second.goto(page.url());
      await expect(second.getByRole("button", { name: /^Review 5 rows/ })).toHaveAttribute("aria-pressed", "true");
      const collapsedSecond = second.locator('[data-part="action-group"]').getByRole("button", { expanded: false }).filter({ hasText: "▸" });
      for (let n = await collapsedSecond.count(); n > 0; n--) await collapsedSecond.first().click();
      // The first tab live-updates, so it is held on the old plan until its own save is answered.
      let held = true;
      await page.route(/\/v1\/triage\?/, (route) => (held && route.request().method() === "GET" ? route.abort() : route.continue()));
      await page.route(/\/v1\/triage\/row$/, async (route) => {
        const res = await route.fetch();
        held = false;
        await route.fulfill({ response: res });
      });
      await second.getByRole("button", nav("Destination for bug-03")).click();
      await second.getByRole("menuitem", { name: "bug-02", exact: true }).click();
      await expect(second.getByRole("button", nav("Destination for bug-03"))).toHaveText(/bug-02/);

      // Its save carries the old revision: refused as stale, and Review reloads with the second tab's edit.
      await expect(page.getByRole("button", nav("Destination for bug-03"))).toHaveText(/bug-01/);
      await pick("Destination for bug-03", "bug-02");
      await expect(page.getByText("The plan changed. Review it again.")).toBeVisible();
      await expect(page.getByRole("button", nav("Destination for bug-03"))).toHaveText(/bug-02/);

      // Put the merge target back; both tabs then read the same revision.
      await pick("Destination for bug-03", "bug-01");
      await expect(page.getByRole("button", nav("Destination for bug-03"))).toHaveText(/bug-01/);
      await expect(second.getByRole("button", nav("Destination for bug-03"))).toHaveText(/bug-01/);

      // Both press Approve: one wins and Write starts, the other is told it was approved.
      // Each tab is held on the plan until its own Approve is answered, so both really send one.
      for (const p of [page, second]) {
        let waiting = true;
        await p.route(/\/v1\/triage\?/, (route) => (waiting && route.request().method() === "GET" ? route.abort() : route.fallback()));
        await p.route(/\/v1\/triage\/approve$/, async (route) => {
          const res = await route.fetch();
          waiting = false;
          await route.fulfill({ response: res });
        });
      }
      const answered = [page, second].map((p) => p.waitForResponse((r) => r.url().endsWith("/v1/triage/approve")).then((r) => r.json()));
      await Promise.all([
        page.getByRole("button", nav("Approve and write")).click({ force: true }),
        second.getByRole("button", nav("Approve and write")).click({ force: true }),
      ]);
      await expect(step(page, "write", "current")).toBeVisible();
      await expect(step(second, "write", "current")).toBeVisible();
      const reasons = (await Promise.all(answered)).map((r) => (r.refused ? r.reason : "approved"));
      expect(reasons.sort()).toEqual(["approved", "triage-already-approved"]);
      // Only one Write session was launched, whichever tab won.
      await expect(page.getByRole("link", { name: /^Chat triage (?!Exited)/ })).toHaveCount(1);
    } finally {
      await other.close();
    }

    // Write: the fake agent fails the first and fifth rows before their move and the second after it.
    const failed = (address: string) => page.getByText(address, { exact: true }).locator("xpath=../..");
    await expect(page.getByText("ticket claimed by another agent")).toHaveCount(2);
    await expect(page.getByText("epic add failed")).toHaveCount(1);
    await expect(page.getByText("done", { exact: true })).toHaveCount(2);
    for (const address of ["demo:follow-ups/01", "demo:follow-ups/05"]) {
      await expect(failed(address).getByRole("button", nav("Retry"))).toBeVisible();
      await expect(failed(address).getByRole("button", nav("Keep for later"))).toBeVisible();
    }
    // A row that already moved cannot be kept in follow-ups: Retry only.
    await expect(failed("demo:follow-ups/02").getByRole("button", nav("Retry"))).toBeVisible();
    await expect(failed("demo:follow-ups/02").getByRole("button", nav("Keep for later"))).toHaveCount(0);

    // The card: no session, no pending row, failures to review.
    await page.evaluate(() => (location.hash = "#/epics"));
    const failedCard = await section(page, "Needs you");
    await expect(failedCard.getByText("demo · 1 bug, 1 idea")).toBeVisible();
    await expect(failedCard.getByRole("button", nav("Review failures"))).toBeVisible();
    await failedCard.getByRole("button", nav("Review failures")).click();
    await expect(page).toHaveURL(/#\/triage\/demo$/);

    // Retry launches a Write session and the row goes done, leaving follow-ups.
    await failed("demo:follow-ups/01").getByRole("button", nav("Retry")).click();
    await expect(page.getByText("Write session running")).toBeVisible();
    await expect(page.getByText(/^→ demo:triage-ready\/\d+$/)).toBeVisible();
    await expect(page.getByText("ticket claimed by another agent")).toHaveCount(1);
    await expect(page.getByText("done", { exact: true })).toHaveCount(3);
    await page.evaluate(() => (location.hash = "#/epics"));
    await expect((await section(page, "Needs you")).getByText("demo · 1 idea")).toBeVisible();
    await page.evaluate(() => (location.hash = "#/triage/demo"));

    // Keep for later leaves the fifth follow-up where it is. The row that moved and then failed retries to done, and the Triage ends.
    await failed("demo:follow-ups/05").getByRole("button", nav("Keep for later")).click();
    await expect(failed("demo:follow-ups/05").getByRole("button", nav("Keep for later"))).toHaveCount(0);
    await failed("demo:follow-ups/02").getByRole("button", nav("Retry")).click();
    const result = page.getByRole("region", nav("Triage result"));
    await expect(result.getByRole("region", nav("Kept for later"))).toBeVisible();

    // The kept follow-up is still in follow-ups; the answers are in the ticket bodies of the ready epic.
    // Writes made through the fixture CLI land late, so each read polls.
    await expect.poll(() => fixture.cli("tickets", "show", "demo:follow-ups/05")).toContain("# idea-05");
    await expect
      .poll(() => fixture.cli("tickets", "show", "demo:triage-ready/02"))
      .toMatch(/Fix it in the parser or the caller\?: parser[\s\S]*Keep the old flag\?: no/);
  });
});
