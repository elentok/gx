import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, expect, test, vi } from "vitest"
import type { DevState, DevStatus } from "../generated/server"
import { renderThemed } from "../ui/render"
import { DeviceContext } from "../useWide"
import { DevPopover } from "./DevPopover"

const showToast = vi.hoisted(() => vi.fn())
vi.mock("../ui/toaster", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../ui/toaster")>()),
  showToast,
}))
const postSettings = vi.hoisted(() => vi.fn())
vi.mock("../settings/settingsApi", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../settings/settingsApi")>()),
  postSettings,
}))
const postJSON = vi.hoisted(() => vi.fn())
vi.mock("../auth/http", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../auth/http")>()),
  postJSON,
}))

beforeEach(() => {
  showToast.mockClear()
  postSettings.mockReset()
  postJSON.mockReset()
})
afterEach(cleanup)

const dev = (state: DevState, extra: Partial<DevStatus> = {}): DevStatus => ({
  state,
  has_checkout: true,
  reason: "because",
  log_tail: ["line one", "line two"],
  checkout: "/src/gx",
  ...extra,
})

async function open(
  d: DevStatus,
  props: { off?: (on: boolean) => void; restart?: () => void } = {},
) {
  renderThemed(<DevPopover dev={d} postDevFrontend={props.off} postDevRestart={props.restart} />)
  fireEvent.click(screen.getByRole("button"))
  await screen.findByText("Turn off Dev mode")
}

const cases: [DevState, string][] = [
  ["installing", "Installing dependencies"],
  ["starting", "Starting Vite"],
  ["running", "Vite is running"],
  ["down", "Vite is down"],
]

test.each(cases)("%s names the pill button with the status", (state, status) => {
  renderThemed(<DevPopover dev={dev(state)} />)
  expect(screen.getByRole("button", { name: `Dev mode: ${status}` })).toBeTruthy()
})

test.each(cases)("%s shows status, reason, log lines and path", async (state, status) => {
  await open(dev(state))
  expect(screen.getByTestId("dev-popover-status").textContent).toBe(status)
  expect(screen.getByText("because")).toBeTruthy()
  expect(screen.getByTestId("dev-popover-log").textContent).toBe("line oneline two")
  expect(screen.getByTestId("dev-popover-path").textContent).toContain("/src/gx")
})

test.each(cases)(
  "%s: Restart Vite only while down; Turn off always writes false",
  async (state) => {
    const off = vi.fn()
    const restart = vi.fn()
    await open(dev(state), { off, restart })
    expect(!!screen.queryByText("Restart Vite")).toBe(state === "down")
    if (state === "down") {
      fireEvent.click(screen.getByText("Restart Vite"))
      await waitFor(() => expect(restart).toHaveBeenCalledTimes(1))
    }
    fireEvent.click(screen.getByText("Turn off Dev mode"))
    await waitFor(() => expect(off).toHaveBeenCalledWith(false))
  },
)

test("a refused write shows the error toast", async () => {
  postSettings.mockResolvedValue({ refused: true, reason: "no write" })
  postJSON.mockResolvedValue({ refused: true, message: "Vite is not down." })
  await open(dev("down"))
  fireEvent.click(screen.getByText("Turn off Dev mode"))
  await waitFor(() => expect(showToast).toHaveBeenCalledWith({ type: "error", title: "no write" }))
  fireEvent.click(screen.getByText("Restart Vite"))
  await waitFor(() =>
    expect(showToast).toHaveBeenCalledWith({ type: "error", title: "Vite is not down." }),
  )
})

test("on a phone both buttons render and long log lines wrap instead of widening the popover", async () => {
  const long = "x".repeat(400)
  renderThemed(
    <DeviceContext.Provider value="phone">
      <DevPopover dev={dev("down", { log_tail: [long], checkout: `/${long}` })} />
    </DeviceContext.Provider>,
  )
  fireEvent.click(screen.getByRole("button"))
  await screen.findByText("Turn off Dev mode")
  expect(screen.getByText("Restart Vite")).toBeTruthy()
  const log = getComputedStyle(screen.getByTestId("dev-popover-log"))
  expect(log.whiteSpace).toBe("pre-wrap")
  expect(log.wordBreak).toBe("break-all")
  expect(log.overflow).toBe("hidden")
  expect(getComputedStyle(screen.getByTestId("dev-popover-path")).wordBreak).toBe("break-all")
})

test("renders nothing when off", () => {
  const { container } = renderThemed(<DevPopover dev={dev("off")} />)
  expect(container.querySelector("button")).toBeNull()
})
