import type { Page } from "@playwright/test"
import { messageBox, sendButton } from "./agentsKit"
import { card, epicsButton, expect, nav, section, test } from "./fixture"

const step = (page: Page, name: string, state: string) =>
  page.locator(`li[data-step="${name}"][data-state="${state}"]`)

const openTriage = (page: Page) => page.evaluate(() => (location.hash = "#/triage/demo"))
const openEpics = (page: Page) => page.evaluate(() => (location.hash = "#/epics"))
const openBacklog = (page: Page) => page.evaluate(() => (location.hash = "#/backlog"))

// The round card's own group; the final "Write this plan?" card is one too.
const questionsOf = (page: Page) => page.getByRole("group", nav("Questions"))

// A message to the live session, typed in the composer's Note box and sent with the round: the
// round is open and unanswered, so Send answers asks first and "Send anyway" sends it.
async function tell(page: Page, text: string) {
  await page.getByRole("button", nav("Note")).click()
  await page.getByRole("textbox", { name: "Message" }).fill(text)
  await page.getByRole("button", nav("Send answers")).click()
  await page.getByRole("button", nav("Send anyway")).click()
}

const sendAnswers = (page: Page) => page.getByRole("button", nav("Send answers")).click()

const sideways = (page: Page) =>
  page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)

test.describe("199. Triage journey", () => {
  test("Progress, the round, the card and the badge, Abandon, areas, Write, Done and the phone", async ({
    page,
    start,
  }) => {
    test.setTimeout(360_000)
    const inboxButton = () => page.getByRole("button", { name: /^Follow-ups · \d+ open$/ })

    // Verify: Progress lists the five steps and the strip counts verdicts.
    await start("triage", { shape: "verify" })
    await openTriage(page)
    const progress = page.getByRole("region", nav("Progress"))
    await expect(progress.locator("li[data-step]")).toHaveText([
      /Verify/,
      /Route/,
      /Questions/,
      /Write/,
      /Done/,
    ])
    await expect(step(page, "verify", "current")).toBeVisible()

    // Route: round 1 waits with the new-area card first and three options. The plan summary is
    // hidden at Questions (mock 06).
    await start("triage", { shape: "route" })
    await openTriage(page)
    await expect(page.getByRole("region", nav("Plan summary"))).toHaveCount(0)
    await expect(step(page, "questions", "current")).toBeVisible()
    await expect(page.getByText(/^Round 1 of 3/)).toBeVisible()
    const round = questionsOf(page)
    await expect(round.getByText("Question 1 of 3")).toBeVisible()
    await expect(round.getByRole("button", { name: /^Create tooling/ })).toBeVisible()
    await expect(round.getByRole("button", { name: /^File under parser/ })).toBeVisible()
    await expect(round.getByRole("button", { name: /^Keep for later/ })).toBeVisible()

    // No area seeded: the new-area card has two options.
    await start("triage", { shape: "route-no-area" })
    await openTriage(page)
    await expect(questionsOf(page).getByRole("button", { name: /^Create tooling/ })).toBeVisible()
    await expect(questionsOf(page).getByRole("button", { name: /^Keep for later/ })).toBeVisible()
    await expect(questionsOf(page).getByRole("button", { name: /^File under/ })).toHaveCount(0)

    // 12 areas: no new-area card; the row is listed under "Decided unless you object".
    await start("triage", { shape: "route-12-areas" })
    await openTriage(page)
    await expect(page.getByText("Decided unless you object")).toBeVisible()
    await expect(page.getByText(/Which area does it go to\?/)).toBeVisible()
    await expect(page.getByText(/Create the area/)).toHaveCount(0)
    await expect(questionsOf(page).getByRole("button", { name: /^Create tooling/ })).toHaveCount(0)

    // A round waits with a live session: the card says "Needs you" with Answer, the badge counts
    // the six open follow-ups, and the sidebar popover opens the page.
    await start("triage", { shape: "questions" })
    await openEpics(page)
    const waiting = card(page, "follow-ups")
    await expect(waiting.getByText("Needs you")).toBeVisible()
    await expect(waiting.getByRole("button", nav("Answer"))).toBeVisible()
    await expect(inboxButton().locator("span")).toHaveText("6")
    await inboxButton().click()
    await page.getByRole("menuitem", { name: /demo/ }).click()
    await expect(page).toHaveURL(/#\/triage\/demo$/)
    await expect(page.getByText(/^Round 1 of 3/)).toBeVisible()

    // The final card: "Write this plan?" and Review plan; an objection to a decided answer turns
    // into a question.
    await start("triage", { shape: "final-approval" })
    await openEpics(page)
    const final = card(page, "follow-ups")
    await expect(final.getByText("Write this plan?")).toBeVisible()
    await expect(final.getByRole("button", nav("Review plan"))).toBeVisible()
    await expect(inboxButton().locator("span")).toHaveText("6")
    await final.getByRole("button", nav("Review plan")).click()
    await expect(page).toHaveURL(/#\/triage\/demo$/)
    await expect(page.getByText("Final question")).toBeVisible()
    await tell(page, "Object to decision 1")
    await expect(questionsOf(page).getByText(/Rename the helper while there\?/)).toBeVisible()

    // The journey: no area exists, Verify takes two sessions.
    const fixture = await start("triage", {
      shape: "journey",
      autoAdvanceDelay: "3s",
      hold: true,
      reloads: 1,
    })
    const followUps = card(page, "follow-ups")

    // Ready to design: count words and a Triage button.
    const ready = await section(page, "Ready to design")
    await expect(ready.getByText("demo · 3 bugs, 2 ideas")).toBeVisible()
    await ready.getByRole("button", nav("Triage")).click()
    await expect(page).toHaveURL(/#\/triage\/demo$/)
    await expect(step(page, "verify", "current")).toBeVisible()

    // The card is Designing now; a second Triage click opens the same page and starts no second record.
    await openEpics(page)
    const designing = await section(page, "Designing")
    await expect(designing.getByRole("link", nav("follow-ups"))).toBeVisible()
    const startedAt = async () =>
      (await fixture.cli("tickets", "triage", "show", "--project", "demo", "--json")).match(
        /"startedAt":\s*"[^"]+"/,
      )?.[0]
    const first = await startedAt()
    expect(first).toBeTruthy()
    await followUps.getByRole("button", nav("Continue")).click()
    await expect(page).toHaveURL(/#\/triage\/demo$/)
    // `triage show` reads one live record: the second click started no other Triage.
    expect(await startedAt()).toBe(first)

    // Verify: the session is held working, then it ends and a countdown chains the second.
    await expect(page.getByText("Working: Verify")).toBeVisible()
    await fixture.release()
    await expect(page.getByText(/Continuing in \ds/)).toBeVisible()
    await expect(step(page, "verify", "done")).toBeVisible()

    // Route chains into Questions on its own, and the page waits there: no countdown, the session
    // idle and still alive.
    await expect(step(page, "questions", "current")).toBeVisible()
    await expect(page.getByRole("region", nav("Plan summary"))).toHaveCount(0)
    await expect(page.getByText(/Triage session running/)).toHaveCount(0)
    await expect(page.getByText(/Continuing in/)).toHaveCount(0)
    await expect(
      page.getByRole("form", nav("Composer")).getByRole("button", nav("Note")),
    ).toBeEnabled()
    await expect(page.getByText(/exited/i)).toHaveCount(0)

    // Round 1: the new-area card comes first, then the bug's question, then the idea's. A design
    // question and a decided one are never asked.
    const round1 = questionsOf(page)
    await expect(page.getByText(/^Round 1 of 3/)).toBeVisible()
    await expect(round1.getByText("Question 1 of 3")).toBeVisible()
    await expect(page.getByText("Decided unless you object")).toBeVisible()
    await expect(page.getByText(/Rename the helper while there\?/)).toBeVisible()
    await expect(page.getByText("Which store should it use?")).toHaveCount(0)
    await expect(round1.getByRole("button", { name: /^Create tooling/ })).toBeVisible()
    await expect(round1.getByRole("button", { name: /^File under/ })).toHaveCount(0)

    // The card, the badge and the sidebar popover while the round waits.
    const needsYou = await (async () => {
      await openEpics(page)
      return section(page, "Needs you")
    })()
    await expect(needsYou.getByTestId("pill")).toHaveText("Needs you")
    await expect(needsYou.getByRole("button", nav("Answer"))).toBeVisible()
    const badge = inboxButton().locator("span")
    await expect(badge).toHaveText("5")
    await expect(badge).toHaveCSS(
      "background-color",
      await page.evaluate(() => {
        const probe = document.createElement("div")
        probe.style.color = "var(--chakra-colors-tone-waiting-fg)"
        document.body.append(probe)
        const color = getComputedStyle(probe).color
        probe.remove()
        return color
      }),
    )
    await inboxButton().click()
    await page.getByRole("menuitem", { name: /demo/ }).click()
    await expect(page).toHaveURL(/#\/triage\/demo$/)

    // Keep for later leaves the rows that were headed for the new area; the next question is the
    // bug's, then the idea's.
    await round1.getByRole("button", { name: /^Keep for later/ }).click()
    await expect(round1.getByText("Question 2 of 3")).toBeVisible()
    await expect(round1.getByText(/Fix it in the parser or the caller\?/)).toBeVisible()
    await round1.getByRole("button", { name: /^parser/ }).click()
    await expect(round1.getByText("Question 3 of 3")).toBeVisible()
    await expect(round1.getByText(/Keep the old flag\?/)).toBeVisible()
    await round1.getByRole("button", { name: /^no/ }).click()
    await sendAnswers(page)

    // The final card. Nothing is written until the yes.
    await expect(page.getByText("Final question")).toBeVisible()
    await expect(page.getByText(/Write this plan\?/).first()).toBeVisible()
    await expect(step(page, "write", "todo")).toBeVisible()
    await openEpics(page)
    const finalCard = card(page, "follow-ups")
    await expect(finalCard.getByText("Write this plan?")).toBeVisible()
    await expect(finalCard.getByRole("button", nav("Review plan"))).toBeVisible()
    await expect(badge).toHaveText("5")
    await finalCard.getByRole("button", nav("Review plan")).click()
    await expect(step(page, "write", "todo")).toBeVisible()
    await questionsOf(page)
      .getByRole("button", { name: /^Yes, write it/ })
      .click()
    await sendAnswers(page)
    await expect(step(page, "write", "current")).toBeVisible()

    // Write: the fake agent fails the first row before its move and the fourth after it.
    const failed = (address: string) =>
      page.getByText(address, { exact: true }).locator("xpath=../../..")
    await expect(page.getByText("ticket claimed by another agent")).toHaveCount(1)
    await expect(page.getByText("epic add failed")).toHaveCount(1)
    await expect(failed("demo:follow-ups/01").getByRole("button", nav("Retry"))).toBeVisible()
    await expect(
      failed("demo:follow-ups/01").getByRole("button", nav("Keep for later")),
    ).toBeVisible()
    // A row that already moved cannot be kept in follow-ups: Retry only.
    await expect(failed("demo:follow-ups/04").getByRole("button", nav("Retry"))).toBeVisible()
    await expect(
      failed("demo:follow-ups/04").getByRole("button", nav("Keep for later")),
    ).toHaveCount(0)

    // The card: failures to review, with Retry failed (2).
    await openEpics(page)
    const failedCard = card(page, "follow-ups")
    await expect(failedCard.getByRole("button", { name: /^Retry failed \(2\)/ })).toBeVisible()
    await expect(failedCard.getByText("3 of 5 written")).toBeVisible()
    await failedCard.getByRole("button", nav("Continue")).click()
    await expect(page).toHaveURL(/#\/triage\/demo$/)

    // Keep for later leaves the first follow-up where it is. The row that moved and then failed
    // retries to done, and the Triage ends.
    await failed("demo:follow-ups/01").getByRole("button", nav("Keep for later")).click()
    await expect(
      failed("demo:follow-ups/01").getByRole("button", nav("Keep for later")),
    ).toHaveCount(0)
    await failed("demo:follow-ups/04").getByRole("button", nav("Retry")).click()
    const result = page.getByRole("region", nav("Triage result"))
    await expect(result.getByRole("region", nav("Kept for later"))).toBeVisible()

    // The kept follow-ups are still in follow-ups; the answer is in the ticket body of the ready epic.
    // Writes made through the fixture CLI land late, so each read polls.
    await expect
      .poll(() => fixture.cli("tickets", "show", "demo:follow-ups/01"))
      .toContain("# bug-01")
    await expect
      .poll(() => fixture.cli("tickets", "show", "demo:follow-ups/02"))
      .toContain("# bug-02")

    // Done: the result groups, each with links where rows moved.
    await expect(
      result.getByRole("region", nav("Ready epic")).getByRole("link").first(),
    ).toBeVisible()
    await expect(result.getByRole("region", nav("Closed"))).toBeVisible()

    // No area row anywhere after the Triage.
    await openEpics(page)
    await section(page, "Ready to design")
    await expect(page.getByText("tooling")).toHaveCount(0)
    await expect(followUps.getByText("demo · 3 bugs")).toBeVisible()
    const queued = () => fixture.cli("server", "queue", "items")
    expect(await queued()).not.toContain("triage-ready")

    // The Queue button sits on the ready line of the Result card, and queues the ready epic.
    await openTriage(page)
    const actions = page.getByTestId("page-header-actions")
    const readyLine = (p: Page) =>
      p.getByRole("region", nav("Triage result")).getByRole("region", nav("Ready epic"))
    const queue = readyLine(page).getByRole("button", nav("Queue it"))
    await expect(queue).toBeVisible()
    await queue.click()
    await expect.poll(queued).toContain("triage-ready")
    await expect(page.getByRole("button", nav("Queue it"))).toHaveCount(0)
    // The chip text moves on (queued, running, finished) while the suite loads, so match the states, never a captured word.
    const queuedChip = (p: Page) =>
      readyLine(p)
        .getByText(/^(queued|running|ready|finished)$/i)
        .first()
    await expect(queuedChip(page)).toBeVisible()

    // The queued state survives a reload, and a second tab shows it too.
    await page.reload()
    await expect(page.getByRole("button", nav("Queue it"))).toHaveCount(0)
    await expect(queuedChip(page)).toBeVisible()
    const tab = await page.context().newPage()
    await tab.goto(page.url())
    await expect(tab.getByRole("region", nav("Triage result"))).toBeVisible()
    await expect(tab.getByRole("button", nav("Queue it"))).toHaveCount(0)
    await expect(queuedChip(tab)).toBeVisible()
    await tab.close()

    // History and Abandon: a second Triage starts from the card, and Abandon changes no ticket
    // and no area.
    await openEpics(page)
    const again = await section(page, "Ready to design")
    await again.getByRole("button", nav("Triage")).click()
    await expect(page).toHaveURL(/#\/triage\/demo$/)
    await expect(step(page, "verify", "done")).toBeVisible()

    // The Agents page labels the Triage sessions.
    const sessions = JSON.parse(await fixture.cli("agents", "list", "--json")) as {
      id: string
      flow?: string
    }[]
    const triageSession = sessions.find((s) => s.flow === "triage")
    expect(triageSession).toBeTruthy()
    await page.evaluate((id) => (location.hash = `#/agents/${id}`), triageSession!.id)
    await expect(page.getByRole("link", nav("Open triage page →"))).toBeVisible()
    await openTriage(page)
    const ticketsBefore = await fixture.cli("tickets", "list", "demo:follow-ups")
    // The epic list's addresses stand for the areas.
    const epicAddresses = async () =>
      (JSON.parse(await fixture.cli("server", "epics", "--json")).epics as { address: string }[])
        .map((e) => e.address)
        .sort()
        .join(",")
    const epicsBefore = await epicAddresses()
    await actions.getByRole("button", nav("More actions")).click()
    await page.getByRole("menuitem", nav("Abandon Triage")).click()
    const abandoned = page.waitForResponse((r) => r.url().includes("/v1/triage/abandon"))
    await page
      .getByRole("dialog", nav("Abandon Triage"))
      .getByRole("button", nav("Abandon"))
      .click()
    expect((await (await abandoned).json()).refused).toBeUndefined()
    const past = page.getByRole("region", nav("Past Triages"))
    await expect(past.locator("[data-history-item]")).toHaveCount(2)
    expect(await fixture.cli("tickets", "list", "demo:follow-ups")).toBe(ticketsBefore)
    expect(await epicAddresses()).toBe(epicsBefore)
    // Newest first: the abandoned Triage, then the done one.
    await expect(past.locator("[data-history-item]").nth(0)).toContainText("abandoned")
    await expect(past.locator("[data-history-item]").nth(1)).toContainText("done")

    // The card shows a plain Triage button again.
    await openEpics(page)
    await expect(
      (await section(page, "Ready to design")).getByRole("button", nav("Triage")),
    ).toBeVisible()

    // Phone: a third Triage waits at Questions with no Progress panel and no sideways scroll; the
    // step line opens the steps sheet, and the header keeps its ring and menu.
    await openEpics(page)
    await (await section(page, "Ready to design")).getByRole("button", nav("Triage")).click()
    await expect(step(page, "questions", "current")).toBeVisible()
    await page.setViewportSize({ width: 390, height: 844 })
    await expect(page.getByRole("region", nav("Progress"))).toHaveCount(0)
    expect(await sideways(page)).toBe(false)
    const stepLine = page.getByRole("button", { name: /^Questions/ }).first()
    await stepLine.click()
    const sheet = page.getByRole("dialog", nav("Triage progress steps"))
    await expect(step(page, "questions", "current")).toBeVisible()
    await sheet.getByRole("button", nav("Close")).click()
    await expect(sheet).toHaveCount(0)
    await expect(page.getByRole("button", nav("Show progress steps"))).toBeVisible()
    await expect(page.getByRole("button", nav("More actions"))).toBeVisible()
    await expect(page.getByRole("heading", nav("Triage: demo"))).toBeVisible()
    expect(await sideways(page)).toBe(false)

    // The sidebar drawer shows the badge and popover without scrolling sideways.
    await page.getByRole("button", nav("Show sidebar")).click()
    await expect(inboxButton()).toBeVisible()
    await inboxButton().click()
    await expect(page.getByRole("menuitem", { name: /demo/ })).toBeVisible()
    expect(await sideways(page)).toBe(false)

    // Backlog: the Epics menu has the item, the sidebar has no row, and each area shows its open count.
    await page.setViewportSize({ width: 1280, height: 800 })
    const backlog = await start("triage", { shape: "backlog" })
    await expect(page.getByRole("link", { name: /Backlog/ })).toHaveCount(0)
    await epicsButton(page).click()
    await page.getByRole("menuitem", { name: "Backlog", exact: true }).click()
    await expect(page).toHaveURL(/#\/backlog$/)
    // The sidebar lists epics too, so rows are scoped to the project's region.
    const area = (name: string) =>
      page
        .getByRole("region", { name: "demo", exact: true })
        .getByRole("listitem")
        .filter({ hasText: new RegExp(`^${name}`) })
    const startDesign = (name: string) => area(name).getByRole("button", { name: "Start Design" })
    const startButton = (name: string) =>
      page
        .getByRole("dialog", nav(`Start Design for ${name}`))
        .getByRole("button", { name: "Start", exact: true })
    await expect(area("parser")).toContainText("11 open")
    await expect(area("sync")).toContainText("5 open")
    await expect(area("webui")).toContainText("0 open")
    await expect(startDesign("webui")).toBeDisabled()

    // The 5-ticket area: the chat prints the card with all five tickets, each with a reason.
    const cards = () => page.getByText("Start this Design?")
    const say = async (text: string) => {
      // A turn typed while the session still works is lost.
      await expect(page.getByText("Working", { exact: true })).toHaveCount(0)
      await messageBox(page).fill(text)
      await sendButton(page).click()
    }
    await startDesign("sync").click()
    await startButton("sync").click()
    await expect(page).toHaveURL(/#\/agents\//)
    await expect(cards()).toHaveCount(1)
    await expect(page.getByText(/^demo:sync\/\d+:/)).toHaveCount(5)
    await expect(page.getByText(/same theme as the other sync tickets/)).toHaveCount(5)
    // A live chat blocks Finish move on its area, so this one goes before the move.
    await backlog.cli("agents", "remove", "--yes", page.url().split("/agents/")[1])

    // The 11-ticket area: the dialog takes a hint, a typed edit reprints the card once, yes prints a link.
    await openBacklog(page)
    await startDesign("parser").click()
    const dialog = page.getByRole("dialog", nav("Start Design for parser"))
    await dialog.getByRole("textbox", { name: "Hint" }).fill("retry theme")
    await startButton("parser").click()
    await expect(page).toHaveURL(/#\/agents\//)
    const chatUrl = page.url()
    await expect(cards()).toHaveCount(1)
    await say("drop 1")
    await expect(cards()).toHaveCount(2)
    await say("yes")
    await expect(page.getByText("Created the Design")).toBeVisible()
    const design = await backlog.cli("tickets", "show", "demo:parser-cleanup")
    expect(design).toContain("kind: map")
    expect(design).toContain("destination_unconfirmed: true")
    expect(design).toContain("- demo:parser/11")
    expect(design).not.toContain("- demo:parser/01")
    // The area keeps the dropped ticket and the claimed one.
    const left = await backlog.cli("tickets", "list", "demo:parser")
    expect(left).toContain("parser-01")
    expect(left).toContain("parser-12")

    // A second Start Design click on that area opens the live chat.
    await openBacklog(page)
    await expect(area("parser")).toContainText("1 open")
    await startDesign("parser").click()
    await startButton("parser").click()
    await expect(page).toHaveURL(chatUrl)

    // Finish move completes the half-filled map with no new chat.
    await openBacklog(page)
    await area("sync").getByRole("button", { name: "Finish move" }).click()
    await expect(area("sync").getByRole("button", { name: "Finish move" })).toHaveCount(0)
    expect(await backlog.cli("tickets", "show", "demo:retry-work")).toContain("sync-04")
    await expect(page).toHaveURL(/#\/backlog$/)

    // Phone: the Start Design dialog is a sheet with no sideways scroll.
    await page.setViewportSize({ width: 390, height: 844 })
    await startDesign("sync").click()
    await expect(page.getByRole("dialog", nav("Start Design for sync"))).toBeVisible()
    expect(await sideways(page)).toBe(false)

    // Old epics: the Route summary lists the moves and the waiting epic; the table areas make
    // no question of their own.
    await page.setViewportSize({ width: 1280, height: 800 })
    // A long countdown keeps Write waiting, so its view is read before the session writes.
    const old = await start("triage", { shape: "old-epics", autoAdvanceDelay: "60s" })
    await page.evaluate(() => (location.hash = "#/triage/gx"))
    await expect(page.getByText("move test-flakes -> area test-health: 2 tickets")).toBeVisible()
    await expect(
      page.getByText("move agent-guidance -> area agent-guidance-and-skills: 1 tickets"),
    ).toBeVisible()
    await expect(
      page.getByText("move ticket-authoring -> area ticket-authoring: 1 tickets"),
    ).toBeVisible()
    await expect(
      page.getByText("waiting: stuck-ticket-watchdog holds claimed gx:stuck-ticket-watchdog/01"),
    ).toBeVisible()
    const oldRound = questionsOf(page)
    await expect(oldRound.getByText("Question 1 of 3")).toBeVisible()
    await expect(page.getByText('Create the area "tooling"?', { exact: true })).toBeVisible()
    await expect(
      oldRound.getByText(/test-health|agent-guidance-and-skills|ticket-authoring/),
    ).toHaveCount(0)
    await oldRound.getByRole("button", { name: /^Keep for later/ }).click()
    await oldRound.getByRole("button", { name: /^parser/ }).click()
    await oldRound.getByRole("button", { name: /^no/ }).click()
    await sendAnswers(page)
    await expect(page.getByText(/Write this plan\?/).first()).toBeVisible()
    await questionsOf(page)
      .getByRole("button", { name: /^Yes, write it/ })
      .click()
    await sendAnswers(page)
    // The Write view counts the four old-epic move units with the six rows.
    await expect(page.getByText("Write · 0 of 10 rows")).toBeVisible()
    await expect(page.getByText(/^move test-flakes -> test-health$/)).toBeVisible()
    await page.getByRole("button", nav("Continue Write")).click()
    await expect(page.getByRole("region", nav("Triage result"))).toBeVisible()

    // By CLI: an emptied epic is stamped with only done tickets; the claimed one stayed.
    // An archived epic is out of `tickets show`; the archived list carries its stamp.
    const archived: { name: string; completed_at?: string }[] = JSON.parse(
      await old.cli("server", "epics", "archived", "--json"),
    ).epics
    for (const name of ["test-flakes", "agent-guidance", "scratch-ideas"]) {
      expect(archived.find((e) => e.name === name)?.completed_at).toBeTruthy()
    }
    // Past charting, claimed, and named like its area: all stay, unstamped.
    for (const name of ["tickets-cli", "stuck-ticket-watchdog", "ticket-authoring"]) {
      expect(archived.find((e) => e.name === name)).toBeUndefined()
      expect(await old.cli("tickets", "show", `gx:${name}`)).not.toContain("completed_at")
    }
    expect(await old.cli("tickets", "list", "gx:stuck-ticket-watchdog")).toContain(
      "stuck-ticket-watchdog-01",
    )
    // A second Triage sees the claimed epic as waiting and plans no unit.
    const oldEpicsLeft = await old.cli("tickets", "triage", "oldepics", "--project", "gx", "--json")
    expect(oldEpicsLeft).toContain("stuck-ticket-watchdog")
    expect(oldEpicsLeft).toMatch(/"waiting":\s*true/)
    // A map past charting refuses the move.
    await expect(old.cli("tickets", "move", "gx:tickets-cli/01", "gx:test-health")).rejects.toThrow(
      /move-map-source[\s\S]*Design has started/,
    )
  })
})
