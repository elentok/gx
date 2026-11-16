import { act, fireEvent, within } from "@testing-library/react"
import { MemoryRouter } from "react-router"
import { describe, expect, it, vi } from "vitest"
import { renderThemed } from "../ui/render"
import { SettingsShellView } from "./SettingsShell"
import { treeFromEntries } from "../test-utils/settingsTree"
import type { SettingsEntry, SettingsWriteResult } from "../generated/server"

describe("Models table", () => {
  const m = (agent: string, tier: string, column: string, over: Partial<SettingsEntry> = {}) => {
    const path = tier === "standard" ? ["agents", agent, column] : ["agents", agent, tier, column]
    return {
      key: path.join("."),
      path,
      type: "string",
      section: "Models",
      category: "Agents",
      group: agent === "claude" ? "Claude" : "Codex",
      tier,
      column,
      description: `${agent} ${tier} ${column}`,
      set: false,
      ...over,
    } as SettingsEntry
  }
  const models = [
    m("claude", "light", "model", { default: "haiku" }),
    m("claude", "light", "effort", { default: "" }),
    m("claude", "standard", "model", { set: true, value: "opus", default: "sonnet" }),
    m("claude", "standard", "effort", { default: "high" }),
    m("claude", "strongest", "model", { default: "opus" }),
    m("claude", "strongest", "effort", { default: "max" }),
    m("codex", "light", "model", { default: "mini" }),
    m("codex", "light", "effort", { set: true, value: "", default: "low" }),
  ]
  const render = (
    entries: SettingsEntry[],
    onWrite = vi.fn(async () => ({}) as SettingsWriteResult),
    tableGroups = ["Claude", "Codex"],
    project?: string,
  ) => {
    const categories = treeFromEntries(entries)
    for (const g of categories[0].sections[0].groups) g.table = tableGroups.includes(g.name)
    const view = renderThemed(
      <MemoryRouter initialEntries={["/settings"]}>
        <SettingsShellView
          entries={entries}
          categories={categories}
          projects={[]}
          project={project}
          section="Models"
          totpOn
          onSignOut={() => {}}
          onWrite={onWrite}
        />
      </MemoryRouter>,
    )
    return { ...view, onWrite }
  }
  const cell = (key: string) => document.querySelector(`[data-cell="${key}"]`) as HTMLElement

  it("renders a table group as rows Light, Standard, Strongest with the key pattern", () => {
    const { getByTestId } = render(models)
    const claude = getByTestId("group-Claude")
    expect(within(claude).getByText("Light")).toBeTruthy()
    expect(within(claude).getByText("Standard")).toBeTruthy()
    expect(within(claude).getByText("Strongest")).toBeTruthy()
    expect(within(claude).getByText("Model")).toBeTruthy()
    expect(within(claude).getByText("Effort")).toBeTruthy()
    expect(within(claude).getByText("agents.claude.light.*")).toBeTruthy()
    expect(within(claude).getByText("agents.claude.*")).toBeTruthy()
    expect(within(claude).getByText("agents.claude.strongest.*")).toBeTruthy()
    expect(getByTestId("group-count-Claude").textContent).toBe("1")
    expect(getByTestId("group-count-Codex").textContent).toBe("1")
  })

  it("a group the registry does not mark as a table renders as rows", () => {
    const { getByTestId, queryByText } = render(models, undefined, ["Codex"])
    expect(queryByText("agents.claude.light.*")).toBeNull()
    expect(getByTestId("group-Claude").textContent).toContain("claude light model")
  })

  it("shows a default plainly and an empty effective value as faint Claude default", () => {
    render(models)
    const missing = cell("agents.claude.light.model")
    expect(missing.textContent).toContain("haiku")
    expect(missing.getAttribute("data-set")).toBeNull()
    const empty = cell("agents.claude.light.effort")
    expect(empty.textContent).toContain("Claude default")
    expect(empty.getAttribute("data-set")).toBeNull()
    expect(within(empty).queryByRole("button", { name: /reset/i })).toBeNull()
  })

  it("a key set to an empty string shows Codex default with the amber edge and a reset", async () => {
    const { onWrite } = render(models)
    const set = cell("agents.codex.light.effort")
    expect(set.textContent).toContain("Codex default")
    expect(set.getAttribute("data-set")).toBe("true")
    fireEvent.click(within(set).getByRole("button", { name: /reset/i }))
    await act(async () => {})
    expect(onWrite).toHaveBeenCalledWith([{ key: "agents.codex.light.effort", remove: true }])
  })

  it("a click opens the editor; Enter saves and the cell closes", async () => {
    const { onWrite } = render(models)
    fireEvent.click(within(cell("agents.claude.light.model")).getByText("haiku"))
    const input = within(cell("agents.claude.light.model")).getByRole("textbox")
    fireEvent.change(input, { target: { value: "sonnet" } })
    fireEvent.keyDown(input, { key: "Enter" })
    await act(async () => {})
    expect(onWrite).toHaveBeenCalledWith([{ key: "agents.claude.light.model", value: "sonnet" }])
    expect(within(cell("agents.claude.light.model")).queryByRole("textbox")).toBeNull()
  })

  it("Escape and Cancel close the cell without writing", () => {
    const { onWrite } = render(models)
    fireEvent.click(within(cell("agents.claude.light.model")).getByText("haiku"))
    fireEvent.keyDown(within(cell("agents.claude.light.model")).getByRole("textbox"), {
      key: "Escape",
    })
    expect(within(cell("agents.claude.light.model")).queryByRole("textbox")).toBeNull()
    fireEvent.click(within(cell("agents.claude.strongest.model")).getByText("opus"))
    fireEvent.click(
      within(cell("agents.claude.strongest.model")).getByRole("button", { name: "Cancel" }),
    )
    expect(within(cell("agents.claude.strongest.model")).queryByRole("textbox")).toBeNull()
    expect(onWrite).not.toHaveBeenCalled()
  })

  it("saving an empty field writes an empty string", async () => {
    const { onWrite } = render(models)
    fireEvent.click(within(cell("agents.claude.model")).getByText("opus"))
    const input = within(cell("agents.claude.model")).getByRole("textbox")
    fireEvent.change(input, { target: { value: "" } })
    fireEvent.keyDown(input, { key: "Enter" })
    await act(async () => {})
    expect(onWrite).toHaveBeenCalledWith([{ key: "agents.claude.model", value: "" }])
  })

  it("a refused save keeps the cell open with the draft and the reason under it", async () => {
    const onWrite = vi.fn(
      async () =>
        ({
          refused: true,
          keys: ["agents.claude.light.model"],
          reason: "unknown model",
        }) as SettingsWriteResult,
    )
    const { getByText } = render(models, onWrite)
    fireEvent.click(within(cell("agents.claude.light.model")).getByText("haiku"))
    const input = within(cell("agents.claude.light.model")).getByRole("textbox") as HTMLInputElement
    fireEvent.change(input, { target: { value: "nope" } })
    fireEvent.click(within(cell("agents.claude.light.model")).getByRole("button", { name: "Save" }))
    await act(async () => {})
    expect(input.value).toBe("nope")
    expect(getByText("Not saved: unknown model")).toBeTruthy()
  })

  it("search shows Models keys as ordinary rows with the full key path", () => {
    const { getByLabelText, queryByText, getAllByText, container } = render(models)
    fireEvent.change(getByLabelText("Search settings"), { target: { value: "light" } })
    expect(queryByText("agents.claude.light.*")).toBeNull()
    // The match is split into a <mark>, so compare whole-element text.
    const keyPaths = getAllByText(
      (_, el) => el?.tagName === "P" && el.textContent === "agents.claude.light.model",
    )
    expect(keyPaths.length).toBeGreaterThan(0)
    expect(container.querySelector("table")).toBeNull()
  })

  describe("in a project", () => {
    const g = (over: Partial<SettingsEntry>) => ({ global_key: over.key, ...over })
    const projectModels = [
      m("claude", "light", "model", g({ key: "agents.claude.light.model", global_value: "haiku" })),
      m("claude", "light", "effort", g({ key: "agents.claude.light.effort", default: "" })),
      m(
        "claude",
        "standard",
        "model",
        g({ key: "agents.claude.model", set: true, value: "opus", global_value: "sonnet" }),
      ),
      m(
        "claude",
        "standard",
        "effort",
        g({ key: "agents.claude.effort", set: true, value: "", global_value: "high" }),
      ),
      m(
        "claude",
        "strongest",
        "model",
        g({ key: "agents.claude.strongest.model", default: "opus" }),
      ),
    ]
    const renderProject = (onWrite = vi.fn(async () => ({}) as SettingsWriteResult)) =>
      render(projectModels, onWrite, ["Claude"], "gx")

    it("shows the override note under the title and no inherit rows or Override button", () => {
      const { getByText, queryByText } = renderProject()
      expect(
        getByText("Project overrides for gx. An empty value uses the global setting."),
      ).toBeTruthy()
      expect(queryByText(/Uses the global value/)).toBeNull()
      expect(queryByText(/^Override for/)).toBeNull()
    })

    it("does not show the note in the global view", () => {
      const { queryByText } = render(projectModels)
      expect(queryByText(/Project overrides for/)).toBeNull()
    })

    it("an unset cell shows the faint global effective value, falling back to Claude default", () => {
      renderProject()
      expect(cell("agents.claude.light.model").textContent).toContain("global: haiku")
      expect(cell("agents.claude.strongest.model").textContent).toContain("global: opus")
      expect(cell("agents.claude.light.effort").textContent).toContain("global: Claude default")
      expect(cell("agents.claude.light.model").getAttribute("data-set")).toBeNull()
    })

    it("a cell holding a value has the amber edge and a Use global value reset that removes the key", async () => {
      const { onWrite } = renderProject()
      const set = cell("agents.claude.model")
      expect(set.textContent).toContain("opus")
      expect(set.getAttribute("data-set")).toBe("true")
      fireEvent.click(within(set).getByTitle("Use global value"))
      await act(async () => {})
      expect(onWrite).toHaveBeenCalledWith([{ key: "agents.claude.model", remove: true }])
    })

    it("a cell set to an empty string shows the global value with the amber edge, counts, and resets", async () => {
      const { onWrite, getByTestId } = renderProject()
      const set = cell("agents.claude.effort")
      expect(set.textContent).toContain("global: high")
      expect(set.getAttribute("data-set")).toBe("true")
      expect(getByTestId("group-count-Claude").textContent).toBe("2")
      fireEvent.click(within(set).getByTitle("Use global value"))
      await act(async () => {})
      expect(onWrite).toHaveBeenCalledWith([{ key: "agents.claude.effort", remove: true }])
    })

    it("saving an empty field removes the key", async () => {
      const { onWrite } = renderProject()
      fireEvent.click(within(cell("agents.claude.model")).getByText("opus"))
      const input = within(cell("agents.claude.model")).getByRole("textbox")
      fireEvent.change(input, { target: { value: "" } })
      fireEvent.keyDown(input, { key: "Enter" })
      await act(async () => {})
      expect(onWrite).toHaveBeenCalledWith([{ key: "agents.claude.model", remove: true }])
    })

    it("Enter writes a value, Escape discards, and a refusal shows the reason", async () => {
      const { onWrite } = renderProject()
      const light = () => cell("agents.claude.light.model")
      fireEvent.click(within(light()).getByText("global: haiku"))
      fireEvent.keyDown(within(light()).getByRole("textbox"), { key: "Escape" })
      expect(within(light()).queryByRole("textbox")).toBeNull()
      expect(onWrite).not.toHaveBeenCalled()
      fireEvent.click(within(light()).getByText("global: haiku"))
      const input = within(light()).getByRole("textbox")
      fireEvent.change(input, { target: { value: "sonnet" } })
      fireEvent.keyDown(input, { key: "Enter" })
      await act(async () => {})
      expect(onWrite).toHaveBeenCalledWith([{ key: "agents.claude.light.model", value: "sonnet" }])
    })

    it("a refused save keeps the cell open with the reason", async () => {
      const refuse = vi.fn(
        async () =>
          ({
            refused: true,
            keys: ["agents.claude.strongest.model"],
            reason: "bad",
          }) as SettingsWriteResult,
      )
      const { getByText } = renderProject(refuse)
      const strongest = () => cell("agents.claude.strongest.model")
      fireEvent.click(within(strongest()).getByText("global: opus"))
      fireEvent.change(within(strongest()).getByRole("textbox"), { target: { value: "x" } })
      fireEvent.click(within(strongest()).getByRole("button", { name: "Save" }))
      await act(async () => {})
      expect(getByText("Not saved: bad")).toBeTruthy()
      expect(within(strongest()).getByRole("textbox")).toBeTruthy()
    })
  })
})
