import { createECDH, randomBytes } from "node:crypto"
import { readFile } from "node:fs/promises"
import {
  blockEvents,
  card,
  expect,
  holdRoute,
  nav,
  openEpics,
  section,
  test,
  toasts,
} from "./fixture"
import { expectOption } from "./question"
import { CACHE_PREFIX } from "../src/pwa/workerCore"

// The PWA steps of flow 2 need a registered worker, which only registers with the fixture's untrusted root ignored.
test.use({ serviceWorkers: "allow", launchOptions: { args: ["--ignore-certificate-errors"] } })

test("2. Epic looks right", async ({ page, start, context }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"])
  // 06 asks while 02 merges. With a live paused sibling the run only re-reads the epic on its 30s pause poll,
  // so 03 can start up to 30s after 02 merges, and 03's own merge waits out the debounce after that.
  test.setTimeout(240_000)
  // Recovery would fork an investigation off the failed ticket and turn it into a waiting parent.
  await start("mixed", { config: { "recovery.enabled": "false" } })
  const mixed = card(page, "mixed-epic")
  await expect(mixed.getByText("2/7")).toBeVisible()
  await expect(mixed.getByText("demo", { exact: true })).toBeVisible()
  await mixed.getByRole("link", { name: "mixed-epic" }).click()

  await expect(page.getByText("2/7")).toBeVisible()
  await expect(page.getByRole("img", { name: "Implementation" })).toBeVisible()
  await expect(page.getByTestId("page-header-actions").getByTestId("comet")).toHaveCount(0)
  await expect(page.getByTestId("stats-line")).toHaveText("Not queued · $0.00 · 0s agent time")
  // mixed-epic has no summary, so the header draws no description line.
  await expect(page.getByTestId("page-header-line")).toHaveCount(0)
  // The ⋯ menu holds Move to workspace, Hold, Archive and Copy ID, and the copy is <project>:<name>.
  await page.getByTestId("page-header-actions").getByRole("button", nav("More actions")).click()
  const menu = page.getByRole("menu", nav("More actions"))
  await expect(menu.getByRole("menuitem")).toHaveText([
    /^Move to workspace/,
    "Hold",
    "Archive",
    "Copy ID",
  ])
  await menu.getByRole("menuitem", nav("Copy ID")).click()
  await expect(menu).toHaveCount(0)
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe("demo:mixed-epic")
  await expect(page.getByText("1 done, 1 cancelled")).toBeVisible()
  await expect(page.getByRole("link", { name: "02 open ready" })).toBeVisible()
  await expect(page.getByRole("link", { name: "03 blocked after 02" })).toBeVisible()
  await expect(page.getByRole("link", { name: "04 drafted draft" })).toBeVisible()
  await expect(page.getByRole("link", { name: "06 asks ready" })).toBeVisible()
  await expect(page.getByRole("link", { name: "07 fails ready" })).toBeVisible()
  await expect(page.getByRole("link", { name: "05 dropped cancelled" })).toBeVisible()
  // The epic has no stats until work merges, so there is no More stats card yet.
  await expect(page.getByRole("button", { name: /^More stats/ })).toHaveCount(0)

  // The worker settles with 02 and 03 merged, 06 asking and 07 failing.
  await page.getByRole("button", nav("Queue")).click()
  await expect(page.getByText("2/7")).toBeVisible()
  await expect(page.getByTestId("stats-line")).toHaveText(/^Running · /, { timeout: 30_000 })
  await expect(page.getByTestId("page-header-actions").getByTestId("comet")).toBeVisible()
  // A running card's chip reads the model, tier and effort, with the harness dot.
  const chip = page.locator("[data-chips]").first()
  await expect(chip).toHaveText("Claude Sonnet · Worker · medium", { timeout: 45_000 })
  await expect(chip.locator("i")).toHaveCount(1)
  // 02 then 03 merge one after the other, and every merge waits out the server's 3s finish debounce.
  await expect(page.getByText("4/7")).toBeVisible({ timeout: 120_000 })
  // The innermost block holding both the 06 link and the Answer button is its card.
  const asking = page
    .locator("div")
    .filter({ has: page.locator('a[href$="/t/demo:mixed-epic/06"]') })
    .filter({ has: page.getByRole("button", nav("Answer")) })
    .last()
  await expect(asking.getByText("needs answer")).toBeVisible({ timeout: 45_000 })
  await expect(asking.getByText("Which option should the fake worker take?")).toBeVisible()
  const answer = asking.getByRole("button", nav("Answer"))
  await expect(answer).toBeVisible()
  const title = (await asking.locator('a[href$="/t/demo:mixed-epic/06"]').boundingBox())!
  const answerBox = (await answer.boundingBox())!
  expect(answerBox.x).toBeGreaterThanOrEqual(title.x + title.width)
  const failed = page
    .locator('[data-tone="deadlocked"]')
    .filter({ has: page.getByRole("link", { name: "fails" }) })
  await expect(failed.getByText("needs-repair · Run failed")).toBeVisible({ timeout: 45_000 })

  // 50, 51, 52: the ticket page's question lists each option with letter, id and consequence, and offers Huh? and a Note.
  await test.step("agent question: options, Huh? and Note at both widths", async () => {
    await page.evaluate((hash) => (location.hash = hash), "#/t/demo:mixed-epic/06")
    const form = page.getByRole("form", { name: "Answer" })
    const send = form.getByRole("button", { name: "Send answer" })
    for (const width of [1440, 375]) {
      await page.setViewportSize({ width, height: 900 })
      await expectOption(form, {
        letter: "A",
        label: "Keep the cache",
        id: "a",
        consequence: "Reuses the warm cache and finishes sooner.",
      })
      await expectOption(form, {
        letter: "B",
        label: "Rebuild it",
        id: "b",
        consequence: "Starts clean and takes longer.",
      })
      await expect(form.getByRole("button", { name: "Huh?" })).toBeVisible()
      await expect(form.getByRole("button", { name: /^Note on question/ })).toBeVisible()
      await expect(send).toBeDisabled()
    }

    await page.setViewportSize({ width: 1440, height: 900 })
  })

  await openEpics(page)
  await expect(mixed.getByText("4/7")).toBeVisible()
  await expect(mixed.getByTestId("pill")).toBeVisible()
  await expect(mixed.getByTestId("reason")).toBeVisible()

  // 55, 56: a note alone answers a choice question and the history reads "no option picked". The fake agent
  // asks once per ticket, so this answers 06 after the Epics-page checks above, which count it as asking.
  await test.step("agent question: note-only answer", async () => {
    await page.evaluate((hash) => (location.hash = hash), "#/t/demo:mixed-epic/06")
    const form = page.getByRole("form", { name: "Answer" })
    const send = form.getByRole("button", { name: "Send answer" })
    await form.getByRole("button", { name: /^Note on question/ }).click()
    await expect(send).toBeDisabled()
    await form
      .getByRole("textbox", { name: /^Note text for question/ })
      .fill("Ask me again tomorrow")
    await expect(send).toBeEnabled()
    await send.click()
    const result = page.getByRole("group", { name: "Answered question" })
    await expect(result.getByText("no option picked")).toBeVisible()
    await expect(result.getByText("Ask me again tomorrow")).toBeVisible()
    await expect(
      result.getByRole("status", { name: "Answer result" }).getByText("Delivered"),
    ).toBeVisible()
  })

  // 40: answering an agent that is gone queues the ticket; the card's status line says so.
  await test.step("agent question: answer to a gone agent reads Queued", async () => {
    const queued = await start("answer-queued")
    await queued.waitAgentGone()
    await page.evaluate((hash) => (location.hash = hash), "#/t/demo:queued-answer-epic/01")
    const form = page.getByRole("form", { name: "Answer" })
    await form.getByRole("textbox").fill("go on")
    await form.getByRole("button", { name: "Send answer" }).click({ timeout: 60_000 })
    const result = page.getByRole("group", { name: "Answered question" })
    await expect(result.getByRole("status", { name: "Answer result" })).toContainText("Queued")
    await expect(result.getByRole("status", { name: "Answer result" })).toContainText("in queue")
  })

  // 43: the form of an ask is still open when the ticket moves on and its agent finishes late; the answer finds it adopted.
  await test.step("agent question: answer from a stale form reads The agent had already finished", async () => {
    // Recovery would fork an investigation off the zero-commit pause.
    const finished = await start("answer-finished", { config: { "recovery.enabled": "false" } })
    const addr = "demo:finished-answer-epic/01"
    await page.evaluate((hash) => (location.hash = hash), `#/t/${addr}`)
    const form = page.getByRole("form", { name: "Answer" })
    await form.getByRole("textbox").fill("go on", { timeout: 60_000 })
    // Cut the page off from the server so the form stays on screen while the ticket moves on.
    const unblock = await blockEvents(page)
    await context.setOffline(true)
    await finished.waitAgentGone()
    await finished.cli("server", "tickets", "resume", addr)
    await expect
      .poll(
        async () =>
          (await finished.cli("server", "tickets", "history", addr)).split("\tpaused\t").length - 1,
        { timeout: 90_000 },
      )
      .toBe(2)
    await context.setOffline(false)
    // A refused answer refreshes the page and drops the form, so there is one Send: its request waits
    // until the late finish is on the ticket, and the server adopts that a moment later.
    const ticketPath = /^path: (.+)$/m.exec(await finished.cli("tickets", "show", addr))?.[1] ?? ""
    const finishedAt = async () =>
      /^iteration_finished_at: (.+)$/m.exec(await readFile(ticketPath, "utf8"))?.[1]
    const emptyFinishAt = await finishedAt()
    expect(emptyFinishAt).toBeTruthy()
    let lateFinish!: () => void
    const landed = new Promise<void>((resolve) => (lateFinish = resolve))
    await page.route("**/v1/tickets/answer", async (route) => {
      await landed
      await route.continue()
    })
    await form.getByRole("button", { name: "Send answer" }).click()
    await finished.release()
    await expect.poll(finishedAt, { timeout: 60_000, intervals: [50] }).not.toBe(emptyFinishAt)
    lateFinish()
    const result = page.getByRole("group", { name: "Answered question" })
    await expect(result.getByRole("status", { name: "Answer result" })).toContainText("Sent")
    await expect(result.getByRole("status", { name: "Answer result" })).toContainText(
      "The agent had already finished",
    )
    await expect(result.getByText("go on")).toBeVisible()
    await unblock()
    await page.unroute("**/v1/tickets/answer")
  })

  // 55: Huh? sends the fixed message. The wire side is TestAnswer_HuhDeliversTheMessageAndMarksTheLog,
  // the click is AnswerForm's "Huh? posts the huh flag", and the card is AnswerResult's Huh? test.

  // 49: the ticket detail shows the Latest summary block; the Epics-page card carries none of it.
  await test.step("ticket detail shows Latest summary, the Epics card does not", async () => {
    await start("summary-notes")
    await page.evaluate((hash) => (location.hash = hash), "#/t/demo:summary-epic/01")
    const summary = page.getByRole("region", { name: "Latest summary" })
    await expect(summary).toBeVisible({ timeout: 90_000 })
    // The agent's four parts, then gx's own entry.
    await expect(summary.getByRole("term")).toHaveText([
      "Did:",
      "Went wrong:",
      "Decisions:",
      "Left:",
      "Did:",
      "Went wrong:",
      "Decisions:",
      "Left:",
    ])
    await expect(summary.getByRole("definition").first()).toHaveText("Verified the fake story.")
    await expect(summary.getByRole("definition").nth(4)).toHaveText(
      "launched the agent and settled the run",
    )
    await openEpics(page)
    const finished = await section(page, "Finished")
    await expect(finished.getByRole("link", { name: "summary-epic" })).toBeVisible()
    await expect(card(page, "summary-epic").getByText("Latest summary")).toHaveCount(0)
  })

  // 63: the evolve ticket of a closed design map is a top-level one-off, like a queue epic's evolve.
  await test.step("evolve ticket of a closed map shows as a one-off", async () => {
    await start("map-evolve")
    await expect(page.getByRole("heading", { name: "Epics", level: 1 })).toBeVisible()
    // The evolve one-off is its own top-level card that links to the ticket, beside the closed map's.
    const evolve = page.locator('a[href$="/t/demo:one-offs/01"]')
    await expect(evolve.first()).toBeVisible({ timeout: 60_000 })
    await expect(evolve.first()).toHaveText("evolve-closed-map")
  })

  // 14: a holder that drops its status dials while the run waits shows a retrying card, then clears.
  await test.step("ticket card shows holder unreachable, retrying, then clears", async () => {
    await start("slow-holder", { config: { "recovery.enabled": "false" } })
    await page.evaluate((hash) => (location.hash = hash), "#/e/demo:slow-holder-epic")
    const retrying = page.getByText("holder unreachable, retrying")
    await expect(retrying).toBeVisible({ timeout: 60_000 })
    await expect(retrying).toHaveCount(0, { timeout: 60_000 })
  })
})

// Its own flow: with the other steps it ran close to the 45 s limit.
// On a fresh `mixed` fixture: Force refresh reloads the page once, and the cold open stops the server.
test("2. Epic PWA: Force refresh and cold open", async ({ page, start, context }) => {
  await test.step("PWA: manifest, worker, Force refresh, offline cold open", async () => {
    const fixture = await start("mixed", { config: { "recovery.enabled": "false" }, reloads: 1 })
    await expect(card(page, "mixed-epic")).toBeVisible()

    const href = await page.locator('link[rel="manifest"]').getAttribute("href")
    expect(href).toBeTruthy()
    const manifestURL = new URL(href!, page.url())
    const manifestResponse = await page.request.get(manifestURL.href)
    expect(manifestResponse.status()).toBe(200)
    const manifest = await manifestResponse.json()
    expect(manifest).toMatchObject({
      name: "gx",
      short_name: "gx",
      id: "/",
      start_url: "/",
      scope: "/",
      display: "standalone",
      theme_color: "#0a0d11",
      background_color: "#0a0d11",
    })
    for (const icon of manifest.icons as Array<{ src: string }>) {
      const response = await page.request.get(new URL(icon.src, manifestURL).href)
      expect(response.status(), icon.src).toBe(200)
      expect(response.headers()["content-type"], icon.src).toBe("image/png")
    }

    const workerState = () =>
      page.evaluate(
        async () => (await navigator.serviceWorker.getRegistration())?.active?.state ?? "none",
      )
    await expect.poll(workerState).toBe("activated")

    // With the worker active, a change made outside the page still reaches it through the event stream.
    await fixture.cli(
      "tickets",
      "add",
      "demo:mixed-epic",
      "--slug",
      "pwa-outside",
      "--body",
      "# pwa outside\n\n## What to build\n\nx",
    )
    await page.evaluate((hash) => (location.hash = hash), "#/e/demo:mixed-epic")
    await expect(page.getByRole("link", { name: /pwa outside/ })).toBeVisible({ timeout: 60_000 })

    // Force refresh: no confirmation; the registration and a planted shell-cache entry go, and the page reloads once.
    const planted = `${CACHE_PREFIX}planted`
    await page.evaluate(
      async (name) => (await caches.open(name)).put("/planted", new Response("x")),
      planted,
    )
    expect(await page.evaluate(() => caches.keys())).toContain(planted)
    await page.getByRole("button", nav("Menu")).click()
    await page.getByRole("menuitem", { name: "Force refresh" }).click()
    // The toast goes away with the reload, so it is not asserted here (forceRefresh.test.ts does).
    // A poll that lands mid-navigation throws "Execution context was destroyed"; retry it.
    await expect
      .poll(() =>
        page.evaluate((key) => sessionStorage.getItem(key), "__gxE2eLoads").catch(() => undefined),
      )
      .toBe("1")
    await expect.poll(() => page.evaluate(() => caches.keys())).not.toContain(planted)
    // The fresh page registers a new worker as at start-up.
    await expect.poll(workerState).toBe("activated")

    // Notifications, on a second page so the main page keeps its one reload. It fakes the platform's
    // permission, push manager and app badge; the server, the stored subscription and the fake push
    // service are real.
    const ecdh = createECDH("prime256v1")
    ecdh.generateKeys()
    const subscription = {
      endpoint: `${fixture.pushServiceURL}/push/1`,
      p256dh: ecdh.getPublicKey().toString("base64url"),
      auth: randomBytes(16).toString("base64url"),
    }
    const notif = await context.newPage()
    await notif.setViewportSize({ width: 390, height: 844 })
    await notif.addInitScript((sub) => {
      const state = {
        permissionRequests: 0,
        applied: null as number | null,
        cleared: 0,
      }
      Object.assign(window, { __push: state })
      let permission: NotificationPermission = "default"
      Object.defineProperty(Notification, "permission", {
        get: () => permission,
        configurable: true,
      })
      Notification.requestPermission = async () => {
        state.permissionRequests++
        permission = "granted"
        return "granted"
      }
      let current: unknown = null
      const manager = {
        getSubscription: async () => current,
        subscribe: async (options: { applicationServerKey: BufferSource }) => {
          const key = new Uint8Array(options.applicationServerKey as ArrayBuffer).slice().buffer
          current = {
            endpoint: sub.endpoint,
            options: { applicationServerKey: key },
            toJSON: () => ({
              endpoint: sub.endpoint,
              keys: { p256dh: sub.p256dh, auth: sub.auth },
            }),
            unsubscribe: async () => {
              current = null
              return true
            },
          }
          return current
        },
      }
      Object.defineProperty(ServiceWorkerRegistration.prototype, "pushManager", {
        get: () => manager,
        configurable: true,
      })
      Object.defineProperty(navigator, "setAppBadge", {
        value: async (n: number) => {
          state.applied = n
        },
        configurable: true,
      })
      Object.defineProperty(navigator, "clearAppBadge", {
        value: async () => {
          state.applied = 0
          state.cleared++
        },
        configurable: true,
      })
    }, subscription)
    const pushState = () =>
      notif.evaluate(
        () =>
          (
            window as unknown as {
              __push: { permissionRequests: number; applied: number | null; cleared: number }
            }
          ).__push,
      )
    const go = (hash: string) => notif.evaluate((h) => (location.hash = h), hash)
    await notif.goto(new URL("#/settings", page.url()).href)

    // Story 4: the Account rows no longer hold Notifications; the push card sits in the Notifications section.
    const account = notif.locator('[data-mock-part="account-group"]')
    await expect(account.getByRole("link", { name: /^Notifications/ })).toHaveCount(0)

    // Story 2: opening the page asks for nothing, and the test row waits for the switch.
    await go("#/settings/s/push")
    const toggle = notif.getByRole("switch", { name: "Notifications on this device" })
    await expect(toggle).toHaveAttribute("aria-checked", "false")
    const testRow = notif.getByRole("button", { name: /Send test push/ })
    await expect(testRow).toBeDisabled()
    await expect(testRow).toContainText("Turn notifications on first.")
    expect((await pushState()).permissionRequests).toBe(0)

    // Stories 1, 3: the tap asks once, the switch reads On for the login, and the login row says Push on.
    await toggle.click()
    await expect(toggle).toHaveAttribute("aria-checked", "true")
    await expect(notif.getByText(/^On for “.+”\.$/)).toBeVisible()
    expect((await pushState()).permissionRequests).toBe(1)
    await go("#/settings/logins")
    const loginRow = notif
      .locator('a[href^="#/settings/logins/"]')
      .filter({ hasText: "This device" })
    await expect(loginRow).toContainText("Push on")

    // Stories 13, 15: a test push is sent, a failing fixture shows the failure, a healthy one clears it.
    await go("#/settings/s/push")
    await expect(toggle).toHaveAttribute("aria-checked", "true")
    await expect(testRow).toBeEnabled()
    await testRow.click()
    const result = notif.locator('[data-mock-part="test-result"]')
    await expect(result).toContainText("Sent.")
    const sent = await fixture.pushRequests()
    expect(sent.filter((r) => r.path === "/push/1")).toHaveLength(1)
    expect(sent[0]).toMatchObject({ ttl: "60", authorization: true })
    await fixture.failPush()
    await testRow.click()
    await expect(result).not.toContainText("Sent.")
    await expect(notif.getByText(/^Last push failed: /)).toBeVisible()
    await fixture.pushOk()
    await testRow.click()
    await expect(result).toContainText("Sent.")
    await expect(notif.getByText(/^Last push failed: /)).toHaveCount(0)

    // Story 35: the badge on the app icon is the snapshot's count.
    // The page applies the snapshot's badge only when the field is there, and a zero badge is left out
    // of the snapshot, so the expected value is the field itself, or nothing applied.
    const snapshotBadge = async () => {
      const snapshot = await context.request.get(new URL("/v1/snapshot", page.url()).href)
      return ((await snapshot.json()) as { badge?: number }).badge ?? null
    }
    await expect
      .poll(async () => (await pushState()).applied === (await snapshotBadge()))
      .toBe(true)

    // Stories 46, 47, 50: the Push server switch off disables the device switch and the test row,
    // and the device is still On when the server switch comes back. After the badge step: the page
    // gets no new snapshot when the switch comes back on, so the badge waits for the next change.
    const serverSwitch = notif.getByRole("switch", { name: "Server-wide Push", exact: true })
    await serverSwitch.click()
    await expect(toggle).toBeDisabled()
    await expect(testRow).toBeDisabled()
    await serverSwitch.click()
    await expect(toggle).toBeEnabled()
    await expect(toggle).toHaveAttribute("aria-checked", "true")

    // Story 6: switching off drops the row's chip and clears the badge.
    const clearedBefore = (await pushState()).cleared
    await toggle.click()
    await expect(toggle).toHaveAttribute("aria-checked", "false")
    await expect.poll(async () => (await pushState()).cleared).toBeGreaterThan(clearedBefore)
    await go("#/settings/logins")
    await expect(loginRow).toBeVisible()
    await expect(loginRow).not.toContainText("Push on")

    // Story 18: Remove on the login's page keeps the login and drops the push.
    await go("#/settings/s/push")
    await toggle.click()
    await expect(toggle).toHaveAttribute("aria-checked", "true")
    await go("#/settings/logins")
    await loginRow.click()
    await expect(notif).toHaveURL(/#\/settings\/logins\/.+/)
    await expect(notif.locator('[data-mock-part="push-row"]')).toBeVisible()
    await notif.getByRole("button", { name: "Remove" }).click()
    await expect(notif.locator('[data-mock-part="push-row"]')).toHaveCount(0)
    await go("#/settings/logins")
    await expect(loginRow).toBeVisible()
    await expect(loginRow).not.toContainText("Push on")
    await go("#/settings/s/push")
    await expect(toggle).toHaveAttribute("aria-checked", "false")
    await notif.close()

    // Cold open, last: no per-tab copy exists in a new page, and nothing runs after the server stop.
    await fixture.stopServer()
    const cold = await context.newPage()
    await cold.goto(page.url())
    await expect(cold.getByText("Offline, reconnecting…")).toBeVisible({ timeout: 30_000 })
    await expect(cold.getByText("Can't reach gx. Retrying…")).toBeVisible()
    await expect(cold.getByText("mixed-epic")).toHaveCount(0)
    await expect(cold.getByText("pwa outside")).toHaveCount(0)
  })
})

test("7. Epic completion", async ({ page, start }) => {
  const fixture = await start("finishing", { hold: true })
  const progress = await section(page, "Implementing")
  await expect(progress.getByRole("link", { name: "finishing-epic" })).toBeVisible()
  await expect(card(page, "finishing-epic").getByText("1/2")).toBeVisible()

  // While held, the running ticket's card meta starts with "running · ".
  await card(page, "finishing-epic").getByRole("link", { name: "finishing-epic" }).click()
  await expect(page.locator('a[href$="/t/demo:finishing-epic/02"] [data-meta]')).toHaveText(
    /^running · /,
  )
  await openEpics(page)

  await fixture.release()

  // The last ticket merges; the epic leaves Implementing for Finished.
  await expect(progress.getByRole("link", { name: "finishing-epic" })).toHaveCount(0)
  const finished = await section(page, "Finished")
  await expect(finished.getByRole("link", { name: "finishing-epic" })).toBeVisible()
  await expect(card(page, "finishing-epic").getByText("2/2")).toBeVisible()
  await card(page, "finishing-epic").getByRole("link", { name: "finishing-epic" }).click()
  await expect(page.getByText("Merged · $0.00 · 0s agent time")).toBeVisible()
  await expect(page.getByText("2 done", { exact: true })).toBeVisible()
  await expect(page.getByText("2/2")).toBeVisible()
  await expect(page.getByTestId("page-header-actions").getByTestId("comet")).toHaveCount(0)
  const more = page.getByRole("button", { name: /^More stats/ })
  await expect(more).toHaveAttribute("aria-expanded", "false")
  // The divider comes first and More stats last on the page.
  const divider = (await page.getByText("2 done", { exact: true }).boundingBox())!
  const moreBox = (await more.boundingBox())!
  const lastCard = (await page.locator('a[href*="/t/demo:finishing-epic/"]').last().boundingBox())!
  expect(moreBox.y).toBeGreaterThan(lastCard.y + lastCard.height - 1)
  expect(divider.y).toBeLessThan(lastCard.y)
  await more.click()
  await expect(more).toHaveAttribute("aria-expanded", "true")
})

test("6. Queue an epic, and outside changes show up", async ({ page, start }) => {
  const fixture = await start("ready")
  const ready = await section(page, "Ready to queue")
  await expect(ready.getByRole("link", { name: "ready-epic" })).toBeVisible()

  const release = await holdRoute(page, "**/v1/queue/add")
  await card(page, "ready-epic").getByRole("button", nav("Queue")).click()
  await expect(card(page, "ready-epic").getByRole("button", nav("Queueing…"))).toBeDisabled()
  release()

  await expect(ready.getByRole("link", { name: "ready-epic" })).toHaveCount(0)
  const progress = await section(page, "Implementing")
  await expect(progress.getByRole("link", { name: "ready-epic" })).toBeVisible()
  // The card moving is the feedback: no success toast.
  await expect(toasts(page)).toHaveCount(0)

  // A change made outside the page (CLI) moves the card too.
  await fixture.cli("server", "queue", "remove", "demo:ready-epic")
  await expect(ready.getByRole("link", { name: "ready-epic" })).toBeVisible()
  await fixture.cli("server", "queue", "add", "demo:ready-epic")

  await expect(progress.getByRole("link", { name: "ready-epic" })).toBeVisible()
  await expect(ready.getByRole("link", { name: "ready-epic" })).toHaveCount(0)
})
