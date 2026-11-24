import type { Page } from "@playwright/test"
import { expect, renderedText, test, type Fixture } from "./fixture"

// What jsdom cannot see on the Settings page: the real server round trip into config.json and project.json, and phone layout.
// Per-control behavior lives in SettingsPage.test.tsx.

const globalHash = (section: string) => `#/settings/s/${section}`
const projectHash = (project: string, section: string) => `#/settings/p/${project}/s/${section}`

async function go(page: Page, hash: string, heading: string) {
  await page.evaluate((h) => (location.hash = h), hash)
  await expect(page.getByRole("heading", { name: heading, exact: true })).toBeVisible()
}

/** Only the changed row's Save is enabled. */
const save = (page: Page) =>
  page.getByRole("button", { name: "Save", exact: true }).and(page.locator(":enabled"))

const text = (page: Page, label: string) => page.getByRole("textbox", { name: label, exact: true })

async function saveText(page: Page, label: string, value: string) {
  await text(page, label).fill(value)
  await save(page).click()
  // The Save button turning off means this row's value is stored.
  await expect(saved(page)).toBeVisible()
  await expect(save(page)).toHaveCount(0)
}

// Rows keep their own "Saved" note for a moment, so more than one can show at once.
const saved = (page: Page) => page.getByRole("status").filter({ hasText: "Saved" }).first()

/** The number on a group's changed chip; a group with no changed row has no chip. */
const groupCount = async (page: Page, group: string) => {
  const chip = page.getByTestId(`group-count-${group}`)
  return (await chip.count()) === 0 ? 0 : Number(await renderedText(chip))
}

/** A Models cell: its first button opens it, and its input shows while open. */
const cell = (page: Page, key: string) => page.locator(`[data-cell="${key}"]`)

const noSideScroll = async (page: Page) => {
  await expect
    .poll(() =>
      page.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    )
    .toBe(true)
}

test("200. Settings page, global journey", async ({ page, start }) => {
  // A config that still holds the removed `agent-runner: herdr` reads as auto, with a notice under the row.
  // This start comes first so the `reloads: 1` below still guards the reload at the end.
  await start("herdr-config")
  await go(page, globalHash("agents"), "Agents")
  await expect(
    page.getByRole("note").filter({
      hasText: "agent-runner: herdr was removed; using auto (PTY). Edit the config to drop it.",
    }),
  ).toBeVisible()
  // The runner control is a segmented group: one button per enum value, none for herdr.
  const runner = page.getByRole("group", { name: "Agent runner", exact: true })
  await expect(runner.getByRole("button")).toHaveText(["auto", "headless", "pty"])
  await expect(runner.getByRole("button", { name: "auto", exact: true })).toHaveAttribute(
    "aria-pressed",
    "true",
  )

  // The reload at the end re-reads config.json through the page.
  const fixture: Fixture = await start("settings", { reloads: 1 })
  await go(page, globalHash("execution-queue"), "Execution queue")
  const before = await fixture.globalConfig()

  // The sidebar lists the seven global categories (Project and Checks hold project-only keys).
  const categories = page.locator('[data-mock-part="sidebar"] button[aria-expanded]')
  await expect(categories).toHaveCount(7)
  for (const name of [
    "Agents",
    "Ralph loop",
    "Notifications",
    "Mental health",
    "Web UI",
    "System",
    "Account",
  ]) {
    await expect(categories.filter({ hasText: name })).toHaveCount(1)
  }

  // A number and a duration reach config.json with the other keys intact; a newly set key raises its group's chip by one.
  const waitingGroup = "Waiting and stalled agents"
  const chipBefore = await groupCount(page, waitingGroup)
  await saveText(page, "Max released agents", "5")
  await saveText(page, "Waiting slot grace", "20s")
  await expect.poll(() => groupCount(page, waitingGroup)).toBe(chipBefore + 1)
  const after = await fixture.globalConfig()
  expect(after["execution-queue"]["max-released-agents"]).toBe(5)
  expect(after["execution-queue"]["waiting-slot-grace"]).toBe("20s")
  expect(after.budget).toEqual(before.budget)
  expect(after.notifications).toEqual(before.notifications)

  // A reopened page shows the saved values.
  await page.reload()
  await expect(text(page, "Waiting slot grace")).toHaveValue("20s")
  await expect(text(page, "Max released agents")).toHaveValue("5")

  // The server refuses a value that is not a duration; the draft stays and nothing is written.
  await text(page, "Waiting slot grace").fill("abc")
  await save(page).click()
  await expect(page.getByRole("alert").filter({ hasText: "Not saved:" })).toBeVisible()
  await expect(text(page, "Waiting slot grace")).toHaveValue("abc")
  expect((await fixture.globalConfig())["execution-queue"]["waiting-slot-grace"]).toBe("20s")

  // Reset removes the key from the file.
  // The group lists Waiting slot grace before Max released agents, and both are set.
  await page
    .getByTestId(`group-${waitingGroup}`)
    .getByRole("button", { name: /^Reset to default/ })
    .first()
    .click()
  await expect(saved(page)).toBeVisible()
  const reset = await fixture.globalConfig()
  expect(reset["execution-queue"]["waiting-slot-grace"]).toBeUndefined()
  expect(reset["execution-queue"]["max-released-agents"]).toBe(5)

  // Secrets: the read response and the page carry the state, never the value.
  await go(page, globalHash("desktop"), "Desktop")
  // Banners and sounds sit in the Desktop group.
  const desktop = page.getByTestId("group-Desktop")
  await expect(desktop.getByRole("switch", { name: "OS banners" })).toBeVisible()
  await expect(desktop.getByRole("switch", { name: "Sounds" })).toBeVisible()
  const body = await page.evaluate(async () => (await fetch("/v1/settings?target=global")).text())
  expect(body).toContain("notifications.slack.webhook-url")
  expect(body).not.toContain("fixture-bot-token")
  expect(await page.content()).not.toContain("fixture-bot-token")
  await go(page, globalHash("telegram"), "Telegram")
  const setChips = page.locator("span[data-tone]").filter({ hasText: /^Set$/ })
  await expect(setChips).toHaveCount(1)
  await go(page, globalHash("slack"), "Slack")
  await expect(page.getByTestId("group-Slack").getByText("Not set", { exact: true })).toBeVisible()
  await page.getByTestId("group-Slack").getByRole("button", { name: "Set", exact: true }).click()
  await page.getByLabel("New Slack webhook URL").fill("https://hooks.slack.example/secret-webhook")
  await save(page).click()
  await expect(page.getByTestId("group-Slack").getByText("Not set", { exact: true })).toHaveCount(0)
  expect(await page.content()).not.toContain("secret-webhook")
  expect(JSON.stringify(await fixture.globalConfig())).toContain("secret-webhook")
  const reread = await page.evaluate(async () => (await fetch("/v1/settings?target=global")).text())
  expect(reread).not.toContain("secret-webhook")

  // Notifications: the strip and the kind rows at 900 px.
  await page.setViewportSize({ width: 900, height: 900 })
  await go(page, globalHash("notifications"), "Notifications")
  const strip = page.locator('[data-mock-part="channel-switches"]')
  await expect(strip.getByRole("switch")).toHaveCount(5)
  await expect(page.locator('[data-testid^="kind-row-"]')).toHaveCount(16)
  // One chip changes one cell: the whole list for that channel is written, then written back.
  const slackKinds = async () =>
    (await fixture.globalConfig())["notification-channels"]?.slack?.kinds
  const chip = page
    .getByTestId("kind-row-long-ticket")
    .getByRole("button", { name: "Long ticket on Slack" })
  await expect(chip).toHaveAttribute("aria-pressed", "false")
  await chip.click()
  await expect.poll(async () => (await slackKinds())?.includes("long-ticket")).toBe(true)
  await expect(chip).toHaveAttribute("aria-pressed", "true")
  await chip.click()
  await expect.poll(async () => (await slackKinds())?.includes("long-ticket")).toBe(false)
  // A channel that is switched off keeps its token, chat id and checked kinds; its chips dim.
  await strip.getByRole("switch", { name: "Telegram", exact: true }).click()
  await expect(strip.getByRole("switch", { name: "Telegram", exact: true })).toHaveAttribute(
    "aria-checked",
    "false",
  )
  await expect
    .poll(async () => (await fixture.globalConfig())["notification-channels"]?.telegram?.enabled)
    .toBe(false)
  const telegram = (await fixture.globalConfig()).notifications.telegram
  expect(telegram["bot-token"]).toBeTruthy()
  expect(telegram["chat-id"]).toBe("12345")
  await expect(
    page.getByTestId("kind-row-session-alert").getByRole("button", { name: /on Telegram$/ }),
  ).toHaveCSS("opacity", "0.4")
  await go(page, globalHash("telegram"), "Telegram")
  await expect(page.getByText("Set", { exact: true })).toBeVisible()
  await go(page, globalHash("notifications"), "Notifications")
  await strip.getByRole("switch", { name: "Telegram", exact: true }).click()
  await expect
    .poll(async () => (await fixture.globalConfig())["notification-channels"]?.telegram?.enabled)
    .toBe(true)
  // A hand-typed list that names no catalog kind is refused in the search results and config.json stays.
  const beforeBogus = JSON.stringify(await fixture.globalConfig())
  await page
    .getByRole("textbox", { name: "Search settings" })
    .fill("notification-channels.slack.kinds")
  await page.getByLabel("Add to Kinds").fill("bogus")
  await page.getByRole("button", { name: "Add", exact: true }).click()
  await expect(page.getByRole("alert").filter({ hasText: "Not saved:" })).toBeVisible()
  expect(JSON.stringify(await fixture.globalConfig())).toBe(beforeBogus)
  await page.getByRole("textbox", { name: "Search settings" }).fill("")

  // The Push switch row sits in the Push section; off dims the strip box, the chips and the section, and keeps them editable.
  const pushEnabled = async () =>
    (await fixture.globalConfig())["notification-channels"]?.push?.enabled
  await go(page, globalHash("push"), "Push")
  await page.getByTestId("group-Push").getByRole("button", { name: "Set", exact: true }).click()
  await page.getByLabel("New Push contact email").fill("me@example.com")
  await save(page).click()
  await expect(save(page)).toHaveCount(0)
  await page.getByRole("switch", { name: "Server-wide Push" }).click()
  await expect.poll(pushEnabled).toBe(false)
  expect(JSON.stringify(await fixture.globalConfig())).toContain("push-contact-email")
  await expect(page.getByTestId("group-Push")).toHaveAttribute("data-dimmed", "true")
  await go(page, globalHash("notifications"), "Notifications")
  await expect(strip.getByRole("switch", { name: "Push", exact: true })).toHaveAttribute(
    "aria-checked",
    "false",
  )
  const pushChip = page
    .getByTestId("kind-row-long-ticket")
    .getByRole("button", { name: "Long ticket on Push" })
  await expect(pushChip).toHaveCSS("opacity", "0.4")
  const pushKinds = async () => (await fixture.globalConfig())["notification-channels"]?.push?.kinds
  await pushChip.click()
  await expect.poll(async () => (await pushKinds())?.includes("long-ticket")).toBe(true)
  await pushChip.click()
  await expect.poll(async () => (await pushKinds())?.includes("long-ticket")).toBe(false)
  await go(page, globalHash("push"), "Push")
  await page.getByRole("switch", { name: "Server-wide Push" }).click()
  await expect.poll(pushEnabled).toBe(true)

  // A startup-only key says it needs a restart.
  await go(page, globalHash("agents"), "Agents")
  await saveText(page, "Runner wait", "45s")
  await expect(page.getByText("Restart needed to apply").first()).toBeVisible()

  // Access holds the listen address, the allowed hosts and the one-time code switch.
  await go(page, globalHash("access"), "Access")
  await expect(page.getByTestId("group-Network").getByText("Listen address").first()).toBeVisible()
  await expect(page.getByTestId("group-Network").getByText("Allowed hosts").first()).toBeVisible()
  await expect(
    page.getByTestId("group-Sign-in").getByRole("switch", { name: "Require one-time code" }),
  ).toBeVisible()
  await expect(text(page, "Vi escape sequence")).toHaveCount(0)

  // Display and development, Watchdogs and the Models cells have their own sections.
  await go(page, globalHash("display-and-development"), "Display and development")
  await expect(page.getByRole("switch", { name: "Dev mode" })).toBeVisible()
  await go(page, globalHash("watchdogs"), "Watchdogs")
  await expect(page.getByTestId("group-CPU")).toBeVisible()
  await expect(page.getByTestId("group-Process")).toBeVisible()

  // A Models cell: Enter saves, Escape discards, an empty save writes "", reset removes the key.
  await go(page, globalHash("models"), "Models")
  const strongest = cell(page, "agents.claude.strongest.model")
  const strongestModel = async () => (await fixture.globalConfig()).agents?.claude?.strongest?.model
  expect(await strongestModel()).toBeUndefined()
  await strongest.getByRole("button").first().click()
  // gx's own default for this cell is opus, so the typed values differ from it.
  await strongest.getByRole("textbox").fill("sonnet")
  await strongest.getByRole("textbox").press("Enter")
  await expect.poll(strongestModel).toBe("sonnet")
  await strongest.getByRole("button", { name: "sonnet", exact: true }).click()
  await strongest.getByRole("textbox").fill("haiku")
  await strongest.getByRole("textbox").press("Escape")
  await expect(strongest.getByRole("textbox")).toHaveCount(0)
  await expect(strongest.getByRole("button", { name: "sonnet", exact: true })).toBeVisible()
  expect(await strongestModel()).toBe("sonnet")
  await strongest.getByRole("button", { name: "sonnet", exact: true }).click()
  await strongest.getByRole("textbox").fill("")
  await strongest.getByRole("textbox").press("Enter")
  await expect.poll(strongestModel).toBe("")
  await expect(strongest.getByRole("button", { name: "Claude default" })).toBeVisible()
  await strongest.getByRole("button", { name: "Reset agents.claude.strongest.model" }).click()
  await expect.poll(strongestModel).toBeUndefined()
  await expect(strongest).not.toHaveAttribute("data-set", "true")
  await expect(strongest.getByRole("button", { name: "opus", exact: true })).toBeVisible()

  // The retired slugs land on the sections that took over their keys.
  await go(page, globalHash("web-ui"), "Access")
  await expect(page).toHaveURL(/#\/settings\/s\/access$/)
  await go(page, globalHash("cpu-watchdog"), "Watchdogs")
  await go(page, globalHash("access"), "Access")
  await go(page, globalHash("process-watchdog"), "Watchdogs")
  await expect(page).toHaveURL(/#\/settings\/s\/watchdogs$/)

  // Keybindings: the two Leader keys and vi escape live here.
  await go(page, globalHash("keybindings"), "Keybindings")
  await expect(text(page, "Leader key (Mac)")).toBeVisible()
  await expect(text(page, "Leader key (other platforms)")).toBeVisible()
  await expect(text(page, "Vi escape sequence")).toBeVisible()

  // The help text points to the Chord box, and the page lists no chord letters.
  await expect(page.getByText("Press it to see the chords").first()).toBeVisible()
  await expect(page.getByText("New chat")).toHaveCount(0)
  await expect(page.getByText("New design")).toHaveCount(0)

  // Defaults, and each platform uses its own key (a second and a third page on the same server).
  await expect(text(page, "Leader key (Mac)")).toHaveValue("cmd+e")
  await expect(text(page, "Leader key (other platforms)")).toHaveValue("ctrl+e")
  const onPlatform = async (platform: string) => {
    const p = await page.context().newPage()
    await p.addInitScript((value) => {
      Object.defineProperty(Navigator.prototype, "platform", { get: () => value })
      // The app reads userAgentData.platform first, so a Mac host would still read as a Mac.
      Object.defineProperty(Navigator.prototype, "userAgentData", {
        get: () => ({ platform: value }),
      })
    }, platform)
    await p.goto(`${page.url().split("#")[0]!}#/`)
    return p
  }
  const mac = await onPlatform("MacIntel")
  const other = await onPlatform("Linux x86_64")
  const box = (p: Page) => p.getByRole("menu", { name: "Chords" })
  await mac.bringToFront()
  await mac.keyboard.press("Control+e")
  await expect(box(mac)).toHaveCount(0)
  // The key handler may not be ready right after the page loads, so press again until the box opens.
  await expect(async () => {
    await mac.keyboard.press("Meta+e")
    await expect(box(mac)).toBeVisible({ timeout: 1_000 })
  }).toPass({ timeout: 10_000 })
  await mac.keyboard.press("Escape")
  await expect(box(mac)).toHaveCount(0)
  await other.bringToFront()
  await other.keyboard.press("Meta+e")
  await expect(box(other)).toHaveCount(0)
  await expect(async () => {
    await other.keyboard.press("Control+e")
    await expect(box(other)).toBeVisible({ timeout: 1_000 })
  }).toPass({ timeout: 10_000 })
  await other.keyboard.press("Escape")
  await expect(box(other)).toHaveCount(0)

  // A Leader key with no Cmd, Ctrl or Alt is refused and the draft stays; alt+e saves.
  await page.bringToFront()
  await text(page, "Leader key (Mac)").fill("e")
  await save(page).click()
  await expect(page.getByRole("alert").filter({ hasText: "Not saved:" })).toBeVisible()
  await expect(text(page, "Leader key (Mac)")).toHaveValue("e")
  // Rows keep their own "Saved" note, so wait on the file instead.
  const saveLeader = async (label: string, value: string, configKey: string) => {
    await text(page, label).fill(value)
    await save(page).click()
    await expect
      .poll(async () => (await fixture.globalConfig()).server?.web?.[configKey])
      .toBe(value === "" ? "" : value)
  }
  await saveLeader("Leader key (Mac)", "alt+e", "leader-key-mac")

  // The snapshot reaches the other pages after the save, so each poll presses the key again and
  // closes a box it opened.
  const opens = async (p: Page, combo: string) => {
    await p.keyboard.press(combo)
    const shown = await box(p).isVisible()
    if (shown) await p.keyboard.press("Escape")
    return shown
  }

  // An empty Leader key opens no box on that platform, with no reload.
  await saveLeader("Leader key (other platforms)", "", "leader-key")
  await other.bringToFront()
  await expect.poll(() => opens(other, "Control+e")).toBe(false)

  // Changing the Mac key reaches the open page live: the new key opens the box, the old one does not.
  await page.bringToFront()
  await saveLeader("Leader key (Mac)", "alt+j", "leader-key-mac")
  await mac.bringToFront()
  await expect
    .poll(async () => {
      await mac.keyboard.press("Alt+j")
      return box(mac).isVisible()
    })
    .toBe(true)
  // The box header shows the new key.
  await expect(box(mac)).toContainText("⌥J")
  await mac.keyboard.press("Escape")
  await expect(box(mac)).toHaveCount(0)
  expect(await opens(mac, "Alt+e")).toBe(false)
  await mac.close()
  await other.close()

  // The phone layout has no sideways scroll on a section, the home and the project's section list.
  await page.setViewportSize({ width: 390, height: 844 })
  await noSideScroll(page)
  // Notifications on a phone: the chips sit under the name at the row's width; the strip has two columns.
  await go(page, globalHash("notifications"), "Notifications")
  await noSideScroll(page)
  const row = page.getByTestId("kind-row-session-alert")
  const rowBox = await row.boundingBox()
  const nameBox = await row.locator("p").first().boundingBox()
  const chipsBox = await row.locator("button").first().locator("xpath=..").boundingBox()
  expect(chipsBox!.y).toBeGreaterThanOrEqual(nameBox!.y + nameBox!.height - 1)
  expect(chipsBox!.width).toBeGreaterThan(rowBox!.width - 40)
  const boxes = await page
    .locator('[data-testid^="channel-switch-"]')
    .evaluateAll((els) => els.map((e) => e.getBoundingClientRect().top))
  expect(Math.abs(boxes[0] - boxes[1])).toBeLessThan(2)
  expect(boxes[2]).toBeGreaterThan(boxes[0] + 10)
  await page.getByRole("link", { name: "Back to Settings" }).click()
  await expect(page.getByRole("heading", { name: "Settings", exact: true })).toBeVisible()
  const scope = page.getByRole("combobox", { name: "Settings scope" })
  await expect(scope).toBeVisible()
  await expect(page.getByRole("link", { name: /^Budget/ })).toBeVisible()
  await noSideScroll(page)
  await scope.selectOption("demo")
  await expect(page).toHaveURL(/#\/settings\/p\/demo$/)
  await expect(page.getByText("Account", { exact: true })).toHaveCount(0)
  await noSideScroll(page)
  await page.getByRole("link", { name: /^Checks/ }).click()
  await expect(page.getByRole("heading", { name: "Checks", exact: true })).toBeVisible()
  await noSideScroll(page)
})

test("201. Settings page, project journey", async ({ page, start }) => {
  const fixture = await start("settings")

  // The project tree starts with Project, ends with Checks and has no Notifications or Account.
  await go(page, projectHash("plain", "execution-queue"), "Execution queue")
  const categories = page.locator('[data-mock-part="sidebar"] button[aria-expanded]')
  await expect(categories.first()).toContainText("Project")
  await expect(categories.last()).toContainText("Checks")
  await expect(categories.filter({ hasText: "Notifications" })).toHaveCount(0)
  await expect(categories.filter({ hasText: "Account" })).toHaveCount(0)

  // A key the project does not set shows the global value and an Override button.
  await expect(page.getByText("Max agents per epic").first()).toBeVisible()
  await expect(page.getByText("Uses the global value").first()).toBeVisible()
  await expect(page.getByRole("button", { name: "Override for plain" }).first()).toBeVisible()

  // The Project section's Repository and Delivery groups: read-only facts, and editable rows that reach project.json.
  await go(page, projectHash("demo", "project"), "Project")
  await expect(page.getByTestId("group-Repository")).toBeVisible()
  await expect(page.getByTestId("group-Delivery")).toBeVisible()
  await saveText(page, "Trunk", "develop")
  expect((await fixture.projectConfig("demo")).trunk).toBe("develop")
  const autoFf = (await fixture.projectConfig("demo"))["auto-ff-merge"]
  await page.getByRole("switch", { name: "Auto ff merge" }).click()
  await expect
    .poll(async () => (await fixture.projectConfig("demo"))["auto-ff-merge"])
    .not.toBe(autoFf)

  await go(page, projectHash("demo", "execution-queue"), "Execution queue")

  // Override a cap for the project, then go back to the global value.
  await page.getByRole("button", { name: "Override for demo" }).first().click()
  await expect(saved(page)).toBeVisible()
  const overridden = await fixture.projectConfig("demo")
  expect(overridden["execution-queue"]).toBeDefined()
  await page
    .getByRole("button", { name: /^Use global value/ })
    .first()
    .click()
  await expect(page.getByRole("button", { name: "Override for demo" }).first()).toBeVisible()

  // The per-project cap (demo starts with one) is removed and set again.
  expect((await fixture.projectConfig("demo"))["max-agents"]).toBe(1)
  await page.getByRole("button", { name: "Remove cap" }).click()
  await expect(page.getByRole("button", { name: "Set a cap for demo" })).toBeVisible()
  expect((await fixture.projectConfig("demo"))["max-agents"]).toBeUndefined()
  await page.getByRole("button", { name: "Set a cap for demo" }).click()
  await expect(page.getByRole("button", { name: "Remove cap" })).toBeVisible()
  expect((await fixture.projectConfig("demo"))["max-agents"]).toBeDefined()

  // A hand-written notifications block survives saving another key.
  await go(page, projectHash("notes", "execution-queue"), "Execution queue")
  await page.getByRole("button", { name: "Override for notes" }).first().click()
  await expect(saved(page)).toBeVisible()
  const notes = await fixture.projectConfig("notes")
  expect(notes.notifications.telegram["bot-token"]).toBe("notes-token")

  // A Checks card edit lands in project.json.
  await go(page, projectHash("demo", "checks"), "Checks")
  await page
    .getByRole("group", { name: "format mode" })
    .getByRole("button", { name: "off", exact: true })
    .click()
  await save(page).click()
  await expect.poll(async () => (await fixture.projectConfig("demo")).checks.format).toBeDefined()
  expect((await fixture.projectConfig("demo")).checks.test.full).toBe("true")

  // Models: a project cell sets and resets against project.json, and an empty save removes the key.
  await go(page, projectHash("demo", "models"), "Models")
  const model = async (tier: string) =>
    (await fixture.projectConfig("demo")).agents?.claude?.[tier]?.model
  const light = cell(page, "agents.claude.light.model")
  expect(await model("light")).toBeUndefined()
  await light.getByRole("button").first().click()
  await light.getByRole("textbox").fill("haiku")
  await light.getByRole("textbox").press("Enter")
  await expect.poll(() => model("light")).toBe("haiku")
  await light.getByRole("button", { name: "Reset agents.claude.light.model" }).click()
  await expect.poll(() => model("light")).toBeUndefined()
  const strongest = cell(page, "agents.claude.strongest.model")
  expect(await model("strongest")).toBe("opus")
  await strongest.getByRole("button", { name: "opus", exact: true }).click()
  await strongest.getByRole("textbox").fill("")
  await strongest.getByRole("textbox").press("Enter")
  await expect.poll(() => model("strongest")).toBeUndefined()

  // Retired slugs land on the project's first section.
  await go(page, projectHash("demo", "cpu-watchdog"), "Project")
  await go(page, projectHash("demo", "execution-queue"), "Execution queue")
  await go(page, projectHash("demo", "web-ui"), "Project")
})
