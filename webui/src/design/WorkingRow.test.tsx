import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, expect, test } from "vitest"
import { renderThemed } from "../ui/render"
import { WorkingRow } from "./ChatCore"

afterEach(cleanup)

const row = () => screen.getByRole("status").textContent

test("the row reads step, title, current tool (an older Codex shell call here) and the count of the others", () => {
  renderThemed(
    <WorkingRow
      step="Spec"
      title="first"
      current={{ kind: "shell", label: "make test" }}
      others={[
        { kind: "agent", label: "a" },
        { kind: "search", label: "b" },
      ]}
    />,
  )
  expect(row()).toBe("Working: Spec · first · running shell: make test (+2 more)")
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

test("a file label keeps its tool and file name apart from the part that may shorten", async () => {
  const path = "server/sessions/chat/read.go"
  const call = { kind: "other", label: `Read ${path}`, path }
  renderThemed(<WorkingRow current={call} others={[call]} />)
  const parts = (within: Element) =>
    ["call-tool", "call-dir", "call-file"].map(
      (part) => within.querySelector(`[data-part='${part}']`)?.textContent,
    )
  const split = ["Read ", "server/sessions/chat", "/read.go"]
  expect(row()).toBe(`Working · Read ${path} (+1 more)`)
  expect(parts(screen.getByRole("status"))).toEqual(split)
  fireEvent.click(screen.getByRole("button", { name: "(+1 more)" }))
  const item = await screen.findByRole("listitem")
  expect(item.textContent).toBe(`toolRead ${path}`)
  expect(parts(item)).toEqual(split)
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
