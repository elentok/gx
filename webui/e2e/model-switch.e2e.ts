import { readdir, readFile, writeFile } from "node:fs/promises"
import { join } from "node:path"
import type { Page } from "@playwright/test"
import type { SessionInfo } from "../src/generated/server"
import { expect, nav, test, type Fixture } from "./fixture"
import { go, sessions } from "./agentsKit"

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
  const fixture = await start("model-switch")
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

  // Story 19: with the model query failing, only the current model is listed.
  await editFakes(fixture, (cfg) => {
    cfg.modelsFail = true
  })
  dialog = await openPicker(page, claudeFresh, 1)
  await expect(dialog.getByRole("radiogroup", nav("Model")).getByRole("radio")).toHaveText(
    /^Claude fable\s*current$/,
  )
})
