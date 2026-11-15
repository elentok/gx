import { act, fireEvent, waitFor, within } from "@testing-library/react"
import type { ComponentProps } from "react"
import { MemoryRouter, Route, Routes } from "react-router"
import { describe, expect, it, vi } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import { renderThemed } from "../ui/render"
import { endsInDays } from "./logins"
import { DeviceContext } from "../useWide"
import {
  CertificateView,
  SettingsHomeView,
  SettingsPage,
  WebLoginPage,
  WebLoginsView,
} from "./SettingsPage"
import { SettingsShellView } from "./SettingsShell"
import { treeFromEntries } from "./testTree"
import type { SettingsEntry, SettingsWriteResult } from "../generated/server"
import type { WebLoginEntry } from "./logins"

const NOW = Date.parse("2026-10-30T12:00:00Z")
const DAY = 24 * 60 * 60 * 1000
const login = (over: Partial<WebLoginEntry> = {}): WebLoginEntry => ({
  id: "a",
  name: "Safari on iPhone",
  last_used: new Date(NOW - 60_000 * 30).toISOString(),
  last_ip: "192.168.1.23",
  this_device: false,
  ...over,
})
const usedDaysAgo = (days: number) => new Date(NOW - days * DAY).toISOString()

describe("SettingsPage project scope", () => {
  const auth = async () => ({ totp_on: false, setup_code_required: false }) as never
  const conn = {
    getState: () => "open",
    subscribe: () => () => {},
    onEvent: () => () => {},
  } as unknown as Connection
  const renderAt = (
    path: string,
    fetchSettings: ComponentProps<typeof SettingsPage>["fetchSettings"],
  ) =>
    renderThemed(
      <ConnectionContext.Provider value={conn}>
        <MemoryRouter initialEntries={[path]}>
          <Routes>
            <Route
              path="/settings/p/:project"
              element={<SettingsPage fetchAuth={auth} fetchSettings={fetchSettings} />}
            />
            <Route
              path="/settings/p/:project/s/:section"
              element={<SettingsPage fetchAuth={auth} fetchSettings={fetchSettings} />}
            />
          </Routes>
        </MemoryRouter>
      </ConnectionContext.Provider>,
    )

  it("shows a not-found state for a project the server does not know", async () => {
    const { findByText } = renderAt("/settings/p/nope", async () =>
      Promise.reject(new Error("404")),
    )
    expect(await findByText("Project not found")).toBeTruthy()
  })

  it("opens the first section of a known project", async () => {
    const entries = [
      {
        key: "trunk",
        path: ["trunk"],
        type: "string",
        section: "Project",
        category: "Project",
        group: "Repository",
        description: "Trunk.",
        set: false,
      },
    ]
    const { findByRole } = renderAt("/settings/p/gx", async () => ({
      project: { name: "gx", repo_checks_file: false },
      entries,
      categories: treeFromEntries(entries),
      redirects: {},
    }))
    expect(await findByRole("heading", { name: "Project" })).toBeTruthy()
  })

  describe("retired slugs", () => {
    const e = (section: string, category: string) => ({
      key: `${section}.k`,
      path: [section, "k"],
      type: "string",
      section,
      category,
      group: "",
      description: `${section} row.`,
      set: false,
    })
    const global = [e("Access", "Web UI"), e("Watchdogs", "System")]
    const redirects = { "web-ui": "access", "cpu-watchdog": "watchdogs" }
    const fetchTarget = async (target: string) => {
      const entries = target === "gx" ? [e("Project", "Project"), e("Access", "Web UI")] : global
      return { entries, categories: treeFromEntries(entries), redirects }
    }
    const conn = {
      getState: () => "open",
      subscribe: () => () => {},
      onEvent: () => () => {},
    } as unknown as Connection
    const at = (path: string) =>
      renderThemed(
        <ConnectionContext.Provider value={conn}>
          <MemoryRouter initialEntries={[path]}>
            <Routes>
              <Route
                path="/settings/s/:section"
                element={<SettingsPage fetchSettings={fetchTarget as never} fetchAuth={authOn} />}
              />
              <Route
                path="/settings/p/:project/s/:section"
                element={<SettingsPage fetchSettings={fetchTarget as never} fetchAuth={authOn} />}
              />
            </Routes>
          </MemoryRouter>
        </ConnectionContext.Provider>,
      )
    const authOn = async () => ({ totp_on: false, setup_code_required: false }) as never

    it("sends a retired global slug to the section that holds its keys", async () => {
      const { findByRole } = at("/settings/s/cpu-watchdog")
      expect(await findByRole("heading", { name: "Watchdogs" })).toBeTruthy()
    })

    it("sends a retired project slug to that section when the project has it", async () => {
      const { findByRole } = at("/settings/p/gx/s/web-ui")
      expect(await findByRole("heading", { name: "Access" })).toBeTruthy()
    })

    it("sends a retired project slug to the project's first section when it lacks the target", async () => {
      const { findByRole } = at("/settings/p/gx/s/cpu-watchdog")
      expect(await findByRole("heading", { name: "Project" })).toBeTruthy()
    })
  })
})

describe("SettingsPage on a phone", () => {
  const auth = async () => ({ totp_on: false, setup_code_required: false }) as never
  const conn = {
    getState: () => "open",
    subscribe: () => () => {},
    onEvent: () => () => {},
  } as unknown as Connection
  const entry = (section: string, key: string, set = false) => ({
    key,
    path: [key],
    type: "string",
    section,
    category: section === "Budget" ? "Ralph loop" : section,
    group: "",
    description: `${key} text.`,
    set,
  })
  const fetchSettings = async (target: string) => {
    const entries =
      target === "gx"
        ? [entry("Project", "trunk"), entry("Budget", "cap", true), entry("Checks", "checks")]
        : [entry("Budget", "soft")]
    return { entries, categories: treeFromEntries(entries), redirects: {} } as never
  }
  const fetchEpics = async () => ({ epics: [{ project: "gx" }] }) as never
  const renderAt = (path: string) =>
    renderThemed(
      <ConnectionContext.Provider value={conn}>
        <DeviceContext.Provider value="phone">
          <MemoryRouter initialEntries={[path]}>
            <Routes>
              {[
                "/settings",
                "/settings/s/:section",
                "/settings/p/:project",
                "/settings/p/:project/s/:section",
              ].map((p) => (
                <Route
                  key={p}
                  path={p}
                  element={
                    <SettingsPage
                      fetchAuth={auth}
                      fetchSettings={fetchSettings}
                      fetchEpics={fetchEpics}
                    />
                  }
                />
              ))}
            </Routes>
          </MemoryRouter>
        </DeviceContext.Provider>
      </ConnectionContext.Provider>,
    )

  it("shows the picker and search, then the global categories as headings with section tiles and Account", async () => {
    const { findByRole, getByRole, getByLabelText } = renderAt("/settings")
    expect(await findByRole("link", { name: /^Budget/ })).toBeTruthy()
    expect(getByLabelText("Settings scope")).toBeTruthy()
    expect(getByLabelText("Search settings")).toBeTruthy()
    expect(getByRole("heading", { name: "Ralph loop" })).toBeTruthy()
    expect(getByRole("heading", { name: "Account" })).toBeTruthy()
    expect(getByRole("link", { name: "Web logins" })).toBeTruthy()
    expect(getByRole("link", { name: "Password" })).toBeTruthy()
    expect(getByRole("link", { name: "Certificate" })).toBeTruthy()
    expect(getByRole("button", { name: "Sign out of this device" })).toBeTruthy()
  })

  it("has no Projects list; picking a project navigates to its home", async () => {
    const { findByRole, getByLabelText, queryByRole } = renderAt("/settings")
    await findByRole("link", { name: /^Budget/ })
    expect(queryByRole("link", { name: "gx" })).toBeNull()
    fireEvent.change(getByLabelText("Settings scope"), { target: { value: "gx" } })
    await waitFor(() =>
      expect((getByLabelText("Settings scope") as HTMLSelectElement).value).toBe("gx"),
    )
    expect(await findByRole("link", { name: "Project" })).toBeTruthy()
  })

  it("a project home shows its tree, Project first and Checks last, and no Account", async () => {
    const { findByRole, getByRole, getAllByRole, queryByRole } = renderAt("/settings/p/gx")
    await findByRole("link", { name: "Project" })
    const names = getAllByRole("link").map((l) => l.textContent)
    expect(names[0]).toBe("Project›")
    expect(names.at(-1)).toBe("Checks›")
    expect(getByRole("link", { name: /^Budget/ }).textContent).toContain("1")
    expect(getByRole("link", { name: "Checks" }).getAttribute("href")).toBe(
      "#/settings/p/gx/s/checks",
    )
    expect(queryByRole("heading", { name: "Account" })).toBeNull()
    expect(queryByRole("button", { name: "Sign out of this device" })).toBeNull()
  })

  it("opens a global section with a back arrow home and its category as plain text", async () => {
    const { findByRole, getByRole, getByTestId, queryByRole } = renderAt("/settings/s/budget")
    expect(await findByRole("heading", { name: "Budget" })).toBeTruthy()
    expect(getByRole("link", { name: "Back to Settings" }).getAttribute("href")).toBe("#/settings")
    expect(getByTestId("breadcrumb").textContent).toBe("Ralph loop")
    expect(queryByRole("heading", { name: "Ralph loop" })).toBeNull()
  })

  it("a project section's back arrow returns to the project home", async () => {
    const section = renderAt("/settings/p/gx/s/budget")
    expect(await section.findByRole("heading", { name: "Budget" })).toBeTruthy()
    expect(section.getByRole("link", { name: "Back to gx" }).getAttribute("href")).toBe(
      "#/settings/p/gx",
    )
  })

  it("searches only the picked scope, and a hit links to its section", async () => {
    const { findByRole, getByLabelText, getByRole, queryByRole, queryByText } =
      renderAt("/settings")
    await findByRole("link", { name: /^Budget/ })
    fireEvent.change(getByLabelText("Search settings"), { target: { value: "soft" } })
    expect(getByRole("link", { name: /soft/i }).getAttribute("href")).toBe("#/settings/s/budget")
    expect(queryByText(/Global ·/)).toBeNull()
    fireEvent.change(getByLabelText("Search settings"), { target: { value: "trunk" } })
    expect(queryByRole("link", { name: /trunk/i })).toBeNull()
  })

  it("on a project home the search lists that project's hits as links into the project", async () => {
    const { findByRole, getByLabelText, getByRole } = renderAt("/settings/p/gx")
    await findByRole("link", { name: "Project" })
    fireEvent.change(getByLabelText("Search settings"), { target: { value: "trunk" } })
    expect(getByRole("link", { name: /trunk/i }).getAttribute("href")).toBe(
      "#/settings/p/gx/s/project",
    )
  })
})

describe("SettingsPage Dev checkout", () => {
  const auth = async () => ({ totp_on: false, setup_code_required: false }) as never
  const snap = (has_checkout: boolean) =>
    ({ snapshot: { dev: { state: "off", has_checkout } } }) as unknown as ConnectionState

  it("enables the open dev-frontend row when a refetched snapshot has a Dev checkout, without a reload", async () => {
    let state = snap(false)
    const listeners = new Set<() => void>()
    const conn = {
      getState: () => state,
      subscribe: (l: () => void) => (listeners.add(l), () => listeners.delete(l)),
      onEvent: () => () => {},
    } as unknown as Connection
    const entries = [
      {
        key: "server.web.dev-frontend",
        path: ["server", "web", "dev-frontend"],
        type: "bool",
        section: "Server",
        category: "System",
        group: "Runtime",
        description: "Dev.",
        set: false,
        default: false,
      },
    ]
    const { findByRole } = renderThemed(
      <ConnectionContext.Provider value={conn}>
        <MemoryRouter initialEntries={["/settings/s/server"]}>
          <Routes>
            <Route
              path="/settings/s/:section"
              element={
                <SettingsPage
                  fetchAuth={auth}
                  fetchSettings={async () =>
                    ({ entries, categories: treeFromEntries(entries) }) as never
                  }
                />
              }
            />
          </Routes>
        </MemoryRouter>
      </ConnectionContext.Provider>,
    )
    expect(((await findByRole("switch")) as HTMLButtonElement).disabled).toBe(true)
    state = snap(true)
    act(() => listeners.forEach((l) => l()))
    await waitFor(() =>
      expect((document.querySelector("[role=switch]") as HTMLButtonElement).disabled).toBe(false),
    )
  })
})

describe("CertificateView", () => {
  it("shows both downloads and the fingerprint", () => {
    const cert = {
      fingerprint: "3F9A1C7E 52B0D488",
      profile_path: "/gx-root.mobileconfig",
      cer_path: "/gx-root.cer",
    }
    const { getByRole, getByText } = renderThemed(<CertificateView cert={cert} />)
    expect(getByRole("link", { name: /iPhone or iPad profile/ }).getAttribute("href")).toBe(
      "/gx-root.mobileconfig",
    )
    expect(getByRole("link", { name: /Mac certificate/ }).getAttribute("href")).toBe("/gx-root.cer")
    expect(getByText("3F9A1C7E")).toBeTruthy()
    expect(getByText("52B0D488")).toBeTruthy()
  })

  it("draws a divider between the two download rows only", () => {
    const cert = {
      fingerprint: "3F9A1C7E",
      profile_path: "/gx-root.mobileconfig",
      cer_path: "/gx-root.cer",
    }
    const { getByRole } = renderThemed(<CertificateView cert={cert} />)
    const top = (name: RegExp) => getComputedStyle(getByRole("link", { name })).borderTopWidth
    expect(top(/iPhone or iPad profile/)).not.toBe("1px")
    expect(top(/Mac certificate/)).toBe("1px")
    expect(getComputedStyle(getByRole("link", { name: /Mac certificate/ })).minHeight).toBe("52px")
  })
})

describe("SettingsHomeView", () => {
  it("lists the Security rows with the device count and signs out through the callback", () => {
    const onSignOut = vi.fn()
    const { getByRole, getByText } = renderThemed(
      <SettingsHomeView loginCount={4} totpOn onSignOut={onSignOut} />,
    )
    expect(getByRole("link", { name: /Web logins/ }).textContent).toContain("4 devices")
    expect(getByRole("link", { name: /Web logins/ }).getAttribute("href")).toBe("#/settings/logins")
    for (const name of ["Password", "Authenticator", "Certificate"])
      expect(getByRole("link", { name })).toBeTruthy()
    fireEvent.click(getByText("Sign out of this device"))
    expect(onSignOut).toHaveBeenCalledOnce()
  })

  it("lists the response's sections with a chevron and the changed count, and no project list", () => {
    const entries = [
      {
        key: "budget.soft",
        path: ["budget", "soft"],
        type: "number",
        section: "Budget",
        category: "Ralph loop",
        group: "Spend limits",
        description: "Soft.",
        set: true,
      },
    ]
    const { getByRole, queryByRole } = renderThemed(
      <SettingsHomeView
        totpOn
        entries={entries}
        categories={treeFromEntries(entries)}
        projects={["gx", "ripple"]}
        onSignOut={() => {}}
      />,
    )
    expect(getByRole("link", { name: /^Budget/ }).getAttribute("href")).toBe("#/settings/s/budget")
    expect(getByRole("link", { name: /^Budget/ }).textContent).toBe("Budget1›")
    expect(queryByRole("link", { name: "gx" })).toBeNull()
    expect(queryByRole("link", { name: "ripple" })).toBeNull()
  })

  it("searches the given scope only, grouped by section then group, and a hit opens its section", () => {
    const global = [
      {
        key: "budget.soft",
        path: ["budget", "soft"],
        type: "number",
        section: "Budget",
        category: "Ralph loop",
        group: "Spend limits",
        description: "Soft limit.",
        set: false,
      },
    ]
    const gx = [
      {
        key: "trunk",
        path: ["trunk"],
        type: "string",
        section: "Project",
        category: "Project",
        group: "Repository",
        description: "Trunk branch.",
        set: false,
      },
    ]
    const { getByLabelText, getByRole, queryByText, queryByRole, getByTestId, unmount } =
      renderThemed(
        <SettingsHomeView
          totpOn
          entries={global}
          categories={treeFromEntries(global)}
          projects={["gx"]}
          onSignOut={() => {}}
        />,
      )
    fireEvent.change(getByLabelText("Search settings"), { target: { value: "soft" } })
    expect(getByRole("link", { name: /Soft/ }).getAttribute("href")).toBe("#/settings/s/budget")
    expect(getByRole("heading", { name: "Budget" })).toBeTruthy()
    expect(getByRole("heading", { name: "Spend limits" })).toBeTruthy()
    expect(getByTestId("result-section-Budget").textContent).toContain("Ralph loop")
    expect(queryByText(/Global ·/)).toBeNull()
    expect(queryByText("Sign out of this device")).toBeNull()
    fireEvent.change(getByLabelText("Search settings"), { target: { value: "trunk" } })
    expect(queryByRole("link", { name: /Trunk/ })).toBeNull()
    unmount()
    const project = renderThemed(
      <SettingsHomeView
        totpOn
        project="gx"
        entries={gx}
        categories={treeFromEntries(gx)}
        projects={["gx"]}
        onSignOut={() => {}}
      />,
    )
    fireEvent.change(project.getByLabelText("Search settings"), { target: { value: "trunk" } })
    expect(project.getByRole("link", { name: /Trunk/ }).getAttribute("href")).toBe(
      "#/settings/p/gx/s/project",
    )
  })

  it("reads Add authenticator when TOTP is on and the saved secret was cleared", () => {
    const { getByRole, queryByRole } = renderThemed(
      <SettingsHomeView loginCount={1} totpOn authenticatorMissing onSignOut={() => {}} />,
    )
    expect(getByRole("link", { name: "Add authenticator" }).getAttribute("href")).toBe(
      "#/settings/authenticator",
    )
    expect(queryByRole("link", { name: "Reset authenticator" })).toBeNull()
  })

  it("has no Reset authenticator row while TOTP is off, and counts one device in the singular", () => {
    const { queryByRole, getByRole } = renderThemed(
      <SettingsHomeView loginCount={1} totpOn={false} onSignOut={() => {}} />,
    )
    expect(queryByRole("link", { name: "Reset authenticator" })).toBeNull()
    expect(getByRole("link", { name: /Web logins/ }).textContent).toContain("1 device")
    expect(getByRole("link", { name: /Web logins/ }).textContent).not.toContain("devices")
  })
})

describe("WebLoginsView", () => {
  it("shows the 90-day note, the back link, the name, last used, IP and the This device chip", () => {
    const { getByRole, getByText } = renderThemed(
      <WebLoginsView logins={[login({ this_device: true })]} now={NOW} />,
    )
    expect(getByRole("link", { name: "‹ Settings" }).getAttribute("href")).toBe("#/settings")
    expect(getByText("A web login ends after 90 days without use.")).toBeTruthy()
    expect(getByText("Safari on iPhone")).toBeTruthy()
    expect(getByText("30m ago · 192.168.1.23")).toBeTruthy()
    expect(getByText("This device")).toBeTruthy()
  })

  it("warns with 29 days left and not with 31", () => {
    const { queryByText } = renderThemed(
      <WebLoginsView
        logins={[
          login({ id: "a", name: "Soon", last_used: usedDaysAgo(61) }),
          login({ id: "b", name: "Later", last_used: usedDaysAgo(59) }),
        ]}
        now={NOW}
      />,
    )
    expect(queryByText("Ends in 29 days unless used")).toBeTruthy()
    expect(queryByText(/Ends in 31/)).toBeNull()
    expect(queryByText(/Ends in/)?.textContent).toBe("Ends in 29 days unless used")
  })

  it("lists this device first, then the other logins newest to oldest", () => {
    const { getAllByRole } = renderThemed(
      <WebLoginsView
        logins={[
          login({ id: "a", name: "Old", last_used: usedDaysAgo(3) }),
          login({ id: "b", name: "Mine", last_used: usedDaysAgo(20), this_device: true }),
          login({ id: "c", name: "New", last_used: usedDaysAgo(0.04) }),
        ]}
        now={NOW}
      />,
    )
    const names = getAllByRole("link")
      .map((a) => a.textContent ?? "")
      .filter((t) => /Old|Mine|New/.test(t))
      .map((t) => /Old|Mine|New/.exec(t)?.[0])
    expect(names).toEqual(["Mine", "New", "Old"])
  })

  it("counts the days left at the 30-day edge", () => {
    expect(endsInDays(usedDaysAgo(60), NOW)).toBeUndefined()
    expect(endsInDays(usedDaysAgo(61), NOW)).toBe(29)
  })
})

describe("WebLoginPage", () => {
  const state: ConnectionState = {
    snapshot: undefined as unknown as ConnectionState["snapshot"],
    connected: true,
    loadedOnce: true,
    liveSnapshot: true,
    tick: 0,
    streams: 0,
  }
  const conn = {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: () => () => {},
  } as unknown as Connection
  const entries = [
    login({ id: "a", name: "Safari on iPhone", this_device: true }),
    login({ id: "b", name: "Firefox on Linux" }),
  ]
  const renderAt = (id: string, props: Partial<ComponentProps<typeof WebLoginPage>> = {}) =>
    renderThemed(
      <ConnectionContext.Provider value={conn}>
        <MemoryRouter initialEntries={[`/settings/logins/${id}`]}>
          <Routes>
            <Route
              path="/settings/logins/:id"
              element={<WebLoginPage fetchList={async () => entries} now={() => NOW} {...props} />}
            />
          </Routes>
        </MemoryRouter>
      </ConnectionContext.Provider>,
    )

  it("renames on leaving the name field and refreshes the list", async () => {
    const rename = vi.fn(async () => ({}))
    const fetchList = vi.fn(async () => entries)
    const { findByDisplayValue } = renderAt("b", { rename, fetchList })
    const input = await findByDisplayValue("Firefox on Linux")
    fireEvent.change(input, { target: { value: "Work laptop" } })
    fireEvent.blur(input)
    await waitFor(() => expect(rename).toHaveBeenCalledWith("b", "Work laptop"))
    await waitFor(() => expect(fetchList).toHaveBeenCalledTimes(2))
  })

  it("revokes another web login only after the confirm step", async () => {
    const revoke = vi.fn(async () => ({}))
    const { findByText, getByText, queryByText } = renderAt("b", { revoke })
    fireEvent.click(await findByText("Sign out this device"))
    expect(revoke).not.toHaveBeenCalled()
    expect(getByText("It has to sign in again with the password and a code.")).toBeTruthy()
    fireEvent.click(getByText("Cancel"))
    expect(queryByText("Cancel")).toBeNull()
    fireEvent.click(getByText("Sign out this device"))
    fireEvent.click(getByText("Sign out"))
    await waitFor(() => expect(revoke).toHaveBeenCalledWith("b"))
  })

  it("signs this device out through the gate instead of revoking", async () => {
    const revoke = vi.fn(async () => ({}))
    const signOut = vi.fn()
    const { findByText, getByText } = renderAt("a", { revoke, signOut })
    fireEvent.click(await findByText("Sign out this device"))
    fireEvent.click(getByText("Sign out"))
    await waitFor(() => expect(signOut).toHaveBeenCalledOnce())
    expect(revoke).not.toHaveBeenCalled()
  })
})

describe("section page group blocks", () => {
  const e = (over: Partial<SettingsEntry>): SettingsEntry => ({
    key: "execution-queue.max-agents",
    path: ["execution-queue", "max-agents"],
    type: "number",
    section: "Execution queue",
    category: "Ralph loop",
    group: "Agent limits",
    description: "Live agents.",
    set: false,
    ...over,
  })
  const render = (entries: SettingsEntry[], project?: string, section = "Execution queue") =>
    renderThemed(
      <MemoryRouter initialEntries={["/settings"]}>
        <SettingsShellView
          entries={entries}
          categories={treeFromEntries(entries)}
          projects={[]}
          project={project}
          section={section}
          totpOn
          onSignOut={() => {}}
          onWrite={async () => ({})}
        />
      </MemoryRouter>,
    )

  it("titles each group in server order, with a chip counting set keys and none at zero", () => {
    const { getByTestId, queryByTestId, getAllByText } = render([
      e({ set: true, value: 3 }),
      e({ key: "execution-queue.b", path: ["execution-queue", "b"], set: true, value: 1 }),
      e({
        key: "execution-queue.wait",
        path: ["execution-queue", "wait"],
        group: "Waiting and stalled agents",
        description: "Wait.",
      }),
    ])
    expect(getByTestId("group-Agent limits").textContent).toContain("Agent limits")
    expect(getByTestId("group-count-Agent limits").textContent).toBe("2")
    expect(getByTestId("group-Waiting and stalled agents")).toBeTruthy()
    expect(queryByTestId("group-count-Waiting and stalled agents")).toBeNull()
    expect(getAllByText("Wait.")).toHaveLength(1)
  })

  it("keeps the title of a one-key group", () => {
    const { getByTestId } = render([e({ group: "Location" })])
    expect(getByTestId("group-Location").textContent).toContain("Location")
  })

  it("shows the Project section of a project as read-only Repository and Delivery groups", () => {
    const p = (key: string, group: string, over: Partial<SettingsEntry> = {}) =>
      e({ key, path: [key], section: "Project", category: "Project", group, ...over })
    const { getByTestId } = render(
      [
        p("name", "Repository", { identity: true, read_only: true, value: "gx" }),
        p("trunk", "Delivery", { value: "main" }),
      ],
      "gx",
      "Project",
    )
    expect(getByTestId("group-Repository").textContent).toContain("read-only")
    expect(getByTestId("group-Delivery")).toBeTruthy()
  })
})

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
