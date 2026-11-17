import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { ConnectionContext, type Connection } from "../connection"
import type { EpicListRow, TriageResult } from "../generated/server"
import { renderThemed } from "../ui/render"
import { DoneQueue, DoneView } from "./DoneView"

afterEach(cleanup)

const result: TriageResult = {
  ready_epic: "gx:ready-1",
  groups: [
    { action: "ready", items: [{ address: "gx:follow-ups/1", title: "a" }] },
    {
      action: "backlog",
      items: [{ address: "gx:follow-ups/2", title: "b", target: "gx:map-a/3" }],
    },
    {
      action: "merge",
      items: [{ address: "gx:follow-ups/3", title: "c", target: "gx:follow-ups/9" }],
    },
    {
      action: "close",
      items: [
        { address: "gx:follow-ups/4", title: "d", reason: "obsolete" },
        { address: "gx:follow-ups/5", title: "e", reason: "obsolete" },
        { address: "gx:follow-ups/6", title: "f", reason: "duplicate" },
      ],
    },
    { action: "keep", items: [{ address: "gx:follow-ups/7", title: "g" }] },
  ],
}

test("groups and links render from the payload", () => {
  const { container } = renderThemed(
    <DoneView result={result} queue={<button type="button">Queue epic</button>} />,
  )
  for (const name of ["Ready epic", "Map epics", "Merged", "Closed", "Kept for later"])
    screen.getByRole("region", { name })
  const line = (name: string) => screen.getByRole("region", { name }).firstElementChild
  expect(line("Ready epic")?.textContent).toBe("✓1 ready in gx:ready-1Queue epic")
  expect(line("Map epics")?.textContent).toBe("✓1 to backlogs · map-a 1")
  expect(line("Closed")?.textContent).toBe("✓3 closed · obsolete 2 · duplicate 1")
  expect(line("Kept for later")?.textContent).toBe("✓1 kept for later in follow-ups")
  expect(container.querySelector('a[href="#/e/gx:ready-1"]')).toBeTruthy()
  expect(container.querySelector('a[href="#/t/gx:map-a/3"]')).toBeTruthy()
  screen.getByText("into gx:follow-ups/9")
  for (const a of ["gx:follow-ups/4", "gx:follow-ups/6", "gx:follow-ups/7"])
    expect(container.querySelector(`a[href="#/t/${a}"]`)).toBeTruthy()
})

test("without a ready epic the ready line has no queue", () => {
  renderThemed(<DoneView result={{ ...result, ready_epic: undefined }} />)
  expect(screen.queryByRole("button")).toBeNull()
})

const listRow = (state: string): EpicListRow =>
  ({
    address: "gx:ready-1",
    project: "gx",
    name: "ready-1",
    kind: "loop",
    state,
    done: 0,
    total: 1,
    queued: false,
    queueable: true,
    blocked_by_epics: [],
    addresses: ["gx:ready-1"],
  }) as EpicListRow

function renderQueue(
  fetchEpics: () => Promise<{ epics: EpicListRow[] }>,
  queue = vi.fn(async () => ({})),
) {
  const listeners = new Set<() => void>()
  const conn = {
    getState: () => ({ connected: true }),
    subscribe: () => () => {},
    onEvent: (l: () => void) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
  } as unknown as Connection
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <DoneQueue epic="gx:ready-1" fetchEpics={fetchEpics as never} queue={queue as never} />
    </ConnectionContext.Provider>,
  )
  return {
    queue,
    emit: () =>
      act(() => {
        for (const l of listeners) l()
      }),
  }
}

test("a ready row shows Queue and posts the queue add once", async () => {
  const { queue } = renderQueue(async () => ({ epics: [listRow("ready")] }))
  const button = await screen.findByRole("button", { name: "Queue it" })
  fireEvent.click(button)
  fireEvent.click(button)
  await waitFor(() => expect(queue).toHaveBeenCalledTimes(1))
  expect(queue).toHaveBeenCalledWith({ address: "gx:ready-1", state: "ready" })
})

test("a queued row shows the queued word, with no Queue button, and follows a changed row", async () => {
  let state = "queued"
  const { emit } = renderQueue(async () => ({ epics: [listRow(state)] }))
  await screen.findByText("queued")
  expect(screen.queryByRole("button", { name: "Queue it" })).toBeNull()
  state = "running"
  emit()
  await screen.findByText("running", undefined, { timeout: 2000 })
})
