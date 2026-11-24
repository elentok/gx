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
import { SettingsRow } from "./SettingsRow"
import { SettingsShellView } from "./SettingsShell"
import { treeFromEntries } from "../test-utils/settingsTree"
import type { SettingsEntry } from "../generated/server"
import type { WebLoginEntry } from "./logins"

const push = vi.hoisted(() => ({ on: false }))
const refreshPushDevice = vi.hoisted(() => vi.fn(async () => {}))
vi.mock("../push/deviceState", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../push/deviceState")>()),
  usePushDevice: () => ({ on: push.on }),
  refreshPushDevice,
}))

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
      notification_kinds: [],
      notification_channels: [],
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

  it("requests the models route only on the Models section", async () => {
    const fetchMock = vi.fn(async (url: unknown) => {
      void url
      return new Response(JSON.stringify({ aliases: [], epics: [] }))
    })
    vi.stubGlobal("fetch", fetchMock)
    const modelsCalls = () =>
      fetchMock.mock.calls.filter(([u]) => String(u).includes("/v1/sessions/models"))
    try {
      const entries = ["Project", "Models"].map((section) => ({
        key: `k.${section}`,
        path: ["k", section],
        type: "string",
        section,
        category: section,
        group: "",
        description: `${section} row.`,
        set: false,
      }))
      const fetchSettings = async () => ({
        project: { name: "gx", repo_checks_file: false },
        entries,
        categories: treeFromEntries(entries),
        redirects: {},
        notification_kinds: [],
        notification_channels: [],
      })
      const project = renderAt("/settings/p/gx/s/project", fetchSettings)
      expect(await project.findByRole("heading", { name: "Project" })).toBeTruthy()
      expect(modelsCalls()).toHaveLength(0)
      project.unmount()
      const models = renderAt("/settings/p/gx/s/models", fetchSettings)
      expect(await models.findByRole("heading", { name: "Models" })).toBeTruthy()
      await waitFor(() => expect(modelsCalls()).toHaveLength(1))
    } finally {
      vi.unstubAllGlobals()
    }
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
    return {
      entries,
      categories: treeFromEntries(entries),
      redirects: {},
      notification_kinds: [],
    } as never
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
      subscribe: (l: () => void) => {
        listeners.add(l)
        return () => listeners.delete(l)
      },
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
    act(() =>
      listeners.forEach((l) => {
        l()
      }),
    )
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

  it("shows Push on only for a login with a subscription", () => {
    const { getAllByText } = renderThemed(
      <WebLoginsView
        logins={[
          login({ id: "a", name: "With", push_on: true }),
          login({ id: "b", name: "Without" }),
        ]}
        now={NOW}
      />,
    )
    expect(getAllByText("Push on")).toHaveLength(1)
    expect(getAllByText("Push on")[0].closest("a")?.textContent).toContain("With")
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

  it("shows the Push row and Remove only for a login with a subscription", async () => {
    const pushed = [
      login({ id: "a", name: "Safari on iPhone", this_device: true }),
      login({
        id: "b",
        name: "Firefox on Linux",
        push_on: true,
        push_since: "2026-10-20T12:00:00Z",
      }),
    ]
    const withPush = renderAt("b", { fetchList: async () => pushed })
    expect(await withPush.findByText("On since Oct 20")).toBeTruthy()
    expect(withPush.getByText("Remove")).toBeTruthy()
    withPush.unmount()
    const without = renderAt("a", { fetchList: async () => pushed })
    await without.findByText("Sign out this device")
    expect(without.queryByText("Remove")).toBeNull()
    expect(without.queryByText(/On since/)).toBeNull()
  })

  it("removes that login's push, refetches and keeps the login", async () => {
    let list = [
      login({
        id: "b",
        name: "Firefox on Linux",
        push_on: true,
        push_since: "2026-10-20T12:00:00Z",
      }),
    ]
    const removePush = vi.fn(async () => {
      list = [login({ id: "b", name: "Firefox on Linux" })]
      return {}
    })
    const { findByText, queryByText, getByText } = renderAt("b", {
      removePush,
      fetchList: async () => list,
    })
    fireEvent.click(await findByText("Remove"))
    await waitFor(() => expect(removePush).toHaveBeenCalledWith("b"))
    await waitFor(() => expect(queryByText("Remove")).toBeNull())
    expect(queryByText(/On since/)).toBeNull()
    expect(getByText("Sign out this device")).toBeTruthy()
  })

  it("refreshes this device's push state when it removes its own", async () => {
    refreshPushDevice.mockClear()
    const removePush = vi.fn(async () => ({}))
    const own = [
      login({ id: "a", this_device: true, push_on: true, push_since: "2026-10-20T12:00:00Z" }),
    ]
    const { findByText } = renderAt("a", { removePush, fetchList: async () => own })
    fireEvent.click(await findByText("Remove"))
    await waitFor(() => expect(refreshPushDevice).toHaveBeenCalledOnce())
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

  it("shows an entry's notice under its row, and none for an entry without one", () => {
    const runner = e({
      key: "agent-runner",
      path: ["agent-runner"],
      type: "enum",
      enum: ["auto", "headless", "pty"],
      value: "auto",
      set: true,
      notice: "agent-runner: herdr was removed; using auto (PTY). Edit the config to drop it.",
    })
    const { getAllByRole } = render([runner, e({ description: "Live agents." })])
    const notes = getAllByRole("note")
    expect(notes).toHaveLength(1)
    expect(notes[0]?.textContent).toContain("herdr was removed")
  })

  it("keeps the title of a one-key group", () => {
    const { getByTestId } = render([e({ group: "Location" })])
    expect(getByTestId("group-Location").textContent).toContain("Location")
  })

  describe("Mental health section", () => {
    const m = (key: string, group: string, over: Partial<SettingsEntry> = {}): SettingsEntry =>
      e({
        key: `mental-health.${key}`,
        path: ["mental-health", ...key.split(".")],
        section: "Mental health",
        category: "Mental health",
        group,
        type: "string",
        ...over,
      })
    const entries = [
      m("enabled", "General", { type: "bool", value: true, set: true }),
      m("bedtime.start", "Bedtime", { value: "22:00", set: true }),
      m("bedtime.end", "Bedtime", { value: "07:00", set: true }),
      m("break.work-minutes", "Break", { type: "number", value: 50, set: true }),
    ]

    it("shows the General, Bedtime and Break groups and no unlock or Break counter", () => {
      const { getByTestId, queryByText } = render(entries, undefined, "Mental health")
      for (const g of ["General", "Bedtime", "Break"])
        expect(getByTestId(`group-${g}`)).toBeTruthy()
      expect(queryByText(/unlock/i)).toBeNull()
      expect(queryByText(/\d+\s+breaks?\b/i)).toBeNull()
    })

    it("shows the refusal of a save and keeps the draft", async () => {
      const onWrite = vi.fn(async () => ({
        refused: true,
        keys: ["mental-health.bedtime.start"],
        reason: "must differ from mental-health.bedtime.end",
      }))
      const { getByTestId, getByText, getAllByRole } = renderThemed(
        <MemoryRouter initialEntries={["/settings"]}>
          <SettingsShellView
            entries={entries}
            categories={treeFromEntries(entries)}
            projects={[]}
            section="Mental health"
            totpOn
            onSignOut={() => {}}
            onWrite={onWrite}
          />
        </MemoryRouter>,
      )
      const input = within(getByTestId("group-Bedtime")).getAllByRole(
        "textbox",
      )[0] as HTMLInputElement
      expect(getAllByRole("textbox").length).toBeGreaterThan(0)
      fireEvent.change(input, { target: { value: "07:00" } })
      fireEvent.keyDown(input, { key: "Enter" })
      await waitFor(() =>
        expect(getByText("Not saved: must differ from mental-health.bedtime.end")).toBeTruthy(),
      )
      expect(input.value).toBe("07:00")
    })
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

describe("Notifications section: channel switches and kind rows", () => {
  const auth = async () => ({ totp_on: false, setup_code_required: false }) as never
  const conn = {
    getState: () => "open",
    subscribe: () => () => {},
    onEvent: () => () => {},
  } as unknown as Connection
  const CHANNELS = [
    ["desktop", "Desktop"],
    ["web-toast", "Web toast"],
    ["telegram", "Telegram"],
    ["slack", "Slack"],
    ["push", "Push"],
  ] as const
  const kinds = [
    ...Array.from({ length: 10 }, (_, i) => ({
      id: `kind-${i}`,
      name: `Kind ${i}`,
      hint: i === 0 ? "A hint" : "",
      default: true,
    })),
    { id: "long-ticket", name: "Long ticket", hint: "many turns", default: false },
    ...Array.from({ length: 5 }, (_, i) => ({
      id: `kind-${10 + i}`,
      name: `Kind ${10 + i}`,
      hint: "",
      default: true,
    })),
  ]
  const defaults = kinds.filter((k) => k.default).map((k) => k.id)
  const entry = (key: string, type: string, def: unknown): SettingsEntry => ({
    key,
    path: key.split("."),
    type,
    section: "Notifications",
    category: "Notifications",
    group: "Notifications",
    description: "d",
    set: false,
    default: def,
  })
  const inSection = (e: SettingsEntry, name: string): SettingsEntry => ({
    ...e,
    section: name,
    group: name,
  })
  const snapshot = (over: Record<string, unknown> = {}) => {
    const entries = [
      ...CHANNELS.flatMap(([id]) => {
        const sw = entry(
          `notification-channels.${id}.enabled`,
          "bool",
          over[`${id}.enabled`] ?? true,
        )
        return [
          id === "push"
            ? {
                ...inSection(sw, "Push"),
                label: "Server-wide Push",
                description:
                  "Off here stops Push for every device. Device switches keep their state.",
              }
            : sw,
          entry(
            `notification-channels.${id}.kinds`,
            "string-list",
            over[`${id}.kinds`] ?? defaults,
          ),
        ]
      }),
      inSection(entry("notifications.telegram.chat-id", "string", ""), "Telegram"),
      {
        ...inSection(entry("server.web.push-contact-email", "secret", ""), "Push"),
        label: "Push contact email",
      },
    ]
    return {
      entries,
      categories: treeFromEntries(entries),
      redirects: {},
      notification_kinds: kinds,
      notification_channels: CHANNELS.map(([id, label]) => ({
        id,
        label,
        switch_key: `notification-channels.${id}.enabled`,
        list_key: `notification-channels.${id}.kinds`,
      })),
    } as never
  }
  const renderPage = (
    over: Record<string, unknown> = {},
    postChanges = vi.fn(async () => ({}) as never),
    at = "/settings/s/notifications",
    device?: "phone",
  ) => {
    const view = renderThemed(
      <ConnectionContext.Provider value={conn}>
        <DeviceContext.Provider value={device}>
          <MemoryRouter initialEntries={[at]}>
            <Routes>
              <Route
                path="/settings/s/:section"
                element={
                  <SettingsPage
                    fetchAuth={auth}
                    fetchSettings={async () => snapshot(over)}
                    postChanges={postChanges}
                  />
                }
              />
            </Routes>
          </MemoryRouter>
        </DeviceContext.Provider>
      </ConnectionContext.Provider>,
    )
    return { ...view, postChanges }
  }

  it("on a phone shows the Notifications drill-down page with the strip and the rows", async () => {
    const { findByTestId, container } = renderPage({}, undefined, undefined, "phone")
    await findByTestId("channel-switch-desktop")
    expect(container.querySelector('[data-mock-part="kind-rows"]')).not.toBeNull()
  })

  it("shows five switches in order, and 16 rows with five chips each", async () => {
    const { findByTestId, getByTestId, getAllByRole, queryAllByTestId, container } = renderPage()
    await findByTestId("channel-switch-desktop")
    const strip = container.querySelector('[data-mock-part="channel-switches"]') as HTMLElement
    expect(
      within(strip)
        .getAllByRole("switch")
        .map((s) => s.getAttribute("aria-label")),
    ).toEqual(CHANNELS.map(([, label]) => label))
    const rows = container.querySelector('[data-mock-part="kind-rows"]') as HTMLElement
    expect(rows.querySelectorAll('[data-testid^="kind-row-"]')).toHaveLength(16)
    const row = getByTestId("kind-row-kind-0")
    expect(
      within(row)
        .getAllByRole("button")
        .map((b) => b.textContent),
    ).toEqual(CHANNELS.map(([, label]) => label))
    expect(getAllByRole("switch")).toHaveLength(5)
    // The ten keys are not drawn a second time as generic rows.
    expect(queryAllByTestId("group-Notifications")).toHaveLength(0)
  })

  it("presses a chip when the kind is in the channel's list; Long ticket is off everywhere", async () => {
    const { findByTestId } = renderPage({ "slack.kinds": ["kind-0"] })
    const long = await findByTestId("kind-row-long-ticket")
    for (const b of within(long).getAllByRole("button"))
      expect(b.getAttribute("aria-pressed")).toBe("false")
    const first = within(await findByTestId("kind-row-kind-0")).getAllByRole("button")
    expect(first.map((b) => b.getAttribute("aria-pressed"))).toEqual([
      "true",
      "true",
      "true",
      "true",
      "true",
    ])
    const second = within(await findByTestId("kind-row-kind-1")).getAllByRole("button")
    expect(second.map((b) => b.getAttribute("aria-pressed"))).toEqual([
      "true",
      "true",
      "true",
      "false",
      "true",
    ])
  })

  it("a chip click writes the whole list for that channel", async () => {
    const { findByTestId, postChanges } = renderPage()
    const chip = within(await findByTestId("kind-row-long-ticket")).getByRole("button", {
      name: "Long ticket on Telegram",
    })
    fireEvent.click(chip)
    await waitFor(() => expect(postChanges).toHaveBeenCalled())
    expect(postChanges).toHaveBeenCalledWith("global", [
      {
        key: "notification-channels.telegram.kinds",
        value: [...defaults.slice(0, 10), "long-ticket", ...defaults.slice(10)],
      },
    ])
  })

  it("shows the reason of a refused write", async () => {
    const postChanges = vi.fn(async () => ({ refused: true, reason: "no good" }) as never)
    const { findByTestId, findByRole } = renderPage({}, postChanges)
    fireEvent.click(
      within(await findByTestId("kind-row-kind-0")).getByRole("button", { name: "Kind 0 on Push" }),
    )
    expect((await findByRole("alert")).textContent).toBe("Not saved: no good")
  })

  it("dims a channel that is off and keeps its chips checked and clickable", async () => {
    const { findByTestId, postChanges } = renderPage({ "telegram.enabled": false })
    const strip = await findByTestId("channel-switch-telegram")
    expect(within(strip).getByRole("switch").getAttribute("aria-checked")).toBe("false")
    const chip = within(await findByTestId("kind-row-kind-0")).getByRole("button", {
      name: "Kind 0 on Telegram",
    })
    expect(chip.getAttribute("aria-pressed")).toBe("true")
    expect(getComputedStyle(chip).opacity).toBe("0.4")
    fireEvent.click(chip)
    await waitFor(() => expect(postChanges).toHaveBeenCalled())
  })

  it("dims the body of an off channel's group block and leaves its inputs editable", async () => {
    const { findByTestId } = renderPage(
      { "telegram.enabled": false },
      undefined,
      "/settings/s/telegram",
    )
    const block = await findByTestId("group-Telegram")
    expect(block.getAttribute("data-dimmed")).toBe("true")
    expect(block.getAttribute("data-mock-part")).toBe("channel-settings")
    const input = within(block).getByRole("textbox") as HTMLInputElement
    expect(input.disabled).toBe(false)
  })

  it("does not dim the group block of a channel that is on", async () => {
    const { findByTestId } = renderPage({}, undefined, "/settings/s/telegram")
    expect((await findByTestId("group-Telegram")).getAttribute("data-dimmed")).toBeNull()
  })

  it("has no Web toast section while its strip switch exists", async () => {
    const { findByTestId, queryByRole } = renderPage()
    await findByTestId("channel-switch-web-toast")
    expect(queryByRole("link", { name: /Web toast/ })).toBeNull()
  })

  it("puts the Push switch row in the Push group beside the contact email, and posts its key", async () => {
    const { findByTestId, postChanges } = renderPage({}, undefined, "/settings/s/push")
    const block = await findByTestId("group-Push")
    expect(block.textContent).toContain("Off here stops Push for every device")
    const sw = within(block).getByRole("switch", { name: "Server-wide Push" })
    expect(sw.getAttribute("aria-checked")).toBe("true")
    expect(block.textContent).toContain("Push contact email")
    fireEvent.click(sw)
    await waitFor(() =>
      expect(postChanges).toHaveBeenCalledWith("global", [
        { key: "notification-channels.push.enabled", value: false },
      ]),
    )
  })

  it("draws the strip switch with BoolControl, the switch of a bool row", async () => {
    const { findByTestId } = renderPage()
    const strip = within(await findByTestId("channel-switch-desktop")).getByRole("switch")
    const row = renderThemed(
      <SettingsRow
        entry={entry("a.b", "bool", true)}
        pendingRestart={false}
        onWrite={async () => ({})}
      />,
    )
    expect(strip.className).toBe(within(row.container).getByRole("switch").className)
  })
})
