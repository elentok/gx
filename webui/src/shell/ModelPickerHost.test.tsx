import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, test, vi } from "vitest"
import { session } from "../agents/agentsTestKit"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import type { SessionInfo, SessionModelRow, SessionModelsResponse } from "../generated/server"
import { renderThemed } from "../ui/render"
import { ModelPickerHost } from "./ModelPickerHost"
import { closeModelPicker, openModelPicker } from "./modelPickerOpen"
import { buildGroups, effortFor } from "./modelPickerList"

afterEach(() => {
  closeModelPicker()
  cleanup()
})

const row = (id: string, over: Partial<SessionModelRow> = {}): SessionModelRow => ({
  id,
  efforts: ["low", "medium", "high"],
  ...over,
})
const list = (...models: SessionModelRow[]): SessionModelsResponse => ({
  models,
  aliases: [],
  refreshing: false,
  default_model: "",
  default_effort: "",
})

const chat = (over: Partial<SessionInfo> = {}): SessionInfo => ({
  ...session("s1", "gamma"),
  harness: "codex",
  model: "gpt-5.5-sol",
  effort: "high",
  status: { state: "idle" },
  ...over,
})

const MODELS = list(
  row("gpt-5.5-sol", {
    current: true,
    family: "sol",
    version: { major: 5, minor: 5 },
    latest: "gpt-6.1-sol",
  }),
  row("gpt-6.1-sol", {
    family: "sol",
    version: { major: 6, minor: 1 },
    efforts: ["low", "high", "xhigh"],
    switch_effort: "high",
  }),
  row("gpt-6-luna", { family: "luna", version: { major: 6, minor: 0 } }),
  row("mystery"),
)

// The picker reads the live session from the connection; an empty snapshot keeps the opened one.
function renderHost() {
  const state = { snapshot: { sessions: [] }, connected: true } as unknown as ConnectionState
  const conn = {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: () => () => {},
  } as unknown as Connection
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <ModelPickerHost />
    </ConnectionContext.Provider>,
  )
}

function open(over: Partial<SessionInfo> = {}, models: SessionModelsResponse | null = MODELS) {
  const switchModel = vi.fn(async () => ({}))
  renderHost()
  act(() =>
    openModelPicker({
      session: chat(over),
      fetchModels: async () => models,
      switchModel,
    }),
  )
  return switchModel
}

const names = () => screen.getAllByRole("radio", { name: /GPT|mystery/i }).map((r) => r.textContent)
const switchButton = () => screen.getByRole("button", { name: "Switch" }) as HTMLButtonElement

describe("the list", () => {
  test("groups by family newest first, models newest first, Other last", async () => {
    open()
    await screen.findByRole("radio", { name: /GPT-6.1-Sol/ })
    expect(names()).toEqual([
      expect.stringContaining("GPT-6.1-Sol"),
      expect.stringContaining("GPT-5.5-Sol"),
      expect.stringContaining("GPT-6-Luna"),
      expect.stringContaining("mystery"),
    ])
  })

  test("tags the current and the outdated model, and selects the current one", async () => {
    open()
    await screen.findByRole("radio", { name: /GPT-6.1-Sol/ })
    const cur = screen.getByRole("radio", { name: /GPT-5.5-Sol/ })
    expect(cur.getAttribute("aria-checked")).toBe("true")
    expect(cur.textContent).toContain("current")
    expect(cur.textContent).toContain("newer available")
    expect(screen.getByRole("radio", { name: /mystery/ }).textContent).not.toContain("current")
  })

  test("a missing list shows only the current model", async () => {
    open({}, null)
    await screen.findByRole("radio", { name: /GPT-5.5-Sol/ })
    expect(screen.getAllByRole("radio", { name: /GPT/ })).toHaveLength(1)
  })

  test("a family orders by its newest version, ties alphabetical", () => {
    const groups = buildGroups(
      list(
        row("b", { family: "beta", version: { major: 2, minor: 0 } }),
        row("a", { family: "alpha", version: { major: 2, minor: 0 } }),
        row("c", { family: "gamma", version: { major: 3, minor: 0 } }),
      ),
      chat(),
    )
    expect(groups.map((g) => g.family)).toEqual(["gamma", "alpha", "beta", undefined])
  })
})

describe("filter and focus", () => {
  test("opens with the filter focused and typing filters fuzzily", async () => {
    open()
    const filter = await screen.findByRole("searchbox", { name: "Filter models" })
    expect(document.activeElement).toBe(filter)
    fireEvent.change(filter, { target: { value: "lna" } })
    expect(names()).toEqual([expect.stringContaining("GPT-6-Luna")])
  })
})

describe("effort", () => {
  test("starts on the session's effort and shows only the model's own efforts", async () => {
    open()
    await screen.findByRole("radio", { name: /GPT-6.1-Sol/ })
    expect(screen.getByRole("radio", { name: "high" }).getAttribute("aria-checked")).toBe("true")
    expect(screen.queryByRole("radio", { name: "xhigh" })).toBeNull()
  })

  test.each([
    ["listed wanted is kept", ["low", "high"], undefined, "high", "claude", "high"],
    ["route default wins", ["low", "xhigh"], "xhigh", "high", "claude", "xhigh"],
    [
      "Claude takes the nearest lower",
      ["low", "medium", "xhigh"],
      undefined,
      "high",
      "claude",
      "medium",
    ],
    [
      "Claude takes the lowest when none is lower",
      ["high", "xhigh"],
      undefined,
      "low",
      "claude",
      "high",
    ],
    [
      "Codex without a default takes the lowest",
      ["medium", "high"],
      undefined,
      "xhigh",
      "codex",
      "medium",
    ],
    ["empty wanted stays empty", ["low", "high"], "high", "", "claude", ""],
    ["no effort control", [], undefined, "high", "claude", ""],
  ])("effortFor: %s", (_n, efforts, defaultEffort, wanted, harness, want) => {
    const base = { id: "m", label: "m" as never, current: false, outdated: false }
    expect(effortFor({ ...base, efforts, defaultEffort }, wanted, harness)).toBe(want)
  })

  test("an empty session effort keeps Switch disabled and sends no effort until one is picked", async () => {
    const switchModel = open({ effort: "" })
    await screen.findByRole("radio", { name: /GPT-6.1-Sol/ })
    expect(switchButton().disabled).toBe(true)
    fireEvent.click(screen.getByRole("radio", { name: /GPT-6.1-Sol/ }))
    expect(switchButton().disabled).toBe(false)
    fireEvent.click(switchButton())
    await waitFor(() => expect(switchModel).toHaveBeenCalledOnce())
    expect(switchModel).toHaveBeenCalledExactlyOnceWith(
      expect.not.objectContaining({ effort: expect.anything() }),
    )
  })

  test("a running switch blocks the picker even when the state is idle", async () => {
    open({ switching_model: "gpt-5.5-sol", switching_back: true })
    await screen.findByRole("radio", { name: /GPT-6.1-Sol/ })
    expect(switchButton().disabled).toBe(true)
    expect(screen.getByRole("status").textContent).toContain("working")
  })

  test("a model with no effort control shows none", async () => {
    open({}, list(row("gpt-5.5-sol", { current: true, efforts: [] })))
    await screen.findByRole("radio", { name: /GPT-5.5-Sol/ })
    expect(screen.queryByText("Effort")).toBeNull()
  })
})

describe("switching", () => {
  test("Switch is disabled until the model or the effort differs", async () => {
    open()
    await screen.findByRole("radio", { name: /GPT-6.1-Sol/ })
    expect(switchButton().disabled).toBe(true)
    fireEvent.click(screen.getByRole("radio", { name: "medium" }))
    expect(switchButton().disabled).toBe(false)
    fireEvent.click(screen.getByRole("radio", { name: "high" }))
    expect(switchButton().disabled).toBe(true)
    fireEvent.click(screen.getByRole("radio", { name: /GPT-6.1-Sol/ }))
    expect(switchButton().disabled).toBe(false)
  })

  test("arrow keys move the model and the effort, Enter sends the full id and closes", async () => {
    const switchModel = open()
    const filter = await screen.findByPlaceholderText("Filter models…")
    fireEvent.keyDown(filter, { key: "ArrowUp" })
    fireEvent.keyDown(filter, { key: "ArrowLeft" })
    fireEvent.submit(filter.closest("form")!)
    await waitFor(() => expect(screen.queryByPlaceholderText("Filter models…")).toBeNull())
    expect(switchModel).toHaveBeenCalledExactlyOnceWith(
      expect.objectContaining({ id: "s1", model: "gpt-6.1-sol", effort: "low" }),
    )
  })

  test("Esc closes and sends nothing", async () => {
    const switchModel = open()
    const filter = await screen.findByPlaceholderText("Filter models…")
    fireEvent.keyDown(filter, { key: "Escape" })
    expect(screen.queryByPlaceholderText("Filter models…")).toBeNull()
    expect(switchModel).not.toHaveBeenCalled()
  })

  test("a refusal shows inside the open dialog", async () => {
    const switchModel = vi.fn(async () => ({ refused: true, message: "Not idle" }))
    renderHost()
    act(() => openModelPicker({ session: chat(), fetchModels: async () => MODELS, switchModel }))
    fireEvent.click(await screen.findByRole("radio", { name: /GPT-6.1-Sol/ }))
    fireEvent.click(switchButton())
    expect(await screen.findByText("Not idle")).toBeTruthy()
    expect(screen.getByPlaceholderText("Filter models…")).toBeTruthy()
  })
})

describe("token cost warning", () => {
  const WARNING =
    "Changing the model costs tokens. The whole chat is sent again at the new model's price."

  test("a chat with history shows it and the Switch button stays enabled", async () => {
    open({ has_history: true })
    expect(await screen.findByText(WARNING)).toBeTruthy()
    fireEvent.click(await screen.findByRole("radio", { name: /GPT-6.1-Sol/ }))
    expect(switchButton().disabled).toBe(false)
  })

  test("a chat with no turns shows nothing", async () => {
    open()
    await screen.findByRole("radio", { name: /GPT-6.1-Sol/ })
    expect(screen.queryByText(WARNING)).toBeNull()
  })
})

describe("not switchable now", () => {
  test("a working agent disables everything and says why", async () => {
    open({ status: { state: "working" } })
    await screen.findByRole("radio", { name: /GPT-6.1-Sol/ })
    expect(screen.getByText("The agent is working. Wait until it is idle to switch.")).toBeTruthy()
    expect((screen.getByPlaceholderText("Filter models…") as HTMLInputElement).disabled).toBe(true)
    expect((screen.getByRole("radio", { name: /GPT-6.1-Sol/ }) as HTMLButtonElement).disabled).toBe(
      true,
    )
    expect((screen.getByRole("radio", { name: "low" }) as HTMLButtonElement).disabled).toBe(true)
    expect(switchButton().disabled).toBe(true)
    expect(screen.getByRole("button", { name: "Close" })).toBeTruthy()
  })

  test("an exited session says to resume it", async () => {
    open({ status: { state: "exited" } })
    expect(await screen.findByText("Resume the session to switch.")).toBeTruthy()
  })
})
