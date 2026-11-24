import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { HoldQueuedText, ReasonNeedsYes, type Workspace } from "../generated/server"
import { session } from "../agents/agentsTestKit"
import { rowToken } from "../design/open"
import { closeConfirm } from "../actions/confirmRequest"
import { ConfirmHost } from "../shell/ConfirmHost"
import { renderThemed } from "../ui/render"
import { WideContext } from "../useWide"
import { ItemMenu, postHold, type ItemRef, type ItemWriters } from "./itemActions"

const { postRemoveSession } = vi.hoisted(() => ({ postRemoveSession: vi.fn() }))
vi.mock("../agents/sessionWrites", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../agents/sessionWrites")>()),
  postRemoveSession,
}))

const { chatRead, chatRemove } = vi.hoisted(() => ({ chatRead: vi.fn(), chatRemove: vi.fn() }))
vi.mock("../design/chatDrafts", async (importOriginal) => {
  const original = await importOriginal<typeof import("../design/chatDrafts")>()
  return {
    ...original,
    realChatDrafts: { ...original.realChatDrafts, read: chatRead, remove: chatRemove },
  }
})

const { readDraft, deleteDraft } = vi.hoisted(() => ({ readDraft: vi.fn(), deleteDraft: vi.fn() }))
vi.mock("../design/drafts", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../design/drafts")>()),
  readDraft,
  deleteDraft,
}))

afterEach(() => {
  cleanup()
  closeConfirm()
  chatRead.mockReset()
  chatRemove.mockReset()
  postRemoveSession.mockReset()
  readDraft.mockReset()
  deleteDraft.mockReset()
})

const workspaces: Workspace[] = [
  { name: "Default", default: true, members: 1 },
  { name: "Work", members: 2 },
]

const writers = (over: Partial<ItemWriters> = {}): ItemWriters => ({
  move: vi.fn(async () => ({})),
  hold: vi.fn(async () => ({})),
  unhold: vi.fn(async () => ({})),
  ...over,
})

const design: ItemRef = { key: "p:shop", name: "Shop", kind: "design", workspace: "Work" }

async function open(
  item: ItemRef,
  w: ItemWriters,
  props: { modal?: boolean; onRemoved?: () => void } = {},
) {
  renderThemed(
    <WideContext.Provider value={true}>
      <ItemMenu item={item} workspaces={workspaces} writers={w} {...props} />
      <ConfirmHost />
    </WideContext.Provider>,
  )
  fireEvent.click(screen.getByRole("button", { name: "More actions" }))
  await screen.findAllByRole("menuitem")
}

const labels = () => screen.getAllByRole("menuitem").map((i) => i.textContent)

test("a Design offers Move, Hold, Archive and Copy ID in group order, never Remove; Move lists every workspace with the current one marked and posts the pick", async () => {
  const w = writers()
  await open(design, w)
  expect(labels()).toEqual(["Move to workspace›", "Hold", "Archive", "Copy ID"])
  expect(screen.getAllByRole("separator").length).toBeGreaterThan(0)
  fireEvent.click(screen.getByRole("menuitem", { name: "Move to workspace" }))
  const work = await screen.findByRole("menuitem", { name: /Work/ })
  expect(work.getAttribute("aria-current")).toBe("true")
  expect(work.textContent).toContain("✓")
  const dflt = screen.getByRole("menuitem", { name: "Default" })
  expect(dflt.getAttribute("aria-current")).toBe("false")
  fireEvent.click(dflt)
  await waitFor(() => expect(w.move).toHaveBeenCalledWith("p:shop", "Default"))
})

test("a Default member lists Default as the current workspace", async () => {
  await open({ ...design, workspace: "Default" }, writers())
  fireEvent.click(screen.getByRole("menuitem", { name: "Move to workspace" }))
  expect(
    (await screen.findByRole("menuitem", { name: /Default/ })).getAttribute("aria-current"),
  ).toBe("true")
})

test("a queued epic's Hold is disabled with the reason", async () => {
  const w = writers()
  await open({ ...design, kind: "epic", queued: true }, w)
  const hold = screen.getByRole("menuitem", { name: /Hold/ })
  expect(hold.textContent).toContain(HoldQueuedText)
  expect(hold.getAttribute("aria-disabled") ?? hold.getAttribute("data-disabled")).not.toBeNull()
})

test("holding a Design with live sessions names them, then re-posts with yes", async () => {
  const hold = vi
    .fn<ItemWriters["hold"]>()
    .mockResolvedValueOnce({
      refused: true,
      reason: ReasonNeedsYes,
      sessions: ["scratch", "draft"],
    })
    .mockResolvedValueOnce({})
  await open(design, writers({ hold }))
  fireEvent.click(screen.getByRole("menuitem", { name: "Hold" }))
  expect((await screen.findByText(/scratch, draft/)).textContent).toContain(
    "ends its live sessions",
  )
  // The closing menu hides its siblings from the accessibility tree for a moment.
  expect(await screen.findByRole("alertdialog")).toBeTruthy()
  expect(hold).toHaveBeenCalledTimes(1)
  expect(screen.queryByText(/Hold and end sessions/)).toBeNull()
  expect(await screen.findByRole("button", { name: /^Cancel/ })).toBeTruthy()
  fireEvent.click(screen.getByRole("button", { name: /^Hold/ }))
  await waitFor(() => expect(hold).toHaveBeenLastCalledWith("p:shop", true))
})

test("the default writers send one target and unwrap its result", async () => {
  const fetchMock = vi.fn(
    async () =>
      new Response(
        JSON.stringify({
          held: [],
          results: [{ target: "p:shop", status: "refused", reason: "needs-yes", sessions: ["s1"] }],
        }),
        { status: 200 },
      ),
  )
  vi.stubGlobal("fetch", fetchMock)
  try {
    await expect(postHold("p:shop", true)).resolves.toMatchObject({
      refused: true,
      reason: ReasonNeedsYes,
      sessions: ["s1"],
    })
    const init = (fetchMock.mock.calls[0] as unknown[])[1] as RequestInit
    expect(JSON.parse(String(init.body))).toEqual({ targets: ["p:shop"], yes: true })
  } finally {
    vi.unstubAllGlobals()
  }
})

test("Take off hold posts with no confirmation", async () => {
  const w = writers()
  await open({ ...design, held: true }, w)
  fireEvent.click(screen.getByRole("menuitem", { name: "Take off hold" }))
  await waitFor(() => expect(w.unhold).toHaveBeenCalledWith("p:shop"))
})

test("a chat can be moved but not held", async () => {
  await open({ ...design, kind: "chat" }, writers())
  expect(labels()).toContain("Move to workspace›")
  expect(labels()).not.toContain("Hold")
})

test("a confirm from a modal menu (the Jump palette) is a dialog, with y and n hints", async () => {
  await open({ ...chat("exited") }, writers(), { modal: true })
  fireEvent.click(screen.getByRole("menuitem", { name: "Remove" }))
  const dialog = await screen.findByRole("dialog")
  expect(dialog.textContent).toMatch(/Cancel\s*n/)
  expect(dialog.textContent).toMatch(/Remove\s*y/)
})

const chat = (state: "idle" | "exited"): ItemRef => ({
  ...design,
  kind: "chat",
  key: "s3",
  name: "talk",
  session: session("s3", "p", { name: "talk", status: { state } }),
})

test("a live chat's menu is in registry order; Stop asks, then posts the stop", async () => {
  const stop = vi.fn(async () => ({}))
  await open(chat("idle"), writers({ stop }))
  expect(labels()).toEqual([
    "Rename",
    "Move to workspace›",
    "Switch model",
    "Promote to Design",
    "Stop",
    "Copy ID",
    "Remove",
  ])
  fireEvent.click(screen.getByRole("menuitem", { name: "Stop" }))
  fireEvent.click(await screen.findByRole("button", { name: /^Stop/ }))
  await waitFor(() => expect(stop).toHaveBeenCalledWith(expect.objectContaining({ id: "s3" })))
})

test("a Fix agent row lists Stop, Copy ID and Remove (registry group order)", async () => {
  const item = chat("idle")
  await open({ ...item, session: { ...item.session!, fix_of: "p:x/01" } }, writers())
  expect(labels()).toEqual(["Stop", "Copy ID", "Remove"])
})

test("an exited chat has no Stop; Remove asks first, then removes and reports", async () => {
  postRemoveSession.mockResolvedValue({})
  const item = chat("exited")
  const onRemoved = vi.fn()
  await open(item, writers(), { onRemoved })
  expect(labels()).not.toContain("Stop")
  expect(labels().at(-1)).toBe("Remove")
  fireEvent.click(screen.getByRole("menuitem", { name: "Remove" }))
  expect((await screen.findByText("Remove talk? Its session is deleted.")).textContent).toBeTruthy()
  expect(postRemoveSession).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole("button", { name: /^Remove/ }))
  await waitFor(() =>
    expect(postRemoveSession).toHaveBeenCalledExactlyOnceWith({
      id: "s3",
      state: rowToken(item.session!),
    }),
  )
  await waitFor(() => expect(onRemoved).toHaveBeenCalled())
})

test("a queue agent's menu holds Copy ID only", async () => {
  const item = chat("idle")
  await open(
    { ...item, session: { ...item.session!, queue: true } as ItemRef["session"] },
    writers(),
  )
  expect(labels()).toEqual(["Copy ID"])
})

test("Archive shows a spinner with the confirm off while the dry run runs, then the sessions and the trunk warning, then archives with yes", async () => {
  let finish: (r: { sessions: string[]; on_trunk: boolean }) => void = () => {}
  const archive = vi.fn((_key: string, opts: { dryRun?: boolean; yes?: boolean }) =>
    opts.dryRun
      ? new Promise<{ sessions: string[]; on_trunk: boolean }>((r) => (finish = r))
      : Promise.resolve({}),
  )
  await open({ ...design, kind: "epic" }, writers({ archive }))
  fireEvent.click(screen.getByRole("menuitem", { name: "Archive" }))
  const dialog = await screen.findByRole("alertdialog")
  const confirm = screen.getByRole("button", { name: /^Archive/ })
  expect((confirm as HTMLButtonElement).disabled).toBe(true)
  expect(archive).toHaveBeenCalledWith("p:shop", { dryRun: true })
  finish({ sessions: ["agent-1"], on_trunk: false })
  await waitFor(() => expect(dialog.textContent).toContain("Live sessions will stop: agent-1."))
  expect(dialog.textContent).toContain("is not merged to trunk")
  expect((confirm as HTMLButtonElement).disabled).toBe(false)
  fireEvent.click(confirm)
  await waitFor(() => expect(archive).toHaveBeenCalledWith("p:shop", { yes: true }))
})

test("Archive refused by the dry run opens no confirm", async () => {
  const archive = vi.fn(async () => ({ refused: true, message: "the epic is queued" }))
  await open({ ...design, kind: "epic" }, writers({ archive }))
  fireEvent.click(screen.getByRole("menuitem", { name: "Archive" }))
  await waitFor(() => expect(archive).toHaveBeenCalled())
  await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull())
})

test("a chat has no Archive item", async () => {
  await open(
    { key: "s1", name: "Chat", kind: "chat", workspace: "Work", session: session("s1", "p") },
    writers(),
  )
  expect(labels()).not.toContain("Archive")
})

test("a draft offers Rename, Move to workspace and Remove in that order, and posts the move with its key", async () => {
  const w = writers()
  const draft: ItemRef = {
    key: "draft/d1",
    name: "Checkout",
    kind: "draft",
    workspace: "Work",
    draft: {
      id: "d1",
      label: "Checkout",
      project: "p",
      slug: "checkout",
      edited_at: "2026-01-02T00:00:00Z",
      workspace: "Work",
      member_key: "draft/d1",
    },
  }
  await open(draft, w)
  expect(labels()).toEqual(["Rename", "Move to workspace›", "Remove"])
  fireEvent.click(screen.getByRole("menuitem", { name: "Move to workspace" }))
  fireEvent.click(await screen.findByRole("menuitem", { name: "Default" }))
  await waitFor(() => expect(w.move).toHaveBeenCalledExactlyOnceWith("draft/d1", "Default"))
})

test("a chat draft offers Rename, Move to workspace and Remove, and posts the move with its key", async () => {
  const w = writers()
  await open({ key: "chat-draft/c1", name: "Chat", kind: "chat-draft", workspace: "Work" }, w)
  expect(labels()).toEqual(["Rename", "Move to workspace›", "Remove"])
  fireEvent.click(screen.getByRole("menuitem", { name: "Move to workspace" }))
  fireEvent.click(await screen.findByRole("menuitem", { name: "Default" }))
  await waitFor(() => expect(w.move).toHaveBeenCalledExactlyOnceWith("chat-draft/c1", "Default"))
})

const draftRow: ItemRef = {
  key: "draft/d1",
  name: "Checkout",
  kind: "draft",
  workspace: "Work",
  draft: {
    id: "d1",
    label: "Checkout",
    project: "p",
    slug: "",
    edited_at: "2026-01-02T00:00:00Z",
    workspace: "Work",
    member_key: "draft/d1",
  },
}
const stored = (idea: string) => ({
  kind: "ok",
  draft: { id: "d1", idea, slug: "", images: [] },
})

async function removeDraftRow(item: ItemRef = draftRow) {
  const onRemoved = vi.fn()
  await open(item, writers(), { onRemoved })
  fireEvent.click(screen.getByRole("menuitem", { name: "Remove" }))
  return onRemoved
}

test("Remove deletes an empty draft with no confirm", async () => {
  readDraft.mockResolvedValue(stored("  "))
  deleteDraft.mockResolvedValue({ kind: "deleted" })
  const onRemoved = await removeDraftRow()
  await waitFor(() => expect(onRemoved).toHaveBeenCalledTimes(1))
  expect(deleteDraft).toHaveBeenCalledExactlyOnceWith("d1")
  expect(screen.queryByRole("alertdialog")).toBeNull()
})

test.each([
  ["a draft with text", () => readDraft.mockResolvedValue(stored("build a form"))],
  ["a draft that cannot be read", () => readDraft.mockRejectedValue(new Error("down"))],
])("Remove asks before deleting %s", async (_name, arrange) => {
  arrange()
  await removeDraftRow()
  const dialog = await screen.findByRole("alertdialog")
  expect(dialog.textContent).toContain("Its prompt and images are deleted.")
  expect(await screen.findByRole("button", { name: /^Cancel/ })).toBeTruthy()
  expect(deleteDraft).not.toHaveBeenCalled()
})

const chatDraftRow: ItemRef = {
  key: "chat-draft/c1",
  name: "Explore",
  kind: "chat-draft",
  workspace: "Work",
}

test("Remove deletes an empty chat draft through the chat drafts client", async () => {
  chatRead.mockResolvedValue({ kind: "ok", draft: { id: "c1", brief: " ", images: [] } })
  chatRemove.mockResolvedValue({ kind: "deleted" })
  const onRemoved = await removeDraftRow(chatDraftRow)
  await waitFor(() => expect(onRemoved).toHaveBeenCalledTimes(1))
  expect(chatRemove).toHaveBeenCalledExactlyOnceWith("c1")
  expect(deleteDraft).not.toHaveBeenCalled()
})

test("Remove asks before deleting a chat draft with a brief", async () => {
  chatRead.mockResolvedValue({
    kind: "ok",
    draft: { id: "c1", brief: "look at caching", images: [] },
  })
  await removeDraftRow(chatDraftRow)
  await screen.findByRole("alertdialog")
  expect(chatRemove).not.toHaveBeenCalled()
})

test("Remove asks before deleting a chat draft with an image and no brief", async () => {
  chatRead.mockResolvedValue({ kind: "ok", draft: { id: "c1", brief: "", images: ["i1"] } })
  await removeDraftRow(chatDraftRow)
  await screen.findByRole("alertdialog")
  expect(chatRemove).not.toHaveBeenCalled()
})
