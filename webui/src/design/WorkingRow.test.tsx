import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, expect, test } from "vitest"
import { renderThemed } from "../ui/render"
import { WorkingRow } from "./ChatCore"

afterEach(cleanup)

const row = () => screen.getByRole("status").textContent

test("the row reads step, title, current tool, then background tasks", () => {
  renderThemed(
    <WorkingRow
      step="Spec"
      title="first"
      current={{ kind: "shell", label: "make test" }}
      others={[
        { kind: "agent", label: "a" },
        { kind: "search", label: "b" },
      ]}
      background={[{ kind: "shell", label: "x", started_at: new Date().toISOString() }]}
    />,
  )
  expect(row()).toBe(
    "Working: Spec · first · running shell: make test (+2 more) · waiting on 1 shell",
  )
})

test.each([
  [{ kind: "shell", label: "ls" }, "Working · running shell: ls"],
  [{ kind: "agent", label: "review" }, "Working · waiting for sub-agent: review"],
  [{ kind: "search", label: "foo" }, "Working · searching: foo"],
  [{ kind: "other", label: "Read" }, "Working · Read"],
])("%o reads %s", (current, text) => {
  renderThemed(<WorkingRow current={current} />)
  expect(row()).toBe(text)
})

test("nothing in flight reads Working", () => {
  renderThemed(<WorkingRow />)
  expect(row()).toBe("Working")
})

test("the count button opens the list of the others on click and on focus, and Escape closes it", async () => {
  renderThemed(
    <WorkingRow
      current={{ kind: "shell", label: "make" }}
      others={[
        { kind: "agent", label: "reviewer" },
        { kind: "other", label: "Read" },
      ]}
    />,
  )
  const button = screen.getByRole("button", { name: "(+2 more)" })
  expect(screen.queryByText("reviewer")).toBeNull()
  fireEvent.click(button)
  expect(await screen.findByText("reviewer")).toBeTruthy()
  expect(screen.getByText("sub-agent")).toBeTruthy()
  expect(screen.getByText("Read")).toBeTruthy()
  fireEvent.keyDown(button, { key: "Escape" })
  await waitFor(() => expect(screen.queryByText("reviewer")).toBeNull())
  fireEvent.focus(button)
  expect(await screen.findByText("reviewer")).toBeTruthy()
})
