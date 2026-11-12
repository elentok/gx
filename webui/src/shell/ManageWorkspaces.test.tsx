import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import type { Workspace } from "../generated/server"
import { renderThemed } from "../ui/render"
import { ManageWorkspaces, type WorkspaceWriters } from "./ManageWorkspaces"

afterEach(cleanup)

const list: Workspace[] = [
  { name: "Default", default: true, members: 2 },
  { name: "Work", members: 3 },
  { name: "Home", members: 1 },
]

function setup(overrides: Partial<WorkspaceWriters> = {}) {
  const writers = {
    rename: vi.fn(async () => ({})),
    remove: vi.fn(async () => ({})),
    reorder: vi.fn(async () => ({})),
    ...overrides,
  } satisfies WorkspaceWriters
  const onClose = vi.fn()
  renderThemed(<ManageWorkspaces workspaces={list} writers={writers} onClose={onClose} />)
  return { writers, onClose }
}

test("lists the workspaces in order", () => {
  setup()
  const items = within(screen.getByRole("list")).getAllByRole("listitem")
  expect(items.map((li) => li.textContent)).toEqual([
    expect.stringContaining("Default"),
    expect.stringContaining("3 items"),
    expect.stringContaining("1 item"),
  ])
})

test("the Default row has neither rename nor delete", () => {
  setup()
  expect(screen.queryByLabelText("Rename Default")).toBeNull()
  expect(screen.queryByRole("button", { name: "Delete Default" })).toBeNull()
  expect(screen.getByLabelText("Rename Work")).toBeTruthy()
  expect(screen.getByRole("button", { name: "Delete Work" })).toBeTruthy()
})

test("Enter saves a rename", async () => {
  const { writers } = setup()
  const input = screen.getByLabelText("Rename Work")
  input.focus()
  fireEvent.change(input, { target: { value: "Office" } })
  fireEvent.keyDown(input, { key: "Enter" })
  await waitFor(() => expect(writers.rename).toHaveBeenCalledWith("Work", "Office"))
  expect(writers.rename).toHaveBeenCalledTimes(1)
})

test("blur saves a rename", async () => {
  const { writers } = setup()
  const input = screen.getByLabelText("Rename Work")
  input.focus()
  fireEvent.change(input, { target: { value: "Office" } })
  input.blur()
  await waitFor(() => expect(writers.rename).toHaveBeenCalledWith("Work", "Office"))
})

test("Escape cancels a rename and does not close the dialog", () => {
  const { writers, onClose } = setup()
  const input = screen.getByLabelText<HTMLInputElement>("Rename Work")
  fireEvent.change(input, { target: { value: "Office" } })
  fireEvent.keyDown(input, { key: "Escape" })
  expect(input.value).toBe("Work")
  expect(onClose).not.toHaveBeenCalled()
  expect(writers.rename).not.toHaveBeenCalled()
})

test("a refused rename shows the reason inline", async () => {
  setup({
    rename: async () => ({
      refused: true,
      message: "A workspace named Home already exists.",
    }),
  })
  const input = screen.getByLabelText("Rename Work")
  input.focus()
  fireEvent.change(input, { target: { value: "Home" } })
  input.blur()
  expect((await screen.findByRole("alert")).textContent).toBe(
    "A workspace named Home already exists.",
  )
})

test("up and down post the new order; the ends are disabled", async () => {
  const { writers } = setup()
  expect(
    (
      screen.getByRole("button", {
        name: "Move Default up",
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true)
  expect(
    (
      screen.getByRole("button", {
        name: "Move Home down",
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true)
  fireEvent.click(screen.getByRole("button", { name: "Move Work up" }))
  await waitFor(() => expect(writers.reorder).toHaveBeenCalledWith(["Work", "Default", "Home"]))
  await waitFor(() =>
    expect(
      (
        screen.getByRole("button", {
          name: "Move Home up",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false),
  )
  fireEvent.click(screen.getByRole("button", { name: "Move Home up" }))
  await waitFor(() => expect(writers.reorder).toHaveBeenLastCalledWith(["Default", "Home", "Work"]))
})

test("delete asks first and names the member count", async () => {
  const { writers } = setup()
  fireEvent.click(screen.getByRole("button", { name: "Delete Work" }))
  const dialog = screen.getByRole("dialog", { name: "Delete Work" })
  expect(dialog.textContent).toContain("3 items go back to the Default workspace")
  expect(writers.remove).not.toHaveBeenCalled()
  fireEvent.click(within(dialog).getByRole("button", { name: "Delete" }))
  await waitFor(() => expect(writers.remove).toHaveBeenCalledWith("Work"))
})

test("keeping a workspace posts nothing", () => {
  const { writers } = setup()
  fireEvent.click(screen.getByRole("button", { name: "Delete Home" }))
  fireEvent.click(screen.getByRole("button", { name: "Keep" }))
  expect(writers.remove).not.toHaveBeenCalled()
  expect(screen.queryByRole("dialog", { name: "Delete Home" })).toBeNull()
})
