import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, expect, test, vi } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import type { ScheduleRunNowResult, ScheduleShowResult, SchedulesResult } from "../generated/server"
import { appRoutes } from "../routes"
import { renderThemed } from "../ui/render"
import type { PostCreate, PostDelete, PostEdit, PostRunNow, PostSwitch } from "./schedules"

const ran = (over: Partial<ScheduleRunNowResult> = {}): ScheduleRunNowResult => ({
  address: "",
  status: "",
  refused: false,
  reason: "",
  message: "",
  attention_address: "",
  ...over,
})

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

const state = {
  snapshot: undefined,
  connected: true,
  loadedOnce: true,
  liveSnapshot: true,
  tick: 0,
  streams: 0,
} as unknown as ConnectionState
const conn = {
  getState: () => state,
  subscribe: () => () => {},
  onEvent: () => () => {},
  refresh: () => {},
} as unknown as Connection

const shown = (over: Partial<ScheduleShowResult> = {}): ScheduleShowResult => ({
  refused: false,
  reason: "",
  message: "",
  prompt: "Summarise the night",
  last_runs: [],
  schedule: {
    name: "Morning",
    slug: "morning",
    project: "gx",
    days: ["mon", "tue"],
    times: ["08:30"],
    harness: "",
    default_harness: "claude",
    late_limit_hours: 2,
    switch: true,
    project_found: true,
    next_run: "",
    updated_at: "",
  },
  ...over,
})

const list: SchedulesResult = {
  refused: false,
  reason: "",
  message: "",
  schedules: [],
  projects: ["scratch", "gx"],
}

function renderAt(
  path: string,
  fetchers: {
    create?: PostCreate
    edit?: PostEdit
    show?: ScheduleShowResult
    runNow?: PostRunNow
    switchTo?: PostSwitch
    remove?: PostDelete
  } = {},
) {
  sessionStorage.clear()
  const router = createMemoryRouter(
    appRoutes({
      schedules: async () => list,
      scheduleShow: async () => fetchers.show ?? shown(),
      createSchedule: fetchers.create,
      editSchedule: fetchers.edit,
      runNowSchedule: fetchers.runNow,
      switchSchedule: fetchers.switchTo,
      deleteSchedule: fetchers.remove,
      epicList: async () => ({ epics: [], follow_ups: [] }),
    }),
    { initialEntries: [path] },
  )
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )
  return router
}

const type = (label: string, value: string) =>
  fireEvent.change(screen.getByLabelText(label), { target: { value } })

test("a new Schedule shows Create, a project select and the shared PromptBox", async () => {
  renderAt("/schedules/new")
  expect(await screen.findByRole("button", { name: "Create" })).toBeTruthy()
  expect(screen.getByLabelText("Project").tagName).toBe("SELECT")
  expect(screen.getByLabelText("Prompt")).toBeTruthy()
  expect(screen.getByRole("button", { name: "Open the note in a large editor" })).toBeTruthy()
  expect(screen.getByRole("option", { name: "Default" })).toBeTruthy()
  expect(screen.getByRole("button", { name: "Mon" }).getAttribute("aria-pressed")).toBe("true")
  expect(screen.getByRole("button", { name: "Sat" }).getAttribute("aria-pressed")).toBe("false")
})

test("Create blocks on an empty form with inline errors and posts nothing", async () => {
  const create = vi.fn()
  renderAt("/schedules/new", { create })
  fireEvent.click(await screen.findByRole("button", { name: "Create" }))
  expect(screen.getByText("Enter a name.")).toBeTruthy()
  expect(screen.getByText("Enter a prompt.")).toBeTruthy()
  expect(create).not.toHaveBeenCalled()
})

test("Create posts the form and opens the new Schedule", async () => {
  const create = vi.fn<PostCreate>(async () => shown())
  const router = renderAt("/schedules/new", { create })
  await screen.findByRole("button", { name: "Create" })
  type("Name", "Morning")
  type("Prompt", "Summarise the night")
  type("Project", "gx")
  fireEvent.click(screen.getByRole("button", { name: "Create" }))
  await waitFor(() => expect(create).toHaveBeenCalled())
  expect(create.mock.calls[0][0]).toMatchObject({
    name: "Morning",
    prompt: "Summarise the night",
    project: "gx",
    days: ["mon", "tue", "wed", "thu", "fri"],
    times: ["07:00"],
    harness: "",
    late_limit_hours: 2,
  })
  await waitFor(() => expect(router.state.location.pathname).toBe("/schedules/gx:morning"))
})

test("an existing Schedule has no save bar until a field changes, and Discard restores it", async () => {
  renderAt("/schedules/gx:morning")
  await screen.findByDisplayValue("Morning")
  expect(screen.queryByRole("button", { name: "Save" })).toBeNull()
  type("Name", "Evening")
  fireEvent.click(screen.getByRole("button", { name: "Tue" }))
  type("Late limit", "1.5")
  expect(screen.getByText("Unsaved changes")).toBeTruthy()
  fireEvent.click(screen.getByRole("button", { name: "Discard" }))
  expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Morning")
  expect(screen.getByRole("button", { name: "Tue" }).getAttribute("aria-pressed")).toBe("true")
  expect((screen.getByLabelText("Late limit") as HTMLInputElement).value).toBe("2")
  expect(screen.queryByRole("button", { name: "Save" })).toBeNull()
})

test("the Project is read-only text on an existing Schedule", async () => {
  renderAt("/schedules/gx:morning")
  await screen.findByDisplayValue("Morning")
  expect(screen.queryByLabelText("Project")).toBeNull()
  expect(screen.getByText("gx")).toBeTruthy()
  expect(screen.getByRole("option", { name: "Default (claude)" })).toBeTruthy()
})

test("Save posts an edit and stays on the page", async () => {
  const edit = vi.fn<PostEdit>(async () =>
    shown({ schedule: { ...shown().schedule, name: "Evening" } }),
  )
  const router = renderAt("/schedules/gx:morning", { edit })
  await screen.findByDisplayValue("Morning")
  type("Name", "Evening")
  fireEvent.click(screen.getByRole("button", { name: "Save" }))
  await waitFor(() => expect(edit).toHaveBeenCalled())
  expect(edit.mock.calls[0][0]).toMatchObject({
    project: "gx",
    slug: "morning",
    name: "Evening",
  })
  await waitFor(() => expect(screen.queryByRole("button", { name: "Save" })).toBeNull())
  expect(router.state.location.pathname).toBe("/schedules/gx:morning")
  expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Evening")
})

test("a refusal shows its reason inline", async () => {
  const edit = vi.fn<PostEdit>(async () =>
    shown({
      refused: true,
      reason: "empty-name",
      message: "the name is empty",
    }),
  )
  renderAt("/schedules/gx:morning", { edit })
  await screen.findByDisplayValue("Morning")
  type("Late limit", "3")
  fireEvent.click(screen.getByRole("button", { name: "Save" }))
  expect(await screen.findByText("the name is empty")).toBeTruthy()
})

test("an unknown Schedule shows the refusal and a way back", async () => {
  renderAt("/schedules/gx:gone", {
    show: shown({
      refused: true,
      reason: "unknown-schedule",
      message: "no schedule gx:gone",
    }),
  })
  expect(await screen.findByText("no schedule gx:gone")).toBeTruthy()
  expect(screen.getByRole("link", { name: "← Scheduled prompts" })).toBeTruthy()
})

test("Run now posts, and a refusal shows its reason on the page", async () => {
  const runNow = vi.fn<PostRunNow>(async () =>
    ran({ refused: true, reason: "previous-run-started" }),
  )
  renderAt("/schedules/gx:morning", { runNow })
  await screen.findByDisplayValue("Morning")
  fireEvent.click(screen.getByRole("button", { name: "Run now" }))
  expect(await screen.findByText("previous run started")).toBeTruthy()
  expect(runNow).toHaveBeenCalledWith("gx", "morning")
})

test("a needs-attention refusal links the blocking ticket", async () => {
  const runNow = vi.fn<PostRunNow>(async () =>
    ran({
      refused: true,
      reason: "previous-run-needs-attention",
      attention_address: "gx:epic/3",
    }),
  )
  renderAt("/schedules/gx:morning", { runNow })
  await screen.findByDisplayValue("Morning")
  fireEvent.click(screen.getByRole("button", { name: "Run now" }))
  const link = await screen.findByRole("link", { name: "gx:epic/3" })
  expect(link.getAttribute("href")).toBe("#/t/gx:epic/3")
  expect(screen.getByText(/previous run needs attention/)).toBeTruthy()
})

test("an unknown-project refusal names the project", async () => {
  const runNow = vi.fn<PostRunNow>(async () => ran({ refused: true, reason: "unknown-project" }))
  renderAt("/schedules/gx:morning", { runNow })
  await screen.findByDisplayValue("Morning")
  fireEvent.click(screen.getByRole("button", { name: "Run now" }))
  expect(await screen.findByText("no project gx in the ticket store")).toBeTruthy()
})

test("Run now is enabled with the switch off and the page says so", async () => {
  const off = shown()
  off.schedule.switch = false
  renderAt("/schedules/gx:morning", { show: off })
  await screen.findByDisplayValue("Morning")
  expect((screen.getByRole("button", { name: "Run now" }) as HTMLButtonElement).disabled).toBe(
    false,
  )
  expect(screen.getByText("Runs once now. The switch is off on this machine.")).toBeTruthy()
})

test("the switch posts the opposite state", async () => {
  const switchTo = vi.fn<PostSwitch>(async () => {
    const r = shown()
    r.schedule.switch = false
    return r
  })
  renderAt("/schedules/gx:morning", { switchTo })
  await screen.findByDisplayValue("Morning")
  fireEvent.click(screen.getByRole("switch", { name: "Run on this machine" }))
  await waitFor(() => expect(switchTo).toHaveBeenCalledWith("gx", "morning", false))
  expect(await screen.findByText("Runs once now. The switch is off on this machine.")).toBeTruthy()
})

test("a missing project shows project not found and still offers Run now", async () => {
  const gone = shown()
  gone.schedule.project_found = false
  renderAt("/schedules/gx:morning", { show: gone })
  expect(await screen.findByText("project not found")).toBeTruthy()
  expect(screen.getByRole("button", { name: "Run now" })).toBeTruthy()
})

test("Delete asks inline first and only posts on confirm", async () => {
  const remove = vi.fn<PostDelete>(async () => ({
    refused: false,
    reason: "",
    message: "",
  }))
  const router = renderAt("/schedules/gx:morning", { remove })
  await screen.findByDisplayValue("Morning")
  fireEvent.click(screen.getByRole("button", { name: "Delete this schedule" }))
  expect(screen.getByText(/Queued runs that never started are cancelled/)).toBeTruthy()
  expect(remove).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }))
  fireEvent.click(screen.getByRole("button", { name: "Delete this schedule" }))
  fireEvent.click(screen.getByRole("button", { name: "Delete" }))
  await waitFor(() => expect(remove).toHaveBeenCalledWith("gx", "morning"))
  await waitFor(() => expect(router.state.location.pathname).toBe("/schedules"))
})
