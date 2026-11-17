import { expect, test, vi } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import { ShellContext } from "../shell/ShellContext"
import { renderThemed } from "../ui/render"
import { DeviceContext } from "../useWide"
import { StateHeader } from "./StateHeader"

const state = {
  snapshot: { needs_you_total: 0 },
  connected: true,
  loadedOnce: true,
  tick: 0,
} as unknown as ConnectionState
const conn = {
  getState: () => state,
  subscribe: () => () => {},
  onEvent: () => () => {},
} as unknown as Connection

function shellWith(release: () => void) {
  const claimHeader = vi.fn(() => release)
  return {
    claimHeader,
    shell: {
      sidebarOpen: false,
      toggle: () => {},
      close: () => {},
      closeSliding: () => {},
      claimHeader,
    },
  }
}

test("inside the shell it claims once, releases on unmount and shows the title and one burger", () => {
  const release = vi.fn()
  const { claimHeader, shell } = shellWith(release)
  const { getAllByRole, getByRole, unmount } = renderThemed(
    <DeviceContext.Provider value="phone">
      <ConnectionContext.Provider value={conn}>
        <ShellContext.Provider value={shell}>
          <StateHeader title="Agents" />
        </ShellContext.Provider>
      </ConnectionContext.Provider>
    </DeviceContext.Provider>,
  )
  expect(getAllByRole("heading", { level: 1 })).toHaveLength(1)
  expect(getByRole("heading", { level: 1 }).textContent).toBe("Agents")
  expect(getAllByRole("button", { name: "Show sidebar" })).toHaveLength(1)
  expect(claimHeader).toHaveBeenCalledTimes(1)
  expect(release).not.toHaveBeenCalled()
  unmount()
  expect(release).toHaveBeenCalledTimes(1)
})

test("outside the shell it shows the heading and no burger", () => {
  const { getByRole, queryByRole } = renderThemed(
    <DeviceContext.Provider value="phone">
      <StateHeader title="Agents" />
    </DeviceContext.Provider>,
  )
  expect(getByRole("heading", { level: 1 }).textContent).toBe("Agents")
  expect(queryByRole("button", { name: "Show sidebar" })).toBeNull()
})
