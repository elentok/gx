import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react"
import { ChakraProvider } from "@chakra-ui/react"
import type { ReactElement } from "react"
import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { system } from "../ui/theme"
import { AppToaster } from "../ui/toaster"
import type { SessionInfo } from "../generated/server"
import { renderThemed } from "../ui/render"
import { WideContext } from "../useWide"
import { FooterLine } from "./FooterLine"
import { SwitchLine } from "./modelSwitch"

const { posted, openPicker } = vi.hoisted(() => ({ posted: vi.fn(), openPicker: vi.fn() }))
vi.mock("../shell/modelPickerOpen", () => ({
  postSwitchSession: posted,
  openModelPicker: openPicker,
}))

beforeEach(() => {
  posted.mockReset().mockResolvedValue({})
  openPicker.mockReset()
})
afterEach(cleanup)

const session = (over: Partial<SessionInfo> = {}): SessionInfo =>
  ({
    id: "s-a",
    harness: "claude",
    model: "claude-opus-4-7",
    effort: "high",
    status: { state: "idle" },
    status_changed_at: "2026-01-01T00:00:00Z",
    ...over,
  }) as SessionInfo

const render = (s: SessionInfo, o: { wide?: boolean; ceiling?: number } = {}) => {
  const ceiling = "ceiling" in o ? o.ceiling : 200_000
  return renderThemed(
    <WideContext.Provider value={o.wide ?? true}>
      <FooterLine session={{ ...s, context_ceiling: ceiling }} />
    </WideContext.Provider>,
  )
}

const badge = () => screen.queryByRole("button", { name: "Context" })
const fill = () => Number(screen.getByTestId("context-ring").getAttribute("data-fill"))

test("shows the model and effort, with no badge before the first reply", () => {
  render(session())
  expect(screen.getByText("Claude Opus 4.7 · high")).toBeTruthy()
  expect(badge()).toBeNull()
})

test("a phone drops the effort", () => {
  render(session(), { wide: false })
  expect(screen.getByText("Claude Opus 4.7")).toBeTruthy()
})

test("the badge shows whole thousands and opens context details", async () => {
  render(session({ context_tokens: 70_400, context_level: "green" }))
  expect(badge()!.textContent).toBe("70K")
  fireEvent.click(badge()!)
  const details = await screen.findByRole("dialog", { name: "Context usage" })
  expect(details.textContent).toContain("Latest request input35%")
  expect(details.textContent).toContain("70Kof 200K smart zone")
  expect(details.textContent).not.toContain("headroom")
  expect(details.textContent).not.toContain("over smart zone")
  expect(details.textContent).not.toContain("Used")
})

test("the popover reports usage above the smart zone with zero remaining", async () => {
  render(session({ context_tokens: 185_375, context_level: "red" }), { ceiling: 130_000 })
  fireEvent.click(badge()!)
  const details = await screen.findByRole("dialog", { name: "Context usage" })
  expect(details.textContent).toContain("143%")
  expect(details.textContent).toContain("185Kof 130K smart zone")
  expect(details.textContent).toContain("Remaining to smart zone0")
  expect(details.textContent).not.toContain("headroom")
  expect(details.textContent).not.toContain("over smart zone")
})

test("the popover splits Claude context and shows cumulative usage", async () => {
  render(
    session({
      context_tokens: 70_000,
      context_level: "green",
      usage: {
        input_tokens: 5_000,
        cache_read_tokens: 60_000,
        cache_creation_tokens: 5_000,
        total_tokens: 1_840_000,
        estimated_cost: 4.72,
        compactions: 2,
      },
    }),
  )
  fireEvent.click(badge()!)
  const details = within(await screen.findByRole("dialog", { name: "Context usage" }))
  expect(details.getByText("Reused cache")).toBeTruthy()
  expect(details.getByText("60K")).toBeTruthy()
  expect(details.getByText("New input")).toBeTruthy()
  expect(details.getByText("Newly cached")).toBeTruthy()
  expect(details.getByText("1.8M")).toBeTruthy()
  expect(details.getByText("total tokens")).toBeTruthy()
  expect(details.getByText("≈$4.72")).toBeTruthy()
  expect(details.getByText("2 compactions")).toBeTruthy()
})

test("Codex total tokens render without an estimated cost", async () => {
  render(
    session({ context_tokens: 70_000, context_level: "green", usage: { total_tokens: 300_000 } }),
  )
  fireEvent.click(badge()!)
  const details = within(await screen.findByRole("dialog", { name: "Context usage" }))
  expect(details.getByText("300K")).toBeTruthy()
  expect(details.getByText("total tokens")).toBeTruthy()
  expect(details.queryByText(/\$/)).toBeNull()
})

test("Codex details distinguish context, cumulative subsets, and stale readings", async () => {
  render(
    session({
      context_tokens: 70_000,
      context_level: "green",
      usage: {
        total_tokens: 300_000,
        first_context: 20_000,
        peak_context: 90_000,
        model_context_window: 475_000,
        scope: "thread",
        context_stale: true,
        compactions: 1,
        totals: {
          input_tokens: 80_000,
          cache_read_tokens: 200_000,
          output_tokens: 20_000,
          reasoning_tokens: 10_000,
        },
      },
    }),
  )
  fireEvent.click(badge()!)
  const details = within(await screen.findByRole("dialog", { name: "Context usage" }))
  for (const label of [
    "First request",
    "Peak request",
    "Remaining to smart zone",
    "Reported model capacity",
    "This thread only",
    "Reasoning, included in output",
    "1 compaction",
  ]) {
    expect(details.getByText(label)).toBeTruthy()
  }
  expect(details.getByRole("status").textContent).toContain("Waiting for a new request reading")
  expect(details.queryByText("Cache writes")).toBeNull()
  expect(details.queryByText("Subagents")).toBeNull()
})

test("Claude details show main and subagent accounting including a reported zero", async () => {
  render(
    session({
      context_tokens: 70_000,
      context_level: "green",
      usage: {
        total_tokens: 300_000,
        scope: "conversation",
        main_tokens: 300_000,
        subagent_tokens: 0,
        totals: { input_tokens: 0, output_tokens: 50_000 },
      },
    }),
  )
  fireEvent.click(badge()!)
  const details = within(await screen.findByRole("dialog", { name: "Context usage" }))
  expect(details.getByText("Conversation, including subagents")).toBeTruthy()
  expect(details.getByText("Main agent")).toBeTruthy()
  expect(details.getByText("Subagents")).toBeTruthy()
  expect(details.getByText("Uncached input")).toBeTruthy()
  expect(details.queryByText("First request")).toBeNull()
  expect(details.queryByText("Reported model capacity")).toBeNull()
})

test("each level maps to its own colour", () => {
  const tone = (level: "green" | "yellow" | "orange" | "red") => {
    render(session({ context_tokens: 1000, context_level: level }))
    const t = document.querySelector("[data-tone]")!.getAttribute("data-tone")
    cleanup()
    return t
  }
  const tones = (["green", "yellow", "orange", "red"] as const).map(tone)
  expect(tones).toEqual(["done", "waiting", "paused", "deadlocked"])
})

test("the ring fills with the count and is full at or above the ceiling", () => {
  render(session({ context_tokens: 50_000, context_level: "green" }))
  const quarter = fill()
  cleanup()
  render(session({ context_tokens: 100_000, context_level: "green" }))
  expect(fill()).toBeGreaterThan(quarter)
  cleanup()
  render(session({ context_tokens: 250_000, context_level: "red" }))
  expect(fill()).toBe(1)
})

test("no ceiling, no badge", () => {
  render(session({ context_tokens: 70_000, context_level: "green" }), { ceiling: undefined })
  expect(badge()).toBeNull()
})

test("while a switch runs, the footer shows the target model and effort with a switching hint", () => {
  render(session({ switching_model: "claude-opus-4-8", switching_effort: "max" }))
  const footer = screen.getByTestId("footer-line")
  expect(footer.textContent).toContain("Claude Opus 4.8 · max")
  expect(footer.textContent).not.toContain("Opus 4.7")
  expect(screen.getByTestId("switching-hint").textContent).toContain("switching")
})

test("a switch back shows the old model with the hint", () => {
  render(session({ switching_model: "claude-opus-4-6", switching_back: true }))
  expect(screen.getByTestId("footer-line").textContent).toContain("Claude Opus 4.6")
  expect(screen.getByTestId("switching-hint")).toBeTruthy()
})

test("no hint when no switch runs", () => {
  render(session())
  expect(screen.queryByTestId("switching-hint")).toBeNull()
})

// The same tree as renderThemed, so a rerender keeps the component's state.
const themed = (ui: ReactElement) => (
  <ChakraProvider value={system}>
    {ui}
    <AppToaster />
  </ChakraProvider>
)
const lineText = () => screen.getByTestId("switch-line").textContent

test("the log line follows the phase: switching, then switched", () => {
  const { rerender } = renderThemed(
    <SwitchLine session={session({ switching_model: "claude-opus-4-8" })} />,
  )
  expect(lineText()).toBe("Switching to Claude Opus 4.8…")
  rerender(themed(<SwitchLine session={session({ model: "claude-opus-4-8" })} />))
  expect(lineText()).toBe("Switched to Claude Opus 4.8.")
})

test("a rollback shows the switching back line, then the failed line", () => {
  const { rerender } = renderThemed(
    <SwitchLine session={session({ switching_model: "claude-opus-4-7", switching_back: true })} />,
  )
  expect(lineText()).toBe("Switching back to Claude Opus 4.7…")
  const failed = session({
    switch_failed_model: "claude-opus-4-8",
    switch_failed_old: "claude-opus-4-7",
  })
  rerender(themed(<SwitchLine session={failed} />))
  expect(lineText()).toBe("Switch to Claude Opus 4.8 failed. Still on Claude Opus 4.7.")
})

test("no line when nothing switched", () => {
  renderThemed(<SwitchLine session={session()} />)
  expect(screen.queryByTestId("switch-line")).toBeNull()
})

const outdated = (over: Partial<SessionInfo> = {}) =>
  session({ latest_model: "claude-opus-4-8", latest_effort: "high", ...over })
const popover = () => screen.findByRole("dialog", { name: "Newer model" })
const switchButton = (pop: HTMLElement) =>
  within(pop).getByRole("button", { name: /Switch to/ }) as HTMLButtonElement

test("an outdated model is a red button that opens the switch popover", async () => {
  render(outdated())
  fireEvent.click(screen.getByTestId("outdated-model"))
  const pop = await popover()
  expect(pop.textContent).toContain("A newer Opus model is available")
  expect(pop.textContent).toContain("Keeps effort high. Only this session changes.")
  expect(switchButton(pop).textContent).toBe("Switch to Claude Opus 4.8")
})

test("the switch button posts the latest model and the named effort", async () => {
  render(outdated({ latest_effort: "medium" }))
  fireEvent.click(screen.getByTestId("outdated-model"))
  const pop = await popover()
  expect(pop.textContent).toContain("Changes effort to medium")
  fireEvent.click(switchButton(pop))
  await waitFor(() => expect(posted).toHaveBeenCalledOnce())
  expect(posted.mock.calls[0][0]).toMatchObject({
    id: "s-a",
    model: "claude-opus-4-8",
    effort: "medium",
  })
})

test("an empty effort uses the default and keeps the button enabled", async () => {
  render(outdated({ latest_effort: "", effort: "" }))
  fireEvent.click(screen.getByTestId("outdated-model"))
  const pop = await popover()
  expect(pop.textContent).toContain("Uses the model's default effort.")
  expect(switchButton(pop).disabled).toBe(false)
})

test("a busy agent disables the button and a stopped one asks for a resume", async () => {
  const { unmount } = render(outdated({ status: { state: "running" } } as Partial<SessionInfo>))
  fireEvent.click(screen.getByTestId("outdated-model"))
  let pop = await popover()
  expect(pop.textContent).toContain("Wait until it is idle")
  expect(switchButton(pop).disabled).toBe(true)
  unmount()
  render(outdated({ stopped: true }))
  fireEvent.click(screen.getByTestId("outdated-model"))
  pop = await popover()
  expect(pop.textContent).toContain("Resume the session to switch.")
})

const WARNING =
  "Changing the model costs tokens. The whole chat is sent again at the new model's price."

test("the popover warns about the token cost only for a chat with history, and still switches", async () => {
  const { unmount } = render(outdated({ has_history: true }))
  fireEvent.click(screen.getByTestId("outdated-model"))
  const pop = await popover()
  expect(pop.textContent).toContain(WARNING)
  expect(switchButton(pop).disabled).toBe(false)
  unmount()
  render(outdated())
  fireEvent.click(screen.getByTestId("outdated-model"))
  expect((await popover()).textContent).not.toContain("costs tokens")
})

test("a chat popover links to the picker; a design popover does not", async () => {
  const { unmount } = render(outdated())
  fireEvent.click(screen.getByTestId("outdated-model"))
  fireEvent.click(within(await popover()).getByRole("button", { name: "Choose another model" }))
  expect(openPicker).toHaveBeenCalledOnce()
  unmount()
  render(outdated({ design: true }))
  fireEvent.click(screen.getByTestId("outdated-model"))
  expect(within(await popover()).queryByRole("button", { name: "Choose another model" })).toBeNull()
})

test("the popover names every matching tier and links to Models settings; no tier, no note", async () => {
  const { unmount } = render(outdated({ latest_tiers: ["worker", "light"], design: true }))
  fireEvent.click(screen.getByTestId("outdated-model"))
  const note = within(await popover()).getByTestId("tiers-note")
  expect(note.textContent).toContain("worker, light tiers")
  expect(note.textContent).toContain("This switch does not change Settings")
  expect(
    within(note).getByRole("link", { name: "Open Models settings" }).getAttribute("href"),
  ).toBe("#/settings/s/models")
  unmount()
  render(outdated())
  fireEvent.click(screen.getByTestId("outdated-model"))
  expect(within(await popover()).queryByTestId("tiers-note")).toBeNull()
})

test("a plain chat name opens the picker; a plain design name is text", () => {
  const { unmount } = render(session())
  fireEvent.click(screen.getByTestId("model-name-button"))
  expect(openPicker).toHaveBeenCalledOnce()
  unmount()
  render(session({ design: true }))
  expect(screen.queryByTestId("model-name-button")).toBeNull()
  expect(screen.queryByTestId("outdated-model")).toBeNull()
})
