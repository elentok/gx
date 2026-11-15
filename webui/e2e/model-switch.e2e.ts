import { readdir, readFile, rm, writeFile } from "node:fs/promises"
import { join } from "node:path"
import type { Page } from "@playwright/test"
import type { SessionInfo } from "../src/generated/server"
import { expect, nav, test, type Fixture } from "./fixture"
import { go, moreActions, sessions } from "./agentsKit"

const LEADER = "ControlOrMeta+e"

/** The launch log of every session in a harness's fake dir, keyed by the log's file name: one argv per line. */
async function launches(dir: string) {
  const out: Record<string, string[]> = {}
  for (const name of await readdir(dir)) {
    if (name.endsWith(".launches")) {
      out[name.replace(/\.launches$/, "")] = (await readFile(join(dir, name), "utf8"))
        .split("\n")
        .filter(Boolean)
    }
  }
  return out
}

/** Rewrites the fakes' config: `patch` edits the parsed JSON. */
async function editFakes(fixture: Fixture, patch: (cfg: Record<string, any>) => void) {
  const path = fixture.modelFakes!.fake_config
  const cfg = JSON.parse(await readFile(path, "utf8"))
  patch(cfg)
  await writeFile(path, JSON.stringify(cfg))
}

/** Opens a chat and its picker with the Leader chord, and waits for the model list to arrive. */
async function openPicker(page: Page, session: SessionInfo, rows = 2) {
  await go(page, `#/agents/${session.id}`)
  await expect(page.getByRole("form", nav("Composer"))).toBeVisible()
  await expect(async () => {
    await page.keyboard.press(LEADER)
    await page.keyboard.press("m")
    await expect(page.getByRole("dialog", nav("Switch model"))).toBeVisible({ timeout: 1_000 })
  }).toPass({ timeout: 10_000 })
  const dialog = page.getByRole("dialog", nav("Switch model"))
  await expect(dialog.getByRole("radiogroup", nav("Model")).getByRole("radio")).toHaveCount(rows, {
    timeout: 15_000,
  })
  return dialog
}

/** The server's SessionStateToken, as the page builds it. */
function stateToken(s: SessionInfo) {
  const nanos = (/\.(\d+)/.exec(s.status_changed_at)?.[1] ?? "").padEnd(9, "0").slice(0, 9)
  const seconds = Math.floor(Date.parse(s.status_changed_at.replace(/\.\d+/, "")) / 1000)
  return `${s.id}:${s.status.state}:${BigInt(seconds) * 1_000_000_000n + BigInt(nanos)}`
}

// The model picker over the `model-switch` scenario: an idle Codex chat on an outdated model and two idle Claude chats.
test("205. Model switch", async ({ page, start }) => {
  const fixture = await start("model-switch", { autoAdvanceDelay: "1h" })
  const fakes = fixture.modelFakes!
  const chats = (await sessions(fixture)).filter((s) => !s.design)
  const codex = chats.find((s) => s.harness === "codex")!
  const [claude, claudeFresh] = chats.filter((s) => s.harness !== "codex")
  await page.setViewportSize({ width: 1280, height: 800 })

  // Stories 1, 3, 8 to 12: the chord opens the picker with the filter focused. It lists only Codex's own
  // models, grouped by family, newest first; the outdated ones are dimmed, the unparseable id is last in Other.
  let dialog = await openPicker(page, codex, 5)
  const filter = dialog.getByRole("textbox", nav("Filter models"))
  await expect(filter).toBeFocused()
  const group = dialog.getByRole("radiogroup", nav("Model"))
  const rows = group.getByRole("radio")
  await expect(group.locator("p")).toHaveText(["mini", "codex", "Other"])
  await expect(rows).toHaveText([
    /^Codex GPT-6-Mini$/,
    /^Codex GPT-5\.6-Codex$/,
    /^Codex GPT-5\.5-Codex\s*newer available$/,
    /^Codex GPT-5\.4-Codex\s*newer available\s*current$/,
    /^Codex mystery-model$/,
  ])
  const color = (n: number) => rows.nth(n).evaluate((el) => getComputedStyle(el).color)
  const plain = await color(4)
  expect(await color(0)).toBe(plain)
  expect(await color(2)).not.toBe(plain)
  expect(await color(3)).not.toBe(plain)
  await expect(group).not.toContainText("hidden")
  await expect(dialog.getByRole("radio", { name: /5\.3/ })).toHaveCount(0)

  // Stories 4, 5, 6, 14: typing filters; Down changes the model and Right the effort; Switch waits for a change.
  const switchButton = dialog.getByRole("button", nav("Switch"))
  const checked = group.getByRole("radio", { checked: true })
  const effortOf = (name: string) => dialog.getByRole("radio", nav(name))
  await filter.fill("sol")
  await expect(rows).toHaveText([/^Codex mystery-model$/])
  await filter.fill("")
  await expect(rows).toHaveCount(5)
  await expect(checked).toHaveText(/GPT-5\.4-Codex/)
  await expect(switchButton).toBeDisabled()
  await page.keyboard.press("ArrowDown")
  await expect(checked).toHaveText(/mystery-model/)
  await expect(switchButton).toBeEnabled()
  await page.keyboard.press("ArrowUp")
  await expect(checked).toHaveText(/GPT-5\.4-Codex/)
  await expect(switchButton).toBeDisabled()
  await expect(effortOf("medium")).toBeChecked()
  await page.keyboard.press("ArrowRight")
  await expect(effortOf("high")).toBeChecked()
  await expect(switchButton).toBeEnabled()
  await page.keyboard.press("ArrowLeft")
  await expect(effortOf("medium")).toBeChecked()
  await expect(switchButton).toBeDisabled()

  // Story 7: Esc closes and nothing changes.
  await page.keyboard.press("ArrowRight")
  await page.keyboard.press("Escape")
  await expect(dialog).toHaveCount(0)
  expect(Object.values(await launches(fakes.codex_dir)).map((l) => l.length)).toEqual([1])

  // Story 7 again: Enter on a changed choice starts the switch, and the fake agent logs the relaunch.
  // The held launch keeps the chat busy for the next steps.
  await editFakes(fixture, (cfg) => {
    for (const m of cfg.models) if (m.value === "gpt-5.4-codex") m.behaviour = "hold"
  })
  dialog = await openPicker(page, codex, 5)
  await page.keyboard.press("ArrowRight")
  await page.keyboard.press("Enter")
  await expect(dialog).toHaveCount(0)
  await expect(async () => {
    const logs = await launches(fakes.codex_dir)
    expect(Object.values(logs).map((l) => l.length)).toEqual([2])
    expect(Object.values(logs)[0][1]).toContain("gpt-5.4-codex")
    expect(Object.values(logs)[0][1]).toContain("high")
  }).toPass({ timeout: 15_000 })

  // Stories 15, 16: with the agent working the picker is disabled with the busy note, and the route refuses.
  dialog = await openPicker(page, codex, 5)
  await expect(dialog.getByRole("status")).toHaveText(
    "The agent is working. Wait until it is idle to switch.",
  )
  await expect(filter).toBeDisabled()
  await expect(dialog.getByRole("radio").first()).toBeDisabled()
  await expect(switchButton).toBeDisabled()
  const busy = (await sessions(fixture)).find((s) => s.id === codex.id)!
  expect(busy.status.state).not.toBe("idle")
  const refused = await page.request.post(new URL("/v1/sessions/switch", page.url()).href, {
    data: { id: busy.id, state: stateToken(busy), model: "gpt-5.5-codex", effort: "high" },
  })
  expect(refused.status()).toBeGreaterThanOrEqual(400)
  expect(refused.status()).toBeLessThan(500)
  await page.keyboard.press("Escape")
  await expect(dialog).toHaveCount(0)

  // After a stop the note asks for a resume, and Switch stays disabled.
  // The held launch is released first, so the stop ends a running agent and the session can resume later.
  const [codexKey] = Object.keys(await launches(fakes.codex_dir))
  await writeFile(join(fakes.codex_dir, `${codexKey}.release`), "")
  await expect(async () => {
    expect((await sessions(fixture)).find((s) => s.id === codex.id)!.status.state).toBe("idle")
  }).toPass({ timeout: 20_000 })
  await fixture.cli("agents", "stop", codex.id)
  dialog = await openPicker(page, codex, 5)
  await expect(dialog.getByRole("status")).toHaveText("Resume the session to switch.")
  await expect(switchButton).toBeDisabled()
  await page.keyboard.press("Escape")
  await expect(dialog).toHaveCount(0)

  // Story 13, 9: on a Claude chat the list shows resolved ids, a 4.6 model has no xhigh and Haiku has no effort control.
  dialog = await openPicker(page, claude, 6)
  const claudeRows = dialog.getByRole("radiogroup", nav("Model")).getByRole("radio")
  await expect(claudeRows).toHaveText([
    /^Claude Haiku 5\.5$/,
    /^Claude Opus 5\.5$/,
    /^Claude Opus 4\.6$/,
    /^Claude Fable 5\.2$/,
    /^Claude Fable 5\.1\s*newer available\s*current$/,
    /^Claude mystery-model$/,
  ])
  await expect(dialog.getByText("Claude Opus", { exact: true })).toHaveCount(0)
  await expect(dialog.getByRole("radio", { name: /default/i })).toHaveCount(0)
  await expect(effortOf("xhigh")).toBeVisible()
  await claudeRows.filter({ hasText: "Opus 4.6" }).click()
  await expect(effortOf("high")).toBeVisible()
  await expect(effortOf("xhigh")).toHaveCount(0)
  await claudeRows.filter({ hasText: "Haiku" }).click()
  await expect(dialog.getByText("Effort", { exact: true })).toHaveCount(0)
  await page.keyboard.press("Escape")
  await expect(dialog).toHaveCount(0)

  await editFakes(fixture, (cfg) => {
    for (const m of cfg.models) delete m.behaviour
  })

  // Stories 20, 21, 23: a held switch logs "Switching to", the footer shows the target with a hint and the box is
  // disabled. After the release the log reads "Switched to" and the footer keeps the chosen model and effort.
  const setBehaviour = (model: string, behaviour: string, errorCode?: string) =>
    editFakes(fixture, (cfg) => {
      for (const m of cfg.models) {
        if (m.value === model) Object.assign(m, { behaviour, errorCode })
      }
    })
  const startSwitch = async (session: SessionInfo, harness: "claude" | "codex", row: RegExp) => {
    const picker = await openPicker(page, session, harness === "claude" ? 6 : 5)
    await picker.getByRole("radio", { name: row }).click()
    await picker.getByRole("button", nav("Switch")).click()
    await expect(picker).toHaveCount(0)
  }
  const switchLine = page.getByTestId("switch-line")
  const footer = page.getByTestId("footer-line")
  // The prompt box is the Send button: it is off while a switch runs, with text typed.
  const composerForm = page.getByRole("form", nav("Composer"))
  const sendButton = composerForm.getByRole("button", { name: /^Send/ })
  const boxOff = async () => {
    await composerForm.getByRole("textbox").fill("hello")
    await expect(sendButton).toBeDisabled()
  }
  const boxOn = async () => {
    await composerForm.getByRole("textbox").fill("hello")
    await expect(sendButton).toBeEnabled()
  }
  const agentModel = async (s: SessionInfo) =>
    (await sessions(fixture)).find((x) => x.id === s.id)!.model
  // The fake logs by its conversation id, which the session list does not show: the chat's log is the one that grows.
  const lengths = async () =>
    Object.fromEntries(
      Object.entries(await launches(fakes.claude_dir)).map(([k, v]) => [k, v.length]),
    )
  let claudeKey = ""
  const claudeLog = async () => (await launches(fakes.claude_dir))[claudeKey] ?? []

  await setBehaviour("claude-fable-5-2", "hold")
  const seen = await lengths()
  await startSwitch(claude, "claude", /Fable 5\.2/)
  await expect(async () => {
    const grown = Object.entries(await lengths()).filter(([k, n]) => n > (seen[k] ?? 0))
    expect(grown).toHaveLength(1)
    claudeKey = grown[0][0]
  }).toPass({ timeout: 15_000 })
  await expect(switchLine).toHaveText("Switching to Claude Fable 5.2…")
  await expect(footer).toContainText("Fable 5.2")
  await expect(page.getByTestId("switching-hint")).toBeVisible()
  await boxOff()
  await writeFile(join(fakes.claude_dir, `${claudeKey}.release`), "")
  await expect(switchLine).toHaveText("Switched to Claude Fable 5.2.")
  await expect(page.getByTestId("switching-hint")).toHaveCount(0)
  await expect(footer).toContainText(/Fable 5\.2 · (low|medium|high|xhigh)/)
  await boxOn()
  const oldModel = await agentModel(claude)
  expect(oldModel).toContain("fable-5-2")

  // Story 22 (a): the new process exits at start. The old model stays in the footer and in the agents list, the
  // fake logs a relaunch with the old flags and the chat says so.
  await setBehaviour("opus", "exit-at-start")
  const before = (await claudeLog()).length
  await startSwitch(claude, "claude", /Opus 5\.5/)
  await expect(switchLine).toHaveText(
    "Switch to Claude Opus 5.5 failed. Still on Claude Fable 5.2.",
  )
  await expect(footer).toContainText("Fable 5.2")
  await expect(footer).not.toContainText("Opus 5.5")
  expect(await agentModel(claude)).toBe(oldModel)
  await expect(async () => {
    const log = await claudeLog()
    expect(log.length).toBe(before + 2)
    expect(log[log.length - 1]).toContain("claude-fable-5-2")
    expect(log[log.length - 2]).toContain("opus")
  }).toPass({ timeout: 15_000 })
  await boxOn()

  // Story 22 (b): the first message gets model_not_found. The same checks hold.
  // The old model's relaunch is held, so the box stays off while it runs.
  const release = join(fakes.claude_dir, `${claudeKey}.release`)
  const sendMessage = async (text: string) => {
    await composerForm.getByRole("textbox").fill(text)
    await sendButton.click()
  }
  await setBehaviour("opus", "model-not-found")
  await startSwitch(claude, "claude", /Opus 5\.5/)
  await expect(switchLine).toHaveText("Switched to Claude Opus 5.5.")
  await rm(release, { force: true })
  await setBehaviour("claude-fable-5-2", "hold")
  await setBehaviour("fable", "hold")
  await sendMessage("are you there?")
  await expect(switchLine).toHaveText("Switching back to Claude fable…")
  await boxOff()
  await writeFile(release, "")
  await expect(switchLine).toHaveText("Switch to Claude Opus 5.5 failed. Still on Claude fable.")
  await expect(footer).toContainText("fable")
  await expect(footer).not.toContainText("Opus 5.5")
  expect(await agentModel(claude)).toContain("fable")
  await expect(async () => {
    expect((await claudeLog()).at(-1)).toContain("fable")
  }).toPass({ timeout: 15_000 })
  await boxOn()

  // A fake error of another kind leaves the switch in place: no rollback line.
  await setBehaviour("claude-fable-5-2", "")
  await setBehaviour("opus", "error", "overloaded")
  await startSwitch(claude, "claude", /Opus 5\.5/)
  await expect(switchLine).toHaveText("Switched to Claude Opus 5.5.")
  await sendMessage("still there?")
  await expect(footer).toContainText("Opus 5.5")
  await expect(switchLine).toHaveText("Switched to Claude Opus 5.5.")

  // Story 22 (b) on Codex: the rollout's error form. The chat shows no error failure and the menu no "switched by hand".
  await setBehaviour("gpt-5.4-codex", "")
  await fixture.cli("agents", "resume", codex.id)
  await setBehaviour("gpt-5.5-codex", "model-not-found")
  await expect(async () => {
    expect((await sessions(fixture)).find((s) => s.id === codex.id)!.status.state).toBe("idle")
  }).toPass({ timeout: 20_000 })
  await startSwitch(codex, "codex", /GPT-5\.5-Codex/)
  await expect(footer).toContainText("GPT-5.5-Codex")
  await sendMessage("are you there?")
  await expect(switchLine).toHaveText(
    "Switch to Codex GPT-5.5-Codex failed. Still on Codex GPT-5.4-Codex.",
  )
  await expect(footer).toContainText("GPT-5.4-Codex")
  await boxOn()
  await expect(page.getByText("Agent hit an error")).toHaveCount(0)
  expect(await agentModel(codex)).toContain("5.4")
  await moreActions(page).click()
  const sessionMenu = page.getByRole("menu", nav("More actions"))
  await expect(sessionMenu).toBeVisible()
  await expect(sessionMenu).not.toContainText("switched by hand")
  await page.keyboard.press("Escape")

  // Story 43: a switch on the design session stops the auto-advance countdown and the auto-queue one. (The "Countdown
  // stopped" text is an OS banner, so the page cannot show it: the countdown going away is what the flow sees.)
  // A refused switch (a stale state token) leaves the countdown running.
  await setBehaviour("opus", "")
  const design = (await sessions(fixture)).find((s) => s.design)!
  await go(page, "#/d/demo:switch-design")
  const questions = page.getByRole("group", nav("Questions"))
  await questions.getByRole("button", { name: /SQLite/ }).click()
  await questions.getByRole("button", { name: /Yes/ }).click()
  await page.getByRole("button", nav("Send answers")).click()
  const advancing = page.getByText(/Continuing in/)
  await expect(advancing).toBeVisible({ timeout: 30_000 })
  // The route wants the page's Origin; a refusal is a 200 with `refused`.
  const postSwitch = async (data: Record<string, string>) => {
    const res = await page.request.post(new URL("/v1/sessions/switch", page.url()).href, {
      data,
      headers: { Origin: new URL(page.url()).origin },
    })
    const body = await res.text()
    expect(res.ok(), body).toBe(true)
    return JSON.parse(body) as { refused?: boolean }
  }
  const stale = await postSwitch({
    id: design.id,
    state: `${design.id}:idle:1`,
    model: "claude-fable-5-2",
    effort: "medium",
  })
  expect(stale.refused).toBe(true)
  await expect(advancing).toBeVisible()
  // The design chat's composer is off, so the chord has no focus to start from: the footer's model button opens the
  // "Newer model" popover, whose button switches in one step.
  const switchDesign = async () => {
    await go(page, `#/agents/${design.id}`)
    await composerForm.getByRole("button", { name: / · (low|medium|high|xhigh)$/ }).click()
    await page.getByRole("button", nav("Switch to Claude Fable 5.2")).click()
  }
  await switchDesign()
  await go(page, "#/d/demo:switch-design")
  await expect(page.getByRole("region", nav("Chat"))).toContainText("Switched to Claude Fable 5.2.")
  await expect(advancing).toHaveCount(0)

  // The same with the auto-queue countdown: the session is on the newest model now, so no
  // footer popover or chord offers a switch, and the route does it.
  await page.getByRole("button", { name: /^Auto:/ }).click()
  await page
    .getByRole("dialog", nav("Auto"))
    .getByRole("switch", { name: "Auto-queue off" })
    .click()
  await page.keyboard.press("Escape")
  await fixture.cli("tickets", "set", "demo:switch-design", "--design-step", "branch")
  const queuing = page.getByText(/Queuing switch-design in \d+s/)
  await expect(queuing).toBeVisible()
  await expect(async () => {
    const idle = (await sessions(fixture)).find((s) => s.id === design.id)!
    expect(idle.status.state).toBe("idle")
    const res = await postSwitch({
      id: idle.id,
      state: stateToken(idle),
      model: "opus",
      effort: "medium",
    })
    expect(res.refused).toBeFalsy()
  }).toPass({ timeout: 15_000 })
  await go(page, "#/d/demo:switch-design")
  await expect(queuing).toHaveCount(0)

  // Story 19: with the model query failing, only the current model is listed.
  await editFakes(fixture, (cfg) => {
    cfg.modelsFail = true
  })
  dialog = await openPicker(page, claudeFresh, 1)
  await expect(dialog.getByRole("radiogroup", nav("Model")).getByRole("radio")).toHaveText(
    /^Claude fable\s*current$/,
  )
})
