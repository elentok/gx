import { act, renderHook, waitFor } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { session } from "../agents/agentsTestKit"
import {
  ActionStartOver,
  HoldQueuedText,
  ReasonNeedsYes,
  type Workspace,
} from "../generated/server"
import { rowToken } from "../design/open"
import { registerDraftPage } from "../shell/draftPageActions"
import { ActionCancel, CANCEL_NOTICE, DISCARD_NOTICE } from "../ticket/recover"
import {
  closeConfirm,
  currentConfirm,
  type ConfirmBody,
  type SurfaceContext,
} from "./confirmRequest"
import type { Entity } from "./entity"
import { isDraftEmpty } from "./draftEmpty"
import { useActions, type ActionContext, type ActionWriters } from "./useActions"

const { openRename, openModelPicker, showToast, removeToast, forgetLast } = vi.hoisted(() => ({
  openRename: vi.fn(),
  openModelPicker: vi.fn(),
  showToast: vi.fn(),
  removeToast: vi.fn(),
  forgetLast: vi.fn(),
}))
vi.mock("../shell/renameOpen", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../shell/renameOpen")>()),
  openRename,
}))
vi.mock("../shell/modelPickerOpen", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../shell/modelPickerOpen")>()),
  openModelPicker,
}))
vi.mock("../agents/lastAgent", () => ({ forgetLast }))
vi.mock("../ui/toaster", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../ui/toaster")>()),
  showToast,
  toaster: { remove: removeToast },
}))

afterEach(() => {
  closeConfirm()
  vi.clearAllMocks()
})

const workspaces: Workspace[] = [
  { name: "Default", default: true, members: 1 },
  { name: "Work", members: 2 },
]

const modal: SurfaceContext = { mode: "modal" }

const epic: Entity = { kind: "epic", key: "demo:mixed-epic", name: "Mixed", workspace: "Work" }
const design: Entity = {
  kind: "design",
  key: "demo:design-epic",
  name: "Design",
  workspace: "Work",
}

function setup(
  entity: Entity | undefined,
  over: Partial<ActionWriters> = {},
  ctx: Partial<ActionContext> = {},
) {
  const writers = {
    move: vi.fn(async () => ({})),
    hold: vi.fn(async () => ({})),
    unhold: vi.fn(async () => ({})),
    archive: vi.fn(async () => ({})),
    copy: vi.fn(async () => true),
    ...over,
  } satisfies Partial<ActionWriters>
  const hook = renderHook(() => useActions(entity, { surface: modal, workspaces, writers, ...ctx }))
  const act1 = (id: string) => {
    const a = hook.result.current.get(id)
    if (!a) throw new Error(`no action ${id}`)
    return a
  }
  return { writers, hook, act1, labels: () => hook.result.current.actions.map((a) => a.label) }
}

test("a Design offers Move and Hold, never Remove, in registry order with Hold or Take off hold one at a time", () => {
  expect(setup(design).labels()).toEqual([
    "Open design epic",
    "Move to workspace",
    "New session",
    "Hold",
    "Archive",
    "Copy ID",
    "Delete design",
  ])
  const held = setup({ ...design, held: true }).labels()
  expect(held).toContain("Take off hold")
  expect(held).not.toContain("Hold")
})

test("an epic, a chat, a draft and a ticket each offer their own actions", () => {
  expect(setup({ ...epic, prUrl: "http://pr" }).labels()).toEqual([
    "Open PR",
    "Move to workspace",
    "Hold",
    "Mark as merged",
    "Archive",
    "Copy ID",
  ])
  const chat: Entity = { kind: "chat", key: "c1", name: "Chat", session: session("c1", "p") }
  expect(setup(chat).labels()).toEqual([
    "Rename",
    "Move to workspace",
    "Switch model",
    "Promote to Design",
    "Stop",
    "Copy ID",
    "Remove",
  ])
  expect(setup({ kind: "design-draft", key: "d1", name: "Draft" }).labels()).toEqual([
    "Rename",
    "Move to workspace",
    "Remove",
  ])
  const ticket: Entity = {
    kind: "ticket",
    key: "p:one-offs/01",
    name: "T",
    serverActions: [ActionStartOver],
    status: "open",
  }
  expect(setup(ticket).labels()).toEqual(["Copy ID", "Start over", "Cancel run"])
})

test("Move lists every workspace with the current one marked and posts the entity key", async () => {
  const { act1, writers } = setup(design)
  const items = act1("move").items ?? []
  expect(items.map((i) => [i.label, !!i.current])).toEqual([
    ["Default", false],
    ["Work", true],
  ])
  act(() => items[0]?.run())
  await waitFor(() => expect(writers.move).toHaveBeenCalledWith("demo:design-epic", "Default"))
  const draft: Entity = { kind: "design-draft", key: "Work/d1", name: "Draft", workspace: "Work" }
  const d = setup(draft)
  act(() => d.act1("move").items?.[1]?.run())
  await waitFor(() => expect(d.writers.move).toHaveBeenCalledWith("Work/d1", "Work"))
})

test("Hold on live sessions asks once, and the confirm re-posts with yes", async () => {
  const hold = vi.fn(async (_k: string, yes?: boolean) =>
    yes ? {} : { refused: true, reason: ReasonNeedsYes, sessions: ["s1", "s2"] },
  )
  const { act1 } = setup({ ...epic, liveSessions: 2 }, { hold })
  act(() => act1("hold").run())
  await waitFor(() => expect(currentConfirm()).not.toBeNull())
  expect(hold).toHaveBeenCalledTimes(1)
  expect(currentConfirm()?.body.lines.join(" ")).toContain("s1, s2")
  act(() => currentConfirm()?.onConfirm())
  await waitFor(() => expect(hold).toHaveBeenLastCalledWith("demo:mixed-epic", true))
  expect(showToast).not.toHaveBeenCalled()
})

test("Hold on a queued epic is disabled with the reason; Take off hold posts with no request", async () => {
  const queued = setup({ ...epic, queued: true })
  expect(queued.act1("hold")).toMatchObject({ disabled: true, reason: HoldQueuedText })
  const { act1, writers } = setup({ ...epic, held: true })
  act(() => act1("unhold").run())
  await waitFor(() => expect(writers.unhold).toHaveBeenCalledWith("demo:mixed-epic"))
  expect(currentConfirm()).toBeNull()
})

test("Archive opens the confirm at once and prepares with a dry run; a refusal is handed to the host, and the confirm posts yes with the busy toast", async () => {
  const refused = setup(epic, {
    archive: vi.fn(async () => ({ refused: true, message: "not yet" })),
  })
  act(() => refused.act1("archive").run())
  expect(refused.writers.archive).not.toHaveBeenCalled()
  await expect(currentConfirm()?.prepare?.()).resolves.toMatchObject({
    refused: true,
    message: "not yet",
  })
  closeConfirm()

  const archive = vi.fn(async (_k: string, o: { dryRun?: boolean; yes?: boolean }) =>
    o.dryRun ? { sessions: ["s1"], on_trunk: false } : {},
  )
  const { act1 } = setup(epic, { archive })
  act(() => act1("archive").run())
  expect(currentConfirm()?.confirmLabel).toBe("Archive")
  const prepared = (await currentConfirm()?.prepare?.()) as ConfirmBody
  expect(archive).toHaveBeenCalledExactlyOnceWith("demo:mixed-epic", { dryRun: true })
  expect(prepared.prepared).toMatchObject({ sessions: ["s1"], on_trunk: false })
  expect(prepared.lines.join(" ")).toContain("Live sessions will stop: s1.")
  expect(prepared.warnings?.[0]).toContain("not merged to trunk")
  act(() => currentConfirm()?.onConfirm())
  await waitFor(() => expect(archive).toHaveBeenLastCalledWith("demo:mixed-epic", { yes: true }))
  expect(showToast).toHaveBeenCalledWith(expect.objectContaining({ title: "Archiving Mixed…" }))
  await waitFor(() => expect(removeToast).toHaveBeenCalledWith("item-archive-busy"))
  await waitFor(() =>
    expect(showToast).toHaveBeenCalledWith({ type: "success", title: "Archived Mixed" }),
  )
})

test("Stop always asks, also for a Design session, and sends the row token", async () => {
  const s = session("d1", "p")
  const stop = vi.fn(async () => ({}))
  const { act1 } = setup({ ...design, surface: "page", session: s }, { stop })
  act(() => act1("stop").run())
  expect(currentConfirm()?.confirmLabel).toBe("Stop")
  expect(stop).not.toHaveBeenCalled()
  act(() => currentConfirm()?.onConfirm())
  await waitFor(() => expect(stop).toHaveBeenCalledWith({ id: "d1", state: rowToken(s) }))
  const two = setup({ ...design, surface: "row", liveSessions: 2, session: s })
  expect(two.labels()).not.toContain("Stop")
})

test("Remove asks to stop and remove a live chat, only remove an exited one, then forgets and navigates", async () => {
  const remove = vi.fn(async () => ({}))
  const onRemoved = vi.fn()
  const live = session("c1", "p", { name: "Live" })
  const a = setup(
    { kind: "chat", key: "c1", name: "Live", session: live },
    { remove },
    { onRemoved },
  )
  act(() => a.act1("remove").run())
  expect(currentConfirm()?.body.lines[0]).toContain("Stop and remove Live")
  act(() => currentConfirm()?.onConfirm())
  await waitFor(() => expect(onRemoved).toHaveBeenCalled())
  expect(remove).toHaveBeenCalledWith({ id: "c1", state: rowToken(live) })
  expect(forgetLast).toHaveBeenCalledWith("c1")
  closeConfirm()
  const done = session("c2", "p", { name: "Done", status: { state: "exited" } })
  const b = setup({ kind: "chat", key: "c2", name: "Done", session: done })
  act(() => b.act1("remove").run())
  expect(currentConfirm()?.body.lines[0]).toBe("Remove Done? Its session is deleted.")
})

test("Remove on an empty draft or chat draft deletes at once; a full or unreadable one asks", async () => {
  for (const kind of ["design-draft", "chat-draft"] as const) {
    const deleteDraft = vi.fn(async () => ({ kind: "ok" }) as never)
    const deleteChatDraft = vi.fn(async () => ({ kind: "deleted" }) as never)
    const { act1 } = setup(
      { kind, key: "w/d1", name: "D", draftEmpty: true },
      { deleteDraft, deleteChatDraft },
      { draft: { id: "d1", slug: "", project: "" } },
    )
    act(() => act1("remove").run())
    const used = kind === "chat-draft" ? deleteChatDraft : deleteDraft
    await waitFor(() => expect(used).toHaveBeenCalledWith("d1"))
    expect(kind === "chat-draft" ? deleteDraft : deleteChatDraft).not.toHaveBeenCalled()
    expect(currentConfirm()).toBeNull()
  }
  const full = setup({ kind: "design-draft", key: "w/d1", name: "D", draftEmpty: false })
  act(() => full.act1("remove").run())
  await waitFor(() => expect(currentConfirm()).not.toBeNull())
  expect(currentConfirm()?.dismissLabel).toBe("Cancel")
  expect(currentConfirm()?.body.lines[0]).toContain("Its prompt and images are deleted.")
  closeConfirm()
  const readDraft = vi.fn(async () => {
    throw new Error("gone")
  })
  const unreadable = setup({ kind: "design-draft", key: "w/d1", name: "D" }, { readDraft })
  act(() => unreadable.act1("remove").run())
  await waitFor(() => expect(currentConfirm()).not.toBeNull())
})

test("Rename on the draft's own page focuses the title; elsewhere it opens the rename form", () => {
  const draft = { id: "d1", slug: "s", project: "p" }
  const entity: Entity = { kind: "design-draft", key: "w/d1", name: "D" }
  const rename = vi.fn()
  const off = registerDraftPage({
    rename,
    remove: vi.fn(),
    isEmpty: () => true,
    focusPrompt: vi.fn(),
  })
  const own = setup(entity, {}, { draft, surface: { mode: "modal", page: "w/d1" } })
  act(() => own.act1("rename").run())
  expect(rename).toHaveBeenCalled()
  expect(openRename).not.toHaveBeenCalled()
  const other = setup(entity, {}, { draft })
  act(() => other.act1("rename").run())
  expect(openRename).toHaveBeenCalledWith({ draft })
  off()
})

test("Copy ID writes the address and toasts it, from a menu and from a chord surface", async () => {
  for (const [entity, key] of [
    [epic, "demo:mixed-epic"],
    [design, "demo:design-epic"],
  ] as const) {
    for (const surface of [modal, { mode: "anchor", anchor: document.body } as SurfaceContext]) {
      const { act1, writers } = setup(entity, {}, { surface })
      act(() => act1("copy-id").run())
      await waitFor(() => expect(writers.copy).toHaveBeenCalledWith(key))
      await waitFor(() =>
        expect(showToast).toHaveBeenCalledWith({ type: "success", title: `Copied ${key}` }),
      )
      vi.clearAllMocks()
    }
  }
})

test("Switch model opens the picker; Promote posts the promote prompt", async () => {
  const s = session("c1", "p")
  const post = vi.fn(async () => ({}))
  const chat: Entity = { kind: "chat", key: "c1", name: "Chat", session: s }
  const { act1 } = setup(chat, { post }, { promote: { lastMessage: "m9", connected: true } })
  act(() => act1("switch-model").run())
  expect(openModelPicker).toHaveBeenCalledWith({ session: s })
  act(() => act1("promote").run())
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith({ id: "c1", text: "", last_message: "m9", promote: true }),
  )
})

test("Start over asks for a reason and refuses an empty one; Cancel run has its own labels", async () => {
  const recover = vi.fn(async () => ({}))
  const ticket: Entity = {
    kind: "ticket",
    key: "p:one-offs/01",
    name: "T",
    serverActions: [ActionStartOver],
    status: "open",
  }
  const { act1 } = setup(ticket, { recover }, { state: "tok", tooltip: "Throw it away." })
  act(() => act1("start-over").run())
  const req = currentConfirm()
  expect(req).toMatchObject({ title: "Start over", confirmLabel: "Start over" })
  expect(req?.body.input).toEqual({
    label: "Reason",
    required: true,
    name: "recovery-reason",
    placeholder: "Why start over?",
  })
  expect(req?.body.lines).toEqual(["Throw it away.", DISCARD_NOTICE])
  await act(async () => void (await req?.onConfirm("  ")))
  expect(recover).not.toHaveBeenCalled()
  await act(async () => void (await req?.onConfirm(" bad base ")))
  expect(recover).toHaveBeenCalledWith(ActionStartOver, {
    address: "p:one-offs/01",
    state: "tok",
    reason: "bad base",
  })
  act(() => act1("cancel-run").run())
  expect(currentConfirm()).toMatchObject({
    title: "Cancel run of T",
    confirmLabel: "Cancel run",
    dismissLabel: "Cancel",
    body: { lines: [CANCEL_NOTICE] },
  })
  await act(async () => void (await currentConfirm()?.onConfirm()))
  expect(recover).toHaveBeenLastCalledWith(ActionCancel, { address: "p:one-offs/01", state: "tok" })
})

test("a confirm's write resolves with the refusal so the popover can show it", async () => {
  const recover = vi.fn(async () => ({ refused: true, message: "The pause moved on." }))
  const ticket: Entity = {
    kind: "ticket",
    key: "p:one-offs/01",
    name: "T",
    serverActions: [ActionStartOver],
    status: "open",
  }
  const { act1 } = setup(ticket, { recover })
  act(() => act1("start-over").run())
  expect(await currentConfirm()?.onConfirm("why")).toMatchObject({ refused: true })
})

test("while a recovery write is pending, Start over and Cancel run are disabled", async () => {
  let release: () => void = () => {}
  const recover = vi.fn(() => new Promise<object>((r) => (release = () => r({}))))
  const ticket: Entity = {
    kind: "ticket",
    key: "p:one-offs/01",
    name: "T",
    serverActions: [ActionStartOver],
    status: "open",
  }
  const { act1, hook } = setup(ticket, { recover })
  expect(act1("start-over").disabled).toBe(false)
  act(() => act1("cancel-run").run())
  let sent: Promise<unknown> | undefined
  act(() => {
    sent = Promise.resolve(currentConfirm()?.onConfirm())
  })
  expect(act1("start-over").disabled).toBe(true)
  expect(act1("cancel-run").disabled).toBe(true)
  await act(async () => {
    release()
    await sent
  })
  expect(hook.result.current.get("start-over")?.disabled).toBe(false)
})

test("a confirm request carries the anchor for a menu surface and modal for a chord", () => {
  const anchor = document.body
  const menu = setup(
    { kind: "chat", key: "c1", name: "C", session: session("c1", "p") },
    {},
    { surface: { mode: "anchor", anchor, page: "p1" } },
  )
  act(() => menu.act1("stop").run())
  expect(currentConfirm()?.surface).toEqual({ mode: "anchor", anchor, page: "p1" })
  closeConfirm()
  const chord = setup({ kind: "chat", key: "c1", name: "C", session: session("c1", "p") })
  act(() => chord.act1("stop").run())
  expect(currentConfirm()?.surface).toEqual({ mode: "modal" })
})

test("an action is disabled while a write runs, and a refusal toasts as useWrite does", async () => {
  let release: () => void = () => {}
  const hold = vi.fn(
    () =>
      new Promise<{ refused?: boolean; message?: string }>((r) => {
        release = () => r({ refused: true, message: "nope" })
      }),
  )
  const { hook, act1 } = setup(epic, { hold })
  act(() => act1("hold").run())
  await waitFor(() => expect(hook.result.current.get("archive")?.disabled).toBe(true))
  act(() => release())
  await waitFor(() => expect(showToast).toHaveBeenCalledWith({ type: "error", title: "nope" }))
  expect(hook.result.current.get("archive")?.disabled).toBe(false)
})

test("isDraftEmpty: an empty draft is empty, one with content or unreadable is not", async () => {
  const ok = (draft: object) => async () => ({ kind: "ok", draft }) as never
  const readers = (design: object, chat: object) => ({
    readDraft: vi.fn(ok(design)),
    readChatDraft: vi.fn(ok(chat)),
  })
  const blankDesign = { idea: " ", slug: "", images: [] }
  const blankChat = { brief: "", images: [] }
  const empty = readers(blankDesign, blankChat)
  expect(await isDraftEmpty("design-draft", "d1", empty)).toBe(true)
  expect(await isDraftEmpty("chat-draft", "c1", empty)).toBe(true)
  expect(empty.readDraft).toHaveBeenCalledWith("d1")
  expect(empty.readChatDraft).toHaveBeenCalledWith("c1")
  const full = readers({ ...blankDesign, idea: "x" }, { ...blankChat, brief: "x" })
  expect(await isDraftEmpty("design-draft", "d1", full)).toBe(false)
  expect(await isDraftEmpty("chat-draft", "c1", full)).toBe(false)
  const broken = {
    ...empty,
    readDraft: vi.fn(async () => {
      throw new Error("gone")
    }),
  }
  expect(await isDraftEmpty("design-draft", "d1", broken)).toBe(false)
})
