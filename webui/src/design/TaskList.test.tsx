import { ChakraProvider } from "@chakra-ui/react"
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react"
import type { ReactElement } from "react"
import { afterEach, beforeEach, expect, test, vi } from "vitest"
import type { SessionTask } from "../generated/server"
import { system } from "../ui/theme"
import { TaskList } from "./TaskList"
import { elapsedText, formatElapsed, taskDetails, taskSummary, tokenText } from "./taskRows"

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

test.each([
  [412, "412"],
  [4200, "4.2k"],
  [41_000, "41k"],
  [1_200_000, "1.2M"],
])("tokenText(%i) is %s", (n, want) => {
  expect(tokenText(n)).toBe(want)
})

test("details lines per kind", () => {
  const now = T0 + 65_000
  const d = (over: Partial<SessionTask>) => taskDetails(task(over), now)?.text
  expect(d({ output_bytes: 4200, last_line: "✓ 214 tests passed" })).toBe(
    "4.2 KB output · last: ✓ 214 tests passed",
  )
  expect(d({ output_bytes: 0 })).toBe("0 B output")
  expect(d({})).toBeUndefined()
  expect(d({ finished: true, exit_code: 0, output_bytes: 1100 })).toBe("exit 0 · 1.1 KB output")
  expect(d({ finished: true, exit_code: 0 })).toBe("exit 0")
  expect(d({ last_line: "x".repeat(200) })?.length).toBeLessThan(100)
  expect(
    d({
      kind: "subagent",
      model: "opus",
      tokens: 41_000,
      tool_calls: 18,
      current_call: "Read store.go",
    }),
  ).toBe("opus · 41k tokens · 18 tool calls · now: Read store.go")
  expect(
    d({
      kind: "subagent",
      finished: true,
      model: "sonnet",
      tokens: 22_000,
      tool_calls: 1,
      current_call: "x",
    }),
  ).toBe("sonnet · 22k tokens · 1 tool call")
  expect(d({ kind: "subagent" })).toBeUndefined()
  expect(d({ kind: "call" })).toBeUndefined()
  expect(d({ kind: "call", finished: true, exit_code: 3 })).toBe("exit 3")
  expect(d({ kind: "monitor", event_count: 7, last_event_at: at(53) })).toBe(
    "7 events · last 12s ago",
  )
  expect(d({ kind: "monitor", event_count: 1, last_event_at: at(53) })).toBe(
    "1 event · last 12s ago",
  )
  expect(d({ kind: "monitor" })).toBe("0 events")
  expect(
    taskDetails(task({ kind: "monitor", label: "mon", command: "tail -f x" }), now)?.command,
  ).toBe("tail -f x")
  expect(
    taskDetails(task({ kind: "monitor", label: "tail -f x", command: "tail -f x" }), now)?.command,
  ).toBeUndefined()
})

test("rows with details open independently, survive the tick and a closed summary; others are inert", () => {
  const fetch = vi.fn()
  vi.stubGlobal("fetch", fetch)
  renderThemed(
    <TaskList
      tasks={[
        task({ label: "mon", kind: "monitor", event_count: 7, last_event_at: at(53) }),
        task({ label: "sh", output_bytes: 0 }),
        task({ label: "bare" }),
      ]}
    />,
  )
  const summary = screen.getByRole("button", { name: /tasks done/ })
  fireEvent.click(summary)
  const buttons = () => screen.getAllByRole("button").filter((b) => b !== summary)
  expect(buttons().length).toBe(2)
  const [mon, sh] = buttons() as [HTMLElement, HTMLElement]
  expect(mon.getAttribute("aria-expanded")).toBe("false")
  fireEvent.click(mon)
  fireEvent.click(sh)
  expect(screen.getByText("7 events · last 12s ago")).toBeTruthy()
  expect(screen.getByText("0 B output")).toBeTruthy()
  act(() => {
    vi.advanceTimersByTime(3000)
  })
  expect(screen.getByText("7 events · last 15s ago")).toBeTruthy()
  fireEvent.click(summary)
  fireEvent.click(summary)
  expect(screen.getByText("7 events · last 15s ago")).toBeTruthy()
  fireEvent.click(buttons()[0]!)
  expect(screen.queryByText(/7 events/)).toBeNull()
  expect(screen.getByText("0 B output")).toBeTruthy()
  expect(screen.queryByRole("link")).toBeNull()
  expect(fetch).not.toHaveBeenCalled()
})

test("two identical rows open on their own, and a finished twin keeps its own open state", () => {
  const twin = (over: Partial<SessionTask> = {}) => task({ output_bytes: 0, ...over })
  const { rerender } = renderThemed(<TaskList tasks={[twin(), twin()]} />)
  fireEvent.click(screen.getByRole("button", { name: /tasks done/ }))
  const rows = () =>
    screen
      .getAllByRole("button")
      .filter((b) => b.hasAttribute("aria-expanded") && !/tasks done/.test(b.textContent ?? ""))
      .map((b) => b.getAttribute("aria-expanded"))
  expect(rows()).toEqual(["false", "false"])
  fireEvent.click(screen.getAllByRole("button", { name: /make/ })[0]!)
  expect(rows()).toEqual(["true", "false"])
  // The first launched twin finishes: it moves below the still running one.
  rerender(<TaskList tasks={[twin({ finished: true, finished_at: at(3) }), twin()]} />)
  expect(rows()).toEqual(["false", "true"])
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
