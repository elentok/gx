import { ChakraProvider } from "@chakra-ui/react"
import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, expect, test } from "vitest"
import { MemoryRouter, useLocation } from "react-router"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import type { FixAgent } from "../epic/fixAgent"
import type { MigrationFailure, RetryMigrationResult, SessionInfo } from "../generated/server"
import { DetailPage } from "../page/DetailPage"
import { AppShell } from "../shell/AppShell"
import { renderThemed } from "../ui/render"
import { system } from "../ui/theme"
import { toaster } from "../ui/toaster"
import { FailedMigrationStrip } from "./FailedMigrationStrip"

afterEach(() => {
  cleanup()
  toaster.remove()
})

const fail = (project: string, error: string): MigrationFailure => ({
  project,
  error,
  retry_command: "gx tickets migrate",
})

function setup(
  failures: MigrationFailure[],
  ui: (fix?: FixAgent) => React.ReactNode,
  fixAgent?: FixAgent,
) {
  let state = {
    snapshot: { migration_failures: failures, seq: 1 },
    connected: true,
    refresh: () => {},
  } as unknown as ConnectionState
  const subs = new Set<() => void>()
  const conn = {
    getState: () => state,
    subscribe: (fn: () => void) => (subs.add(fn), () => subs.delete(fn)),
    refresh: () => {},
    onEvent: () => () => {},
  } as unknown as Connection
  const { rerender } = renderThemed(
    <ConnectionContext.Provider value={conn}>{ui(fixAgent)}</ConnectionContext.Provider>,
  )
  return {
    rerender: (node: React.ReactNode) =>
      rerender(
        <ChakraProvider value={system}>
          <ConnectionContext.Provider value={conn}>{node}</ConnectionContext.Provider>
        </ChakraProvider>,
      ),
    set: (next: MigrationFailure[]) =>
      act(() => {
        state = {
          ...state,
          snapshot: { ...state.snapshot!, migration_failures: next },
        } as ConnectionState
        subs.forEach((fn) => fn())
      }),
  }
}

test("two failures render two rows with name, error and retry command", () => {
  setup([fail("gx", "bad schema"), fail("web", "disk full")], (f) => (
    <FailedMigrationStrip fixAgent={f} />
  ))
  expect(screen.getByText("gx: Migration blocked")).toBeTruthy()
  expect(screen.getByText("bad schema")).toBeTruthy()
  expect(screen.getByText("web: Migration blocked")).toBeTruthy()
  expect(screen.getByText("disk full")).toBeTruthy()
  expect(screen.getAllByText("gx tickets migrate")).toHaveLength(2)
  expect(screen.getAllByRole("button", { name: "Fix agent" })).toHaveLength(2)
  expect(screen.getAllByRole("button", { name: "Retry migration" })).toHaveLength(2)
})

test("Retry migration posts the project of its row", async () => {
  const calls: unknown[] = []
  setup([fail("gx", "bad schema"), fail("web", "disk full")], () => (
    <FailedMigrationStrip
      retry={async (body) => {
        calls.push(body)
        return { started: true }
      }}
    />
  ))
  fireEvent.click(screen.getAllByRole("button", { name: "Retry migration" })[1])
  await waitFor(() => expect(calls).toEqual([{ project: "web" }]))
})

test("Retry migration is busy while the call runs and refuses a second click", async () => {
  let release: (r: RetryMigrationResult) => void = () => {}
  let n = 0
  setup([fail("gx", "bad schema")], () => (
    <FailedMigrationStrip
      retry={() => {
        n++
        return new Promise<RetryMigrationResult>((r) => (release = r))
      }}
    />
  ))
  const btn = screen.getByRole("button", { name: "Retry migration" })
  fireEvent.click(btn)
  await waitFor(() => expect((btn as HTMLButtonElement).disabled).toBe(true))
  fireEvent.click(btn)
  expect(n).toBe(1)
  await act(async () => release({ started: true }))
  await waitFor(() => expect((btn as HTMLButtonElement).disabled).toBe(false))
})

test("a retry refusal shows its message", async () => {
  setup([fail("gx", "bad schema")], () => (
    <FailedMigrationStrip
      retry={async () => ({
        started: false,
        refused: true,
        reason: "migrating",
        message: "project gx is migrating",
      })}
    />
  ))
  fireEvent.click(screen.getByRole("button", { name: "Retry migration" }))
  expect(await screen.findByText("project gx is migrating")).toBeTruthy()
})

const Where = () => <p data-testid="where">{useLocation().pathname}</p>
const inRouter = (f?: FixAgent) => (
  <MemoryRouter>
    <FailedMigrationStrip fixAgent={f} />
    <Where />
  </MemoryRouter>
)

test("Fix agent opens the dialog; Start posts the project, closes it and opens the agent's page", async () => {
  const calls: unknown[] = []
  setup([fail("gx", "bad schema")], inRouter, async (body) => {
    calls.push(body)
    return { remote_control: false, session: { id: "fix-gx" } as SessionInfo }
  })
  fireEvent.click(screen.getByRole("button", { name: "Fix agent" }))
  fireEvent.click(await screen.findByRole("button", { name: "Start gx agent" }))
  await waitFor(() => expect(screen.getByTestId("where").textContent).toBe("/agents/fix-gx"))
  expect(calls).toEqual([{ project: "gx" }])
  expect(screen.queryByRole("dialog")).toBeNull()
})

test("a name-taken refusal keeps the dialog open with the reason", async () => {
  setup([fail("gx", "bad schema")], inRouter, async () => ({
    remote_control: false,
    refused: true,
    reason: "name-taken",
    message: "an agent is already fixing gx",
  }))
  fireEvent.click(screen.getByRole("button", { name: "Fix agent" }))
  fireEvent.click(await screen.findByRole("button", { name: "Start gx agent" }))
  expect(await screen.findByText("an agent is already fixing gx")).toBeTruthy()
  expect(screen.getByRole("dialog")).toBeTruthy()
  expect(screen.getByTestId("where").textContent).toBe("/")
})

test("a row disappears when the snapshot drops it", () => {
  const { set } = setup([fail("gx", "bad schema"), fail("web", "disk full")], () => (
    <FailedMigrationStrip />
  ))
  set([fail("web", "disk full")])
  expect(screen.queryByText("gx: Migration blocked")).toBeNull()
  expect(screen.getByText("web: Migration blocked")).toBeTruthy()
  set([])
  expect(screen.queryByText("web: Migration blocked")).toBeNull()
})

test("the page shell shows the strip on two different pages", () => {
  const page = (active: "epics" | "agents") => (
    <MemoryRouter>
      <AppShell fetchEpicList={async () => ({ epics: [], follow_ups: [] })}>
        <DetailPage title={{ kind: "Epics" }}>
          <p>{active} body</p>
        </DetailPage>
      </AppShell>
    </MemoryRouter>
  )
  const { rerender } = setup([fail("gx", "bad schema")], () => page("epics"))
  expect(screen.getByText("gx: Migration blocked")).toBeTruthy()
  rerender(page("agents"))
  expect(screen.getByText("epics body".replace("epics", "agents"))).toBeTruthy()
  expect(screen.getByText("gx: Migration blocked")).toBeTruthy()
})
