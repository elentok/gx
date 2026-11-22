import { cleanup, fireEvent, screen } from "@testing-library/react"
import { afterEach, expect, test } from "vitest"
import {
  SessionNoticeNoComposerMarker,
  SessionNoticeUntrustedRead,
  type SessionInfo,
} from "../generated/server"
import { renderThemed } from "../ui/render"
import { noticeText, SessionNotices } from "./SessionNotices"

afterEach(cleanup)

const session = (notices: string[]): SessionInfo => ({
  id: "s-a",
  project: "gx",
  dir: "/x",
  harness: "claude",
  status: { state: "idle" },
  status_changed_at: new Date().toISOString(),
  run_started_at: new Date().toISOString(),
  unread: {},
  actionable: {},
  auto_advance: true,
  auto_queue: false,
  notices,
})

test("each kind shows its own fixed text", () => {
  expect(noticeText[SessionNoticeUntrustedRead]).not.toBe(noticeText[SessionNoticeNoComposerMarker])
  renderThemed(
    <SessionNotices
      session={session([SessionNoticeUntrustedRead, SessionNoticeNoComposerMarker])}
    />,
  )
  expect(screen.getByText(noticeText[SessionNoticeUntrustedRead])).toBeTruthy()
  expect(screen.getByText(noticeText[SessionNoticeNoComposerMarker])).toBeTruthy()
})

test("dismiss removes only that notice", () => {
  renderThemed(
    <SessionNotices
      session={session([SessionNoticeUntrustedRead, SessionNoticeNoComposerMarker])}
    />,
  )
  fireEvent.click(screen.getAllByRole("button", { name: "Dismiss" })[0])
  expect(screen.queryByText(noticeText[SessionNoticeUntrustedRead])).toBeNull()
  expect(screen.getByText(noticeText[SessionNoticeNoComposerMarker])).toBeTruthy()
})

test("no notices, or an unknown kind, shows nothing", () => {
  renderThemed(<SessionNotices session={session(["from-the-future"])} />)
  expect(screen.queryByRole("status")).toBeNull()
})
