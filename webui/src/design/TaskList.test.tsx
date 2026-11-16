import { ChakraProvider } from "@chakra-ui/react"
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react"
import type { ReactElement } from "react"
import { afterEach, beforeEach, expect, test, vi } from "vitest"
import type { SessionTask } from "../generated/server"
import { system } from "../ui/theme"
import { TaskList } from "./TaskList"
import { elapsedText, formatElapsed, taskSummary } from "./taskRows"

const T0 = Date.parse("2026-01-01T12:00:00Z")
const at = (s: number) => new Date(T0 + s * 1000).toISOString()
const task = (over: Partial<SessionTask>): SessionTask => ({
  kind: "shell",
  label: "make",
  started_at: at(0),
  ...over,
})

// Without AppToaster: its timers do not mix with the fake clock.
const wrap = (ui: ReactElement) => <ChakraProvider value={system}>{ui}</ChakraProvider>
const renderThemed = (ui: ReactElement) => {
  const r = render(wrap(ui))
  return { ...r, rerender: (next: ReactElement) => r.rerender(wrap(next)) }
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(T0 + 65_000)
})
afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

test.each([
  [41_000, "41s"],
  [312_000, "5m 12s"],
  [188_000, "3m 08s"],
  [3_900_000, "1h 05m"],
  [-5_000, "0s"],
])("formatElapsed(%i) is %s", (ms, want) => {
  expect(formatElapsed(ms)).toBe(want)
})

test("the time cell of a finished row reads its outcome", () => {
  const fin = { finished: true, started_at: at(0), finished_at: at(12) }
  expect(elapsedText(task({ ...fin, outcome: "success" }), 0)).toBe("12s")
  expect(elapsedText(task({ ...fin, outcome: "failure", exit_code: 2 }), 0)).toBe("exit 2 · 12s")
  expect(elapsedText(task({ ...fin, outcome: "failure" }), 0)).toBe("failed · 12s")
  expect(elapsedText(task({ ...fin, outcome: "killed" }), 0)).toBe("killed · 12s")
  expect(elapsedText(task({ ...fin }), 0)).toBe("12s")
  expect(elapsedText(task({ finished: true, no_duration: true }), 0)).toBe("")
})

test("the summary counts every task and takes the longest running one", () => {
  const s = taskSummary(
    [task({ finished: true }), task({ started_at: at(10) }), task({ started_at: at(0) })],
    T0 + 65_000,
  )
  expect(s).toEqual({ done: 1, total: 3, longest: "1m 05s" })
})

test("no tasks render nothing, one task renders a row without a summary", () => {
  const { container, rerender } = renderThemed(<TaskList tasks={[]} />)
  expect(container.textContent).toBe("")
  rerender(<TaskList tasks={[task({ label: "only" })]} />)
  expect(screen.queryByRole("button")).toBeNull()
  expect(screen.getByRole("list", { name: "Tasks" })).toBeTruthy()
})

test("the summary opens and closes the rows; running rows come first, dimmed finished after", () => {
  renderThemed(
    <TaskList
      tasks={[
        task({ label: "a", finished: true, outcome: "failure", exit_code: 2, finished_at: at(12) }),
        task({ label: "b", kind: "subagent" }),
        task({ label: "c", kind: "monitor" }),
        task({ label: "d", finished: true }),
      ]}
    />,
  )
  const toggle = screen.getByRole("button", { name: /2 of 4 tasks done/ })
  expect(toggle.textContent).toContain("longest 1m 05s")
  expect(screen.queryByRole("list", { name: "Tasks" })).toBeNull()
  fireEvent.click(toggle)
  expect(toggle.getAttribute("aria-expanded")).toBe("true")
  const rows = within(screen.getByRole("list", { name: "Tasks" })).getAllByRole("listitem")
  expect(rows.map((r) => r.textContent)).toEqual([
    "subagentb1m 05s",
    "monitorc1m 05s",
    "✕shellaexit 2 · 12s",
    "–shelld",
  ])
  expect(within(rows[2]!).getByRole("img", { name: "failed" })).toBeTruthy()
  expect(within(rows[3]!).getByRole("img", { name: "finished" })).toBeTruthy()
  expect(screen.queryByRole("link")).toBeNull()
  fireEvent.click(toggle)
  expect(screen.queryByRole("list", { name: "Tasks" })).toBeNull()
})

test("a running row ticks, a finished row stays fixed, and nothing is fetched", () => {
  const fetch = vi.fn()
  vi.stubGlobal("fetch", fetch)
  renderThemed(
    <TaskList
      tasks={[
        task({ label: "run" }),
        task({ label: "fin", finished: true, outcome: "success", finished_at: at(41) }),
      ]}
    />,
  )
  fireEvent.click(screen.getByRole("button"))
  expect(screen.getByText("1m 05s", { selector: "span" })).toBeTruthy()
  act(() => {
    vi.advanceTimersByTime(5000)
  })
  expect(screen.getAllByText("1m 10s").length).toBeGreaterThan(0)
  expect(screen.getByText("41s")).toBeTruthy()
  expect(fetch).not.toHaveBeenCalled()
})

test("the clock stops once the last row finished", () => {
  const { rerender } = renderThemed(<TaskList tasks={[task({}), task({ label: "x" })]} />)
  const clear = vi.spyOn(globalThis, "clearInterval")
  rerender(
    <TaskList
      tasks={[
        task({ finished: true, finished_at: at(3) }),
        task({ label: "x", finished: true, finished_at: at(4) }),
      ]}
    />,
  )
  expect(clear).toHaveBeenCalled()
  expect(vi.getTimerCount()).toBe(0)
})
