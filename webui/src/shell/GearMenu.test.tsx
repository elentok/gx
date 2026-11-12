import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { CertContext, type CertContextValue } from "../auth/cert"
import { TEST_CERT } from "../auth/testCert"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import type { Workspace } from "../generated/server"
import { renderThemed } from "../ui/render"
import { type Device, DeviceContext } from "../useWide"
import { WriteError } from "../useWrite"
import { GearMenu } from "./GearMenu"
import { ShellContext } from "./ShellContext"

const showToast = vi.hoisted(() => vi.fn())
vi.mock("../ui/toaster", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../ui/toaster")>()),
  showToast,
}))

const install = vi.hoisted(() => ({
  state: { mode: "none", desktopCaption: false } as { mode: string; desktopCaption: boolean },
  promptInstall: vi.fn(async () => {}),
}))
vi.mock("../pwa/install", () => ({
  useInstall: () => ({ ...install.state, promptInstall: install.promptInstall }),
}))

const forceRefresh = vi.hoisted(() => vi.fn(async () => {}))
vi.mock("../pwa/forceRefresh", () => ({ forceRefresh }))

const registration = vi.hoisted(() => ({
  state: { state: "idle" } as { state: string; error?: string },
}))
vi.mock("../pwa/registerWorker", () => ({ useWorkerRegistration: () => registration.state }))

afterEach(() => {
  registration.state = { state: "idle" }
  forceRefresh.mockClear()
  install.state = { mode: "none", desktopCaption: false }
  install.promptInstall.mockClear()
  cleanup()
  showToast.mockClear()
  window.location.hash = ""
})

const research: Workspace = { name: "research", sort: "activity" } as Workspace

function setup(
  snapshot: Record<string, unknown> = {},
  props: Parameters<typeof GearMenu>[0] = {},
  closeSliding = () => {},
  connected = true,
  certValue: CertContextValue = {},
  device: Device = "desktop",
) {
  const state = { snapshot, connected, loadedOnce: true, tick: 0 } as unknown as ConnectionState
  const conn = {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: () => () => {},
  } as unknown as Connection
  const shell = {
    sidebarOpen: true,
    toggle: () => {},
    close: () => {},
    closeSliding,
    claimHeader: () => () => {},
  }
  renderThemed(
    <DeviceContext.Provider value={device}>
      <ConnectionContext.Provider value={conn}>
        <ShellContext.Provider value={shell}>
          <CertContext.Provider value={certValue}>
            <GearMenu {...props} />
          </CertContext.Provider>
        </ShellContext.Provider>
      </ConnectionContext.Provider>
    </DeviceContext.Provider>,
  )
  fireEvent.click(screen.getByRole("button", { name: "Menu" }))
}

test("lists AFK, Vi mode, Sort, Restart server and More settings in order, with no build or limits", async () => {
  setup({ build: "v1.2.3" }, { activeWorkspace: research })
  await screen.findByRole("menuitemcheckbox", { name: "AFK" })
  const items = [...screen.getByRole("menu").querySelectorAll('[role^="menuitem"]')].map((el) =>
    el.textContent?.replace(/\s+/g, " ").trim(),
  )
  expect(items).toEqual([
    "AFK",
    "Vi mode",
    "Sort · research›",
    "Restart server",
    "More settings...",
    "Cleanup",
    "Tips",
    "Force refresh",
  ])
  expect(screen.queryByTestId("server-build")).toBeNull()
  expect(screen.queryByText("Claude limits")).toBeNull()
})

test("one divider separates AFK, Vi mode and Sort from Restart server and More settings", async () => {
  setup({}, { activeWorkspace: research })
  await screen.findByRole("menuitemcheckbox", { name: "AFK" })
  const menu = screen.getByRole("menu")
  const separators = menu.querySelectorAll('[role="separator"]')
  expect(separators).toHaveLength(2)
  const sort = screen.getByRole("menuitem", { name: /Sort · research/ })
  const restart = screen.getByRole("menuitem", { name: "Restart server" })
  expect(
    sort.compareDocumentPosition(separators[0]) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy()
  expect(
    restart.compareDocumentPosition(separators[0]) & Node.DOCUMENT_POSITION_PRECEDING,
  ).toBeTruthy()
})

test("the AFK and Vi mode items post the opposite of the snapshot", async () => {
  const postAfk = vi.fn(async () => {})
  const postViMode = vi.fn(async () => {})
  setup({ afk: true, vi_mode: false }, { postAfk, postViMode })
  const afk = await screen.findByRole("menuitemcheckbox", { name: "AFK" })
  expect(afk.getAttribute("aria-checked")).toBe("true")
  fireEvent.click(afk)
  await waitFor(() => expect(postAfk).toHaveBeenCalledWith(false))
  fireEvent.click(screen.getByRole("menuitemcheckbox", { name: "Vi mode" }))
  await waitFor(() => expect(postViMode).toHaveBeenCalledWith(true))
})

const menuItems = () =>
  [...screen.getByRole("menu").querySelectorAll('[role^="menuitem"]')].map((el) =>
    el.textContent?.replace(/\s+/g, " ").trim(),
  )

test("with a Dev checkout the Dev mode row sits under Vi mode, follows the Dev state and posts the opposite", async () => {
  const postDevFrontend = vi.fn()
  setup({ dev: { state: "running", has_checkout: true } }, { postDevFrontend })
  const row = await screen.findByRole("menuitemcheckbox", { name: "Dev mode" })
  expect(menuItems()?.slice(0, 3)).toEqual(["AFK", "Vi mode", "Dev mode"])
  expect(row.getAttribute("aria-checked")).toBe("true")
  fireEvent.click(row)
  await waitFor(() => expect(postDevFrontend).toHaveBeenCalledWith(false))
})

test("on a phone the Dev mode row keeps its place and the same drawn switch as Vi mode", async () => {
  setup(
    { vi_mode: true, dev: { state: "running", has_checkout: true } },
    {},
    () => {},
    true,
    {},
    "phone",
  )
  const row = await screen.findByRole("menuitemcheckbox", { name: "Dev mode" })
  expect(menuItems()?.slice(0, 3)).toEqual(["AFK", "Vi mode", "Dev mode"])
  const drawn = (el: Element) => [...el.querySelectorAll("[aria-hidden]")].map((n) => n.outerHTML)
  const vi = screen.getByRole("menuitemcheckbox", { name: "Vi mode" })
  expect(drawn(row).length).toBeGreaterThan(0)
  expect(drawn(row)).toEqual(drawn(vi))
})

test("an off Dev state shows the row unchecked and flipping it posts true", async () => {
  const postDevFrontend = vi.fn()
  setup({ dev: { state: "off", has_checkout: true } }, { postDevFrontend })
  const row = await screen.findByRole("menuitemcheckbox", { name: "Dev mode" })
  expect(row.getAttribute("aria-checked")).toBe("false")
  fireEvent.click(row)
  await waitFor(() => expect(postDevFrontend).toHaveBeenCalledWith(true))
})

test("without a Dev checkout there is no Dev mode row", async () => {
  setup({ dev: { state: "off", has_checkout: false } })
  await screen.findByRole("menuitemcheckbox", { name: "AFK" })
  expect(screen.queryByRole("menuitemcheckbox", { name: "Dev mode" })).toBeNull()
})

test("Sort is disabled until the active workspace resolves", async () => {
  setup()
  const sort = await screen.findByRole("menuitem", { name: "Sort" })
  expect(sort.getAttribute("aria-disabled")).toBe("true")
})

test("the Sort submenu offers three radios with the stored value checked, and a pick posts it for the active workspace", async () => {
  const postSort = vi.fn(async () => {})
  setup({}, { activeWorkspace: research, postSort })
  fireEvent.click(await screen.findByRole("menuitem", { name: /Sort · research/ }))
  const radios = await screen.findAllByRole("menuitemradio")
  expect(radios.map((r) => [r.textContent, r.getAttribute("aria-checked")])).toEqual([
    ["Needs you first, then by name", "false"],
    ["By last activity, newest first", "true"],
    ["Alphabetically by name", "false"],
    ["By project, then by name", "false"],
    ["By status, then by name", "false"],
  ])
  fireEvent.click(radios[2])
  await waitFor(() => expect(postSort).toHaveBeenCalledWith("research", "name"))
})

test("a refused sort write shows the toast and the radio stays on the stored value", async () => {
  const postSort = vi.fn(async () => {
    throw new WriteError("no such workspace")
  })
  setup({}, { activeWorkspace: research, postSort })
  fireEvent.click(await screen.findByRole("menuitem", { name: /Sort · research/ }))
  fireEvent.click((await screen.findAllByRole("menuitemradio"))[0])
  await waitFor(() =>
    expect(showToast).toHaveBeenCalledWith({ type: "error", title: "no such workspace" }),
  )
  await waitFor(() => expect(screen.queryByRole("menu")).toBeNull())
  fireEvent.click(screen.getByRole("button", { name: "Menu" }))
  fireEvent.click(await screen.findByRole("menuitem", { name: /Sort · research/ }))
  const checked = (await screen.findAllByRole("menuitemradio")).filter(
    (r) => r.getAttribute("aria-checked") === "true",
  )
  expect(checked.map((r) => r.textContent)).toEqual(["By last activity, newest first"])
})

test("More settings... opens Settings and closes a sliding sidebar", async () => {
  const closeSliding = vi.fn()
  setup({}, {}, closeSliding)
  fireEvent.click(await screen.findByRole("menuitem", { name: "More settings..." }))
  expect(window.location.hash).toBe("#/settings")
  expect(closeSliding).toHaveBeenCalled()
  await waitFor(() => expect(screen.queryByRole("menu")).toBeNull())
})

test("Cleanup opens the Cleanup page and closes a sliding sidebar", async () => {
  const closeSliding = vi.fn()
  setup({}, {}, closeSliding)
  fireEvent.click(await screen.findByRole("menuitem", { name: "Cleanup" }))
  expect(window.location.hash).toBe("#/cleanup")
  expect(closeSliding).toHaveBeenCalled()
})

test("Tips opens the Tips page and closes a sliding sidebar", async () => {
  const closeSliding = vi.fn()
  setup({}, {}, closeSliding)
  fireEvent.click(await screen.findByRole("menuitem", { name: "Tips" }))
  expect(window.location.hash).toBe("#/tips")
  expect(closeSliding).toHaveBeenCalled()
})

test("Restart server opens the restart dialog", async () => {
  setup()
  fireEvent.click(await screen.findByRole("menuitem", { name: "Restart server" }))
  expect(await screen.findByRole("dialog")).toBeTruthy()
})

test("mode none shows no Install row", async () => {
  setup()
  await screen.findByRole("menuitem", { name: "Tips" })
  expect(screen.queryByRole("menuitem", { name: /Install app/ })).toBeNull()
})

test("Force refresh always shows, last, and a click hands the connected flag to forceRefresh", async () => {
  setup()
  fireEvent.click(await screen.findByRole("menuitem", { name: "Force refresh" }))
  expect(forceRefresh).toHaveBeenCalledWith(true)
})

test("Force refresh shows with standalone faked, and offline passes connected false", async () => {
  vi.stubGlobal("matchMedia", (q: string) => ({
    matches: q.includes("standalone"),
    media: q,
    addEventListener() {},
    removeEventListener() {},
  }))
  try {
    setup({}, {}, () => {}, false)
    fireEvent.click(await screen.findByRole("menuitem", { name: "Force refresh" }))
    expect(forceRefresh).toHaveBeenCalledWith(false)
  } finally {
    vi.unstubAllGlobals()
  }
})

test("prompt mode lists Install app above Force refresh under their own separator, with the desktop caption, and a click prompts", async () => {
  install.state = { mode: "prompt", desktopCaption: true }
  setup()
  const row = await screen.findByRole("menuitem", { name: /Install app/ })
  expect(row.textContent).toContain("Chrome / Edge")
  const menu = screen.getByRole("menu")
  expect(menu.querySelectorAll('[role="separator"]')).toHaveLength(2)
  const items = [...menu.querySelectorAll('[role^="menuitem"]')]
  expect(items[items.length - 2]).toBe(row)
  fireEvent.click(row)
  expect(install.promptInstall).toHaveBeenCalledTimes(1)
})

test("prompt mode without the desktop caption shows a bare Install app row", async () => {
  install.state = { mode: "prompt", desktopCaption: false }
  setup()
  expect((await screen.findByRole("menuitem", { name: /Install app/ })).textContent?.trim()).toBe(
    "Install app",
  )
})

test("ios mode opens the Share steps sheet and Done closes it", async () => {
  install.state = { mode: "ios", desktopCaption: false }
  setup()
  fireEvent.click(await screen.findByRole("menuitem", { name: /Install app/ }))
  const dialog = await screen.findByRole("dialog")
  expect(dialog.textContent).toContain("Share")
  expect(dialog.textContent).toContain("Add to Home Screen")
  expect(install.promptInstall).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole("button", { name: "Done" }))
  expect(screen.queryByRole("dialog")).toBeNull()
})

test("mac-safari mode shows File then Add to Dock", async () => {
  install.state = { mode: "mac-safari", desktopCaption: false }
  setup()
  fireEvent.click(await screen.findByRole("menuitem", { name: /Install app/ }))
  const text = (await screen.findByRole("dialog")).textContent ?? ""
  expect(text.indexOf("File")).toBeGreaterThan(-1)
  expect(text.indexOf("Add to Dock")).toBeGreaterThan(text.indexOf("File"))
})

test("a SecurityError registration failure replaces the Install row with a hint linking the certificate", async () => {
  install.state = { mode: "prompt", desktopCaption: false }
  registration.state = { state: "failed", error: "SecurityError" }
  setup({}, {}, () => {}, true, { cert: TEST_CERT })
  const link = await screen.findByRole("link", { name: "Install the certificate" })
  expect(link.getAttribute("href")).toBe("/gx-root.cer")
  expect(screen.queryByRole("menuitem", { name: /Install app/ })).toBeNull()
})

test("no registration failure shows no root-trust hint", async () => {
  setup({}, {}, () => {}, true, { cert: TEST_CERT })
  await screen.findByRole("menuitem", { name: "Tips" })
  expect(screen.queryByRole("link", { name: "Install the certificate" })).toBeNull()
})

test("another registration error shows no root-trust hint and keeps the Install row", async () => {
  install.state = { mode: "prompt", desktopCaption: false }
  registration.state = { state: "failed", error: "TypeError" }
  setup({}, {}, () => {}, true, { cert: TEST_CERT })
  await screen.findByRole("menuitem", { name: /Install app/ })
  expect(screen.queryByRole("link", { name: "Install the certificate" })).toBeNull()
})

test("the ios sheet text does not mention trust", async () => {
  install.state = { mode: "ios", desktopCaption: false }
  setup()
  fireEvent.click(await screen.findByRole("menuitem", { name: /Install app/ }))
  expect((await screen.findByRole("dialog")).textContent).not.toMatch(/trust/i)
})

test("the Menu trigger is three vertical dots", () => {
  setup()
  const svg = screen.getByRole("button", { name: "Menu", hidden: true }).querySelector("svg")!
  expect(svg.querySelectorAll("circle")).toHaveLength(3)
  expect(svg.querySelector("path")).toBeNull()
})
