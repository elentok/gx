import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { postStopSession, rowToken } from "../design/open"
import type { SessionInfo } from "../generated/server"
import { RemoveConfirmHost } from "../shell/RemoveConfirmHost"
import { renderThemed } from "../ui/render"
import { SessionActions } from "./SessionActions"
import { exitedDesigns, postRemoveSession, removeSession } from "./sessionWrites"

afterEach(cleanup)

const session = (over: Partial<SessionInfo> = {}): SessionInfo => ({
  id: "s-a",
  name: "worker",
  project: "gx",
  dir: "/x",
  harness: "claude",
  status: { state: "idle" },
  status_changed_at: "2026-01-01T00:00:00Z",
  run_started_at: "2026-01-01T00:00:00Z",
  unread: {},
  actionable: {},
  auto_advance: true,
  auto_queue: false,
  ...over,
})
const openMenu = async () => fireEvent.click(screen.getByRole("button", { name: "More actions" }))
const exited = (s: SessionInfo): SessionInfo => ({
  ...s,
  status: { state: "exited" },
  status_changed_at: "2026-01-01T00:00:05Z",
})

test("Remove on a live session is one call with the row token", async () => {
  const live = session()
  const stop = vi.fn()
  const remove = vi.fn(async () => ({}))
  const onRemoved = vi.fn()
  renderThemed(
    <>
      <SessionActions session={live} onRemoved={onRemoved} stop={stop} remove={remove} />
      <RemoveConfirmHost />
    </>,
  )
  await openMenu()
  fireEvent.click(await screen.findByRole("menuitem", { name: "Remove" }))
  fireEvent.click(await screen.findByRole("button", { name: "Remove" }))
  await waitFor(() => expect(onRemoved).toHaveBeenCalled())
  expect(stop).not.toHaveBeenCalled()
  expect(remove).toHaveBeenCalledExactlyOnceWith({ id: "s-a", state: rowToken(live) })
})

test("Stop sends the row token", async () => {
  const live = session()
  const stop = vi.fn(async () => ({}))
  renderThemed(<SessionActions session={live} onRemoved={() => {}} stop={stop} />)
  await openMenu()
  fireEvent.click(await screen.findByRole("menuitem", { name: "Stop" }))
  await waitFor(() => expect(stop).toHaveBeenCalledWith({ id: "s-a", state: rowToken(live) }))
})

test("postStopSession and postRemoveSession send one target; remove confirms with yes", async () => {
  const fetched: { url: string; body: unknown }[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init: RequestInit) => {
      fetched.push({ url, body: JSON.parse(init.body as string) })
      return new Response(
        JSON.stringify({
          results: [
            { target: "s-a", status: "refused", reason: "stale", message: "changed, reselect" },
          ],
        }),
      )
    }),
  )
  const stopped = await postStopSession({ id: "s-a", state: "tok" })
  const removed = await postRemoveSession({ id: "s-a", state: "tok" })
  vi.unstubAllGlobals()
  expect(fetched.map((f) => f.body)).toEqual([
    { targets: [{ id: "s-a", state: "tok" }], yes: false },
    { targets: [{ id: "s-a", state: "tok" }], yes: true },
  ])
  expect(stopped).toEqual({ refused: true, reason: "stale", message: "changed, reselect" })
  expect(removed.refused).toBe(true)
})

test("Remove forgets the session as the Agents page's last agent, so the page does not restore it", async () => {
  localStorage.setItem("gx.agents.last", "s-a")
  const live = session()
  const done = exited(live)
  renderThemed(
    <>
      <SessionActions
        session={done}
        onRemoved={() => {}}
        remove={vi.fn(async () => ({ session: done }))}
      />
      <RemoveConfirmHost />
    </>,
  )
  await openMenu()
  fireEvent.click(await screen.findByRole("menuitem", { name: "Remove" }))
  fireEvent.click(await screen.findByRole("button", { name: "Remove" }))
  await waitFor(() => expect(localStorage.getItem("gx.agents.last")).toBeNull())
})

test("an exited session offers Remove but no Stop", async () => {
  renderThemed(<SessionActions session={exited(session())} onRemoved={() => {}} />)
  await openMenu()
  expect(await screen.findByRole("menuitem", { name: "Remove" })).toBeTruthy()
  expect(screen.queryByRole("menuitem", { name: "Stop" })).toBeNull()
})

test("a Queue agent's menu holds the Agent block but no Stop or Remove", async () => {
  renderThemed(
    <SessionActions
      session={session({ queue: true, tier: "worker", model_switched: true })}
      onRemoved={() => {}}
    />,
  )
  await openMenu()
  expect(await screen.findByText("Agent")).toBeTruthy()
  expect(screen.getByText("Tier")).toBeTruthy()
  expect(screen.getByText("switched by hand")).toBeTruthy()
  expect(screen.queryByRole("menuitem", { name: "Stop" })).toBeNull()
  expect(screen.queryByRole("menuitem", { name: "Remove" })).toBeNull()
})

test("a managed session's menu holds the Agent block above Stop and Remove", async () => {
  renderThemed(<SessionActions session={session({ tier: "worker" })} onRemoved={() => {}} />)
  await openMenu()
  expect(await screen.findByText("Tier")).toBeTruthy()
  expect(screen.getByRole("menuitem", { name: "Stop" })).toBeTruthy()
  expect(screen.getByRole("menuitem", { name: "Remove" })).toBeTruthy()
})

test("a refused remove is returned as the result", async () => {
  const remove = vi.fn(async () => ({ refused: true, reason: "stale" }))
  const res = await removeSession(session(), { remove })
  expect(res.refused).toBe(true)
})

test("exitedDesigns picks only exited design sessions", () => {
  const list = [
    session({ id: "1", design: true }),
    exited(session({ id: "2", design: true })),
    exited(session({ id: "3" })),
  ]
  expect(exitedDesigns(list).map((s) => s.id)).toEqual(["2"])
})
