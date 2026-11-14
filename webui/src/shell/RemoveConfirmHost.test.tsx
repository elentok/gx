import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { session } from "../agents/agentsTestKit"
import { rowToken } from "../design/open"
import { renderThemed } from "../ui/render"
import { RemoveConfirmHost } from "./RemoveConfirmHost"
import { closeRemoveConfirm, openRemoveConfirm } from "./removeConfirmOpen"

const { postRemoveSession } = vi.hoisted(() => ({ postRemoveSession: vi.fn() }))
vi.mock("../agents/sessionWrites", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../agents/sessionWrites")>()),
  postRemoveSession,
}))

afterEach(() => {
  closeRemoveConfirm()
  cleanup()
  postRemoveSession.mockReset()
  postRemoveSession.mockResolvedValue({})
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
  renderThemed(<RemoveConfirmHost />)
  act(() => openRemoveConfirm({ session: live, onRemoved: () => {} }))
  const keep = screen.getByRole("button", { name: "Keep" })
  expect(document.activeElement).toBe(keep)
  fireEvent.click(keep)
  expect(screen.queryByText(/Its session is deleted/)).toBeNull()
  expect(postRemoveSession).not.toHaveBeenCalled()
})

test("Remove calls the writer, then the opener's callback", async () => {
  const onRemoved = vi.fn()
  renderThemed(<RemoveConfirmHost />)
  act(() => openRemoveConfirm({ session: live, onRemoved }))
  fireEvent.click(screen.getByRole("button", { name: "Remove" }))
  await waitFor(() => expect(onRemoved).toHaveBeenCalled())
  expect(postRemoveSession).toHaveBeenCalledExactlyOnceWith({ id: "s1", state: rowToken(live) })
})
