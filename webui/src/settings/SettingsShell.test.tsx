import { act, fireEvent, waitFor, within } from "@testing-library/react"
import { useState } from "react"
import { MemoryRouter, Route, Routes } from "react-router"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import type { SettingsEntry } from "../generated/server"
import { renderThemed } from "../ui/render"
import { DeviceContext } from "../useWide"
import { SettingsShellView } from "./SettingsShell"
import { treeFromEntries } from "../test-utils/settingsTree"

const push = vi.hoisted(() => ({ on: false }))
vi.mock("../push/deviceState", () => ({ usePushDevice: () => ({ on: push.on }) }))

const entry = (over: Partial<SettingsEntry>): SettingsEntry => ({
  key: "execution-queue.max-agents",
  path: ["execution-queue", "max-agents"],
  type: "number",
  section: "Execution queue",
  category: "Ralph loop",
  group: "Agent limits",
  description: "Live agents across every project.",
  set: false,
  ...over,
})

const entries = [
  entry({ set: true, value: 6, default: 4 }),
  entry({
    key: "execution-queue.poll",
    path: ["execution-queue", "poll"],
    description: "Poll.",
    set: true,
    value: "5s",
  }),
  entry({
    key: "budget.soft",
    path: ["budget", "soft"],
    section: "Budget",
    description: "Soft limit.",
    default: 100,
  }),
  entry({
    key: "scratch.thing",
    path: ["scratch", "thing"],
    section: "Scratch pad",
    category: "Experiments",
    description: "Scratch.",
    set: true,
    value: 1,
  }),
]

const renderShell = (section?: string, onSignOut = () => {}, totpOn = true) =>
  renderThemed(
    <MemoryRouter initialEntries={["/settings"]}>
      <Routes>
        <Route
          path="*"
          element={
            <SettingsShellView
              entries={entries}
              categories={treeFromEntries(entries)}
              projects={["gx", "ripple"]}
              section={section}
              totpOn={totpOn}
              onSignOut={onSignOut}
              onWrite={async () => ({})}
            />
          }
        />
      </Routes>
    </MemoryRouter>,
  )

describe("lagging alias note", () => {
  const model = (key: string, value: string) =>
    entry({
      key,
      path: key.split("."),
      section: "Models",
      type: "string",
      description: "Model.",
      set: true,
      value,
    })
  const aliases = [
    { alias: "fable", id: "claude-fable-5-1", latest: "claude-fable-5-2" },
    { alias: "opus", id: "claude-opus-5-5", latest: "claude-opus-5-5" },
  ]
  const fetchMock = vi.fn(async () => new Response(JSON.stringify({ aliases })))
  beforeEach(() => {
    fetchMock.mockClear()
    vi.stubGlobal("fetch", fetchMock)
  })
  afterEach(() => vi.unstubAllGlobals())

  const renderSection = (rows: SettingsEntry[], section: string) =>
    renderThemed(
      <MemoryRouter initialEntries={["/settings"]}>
        <SettingsShellView
          entries={rows}
          categories={treeFromEntries(rows)}
          projects={[]}
          section={section}
          totpOn
          onSignOut={() => {}}
          onWrite={async () => ({})}
        />
      </MemoryRouter>,
    )
  const notes = async (rows: SettingsEntry[]) => {
    const { queryAllByTestId } = renderSection(rows, "Models")
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    await act(async () => {
      await fetchMock.mock.results[0]?.value
    })
    return queryAllByTestId("alias-note").map((n) => n.textContent)
  }

  it("notes a Claude alias that resolves below the family's latest", async () => {
    const { findAllByTestId } = renderSection([model("agents.claude.model", "fable")], "Models")
    expect((await findAllByTestId("alias-note")).map((n) => n.textContent)).toEqual([
      "fable resolves to claude-fable-5-1. claude-fable-5-2 is newer. Enter the full id to use it.",
    ])
  })

  it("notes the alias under a Models table, which has no row to carry it", async () => {
    const rows = [
      {
        ...model("agents.claude.model", "fable"),
        group: "Claude",
        tier: "standard",
        column: "model",
      } as SettingsEntry,
    ]
    const categories = treeFromEntries(rows)
    for (const g of categories[0].sections[0].groups) g.table = true
    const { container, findAllByTestId } = renderThemed(
      <MemoryRouter initialEntries={["/settings"]}>
        <SettingsShellView
          entries={rows}
          categories={categories}
          projects={[]}
          section="Models"
          totpOn
          onSignOut={() => {}}
          onWrite={async () => ({})}
        />
      </MemoryRouter>,
    )
    expect(container.querySelector("table")).not.toBeNull()
    expect((await findAllByTestId("alias-note")).map((n) => n.textContent)).toEqual([
      "fable resolves to claude-fable-5-1. claude-fable-5-2 is newer. Enter the full id to use it.",
    ])
  })

  it("shows no note for a full id, a Codex model or an alias at the latest", async () => {
    expect(
      await notes([
        model("agents.claude.model", "claude-fable-5-1"),
        model("agents.claude.light.model", "opus"),
        model("agents.codex.model", "fable"),
      ]),
    ).toEqual([])
  })

  it("makes no models request outside the Models section", async () => {
    const { findByText } = renderSection(entries, "Budget")
    await findByText("Soft")
    expect(fetchMock).not.toHaveBeenCalled()
  })
})

const Harness = () => {
  const [section, setSection] = useState("Budget")
  return (
    <>
      <button type="button" onClick={() => setSection("Scratch pad")}>
        go
      </button>
      <SettingsShellView
        entries={entries}
        categories={treeFromEntries(entries)}
        projects={[]}
        section={section}
        totpOn
        onSignOut={() => {}}
        onWrite={async () => ({})}
      />
    </>
  )
}

describe("SettingsShellView", () => {
  it("keeps the sidebar at tablet width after a section click, in a 200px column", () => {
    const { getByRole } = renderThemed(
      <DeviceContext.Provider value="tablet">
        <MemoryRouter initialEntries={["/settings"]}>
          <Routes>
            <Route
              path="*"
              element={
                <SettingsShellView
                  entries={entries}
                  categories={treeFromEntries(entries)}
                  projects={["gx"]}
                  totpOn
                  onSignOut={() => {}}
                  onWrite={async () => ({})}
                />
              }
            />
          </Routes>
        </MemoryRouter>
      </DeviceContext.Provider>,
    )
    const nav = getByRole("navigation")
    expect(getComputedStyle(nav.parentElement as HTMLElement).gridTemplateColumns).toContain(
      "200px",
    )
    fireEvent.click(getByRole("link", { name: /^Budget$/ }))
    expect(getByRole("navigation")).toBeTruthy()
  })

  it("builds the sidebar from the response: a section the web never named shows up", () => {
    const { getByRole } = renderShell("Scratch pad")
    const link = getByRole("link", { name: /^Scratch pad/ })
    expect(link.getAttribute("href")).toBe("/settings/s/scratch-pad")
  })

  it("lists the categories in response order, then Account", () => {
    const { getByRole } = renderShell()
    const names = Array.from(getByRole("navigation").querySelectorAll("button")).map((b) =>
      (b.textContent ?? "").replace(/[▾▸\d]/g, "").trim(),
    )
    expect(names).toEqual(["Ralph loop", "Experiments", "Account"])
  })

  it("starts only the current section's category open", () => {
    const { getByRole, queryByRole } = renderShell("Budget")
    expect(getByRole("link", { name: /^Budget$/ })).toBeTruthy()
    expect(queryByRole("link", { name: /^Scratch pad/ })).toBeNull()
    expect(queryByRole("link", { name: /Web logins/ })).toBeNull()
  })

  it("toggles a category on a heading click and keeps the choice", () => {
    const { getByRole, queryByRole } = renderShell("Budget")
    fireEvent.click(getByRole("button", { name: /^Experiments/ }))
    expect(getByRole("link", { name: /^Scratch pad/ })).toBeTruthy()
    fireEvent.click(getByRole("button", { name: /^Ralph loop/ }))
    expect(queryByRole("link", { name: /^Budget$/ })).toBeNull()
    expect(getByRole("link", { name: /^Scratch pad/ })).toBeTruthy()
  })

  it("opens the category of a section opened from a link", () => {
    const { getByRole, queryByRole } = renderThemed(
      <MemoryRouter>
        <Harness />
      </MemoryRouter>,
    )
    expect(queryByRole("link", { name: /^Scratch pad/ })).toBeNull()
    fireEvent.click(getByRole("button", { name: "go" }))
    expect(getByRole("link", { name: /^Scratch pad/ })).toBeTruthy()
  })

  it("sums the changed counts on a collapsed category and counts per section in an open one", () => {
    const { getByRole } = renderShell("Budget")
    expect(getByRole("link", { name: /^Execution queue\s*2$/ })).toBeTruthy()
    expect(getByRole("link", { name: /^Budget$/ })).toBeTruthy()
    expect(getByRole("button", { name: /^Experiments\s*1$/ })).toBeTruthy()
    fireEvent.click(getByRole("button", { name: /^Ralph loop/ }))
    expect(getByRole("button", { name: /^Ralph loop\s*2$/ })).toBeTruthy()
    fireEvent.click(getByRole("button", { name: /^Experiments/ }))
    expect(getByRole("button", { name: /^Experiments$/ })).toBeTruthy()
  })

  it("shows the category as a breadcrumb that is not a heading", () => {
    const { getByRole, getByText, queryByRole } = renderShell("Budget")
    expect(getByRole("heading", { name: "Budget" })).toBeTruthy()
    expect(getByText("Ralph loop", { selector: "p" })).toBeTruthy()
    expect(queryByRole("heading", { name: "Ralph loop" })).toBeNull()
  })

  it("shows the section's rows read-only: label, description, key path and value", () => {
    const { getByText, getByLabelText, queryByText } = renderShell("Execution queue")
    expect(getByText("Max agents")).toBeTruthy()
    expect(getByText("Live agents across every project.")).toBeTruthy()
    expect(getByText("execution-queue.max-agents")).toBeTruthy()
    expect((getByLabelText("Max agents") as HTMLInputElement).value).toBe("6")
    expect(queryByText("Soft limit.")).toBeNull()
  })

  it("shows the default for a key the file does not hold", () => {
    const { getByLabelText } = renderShell("Budget")
    expect((getByLabelText("Soft") as HTMLInputElement).value).toBe("100")
  })

  it("lists the scope picker's projects after Global settings", () => {
    const { getByLabelText } = renderShell()
    const options = Array.from(getByLabelText("Settings scope").querySelectorAll("option")).map(
      (o) => o.textContent,
    )
    expect(options).toEqual(["Global settings", "Project: gx", "Project: ripple"])
  })

  it("keeps the Account rows and their hrefs, and signs out", () => {
    const onSignOut = vi.fn()
    const { getByRole, getByText } = renderShell(undefined, onSignOut)
    fireEvent.click(getByRole("button", { name: /Account/ }))
    for (const [name, href] of [
      ["Web logins", "#/settings/logins"],
      ["Password", "#/settings/password"],
      ["Authenticator", "#/settings/authenticator"],
      ["Certificate", "#/settings/certificate"],
    ])
      expect(getByRole("link", { name: new RegExp(name) }).getAttribute("href")).toBe(href)
    fireEvent.click(getByText("Sign out of this device"))
    expect(onSignOut).toHaveBeenCalledOnce()
  })

  describe("search", () => {
    const type = (getByLabelText: (s: string) => HTMLElement, v: string) =>
      fireEvent.change(getByLabelText("Search settings"), { target: { value: v } })

    // The match is split into a <mark>, so compare whole-element text.
    const keyPath = (key: string) => (_: string, el: Element | null) =>
      el?.tagName === "P" && el.textContent === key

    it("finds a key by name, key path or description word", () => {
      const { getByLabelText, getByText, queryByText } = renderShell("Budget")
      type(getByLabelText, "max agents")
      expect(getByText(keyPath("execution-queue.max-agents"))).toBeTruthy()
      type(getByLabelText, "BUDGET.SOFT")
      expect(getByText(keyPath("budget.soft"))).toBeTruthy()
      expect(queryByText(keyPath("execution-queue.max-agents"))).toBeNull()
      type(getByLabelText, "scratch")
      expect(getByText(keyPath("scratch.thing"))).toBeTruthy()
    })

    it("replaces the page with a counted heading, groups hits by section and group, and clearing restores the section", () => {
      const { container, getByLabelText, getByText, queryByText, getByRole } = renderShell("Budget")
      type(getByLabelText, "o")
      expect(getByRole("heading", { name: '3 results for "o"' })).toBeTruthy()
      const page = within(container.querySelector("[data-mock-part=page]") as HTMLElement)
      expect(page.getByRole("heading", { level: 2, name: "Execution queue" })).toBeTruthy()
      expect(page.getByRole("heading", { level: 2, name: "Budget" })).toBeTruthy()
      expect(page.getAllByRole("heading", { level: 3, name: "Agent limits" })).toHaveLength(2)
      // The category is a plain-text breadcrumb, not a heading.
      expect(page.getAllByText("Ralph loop")).toHaveLength(2)
      expect(page.queryByRole("heading", { name: "Ralph loop" })).toBeNull()
      expect(page.getAllByTestId(/^result-section-/).map((s) => s.dataset.testid)).toEqual([
        "result-section-Execution queue",
        "result-section-Budget",
      ])
      type(getByLabelText, "")
      expect(getByRole("heading", { name: "Budget" })).toBeTruthy()
      expect(queryByText("execution-queue.poll")).toBeNull()
      expect(getByText("budget.soft")).toBeTruthy()
    })

    it("highlights the match in the row name and key path", () => {
      const { container, getByLabelText } = renderShell()
      type(getByLabelText, "soft")
      const marks = [...container.querySelectorAll("mark")].map((m) => m.textContent)
      expect(marks.length).toBeGreaterThanOrEqual(2)
      expect(new Set(marks.map((m) => m?.toLowerCase()))).toEqual(new Set(["soft"]))
    })

    it("shows hit counts in the sidebar, lists only categories with hits and restores it on clear", () => {
      const { container, getByLabelText, getByRole, queryByRole } = renderShell("Budget")
      const nav = within(container.querySelector("[data-mock-part=sidebar]") as HTMLElement)
      type(getByLabelText, "scratch")
      expect(queryByRole("button", { name: /Ralph loop/ })).toBeNull()
      const cat = getByRole("button", { name: /Experiments/ })
      expect(cat.getAttribute("aria-expanded")).toBe("true")
      expect(cat.textContent).toContain("1")
      expect(nav.getByRole("link", { name: /Scratch pad/ }).textContent).toContain("1")
      type(getByLabelText, "")
      expect(getByRole("button", { name: /Ralph loop/ })).toBeTruthy()
      expect(getByRole("button", { name: /Experiments/ }).getAttribute("aria-expanded")).toBe(
        "false",
      )
    })

    it("shows an empty state when nothing matches", () => {
      const { getByLabelText, getByText } = renderShell()
      type(getByLabelText, "zzz")
      expect(getByText("No settings match")).toBeTruthy()
    })
  })

  it("has no Authenticator row while TOTP is off", () => {
    const { getByRole, queryByRole } = renderShell(undefined, () => {}, false)
    fireEvent.click(getByRole("button", { name: /Account/ }))
    expect(getByRole("link", { name: /Certificate/ })).toBeTruthy()
    expect(queryByRole("link", { name: /Authenticator/ })).toBeNull()
  })
})

describe("SettingsShellView in a project", () => {
  const p = (key: string, section: string, over: Partial<SettingsEntry> = {}) =>
    entry({
      key,
      path: [key],
      section,
      group: over.identity ? "Repository" : section === "Project" ? "Delivery" : "Agent limits",
      description: `About ${key}.`,
      type: "string",
      ...over,
    })
  const projectEntries = [
    p("name", "Project", { read_only: true, identity: true, value: "gx", category: "Project" }),
    p("repo", "Project", {
      read_only: true,
      identity: true,
      label: "Repository",
      value: "/Users/david/dev/gx",
      category: "Project",
    }),
    p("vcs", "Project", {
      read_only: true,
      identity: true,
      label: "VCS",
      value: "git",
      category: "Project",
    }),
    p("trunk", "Project", { set: true, value: "main", category: "Project" }),
    p("landing", "Project", {
      type: "enum",
      enum: ["on-landed", "on-done"],
      set: true,
      value: "on-landed",
      category: "Project",
    }),
    p("auto-ff-merge", "Project", { type: "bool", value: false, category: "Project" }),
    p("project.max-agents", "Execution queue", { type: "number", set: true, value: 2 }),
    p("checks", "Checks", { type: "checks", category: "Checks" }),
  ]

  const renderProject = (
    section?: string,
    onWrite = vi.fn(async () => ({})),
    shown = projectEntries,
  ) => ({
    onWrite,
    ...renderThemed(
      <MemoryRouter initialEntries={["/settings"]}>
        <Routes>
          <Route
            path="*"
            element={
              <SettingsShellView
                entries={shown}
                categories={treeFromEntries(shown)}
                projects={["gx"]}
                project="gx"
                section={section}
                totpOn
                onSignOut={() => {}}
                onWrite={onWrite}
              />
            }
          />
        </Routes>
      </MemoryRouter>,
    ),
  })

  it("lists Project first, the categories holding a key, then Checks, and never Account", () => {
    const { getByRole, queryByRole } = renderProject()
    const names = Array.from(getByRole("navigation").querySelectorAll("button")).map((b) =>
      (b.textContent ?? "").replace(/[▾▸\d]/g, "").trim(),
    )
    expect(names).toEqual(["Project", "Ralph loop", "Checks"])
    expect(queryByRole("button", { name: /Account/ })).toBeNull()
  })

  it("counts the keys in project.json on the category chip and on the section in an open category", () => {
    const { getByRole } = renderProject("Project")
    expect(getByRole("link", { name: /^Project\s*2$/ })).toBeTruthy()
    expect(getByRole("button", { name: /Ralph loop\s*1$/ })).toBeTruthy()
  })

  it("shows no chip on the Repository group or the Project entry for set identity keys", () => {
    const shown = projectEntries
      .filter((e) => e.key !== "trunk" && e.key !== "landing")
      .map((e) => (e.identity ? { ...e, set: true } : e))
    const { getByRole, getByTestId } = renderProject("Project", undefined, shown)
    expect(getByRole("link", { name: /^Project$/ })).toBeTruthy()
    expect(getByTestId("group-Repository").textContent).not.toMatch(/\d+\s*$/)
  })

  it("shows the Repository group as read-only text with no control", () => {
    const { container, getByTestId } = renderProject("Project")
    const identity = getByTestId("group-Repository")
    expect(identity.textContent).toContain("/Users/david/dev/gx")
    expect(identity.querySelectorAll("input, button, select")).toHaveLength(0)
    expect(within(identity).getAllByText(/read-only/)).toHaveLength(3)
    expect(container.querySelector("[aria-label=Trunk]")).toBeTruthy()
  })

  it("sends project-scope changes for trunk, landing and auto-ff-merge", async () => {
    const { getByLabelText, getByRole, onWrite } = renderProject("Project")
    fireEvent.change(getByLabelText("Trunk"), { target: { value: "develop" } })
    fireEvent.click(getByRole("button", { name: "Save" }))
    await waitFor(() => expect(onWrite).toHaveBeenCalledWith([{ key: "trunk", value: "develop" }]))
    fireEvent.click(within(getByLabelText("Landing")).getByText("on-done"))
    await waitFor(() =>
      expect(onWrite).toHaveBeenCalledWith([{ key: "landing", value: "on-done" }]),
    )
    fireEvent.click(getByRole("switch", { name: "Auto ff merge" }))
    await waitFor(() =>
      expect(onWrite).toHaveBeenCalledWith([{ key: "auto-ff-merge", value: true }]),
    )
  })

  it("opens an empty placeholder for Checks", () => {
    const { getByRole, queryByLabelText } = renderProject("Checks")
    expect(getByRole("heading", { name: "Checks" })).toBeTruthy()
    expect(queryByLabelText("Trunk")).toBeNull()
  })
})
