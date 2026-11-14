import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, test, vi } from "vitest"
import { session } from "../agents/agentsTestKit"
import type { DraftResult } from "../design/drafts"
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

describe("a draft request", () => {
  const target = { id: "d1", label: "Checkout", empty: false }
  const openDraft = (empty: boolean) => {
    const remove = vi.fn(async () => ({ kind: "deleted" }) as DraftResult)
    const onRemoved = vi.fn()
    renderThemed(<RemoveConfirmHost />)
    act(() => openRemoveConfirm({ draft: { ...target, empty }, onRemoved, remove }))
    return { remove, onRemoved }
  }

  test("a non-empty draft asks; Keep removes nothing", () => {
    const { remove } = openDraft(false)
    expect(screen.getByText("Remove Checkout? Its prompt and images are deleted.")).toBeTruthy()
    fireEvent.click(screen.getByRole("button", { name: "Keep" }))
    expect(screen.queryByText(/Its prompt and images/)).toBeNull()
    expect(remove).not.toHaveBeenCalled()
  })

  test("Remove deletes the draft and calls onRemoved", async () => {
    const { remove, onRemoved } = openDraft(false)
    fireEvent.click(screen.getByRole("button", { name: "Remove" }))
    await waitFor(() => expect(onRemoved).toHaveBeenCalledTimes(1))
    expect(remove).toHaveBeenCalledExactlyOnceWith("d1")
  })

  test("an empty draft is removed with no modal", async () => {
    const { remove, onRemoved } = openDraft(true)
    expect(screen.queryByRole("button", { name: "Keep" })).toBeNull()
    await waitFor(() => expect(onRemoved).toHaveBeenCalledTimes(1))
    expect(remove).toHaveBeenCalledExactlyOnceWith("d1")
  })
})
