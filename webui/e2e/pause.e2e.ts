import type { Locator, Page } from "@playwright/test"
import { realpath } from "node:fs/promises"
import { withState } from "./diagnostics"
import { card, expect, nav, renderedText, test, toasts, type Fixture } from "./fixture"

const recovery = (page: Page) => page.getByRole("form", { name: "Recovery actions" })

// The popover's confirm button carries its key hint ("y") in its name.
const confirmName = { name: /^Start over/ }

// A hash change keeps the page, since the fixture fails a test whose page reloaded.
const openTicket = (page: Page, id: string, epic = "paused-epic") =>
  page.evaluate((hash) => (location.hash = hash), `#/t/demo:${epic}/${id}`)

// The wait loop pauses an agent that stays blocked for its 15 s dwell.
const dwell = { timeout: 30_000 }
// A recovery scenario's agent has to launch, hang (two launches, each with a launch-proof grace) or
// end its turn before the pause appears. The wait stays under its own test's timeout, so a miss fails
// an expect that names the step instead of a test timeout that names none; the margin is the fixture start.
const gateWait = () => ({ timeout: Math.min(150_000, test.info().timeout - 15_000) })
// The header pill; the Runs list repeats the label in a longer line.
const stuck = (page: Page) => page.getByText("Paused · Agent stuck", { exact: true })

// Flows 3 (every pause reason, Repair, the four tooltips) and 115 share one `paused` server.
// Resume and "Retry is disabled while the call runs" are vitest cases (ticket/ActionCard.test.tsx):
// Resume needs ticket 08, which Start over consumes.
test.describe("3. Pause reasons and recovery", () => {
  test("every pause reason recovers", async ({ page, context, start }) => {
    test.setTimeout(240_000)
    await context.grantPermissions(["clipboard-read", "clipboard-write"])
    const fixture = await start("paused")
    await fixture.cli("server", "queue", "add", "demo:paused-epic")

    // The Approve card waits for a real worker session to ask for permission, which is slow under load.
    await expect(card(page, "approve-epic").getByText("wants to run: rg fixture")).toBeVisible({
      timeout: 45_000,
    })
    const pausedCard = card(page, "paused-epic")
    await expect(pausedCard.getByText("Seeded pause: self-reported")).toBeVisible()

    // Repair: the card counts the other pauses and keeps "Fix it with an agent" below the link.
    await expect(pausedCard.getByText(/\+6 more/)).toBeVisible()
    await page.getByRole("link", { name: "paused-epic", exact: true }).click()
    await expect(page.getByText("needs-repair · Run failed").first()).toBeVisible()
    await expect(page.getByRole("button", { name: "Fix it with an agent" })).toHaveCount(3)
    const repair = page
      .locator('[data-tone="deadlocked"]')
      .filter({ has: page.locator('a[href$="/t/demo:paused-epic/07"]') })
      .last()
    await expect(repair.getByText("needs-repair · Run failed")).toBeVisible()
    await expect(repair.getByRole("button").last()).toHaveText("Fix it with an agent")
    const link = (await repair.locator("a").first().boundingBox())!
    const fix = (await repair.getByRole("button", { name: "Fix it with an agent" }).boundingBox())!
    expect(fix.y).toBeGreaterThanOrEqual(link.y + link.height)

    // What a missed wait prints: the recovery card, every open dialog and the ticket.
    const state = (epic: string, id: string) => async () => {
      const read = (p: Promise<string>) => p.catch((e: Error) => `(not readable: ${e.message})`)
      const card = await read(renderedText(recovery(page), 2_000))
      const dialogs = await page
        .getByRole("dialog")
        .allInnerTexts()
        .catch((e: Error) => [`(not readable: ${e.message})`])
      const ticket = await read(fixture.cli("tickets", "show", `demo:${epic}/${id}`))
      return `recovery card:\n${card}\n\nopen dialogs:\n${dialogs.join("\n---\n")}\n\ngx tickets show demo:${epic}/${id}:\n${ticket}`
    }
    // Every step opens the ticket, acts, and waits for its pause to clear.
    const step = (epic: string, id: string, reason: string, act: () => Promise<void>) =>
      test.step(`${epic}/${id}: ${reason}`, async () => {
        await openTicket(page, id, epic)
        await expect(page.getByText(reason).first()).toBeVisible()
        await act()
        await withState(`${epic}/${id}: "${reason}" did not clear`, state(epic, id), async () => {
          await expect(page.getByText(reason)).toHaveCount(0, { timeout: 30_000 })
        })
      })
    // Tooltips are server-supplied title attributes, set before any action changes the tickets.
    const tooltips = [
      ["05", "Merge again", "Merge the ticket's commits onto its target again."],
      ["08", "Resume", "Lift the pause. The ticket waits its turn in the queue."],
      ["09", /^Retry\s*Recommended$/, "Start a fresh agent now. Keeps the branch and its commits."],
      [
        "08",
        "Start over",
        "Throw away the work and run again from the base. A backup branch is kept.",
      ],
    ] as const
    for (const [id, button, title] of tooltips) {
      await openTicket(page, id)
      const target = recovery(page).getByRole(
        "button",
        typeof button === "string" ? nav(button) : { name: button },
      )
      await expect(target).toHaveAttribute("title", title)
    }

    // A question gx asked has no answer card: the first verb is the main button, and the answer page
    // sends the person back to the ticket.
    const gxAsked = async (id: string, verb: string) => {
      await expect(page.getByRole("button", nav("Answer question"))).toHaveCount(0)
      await expect(recovery(page).getByRole("button", nav(verb)).first()).toHaveAttribute(
        "data-emphasis",
        "main",
      )
      await page.evaluate((hash) => (location.hash = hash), `#/answer/demo:paused-epic/${id}`)
      await expect(page.getByText("This question is answered on the ticket page.")).toBeVisible()
      await expect(page.getByRole("button", nav("Send answer"))).toHaveCount(0)
      await page.getByRole("button", nav("Back to ticket")).click()
      await expect(page).toHaveURL(new RegExp(`/t/demo:paused-epic/${id}$`))
    }

    await step("paused-epic", "03", "Waiting for your answer", async () => {
      // The compact card: the asker, the title once and a static button row. The seeded round is
      // gx's own and has no handoff, so the label reads "GX ASKS" and there are no Agent's notes.
      await expect(page.getByText("GX ASKS", { exact: true })).toBeVisible()
      await expect(page.getByText("Seeded pause: self-reported", { exact: true })).toHaveCount(1)
      const answer = page.getByRole("button", nav("Answer question"))
      await expect(answer).toHaveAttribute("data-emphasis", "main")
      await expect(
        recovery(page).getByRole("button", nav("Resume without answering")),
      ).toHaveAttribute("data-emphasis", "secondary")
      await expect(recovery(page)).toHaveCSS("position", "static")
      await answer.click()
      await expect(page).toHaveURL(/#\/answer\/demo:paused-epic\/03$/)
      await page.getByRole("textbox", { name: /^Answer to question/ }).fill("go ahead")
      await page.getByRole("button", nav("Send answer")).click()
      // The answer page's text never shows the pause reason, so the helper's wait would pass
      // at once and the page's late return to the ticket would undo the next step's navigation.
      await expect(page.getByRole("group", { name: "Answered question" })).toBeVisible()
    })
    await step("paused-epic", "04", "Ambiguous base", async () => {
      await expect(recovery(page)).toHaveCSS("position", "sticky")
      await gxAsked("04", "Choose base")
      await page.getByRole("button", nav("01")).click()
      await page.getByRole("button", nav("Choose base")).click()
    })
    await step("paused-epic", "06", "Agent stopped without a result", async () => {
      await gxAsked("06", "Retry")
      await recovery(page).getByRole("button", nav("Retry")).click()
      await expect(page.getByText("Answer sent", { exact: true })).toHaveCount(0)
    })
    await step("paused-epic", "07", "Run failed", async () => {
      await recovery(page).getByRole("button", nav("Retry")).click()
    })
    await step("paused-epic", "05", "Merge conflict", async () => {
      await recovery(page).getByRole("button", nav("Merge again")).click()
    })
    await step("paused-epic", "08", "Paused by you", async () => {
      const more = page.getByTestId("page-header-actions").getByRole("button", nav("More actions"))
      const popover = page.getByRole("alertdialog", nav("Start over"))
      // The popover sits against its anchor, above or below it, and spans the anchor's centre.
      // The popover is positioned after it mounts, so the check polls.
      const anchoredOn = (anchor: Locator) =>
        expect
          .poll(async () => {
            const a = (await anchor.boundingBox())!
            const p = (await popover.boundingBox())!
            const gap = Math.min(Math.abs(p.y - (a.y + a.height)), Math.abs(a.y - (p.y + p.height)))
            const centre = a.x + a.width / 2
            return gap < 60 && centre > p.x && centre < p.x + p.width
          })
          .toBe(true)

      // The ⋯ holds Copy ID and Start over, no Cancel run; Copy ID copies the address.
      await more.click()
      const menu = page.getByRole("menu", nav("More actions"))
      await expect(menu.getByRole("menuitem")).toHaveText(["Copy ID", "Start over"])
      await menu.getByRole("menuitem", nav("Copy ID")).click()
      await expect(menu).toHaveCount(0)
      expect(await page.evaluate(() => navigator.clipboard.readText())).toBe("demo:paused-epic/08")

      // From the ⋯ the popover anchors under the ⋯; the confirm stays off until a reason is typed.
      await more.click()
      await menu.getByRole("menuitem", nav("Start over")).click()
      await expect(popover).toBeVisible()
      await anchoredOn(more)
      const reason = popover.getByRole("textbox", { name: "Reason" })
      await expect(popover.getByRole("button", confirmName)).toBeDisabled()
      await page.keyboard.press("Escape")
      await expect(popover).toHaveCount(0)

      // From the footer it anchors on the footer button.
      const footer = recovery(page).getByRole("button", nav("Start over"))
      await footer.click()
      await expect(popover).toBeVisible()
      await anchoredOn(footer)
      await expect(popover.getByRole("button", confirmName)).toBeDisabled()
      await withState("Start over popover has no Reason textbox", state("paused-epic", "08"), () =>
        reason.fill("wrong base"),
      )
      await popover.getByRole("button", confirmName).click()
    })
    await step("paused-epic", "09", "Run failed", async () => {
      await expect(
        page.getByText("The run hit a transient error; retrying should work."),
      ).toBeVisible()
      const buttons = recovery(page).getByRole("button")
      await expect(buttons.filter({ hasText: "Recommended" })).toHaveCount(1)
      await expect(recovery(page).getByRole("button", nav("Start over"))).not.toContainText(
        "Recommended",
      )
      await buttons.filter({ hasText: "Recommended" }).click()
    })

    // 115: the blocked worker names the answer button, and Approve clears the pause.
    await step("approve-epic", "01", "Needs approval", async () => {
      await expect(stuck(page)).toBeVisible(dwell)
      await expect(page.getByText(/press Approve or Deny on the card/)).toBeVisible()
      await page.getByRole("button", nav("Allow once")).click()
    })
    await expect(stuck(page)).toHaveCount(0)

    // The snapshot carries no herdr health fields: the keys are herdr_unavailable and herdr_text.
    // The word "herdr" itself is not asserted absent: the next scenario's pause text names it.
    const snapshot = await page.evaluate(async () => (await fetch("/v1/snapshot")).text())
    expect(snapshot).not.toContain('"herdr_unavailable"')
    expect(snapshot).not.toContain('"herdr_text"')
    await expect(page.getByText(/herdr unavailable/i)).toHaveCount(0)

    // A ticket a removed herdr runner left with commits pauses as a handle mismatch; Retry resumes it on PTY.
    const leftover = await start("herdr-leftover-work")
    await openTicket(page, "01", "herdr-work-epic")
    await expect(page.getByText("Session mismatch").first()).toBeVisible(gateWait())
    await expect(page.getByText(/runner was removed/).first()).toBeVisible(gateWait())
    await expect(recovery(page).getByRole("button")).toHaveText(
      [/^Retry(\s*Recommended)?$/, /^Start over(\s*Recommended)?$/],
      gateWait(),
    )
    await recovery(page).getByRole("button", nav("Retry")).click()
    await expect(recovery(page)).toHaveCount(0, { timeout: 30_000 })
    // The Retry logs human-relaunch; only an iteration-started after it proves the retry launched a run.
    await expect
      .poll(async () => {
        const history = JSON.parse(
          await leftover.cli("server", "tickets", "history", "demo:herdr-work-epic/01", "--json"),
        ) as { events: { type: string }[] }
        const types = history.events.map((e) => e.type)
        return types.slice(types.lastIndexOf("human-relaunch") + 1).includes("iteration-started")
      }, gateWait())
      .toBe(true)
    await expect
      .poll(async () => {
        const its = JSON.parse(await leftover.cli("server", "iterations", "--json")) as {
          address: string
          runner?: string
        }[]
        return its.filter((i) => i.address === "demo:herdr-work-epic/01").map((i) => i.runner)
      })
      .toEqual(["pty"])
  })

  // Each recovery scenario is its own server and its own test, so the scenarios wait in parallel.
  const recover = async (
    { page, start }: { page: Page; start: (scenario: string) => Promise<Fixture> },
    scenario: string,
    epic: string,
    reason: string,
    buttons: string[],
    act?: (fx: Fixture) => Promise<void>,
  ) => {
    const fx = await test.step("start", () => start(scenario))
    await openTicket(page, "01", epic)
    await test.step("pause shows", () =>
      expect(page.getByText(reason).first()).toBeVisible(gateWait()))
    // 20: while a repair pause's run is still finishing the card has no buttons; they return when it settles.
    await test.step("buttons", () =>
      expect(recovery(page).getByRole("button")).toHaveText(
        buttons.map((b) => new RegExp(`^${b}(\\s*Recommended)?$`)),
        gateWait(),
      ))
    await act?.(fx)
  }
  const retry = (page: Page) => recovery(page).getByRole("button", nav("Retry")).click()
  // The Runs list keeps a past pause's text, so a lifted pause shows as the recovery card going away.
  const cleared = (page: Page) => expect(recovery(page)).toHaveCount(0, { timeout: 30_000 })
  const gate = "Background task still running"

  // 6: the relaunch hangs too; the pause names the cause and Retry ends it.
  test("a hung relaunch recovers", async ({ page, start }) => {
    await recover(
      { page, start },
      "hung-pane-twice",
      "hung-epic",
      "Could not start the agent",
      ["Retry", "Start over"],
      async () => {
        await expect(page.getByText(/pane stopped taking keys, twice/).first()).toBeVisible()
        await retry(page)
        await cleared(page)
      },
    )
  })

  // A verify agent that reports finished with an unchecked story is told twice, then the ticket pauses
  // iteration-error naming the story. Retry ends the pause and starts the count again, so the same
  // agent is blocked twice more and pauses again.
  test("a verify finish with an unchecked story pauses", async ({ page, start }) => {
    test.setTimeout(150_000)
    await recover(
      { page, start },
      "verify-unchecked",
      "unchecked-epic",
      "unchecked units: Story 1",
      ["Retry", "Start over"],
      async () => {
        await retry(page)
        await cleared(page)
        await expect(page.getByText("unchecked units: Story 1").first()).toBeVisible(gateWait())
        await expect(recovery(page)).toBeVisible(gateWait())
      },
    )
  })

  // 70, 64, 28: a live Claude task. Keep waiting lifts the pause (the short hold cap pauses it again),
  // and Retry keeps the commits and ends it.
  test("a live task recovers", async ({ page, start }) => {
    await recover(
      { page, start },
      "live-task-gate",
      "live-task-epic",
      gate,
      ["Keep waiting", "Stop task", "Retry", "Start over"],
      async () => {
        await expect(page.getByText("run the soak").first()).toBeVisible()
        await recovery(page).getByRole("button", nav("Keep waiting")).click()
        await test.step("cleared", () => cleared(page))
        await test.step("paused again", () => expect(recovery(page)).toBeVisible(gateWait()))
        await retry(page)
        await test.step("cleared by retry", () => cleared(page))
      },
    )
  })

  // 64: with a dirty worktree a live task still offers all four, and once the task's process exits
  // the card drops to Retry and Start over.
  test("a live task with a dirty worktree recovers", async ({ page, start }) => {
    await recover(
      { page, start },
      "live-task-gate-dirty",
      "live-task-dirty-epic",
      gate,
      ["Keep waiting", "Stop task", "Retry", "Start over"],
      async (fx) => {
        await fx.releaseTask()
        await expect(recovery(page).getByRole("button")).toHaveText(
          [/^Retry(\s*Recommended)?$/, /^Start over$/],
          gateWait(),
        )
      },
    )
  })

  // 65: a Codex task cannot be stopped; Start over states the discard and ends the pause.
  test("a Codex task recovers", async ({ page, start }) => {
    await recover(
      { page, start },
      "live-task-gate-codex",
      "live-task-codex-epic",
      gate,
      ["Keep waiting", "Retry", "Start over"],
      async () => {
        await recovery(page).getByRole("button", nav("Start over")).click()
        const dialog = page.getByRole("alertdialog", nav("Start over"))
        await expect(dialog.getByText(/Start over discards the branch's commits/)).toBeVisible()
        await dialog.getByRole("textbox", { name: "Reason" }).fill("drop the task")
        await dialog.getByRole("button", confirmName).click()
        await cleared(page)
      },
    )
  })

  // 41: every turn ends on a "no verdict" error until the re-sends run out; Retry prompts the live
  // agent. When that agent finishes, the late-finish adoption ends the ticket with no person step.
  test("a classifier outage recovers", async ({ page, start }) => {
    await recover(
      { page, start },
      "classifier-outage",
      "classifier-outage-epic",
      "Classifier gave no verdict",
      ["Retry"],
      async () => {
        await retry(page)
        await expect(toasts(page).getByText(/refused|stale/i)).toHaveCount(0)
      },
    )
  })

  // 16/68: the holder is alive but never answers; the run fails with Retry and Start over.
  test("an unreachable holder recovers", async ({ page, start }) => {
    await recover({ page, start }, "holder-unreachable", "holder-dead-epic", "Run failed", [
      "Retry",
      "Start over",
    ])
  })

  // The watchdog killed a run whose process tree passed the threshold; the pause says gx cannot
  // recover alone, and Retry ends it.
  test("a process storm recovers", async ({ page, start }) => {
    await recover(
      { page, start },
      "process-storm",
      "storm-epic",
      "Too many processes",
      ["Retry", "Start over"],
      async () => {
        await expect(page.getByText(/cannot recover alone/).first()).toBeVisible()
        // The card names the rule that fired and the biggest process names with counts.
        await expect(page.getByText(/\(rule: (streak|hard ceiling)\)/).first()).toBeVisible()
        await expect(page.getByText(/The tree held: \S+ \d+/).first()).toBeVisible()
        await retry(page)
        await cleared(page)
      },
    )
  })

  // The fake agent rejects the skill however often gx refreshes, so the pause says gx already
  // refreshed. Once the fake knows the skill, Retry (which refreshes again) ends the pause.
  test("Unknown skill", async ({ page, start }) => {
    const fixture = await start("unknown-skill")
    await page.evaluate((hash) => (location.hash = hash), "#/t/demo:unknown-epic/01")

    await expect(page.getByText("Could not start the agent").first()).toBeVisible(gateWait())
    await expect(page.getByText(/does not know the skill \/gx-implement/).first()).toBeVisible()
    await expect(page.getByText(/even after gx refreshed the skill install/).first()).toBeVisible()
    await expect(page.getByText(/gx skills install/).first()).toBeVisible()
    await expect(recovery(page).getByRole("button")).toHaveText([/^Retry$/, /^Start over$/])

    await fixture.learnSkill()
    await recovery(page).getByRole("button", nav("Retry")).click()
    await expect(page.getByText("Could not start the agent")).toHaveCount(0)
  })
})

test.describe("147. Resume on worker", () => {
  test("a model-mismatch pause resumes on the worker tier", async ({ page, start }) => {
    await start("model-mismatch")
    await page.evaluate((hash) => (location.hash = hash), "#/t/demo:mismatch-epic/01")

    await expect(page.getByText("Wrong model").first()).toBeVisible()
    await expect(
      recovery(page).getByText("Ran on Claude Opus 4, expected Claude Sonnet 4."),
    ).toBeVisible()
    await expect(recovery(page).getByRole("button")).toHaveText([/^Resume on worker$/])
    await expect(page.getByRole("button", nav("Answer question"))).toHaveCount(0)
    await expect(recovery(page).getByRole("button", nav("Resume on worker"))).toHaveAttribute(
      "data-emphasis",
      "main",
    )
    await page.evaluate((hash) => (location.hash = hash), "#/answer/demo:mismatch-epic/01")
    await expect(page.getByText("This question is answered on the ticket page.")).toBeVisible()
    await expect(page.getByRole("button", nav("Back to ticket"))).toBeVisible()
    await page.getByRole("button", nav("Back to ticket")).click()
    await expect(recovery(page)).toBeVisible()

    await recovery(page).getByRole("button", nav("Resume on worker")).click()
    await expect(page.getByText("Wrong model")).toHaveCount(0)
  })
})

test.describe("213. Question card, long question and One-off Cancel", () => {
  test("a long title clamps in a static card and a One-off's Cancel ends the run", async ({
    page,
    start,
  }) => {
    test.setTimeout(120_000)
    await start("long-question")
    await openTicket(page, "01", "long-question-epic")
    await expect(page.getByText("Paused · Waiting for your answer", { exact: true })).toBeVisible()

    const answer = page.getByRole("button", nav("Answer question"))
    const resume = recovery(page).getByRole("button", nav("Resume without answering"))
    const title = page.locator('[data-mock-part="question-title"]')
    const checkCard = async () => {
      await expect(page.getByText("THE AGENT ASKS", { exact: true })).toBeVisible()
      await expect(title).toHaveCSS("-webkit-line-clamp", "3")
      await expect(answer).toHaveAttribute("data-emphasis", "main")
      await expect(resume).toHaveAttribute("data-emphasis", "secondary")
      await expect(recovery(page)).toHaveCSS("position", "static")
    }
    await test.step("1100px", async () => {
      await page.setViewportSize({ width: 1100, height: 800 })
      await checkCard()
    })
    await test.step("390px", async () => {
      await page.setViewportSize({ width: 390, height: 800 })
      await checkCard()
      await expect
        .poll(async () => {
          const boxes = [await answer.boundingBox(), await resume.boundingBox()]
          return boxes.every(
            (b) =>
              b !== null && b.y >= 0 && b.y + b.height <= 800 && b.x >= 0 && b.x + b.width <= 390,
          )
        })
        .toBe(true)
    })
    await test.step("earlier rounds sit below the card", async () => {
      await page.getByRole("button", { name: /Earlier rounds \(2\)$/ }).click()
      await expect
        .poll(async () => {
          const card = await page.locator('[data-mock-part="question-card"]').boundingBox()
          const rounds = await page.getByText(/^#1 · /).boundingBox()
          return card !== null && rounds !== null && rounds.y > card.y + card.height
        })
        .toBe(true)
    })

    await test.step("One-off Cancel ends the run", async () => {
      const oneOff = await start("one-off-question")
      await page.setViewportSize({ width: 1100, height: 800 })
      await page.evaluate((hash) => (location.hash = hash), "#/t/demo:one-offs/01")
      await expect(answer).toHaveAttribute("data-emphasis", "main")
      const cancel = recovery(page).getByRole("button", nav("Cancel run"))
      await expect(cancel).toHaveAttribute("data-emphasis", "secondary")
      await cancel.click()
      await page
        .getByRole("alertdialog")
        .getByRole("button", { name: /^Cancel run/ })
        .click()
      await expect
        .poll(() => oneOff.cli("tickets", "show", "demo:one-offs/01"))
        .toMatch(/^status: cancelled$/m)
    })
  })
})

const LONG = "demo:long-question-epic"
const answerPage = (page: Page) => page.getByRole("form", { name: "Answer" })
const answerFooter = (page: Page) => page.locator('[data-mock-part="answer-footer"]')
const sendAnswer = (page: Page) => page.getByRole("button", nav("Send answer"))
const noteField = (page: Page) => page.getByPlaceholder("Note on this question")
const openNote = async (page: Page) => {
  await answerPage(page).getByRole("button", { name: /^Note/ }).click()
  return noteField(page)
}
const optionA = (page: Page) => page.getByRole("button", { name: /Keep the current layout/ })
const optionB = (page: Page) => page.getByRole("button", { name: /Clamp the title and fold/ })
// The first bullet of ticket 01's question body.
const QUESTION_BODY = "clamp the title to three lines and show the rest on a tap"
const QUESTION_TITLE = "Should the queue card clamp a long question title"

test.describe("214. Answer page, history, note and Send", () => {
  test("a long question is answered on its own page", async ({ page, start }) => {
    test.setTimeout(120_000)
    const fixture = await start("long-question", { reloads: 3 })
    await page.setViewportSize({ width: 1100, height: 700 })
    await openTicket(page, "01", "long-question-epic")
    await expect(page.getByText("Paused · Waiting for your answer", { exact: true })).toBeVisible()
    const answer = page.getByRole("button", nav("Answer question"))
    const back = page.getByRole("link", nav("← Ticket"))

    await test.step("1. Answer question pushes the page", async () => {
      await answer.click()
      await expect(page).toHaveURL(/#\/answer\/demo:/)
      await expect(back).toBeVisible()
      await expect(page.getByRole("heading", { name: "Answer question" })).toBeVisible()
    })

    await test.step("2. reading and order", async () => {
      await expect(page.getByText(QUESTION_BODY)).toHaveCount(1)
      await expect(page.getByText(QUESTION_TITLE)).toHaveCount(1)
      await expect(page.getByText("THE AGENT ASKS", { exact: true })).toBeVisible()
      await expect(page.getByText("Agent's notes")).toHaveCount(0)
      await expect(page.getByText(/^(agent|web)$/i)).toHaveCount(0)
      await expect(page.getByRole("button", { name: /Earlier rounds/ })).toBeVisible()
      await back.click()
      await expect(page).toHaveURL(/#\/t\/demo:long-question-epic\/01$/)
      await expect(page.getByText(QUESTION_TITLE)).toHaveCount(1)
      await expect(page.getByText("Agent's notes").first()).toBeVisible()
    })

    await test.step("3. footer outside the scroll, narrow column", async () => {
      await answer.click()
      await expect(sendAnswer(page)).toBeDisabled()
      const scroller = answerPage(page).locator("> div").first()
      const column = scroller.locator("> div").first()
      await expect(scroller).toBeVisible()
      await expect(column).toBeVisible()
      await expect
        .poll(async () => {
          const c = (await column.boundingBox())!
          return c.width < page.viewportSize()!.width
        })
        .toBe(true)
      const before = (await sendAnswer(page).boundingBox())!
      await scroller.evaluate((el) => el.scrollTo(0, el.scrollHeight))
      await expect.poll(() => scroller.evaluate((el) => el.scrollTop)).toBeGreaterThan(0)
      await expect
        .poll(async () => {
          const after = (await sendAnswer(page).boundingBox())!
          return after.y === before.y && after.x === before.x
        })
        .toBe(true)
      await expect(answerFooter(page)).toContainText("Send answer")
    })

    await test.step("4. history keeps one ticket entry", async () => {
      await back.click()
      await expect(page).toHaveURL(/#\/t\/demo:long-question-epic\/01$/)
      await answer.click()
      await expect(page).toHaveURL(/#\/answer\//)
      await page.goBack()
      await expect(page).toHaveURL(/#\/t\/demo:long-question-epic\/01$/)
      await page.goBack()
      await expect(page).not.toHaveURL(/#\/(t|answer)\/demo:long-question-epic\/01$/)
      await page.goForward()
      await expect(page).toHaveURL(/#\/t\/demo:long-question-epic\/01$/)
    })

    await test.step("5. the note survives a round trip, a reload clears it", async () => {
      await answer.click()
      await optionB(page).click()
      await expect(optionB(page)).toHaveAttribute("aria-pressed", "true")
      await (await openNote(page)).fill("Keep it short")
      await back.click()
      await expect(page).toHaveURL(/#\/t\/demo:long-question-epic\/01$/)
      await answer.click()
      await expect(optionB(page)).toHaveAttribute("aria-pressed", "true")
      await expect(noteField(page)).toHaveValue("Keep it short")
      await page.reload()
      await expect(optionB(page)).toHaveAttribute("aria-pressed", "false")
      await expect(answerPage(page).getByPlaceholder("Note on this question")).toHaveCount(0)
      // A reload opened the page cold, so leaving replaces it with the ticket page.
      await back.click()
      await expect(page).toHaveURL(/#\/t\/demo:long-question-epic\/01$/)
    })

    // Step 9 of the plan runs before the send: the send answers the only question this ticket has.
    await test.step("9. the phone keyboard keeps Send in view", async () => {
      await page.setViewportSize({ width: 390, height: 844 })
      await answer.click()
      await expect(back).toBeVisible()
      const note = await openNote(page)
      await note.focus()
      await page.evaluate(() => {
        const vv = window.visualViewport!
        Object.defineProperty(vv, "height", {
          configurable: true,
          get: () => window.innerHeight - 300,
        })
        vv.dispatchEvent(new Event("resize"))
      })
      await expect(back).toBeHidden()
      await expect
        .poll(async () => {
          const box = await sendAnswer(page).boundingBox()
          const visible = await page.evaluate(() => window.visualViewport!.height)
          return box !== null && box.y + box.height <= visible + 4
        })
        .toBe(true)
      await page.evaluate(() => {
        const vv = window.visualViewport!
        Object.defineProperty(vv, "height", { configurable: true, get: () => window.innerHeight })
        vv.dispatchEvent(new Event("resize"))
      })
      await note.blur()
      await expect(back).toBeVisible()
      await page.setViewportSize({ width: 1100, height: 700 })
    })

    await test.step("6. Send lands on the ticket page with the Answer sent card", async () => {
      await optionA(page).click()
      await noteField(page).fill("Do not touch the layout")
      await sendAnswer(page).click()
      await expect(page).toHaveURL(/#\/t\/demo:long-question-epic\/01$/)
      const sent = page.getByRole("group", { name: "Answered question" })
      await expect(sent).toBeVisible()
      await expect(sent).toContainText("Answer sent")
      await expect(sent).toContainText(QUESTION_TITLE)
      await expect(sent).toContainText("A: Keep the current layout")
      await expect(sent).toContainText("Note: Do not touch the layout")
      await expect(page.getByText(/^(agent|web)$/i)).toHaveCount(0)
      await expect
        .poll(async () => {
          const notes = await page.getByText("Agent's notes").first().boundingBox()
          const card = await sent.boundingBox()
          return notes !== null && card !== null && notes.y < card.y
        })
        .toBe(true)
      await sent.getByRole("button", { name: /Show question/ }).click()
      await expect(sent.getByText(QUESTION_BODY)).toBeVisible()
      await page.goBack()
      await expect(page).not.toHaveURL(/#\/answer\//)
      await page.goForward()
      await expect(page).toHaveURL(/#\/t\/demo:long-question-epic\/01$/)
      const shown = await fixture.cli("server", "tickets", "detail", `${LONG}/01`)
      // `detail` lists each round's state and title, not the answer text.
      expect(shown).toContain(`round 5 (agent, answered): ${QUESTION_TITLE}`)
    })

    await test.step("7. a queued answer keeps its card after a reload", async () => {
      // No live agent: the send is queued.
      await expect(page.getByRole("status", { name: "Answer result" })).toContainText("Queued")
      await page.reload()
      await expect(page.getByRole("group", { name: "Answered question" })).toBeVisible()
    })

    await test.step("8. a deep link send replaces the entry", async () => {
      await page.goto(`${page.url().split("#")[0]}#/answer/${LONG}/02`)
      await expect(page).toHaveURL(/#\/answer\/demo:long-question-epic\/02$/)
      await optionA(page).click()
      await sendAnswer(page).click()
      await expect(page).toHaveURL(/#\/t\/demo:long-question-epic\/02$/)
      await expect(page.getByRole("group", { name: "Answered question" })).toBeVisible()
      await page.goBack()
      await expect(page).not.toHaveURL(/#\/answer\//)
    })
  })
})

test.describe("215. Answer page, stale and permission refusal", () => {
  test("a stale or refused send keeps the answer", async ({ page, start }) => {
    test.setTimeout(150_000)
    const long = await start("long-question")
    await page.setViewportSize({ width: 1100, height: 700 })
    await openTicket(page, "01", "long-question-epic")
    const answer = page.getByRole("button", nav("Answer question"))
    await answer.click()
    await expect(page).toHaveURL(/#\/answer\/demo:long-question-epic\/01$/)

    await test.step("a CLI answer makes the send stale", async () => {
      await optionA(page).click()
      await (await openNote(page)).fill("Typed before the CLI answered")
      await long.cli("server", "tickets", "answer", `${LONG}/01`, "--choice", "b")
      // The event stream shows the stale banner and drops Send before the person can click it.
      await expect(page.getByText("This question was answered elsewhere")).toBeVisible()
      await expect(noteField(page)).toHaveValue("Typed before the CLI answered")
      await expect(answerFooter(page).getByRole("button")).toHaveText(["Back to ticket"])
      await expect(sendAnswer(page)).toHaveCount(0)
    })

    // The fixture has no way to replace a question under an open page (story 23), so that step is not here.

    await test.step("answered and missing addresses", async () => {
      await page.evaluate((hash) => (location.hash = hash), `#/answer/${LONG}/01`)
      await expect(page.getByText("This question was answered elsewhere")).toBeVisible()
      await expect(page.getByRole("button", nav("Back to ticket"))).toBeVisible()
      await page.evaluate((hash) => (location.hash = hash), "#/answer/demo:missing-epic/01")
      await expect(page.getByText("Ticket not found")).toBeVisible()
    })

    await test.step("a blocked permission prompt refuses the send", async () => {
      const blocked = await start("permission")
      // The page only reads the prompt with the detail, so open it once the agent is blocked.
      await expect
        .poll(
          () => blocked.cli("server", "tickets", "detail", "--json", "demo:permission-epic/01"),
          gateWait(),
        )
        .toMatch(/"permission":/)
      await openTicket(page, "01", "permission-epic")
      const permission = page.getByRole("group", { name: "Permission request" })
      await expect(permission).toBeVisible(gateWait())
      await answer.click()
      await expect(page).toHaveURL(/#\/answer\/demo:permission-epic\/01$/)
      const firstOption = answerPage(page).locator("button[data-answer]").first()
      await firstOption.click()
      await (await openNote(page)).fill("Answer while blocked")
      await sendAnswer(page).click()
      await expect(
        page.getByText("Approve the agent's prompt first, then send again."),
      ).toBeVisible()
      await expect(noteField(page)).toHaveValue("Answer while blocked")
      await expect(page).toHaveURL(/#\/answer\//)
      await page.getByRole("link", nav("← Ticket")).click()
      await expect(page).toHaveURL(/#\/t\/demo:permission-epic\/01$/)
      await expect(permission).toBeVisible()
      await expect(answer).toBeVisible()
      await permission.getByRole("button", nav("Allow once")).click()
      await expect(permission).toHaveCount(0)
      // The send after the approval is not asserted: the fake agent stays busy and refuses it (Spec deviations).
      await expect(answer).toBeVisible()
    })
  })
})

const fixDialog = (page: Page) => page.getByRole("dialog", { name: "Fix it with an agent" })
const fixPrompt = "/gx-investigate demo:paused-epic/07 is paused for repair: Run failed"

const openFixDialog = async (page: Page) => {
  const row = page
    .locator('a[href$="/t/demo:paused-epic/07"]')
    .locator("xpath=ancestor::*[.//button][1]")
  // The running fix ticket's timer re-renders the page every second, so the button is never "stable".
  // A fresh page can show the button before React attaches its handler, which drops the first click.
  await expect(async () => {
    await row.getByRole("button", { name: "Fix it with an agent" }).dispatchEvent("click")
    await expect(fixDialog(page)).toBeVisible({ timeout: 1_000 })
  }).toPass({ timeout: 10_000 })
}

// The Fix-it agent is a session of its own; the queued epic's Queue agents are not.
const sessions = async (fixture: Fixture) =>
  (
    JSON.parse(await fixture.cli("agents", "list", "--json")) as Array<{
      dir: string
      harness: string
      queue?: boolean
    }>
  ).filter((s) => !s.queue)

// The fixture keeps the store at <root>/data/gx/tickets and the project's trunk checkout at <root>/repo.
const trunkDir = async (fixture: Fixture) => {
  const path = (await fixture.cli("tickets", "show", "demo:paused-epic/07")).match(
    /^path: (.+)$/m,
  )![1]
  return realpath(`${path.split("/data/gx/tickets")[0]}/repo`)
}

test.describe("55. Fix it with an agent", () => {
  test("copy, start, and refuse a second agent", async ({ page, context, start }) => {
    await context.grantPermissions(["clipboard-read", "clipboard-write"])
    const fixture = await start("paused")
    await fixture.cli("server", "queue", "add", "demo:paused-epic")
    const before = (await sessions(fixture)).length

    await page.getByRole("link", { name: "paused-epic", exact: true }).click()
    await openFixDialog(page)
    await expect(fixDialog(page).getByText(fixPrompt)).toBeVisible()
    await fixDialog(page).getByRole("button", nav("Copy")).click()
    await expect(fixDialog(page).getByRole("button", nav("Copied"))).toBeVisible()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(fixPrompt)

    const epicUrl = page.url()
    await fixDialog(page).getByRole("button", nav("Start gx agent")).click()
    await expect(page).toHaveURL(/#\/agents\//)
    await expect(fixDialog(page)).toHaveCount(0)
    await expect.poll(async () => (await sessions(fixture)).length).toBe(before + 1)
    const started = (await sessions(fixture)).at(-1)!
    expect(started.harness).toBe("claude")
    expect(await realpath(started.dir)).toBe(await trunkDir(fixture))

    // A reload would fail the test, so the second visit is a fresh page on the same server.
    const second = await context.newPage()
    await second.goto(epicUrl)
    await openFixDialog(second)
    await fixDialog(second).getByRole("button", nav("Start gx agent")).click()
    // The refusal stays in the dialog.
    await expect(fixDialog(second).getByRole("alert")).toContainText(
      "an agent is already fixing demo:paused-epic/07",
    )
    expect((await sessions(fixture)).length).toBe(before + 1)
  })
})

test.describe("116. Retry on a question menu", () => {
  test("Retry lifts the pause", async ({ page, start }) => {
    await start("blocked-menu")

    await page.getByRole("link", { name: "menu-epic", exact: true }).click()
    await expect(page.getByText("needs answer").first()).toBeVisible(dwell)
    await page.locator('a[href$="/t/demo:menu-epic/01"]').filter({ hasNotText: "→" }).click()

    await expect(stuck(page)).toBeVisible()
    await expect(page.getByRole("button", nav("Allow once"))).toHaveCount(0)
    await recovery(page).getByRole("button", nav("Retry")).click()
    await expect(stuck(page)).toHaveCount(0)
    // The agent left the menu and the ticket went on.
    await expect(page.getByText(/^(Paused|needs-answer)/)).toHaveCount(0)
  })
})

test.describe("56. Fix it with an agent, remote control", () => {
  test("opens the new agent's page", async ({ page, start }) => {
    const fixture = await start("paused", { config: { "agents.fix-agent-remote-control": "true" } })
    await fixture.cli("server", "queue", "add", "demo:paused-epic")

    await page.getByRole("link", { name: "paused-epic", exact: true }).click()
    await openFixDialog(page)
    await fixDialog(page).getByRole("button", nav("Start gx agent")).click()
    await expect(page).toHaveURL(/#\/agents\//)
    await expect(fixDialog(page)).toHaveCount(0)
  })
})
