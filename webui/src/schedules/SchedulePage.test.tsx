import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, expect, test, vi } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import type { ScheduleShowResult, SchedulesResult } from "../generated/server"
import { appRoutes } from "../routes"
import { renderThemed } from "../ui/render"
import type { PostCreate, PostEdit } from "./schedules"

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
  fetchers: { create?: PostCreate; edit?: PostEdit; show?: ScheduleShowResult } = {},
) {
  sessionStorage.clear()
  const router = createMemoryRouter(
    appRoutes({
      schedules: async () => list,
      scheduleShow: async () => fetchers.show ?? shown(),
      createSchedule: fetchers.create,
      editSchedule: fetchers.edit,
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
  expect(edit.mock.calls[0][0]).toMatchObject({ project: "gx", slug: "morning", name: "Evening" })
  await waitFor(() => expect(screen.queryByRole("button", { name: "Save" })).toBeNull())
  expect(router.state.location.pathname).toBe("/schedules/gx:morning")
  expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Evening")
})

test("a refusal shows its reason inline", async () => {
  const edit = vi.fn<PostEdit>(async () =>
    shown({ refused: true, reason: "empty-name", message: "the name is empty" }),
  )
  renderAt("/schedules/gx:morning", { edit })
  await screen.findByDisplayValue("Morning")
  type("Late limit", "3")
  fireEvent.click(screen.getByRole("button", { name: "Save" }))
  expect(await screen.findByText("the name is empty")).toBeTruthy()
})

test("an unknown Schedule shows the refusal and a way back", async () => {
  renderAt("/schedules/gx:gone", {
    show: shown({ refused: true, reason: "unknown-schedule", message: "no schedule gx:gone" }),
  })
  expect(await screen.findByText("no schedule gx:gone")).toBeTruthy()
  expect(screen.getByRole("link", { name: "← Scheduled prompts" })).toBeTruthy()
})
