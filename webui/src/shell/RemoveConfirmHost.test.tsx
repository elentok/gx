import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { session } from "../agents/agentsTestKit"
import { renderThemed } from "../ui/render"
import { RemoveConfirmHost } from "./RemoveConfirmHost"
import { closeRemoveConfirm, openRemoveConfirm } from "./removeConfirmOpen"

afterEach(() => {
  closeRemoveConfirm()
  cleanup()
})

const live = { ...session("s1", "gamma"), name: "talk" }
const exited = { ...live, status: { ...live.status, state: "exited" } } as typeof live

test("a live session asks to stop and remove; an exited one only to remove", () => {
  renderThemed(<RemoveConfirmHost />)
  act(() => openRemoveConfirm({ session: live, onRemoved: () => {} }))
  expect(screen.getByText("Stop and remove talk? Its session is deleted.")).toBeTruthy()
  act(() => openRemoveConfirm({ session: exited, onRemoved: () => {} }))
  expect(screen.getByText("Remove talk? Its session is deleted.")).toBeTruthy()
})

test("Keep has focus and closes without removing", () => {
  const remove = vi.fn(async () => ({}))
  renderThemed(<RemoveConfirmHost />)
  act(() => openRemoveConfirm({ session: live, onRemoved: () => {}, remove }))
  const keep = screen.getByRole("button", { name: "Keep" })
  expect(document.activeElement).toBe(keep)
  fireEvent.click(keep)
  expect(screen.queryByText(/Its session is deleted/)).toBeNull()
  expect(remove).not.toHaveBeenCalled()
})

test("Remove calls the writer, then the opener's callback", async () => {
  const remove = vi.fn(async () => ({}))
  const onRemoved = vi.fn()
  renderThemed(<RemoveConfirmHost />)
  act(() => openRemoveConfirm({ session: live, onRemoved, remove }))
  fireEvent.click(screen.getByRole("button", { name: "Remove" }))
  await waitFor(() => expect(onRemoved).toHaveBeenCalled())
  expect(remove).toHaveBeenCalledWith(expect.objectContaining({ id: "s1" }))
})
