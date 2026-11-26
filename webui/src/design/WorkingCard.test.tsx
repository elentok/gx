import { ChakraProvider } from "@chakra-ui/react"
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react"
import type { ComponentProps, ReactElement } from "react"
import { afterEach, beforeEach, expect, test, vi } from "vitest"
import type { SessionTask } from "../generated/server"
import { SubagentOriginContext } from "../subagentOrigin"
import { system } from "../ui/theme"
import { elapsedText, formatElapsed, taskDetails, tokenText } from "./taskRows"
import { WorkingCard as WorkingCardBase } from "./WorkingCard"

const WorkingCard = (props: Omit<ComponentProps<typeof WorkingCardBase>, "sessionId">) => (
  <WorkingCardBase sessionId="s 1" {...props} />
)

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

const list = () => screen.getByRole("list", { name: "Tasks" })
const rows = () => within(list()).getAllByRole("listitem")
const expandable = () =>
  within(list())
    .getAllByRole("button")
    .map((b) => b.getAttribute("aria-expanded"))

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(T0 + 65_000)
})
afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

test("a busy turn with no tasks shows the header only", () => {
  const { container } = renderThemed(<WorkingCard working={{ step: "Spec" }} />)
  expect(screen.getByRole("status").textContent).toBe("Working: Spec")
  expect(screen.queryByRole("list")).toBeNull()
  expect(container.querySelector("[data-live]")).toBeTruthy()
})

test("an idle turn shows the rows with no header and no moving border", () => {
  const { container } = renderThemed(<WorkingCard tasks={[task({})]} />)
  expect(rows().length).toBe(1)
  expect(screen.queryByRole("status")).toBeNull()
  expect(container.querySelector("[data-live]")).toBeNull()
})

test("an idle turn with no tasks renders nothing", () => {
  const { container } = renderThemed(<WorkingCard tasks={[]} />)
  expect(container.textContent).toBe("")
  expect(container.querySelector("[data-tone]")).toBeNull()
})

test("every row shows with no click: running first, then finished, with no kind word", () => {
  renderThemed(
    <WorkingCard
      tasks={[
        task({ label: "a", finished: true, outcome: "failure", exit_code: 2, finished_at: at(12) }),
        task({ label: "b", kind: "subagent" }),
        task({ label: "c", kind: "monitor" }),
        task({ label: "d", finished: true }),
        task({ label: "e", kind: "call" }),
      ]}
    />,
  )
  expect(rows().map((r) => r.textContent)).toEqual([
    "b1m 05s",
    "c1m 05s",
    "e1m 05s",
    "✕aexit 2 · 12s",
    "–d",
  ])
  expect(within(rows()[0]!).getByRole("img", { name: "running" })).toBeTruthy()
  expect(within(rows()[3]!).getByRole("img", { name: "failed" })).toBeTruthy()
  expect(within(rows()[4]!).getByRole("img", { name: "finished" })).toBeTruthy()
  expect(rows().map((r) => r.getAttribute("data-mock-part"))).toEqual([
    "subagent-row-1",
    null,
    null,
    "done-row-1",
    "done-row-2",
  ])
})

test("a running subagent row links to its page and shows its current call", () => {
  const path = "tickets/frontier.go"
  renderThemed(
    <WorkingCard
      tasks={[
        task({
          kind: "subagent",
          label: "Explore",
          call_id: "toolu/1",
          model: "opus",
          current_call: `Read ${path}`,
          current_call_path: path,
        }),
        task({ kind: "subagent", label: "no call" }),
      ]}
    />,
  )
  const link = screen.getByRole("link", { name: "Explore ↗" })
  expect(link.getAttribute("href")).toBe("#/agents/s%201/subagents/toolu%2F1")
  expect(rows()[0]!.textContent).toBe(`Explore ↗1m 05sRead ${path}`)
  expect(rows()[0]!.querySelector("[data-part='call-file']")?.textContent).toBe("/frontier.go")
  // No call id: a plain name. No current call: no second line.
  expect(screen.getAllByRole("link").length).toBe(1)
  expect(rows()[1]!.textContent).toBe("no call1m 05s")
  expect(rows()[1]!.querySelector("p")).toBeNull()
  expect(screen.queryByRole("button")).toBeNull()
})

test("a subagent row link carries the page's origin", () => {
  renderThemed(
    <SubagentOriginContext.Provider value="/triage/demo">
      <WorkingCard tasks={[task({ kind: "subagent", label: "Explore", call_id: "c1" })]} />
    </SubagentOriginContext.Provider>,
  )
  expect(screen.getByRole("link", { name: "Explore ↗" }).getAttribute("href")).toBe(
    "#/agents/s%201/subagents/c1?from=%2Ftriage%2Fdemo",
  )
})

test("a finished subagent row has no link, no second line and does not open", () => {
  renderThemed(
    <WorkingCard
      tasks={[
        task({
          kind: "subagent",
          label: "survey",
          call_id: "c1",
          finished: true,
          outcome: "success",
          finished_at: at(41),
          model: "opus",
          current_call: "Read x",
        }),
      ]}
    />,
  )
  expect(rows()[0]!.textContent).toBe("✓survey41s")
  expect(screen.queryByRole("link")).toBeNull()
  expect(screen.queryByRole("button")).toBeNull()
})

test("shell, Monitor and call rows open and stay open across a tick and a header change", () => {
  const fetch = vi.fn()
  vi.stubGlobal("fetch", fetch)
  const tasks = [
    task({ label: "mon", kind: "monitor", event_count: 7, last_event_at: at(53) }),
    task({ label: "sh", output_bytes: 0 }),
    task({ label: "call", kind: "call", command: "ls\npwd" }),
    task({ label: "bare" }),
  ]
  const { rerender } = renderThemed(<WorkingCard tasks={tasks} working={{}} />)
  expect(expandable()).toEqual(["false", "false", "false"])
  for (const b of within(list()).getAllByRole("button")) fireEvent.click(b)
  expect(screen.getByText("7 events · last 12s ago")).toBeTruthy()
  expect(screen.getByText("0 B output")).toBeTruthy()
  expect(screen.getByTestId("task-command").textContent).toBe("lspwd")
  act(() => {
    vi.advanceTimersByTime(3000)
  })
  // The turn ends: the card changes its element tree, the rows stay open.
  rerender(<WorkingCard tasks={tasks} />)
  expect(expandable()).toEqual(["true", "true", "true"])
  expect(screen.getByText("7 events · last 15s ago")).toBeTruthy()
  rerender(<WorkingCard tasks={tasks} working={{}} />)
  expect(expandable()).toEqual(["true", "true", "true"])
  fireEvent.click(within(list()).getAllByRole("button")[0]!)
  expect(screen.queryByText(/7 events/)).toBeNull()
  expect(screen.getByText("0 B output")).toBeTruthy()
  expect(fetch).not.toHaveBeenCalled()
})

test("two identical rows open on their own, and a finished twin keeps its own open state", () => {
  const twin = (over: Partial<SessionTask> = {}) => task({ output_bytes: 0, ...over })
  const { rerender } = renderThemed(<WorkingCard tasks={[twin(), twin()]} />)
  expect(expandable()).toEqual(["false", "false"])
  fireEvent.click(within(list()).getAllByRole("button")[0]!)
  expect(expandable()).toEqual(["true", "false"])
  // The first launched twin finishes: it moves below the still running one.
  rerender(<WorkingCard tasks={[twin({ finished: true, finished_at: at(3) }), twin()]} />)
  expect(expandable()).toEqual(["false", "true"])
})

test("a running row ticks and a finished row stays fixed", () => {
  renderThemed(
    <WorkingCard
      tasks={[
        task({ label: "run" }),
        task({ label: "fin", finished: true, outcome: "success", finished_at: at(41) }),
      ]}
    />,
  )
  expect(screen.getByText("1m 05s")).toBeTruthy()
  act(() => {
    vi.advanceTimersByTime(5000)
  })
  expect(screen.getByText("1m 10s")).toBeTruthy()
  expect(screen.getByText("41s")).toBeTruthy()
})

test("the clock stops once the last row finished", () => {
  const { rerender } = renderThemed(<WorkingCard tasks={[task({}), task({ label: "x" })]} />)
  rerender(
    <WorkingCard
      tasks={[
        task({ finished: true, finished_at: at(3) }),
        task({ label: "x", finished: true, finished_at: at(4) }),
      ]}
    />,
  )
  expect(vi.getTimerCount()).toBe(0)
})

test("an agent call in the header shows its label with no waiting verb", () => {
  renderThemed(<WorkingCard working={{}} current={{ kind: "agent", label: "survey" }} />)
  expect(screen.getByRole("status").textContent).toBe("Working · survey")
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
