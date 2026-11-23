import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { renderThemed } from "../ui/render"
import {
  BlockingDraft,
  BlockingStarting,
  type SessionAttachEvent,
  type SessionInfo,
} from "../generated/server"
import { BlockingCard, isBlockingCard, type PostRetry } from "./BlockingCard"
import type { SubscribeOwner, TakeOver } from "./inputOwner"

afterEach(cleanup)

const session = (blocking?: string): SessionInfo =>
  ({
    id: "s-a",
    status: { state: "idle" },
    unread: {},
    actionable: { gx_raised: true, reason: "the card reason", blocking },
  }) as SessionInfo

function open(blocking: string) {
  let emit: (ev: SessionAttachEvent) => void = () => {}
  const subscribe: SubscribeOwner = (_t, _c, onEvent) => {
    emit = onEvent
    return () => {}
  }
  const retry = vi.fn<PostRetry>(async () => ({}))
  const takeOver = vi.fn<TakeOver>(async () => ({}))
  const onDone = vi.fn()
  renderThemed(
    <BlockingCard
      session={session(blocking)}
      retry={retry}
      subscribe={subscribe}
      takeOver={takeOver}
      onDone={onDone}
    />,
  )
  return { retry, takeOver, onDone, emit: (ev: SessionAttachEvent) => act(() => emit(ev)) }
}

test.each([BlockingDraft, BlockingStarting])(
  "the %s card shows its reason and Retry",
  async (b) => {
    const { retry, onDone } = open(b)
    expect(screen.getByText("the card reason")).toBeTruthy()
    fireEvent.click(screen.getByRole("button", { name: "Retry" }))
    await waitFor(() => expect(onDone).toHaveBeenCalled())
    expect(retry).toHaveBeenCalledWith({ id: "s-a" })
  },
)

test("the draft card names another owner and Take over runs the route", () => {
  const { emit, takeOver } = open(BlockingDraft)
  emit({ type: "owner" })
  expect(screen.getByText("Input: another client")).toBeTruthy()
  fireEvent.click(screen.getByRole("button", { name: "Take over" }))
  expect(takeOver).toHaveBeenCalledWith("s-a", expect.any(String))
})

test("the draft card offers no Take over to the owner itself", () => {
  const { emit } = open(BlockingDraft)
  emit({ type: "owner", self: true })
  expect(screen.getByText("Input: this client")).toBeTruthy()
  expect(screen.queryByRole("button", { name: "Take over" })).toBeNull()
})

test("the starting card names no owner and offers no Take over", () => {
  const { emit } = open(BlockingStarting)
  emit({ type: "owner" })
  expect(screen.queryByText(/Input:/)).toBeNull()
  expect(screen.queryByRole("button", { name: "Take over" })).toBeNull()
})

test("only a gx-raised card with a blocking kind is a blocking card", () => {
  expect(isBlockingCard(session(BlockingDraft))).toBe(true)
  expect(isBlockingCard(session())).toBe(false)
})
