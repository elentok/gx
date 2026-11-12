import { cleanup, fireEvent, screen } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { renderThemed as render } from "../ui/render"
import { ApproveBar } from "./ApproveBar"

afterEach(cleanup)

const facts = (over = {}) => ({
  open_questions: 0,
  action_counts: { ready: 22, map: 7, merge: 6, close: 8, keep: 2 },
  changed_rows: 0,
  rows: [],
  ...over,
})

const bar = (over = {}, handlers: { onApprove?: () => void; onAnswer?: () => void } = {}) =>
  render(
    <ApproveBar
      view={facts(over)}
      pending={false}
      onApprove={handlers.onApprove ?? (() => {})}
      onAnswer={handlers.onAnswer ?? (() => {})}
    />,
  )

const approveButton = () =>
  screen.getByRole("button", { name: "Approve and write" }) as HTMLButtonElement

test("with questions open it is disabled, says how many are left and links to the Questions view", () => {
  const onAnswer = vi.fn()
  bar({ open_questions: 3 }, { onAnswer })
  screen.getByText("3 questions left.")
  expect(screen.getByRole("group", { name: "Approve" }).textContent).toBe(
    "3 questions left. Answer them to approve.Approve and write",
  )
  expect(approveButton().disabled).toBe(true)
  fireEvent.click(screen.getByRole("button", { name: "Answer them" }))
  expect(onAnswer).toHaveBeenCalledTimes(1)
})

test("with one question left the count is singular", () => {
  bar({ open_questions: 1 })
  screen.getByText("1 question left.")
})

test("with no question it shows the counts per action and an enabled Approve and write", () => {
  const onApprove = vi.fn()
  bar({}, { onApprove })
  screen.getByText("22 ready epic · 7 map epics · 6 merge · 8 close · 2 keep")
  expect(screen.queryByRole("button", { name: "Answer them" })).toBeNull()
  fireEvent.click(approveButton())
  expect(onApprove).toHaveBeenCalledTimes(1)
})

test("it shows how many rows the person changed, and nothing at zero", () => {
  bar({ changed_rows: 3 })
  screen.getByText(/3 changed by you/)
  cleanup()
  bar({ changed_rows: 0 })
  expect(screen.queryByText(/changed by you/)).toBeNull()
})

test("once every orphaned merge row is re-pointed it is enabled again", () => {
  bar({ rows: [{ address: "gx:follow-ups/1", title: "a", merge_orphan: true }] })
  expect(approveButton().disabled).toBe(true)
  cleanup()
  bar({
    rows: [
      { address: "gx:follow-ups/1", title: "a", action: "merge", destination: "gx:follow-ups/2" },
    ],
  })
  expect(screen.queryByText(/merge target is gone/)).toBeNull()
  expect(approveButton().disabled).toBe(false)
})

test("an orphaned merge target disables it with the reason", () => {
  bar({ rows: [{ address: "gx:follow-ups/1", title: "a", merge_orphan: true }] })
  screen.getByText(/merge target is gone/)
  expect(approveButton().disabled).toBe(true)
  expect(screen.queryByRole("button", { name: "Answer them" })).toBeNull()
})
