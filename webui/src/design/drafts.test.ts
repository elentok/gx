import { afterEach, describe, expect, test, vi } from "vitest"
import { readRoundDraft, writeRoundDraft } from "../drafts"
import {
  deleteDraft,
  draftLabel,
  getLastProject,
  listDrafts,
  readDraft,
  saveDraft,
  setLastProject,
  timeAgo,
} from "./drafts"

afterEach(() => vi.unstubAllGlobals())

const stub = (body: unknown, status = 200) => {
  const fetchMock = vi.fn(
    async (..._: unknown[]) =>
      new Response(typeof body === "string" ? body : JSON.stringify(body), { status }),
  )
  vi.stubGlobal("fetch", fetchMock)
  return fetchMock
}

const draft = {
  id: "d1",
  project: "gx",
  slug: "",
  idea: "x",
  images: [],
  revision: 2,
  edited_at: "2026-01-01T00:00:00Z",
}
const req = { id: "d1", project: "gx", slug: "", idea: "x", revision: 1 }

test("label: the shipped label, or a blank new draft", () => {
  expect(draftLabel({ label: "my-slug" })).toBe("my-slug")
  expect(draftLabel()).toBe("New draft")
})

const now = new Date("2026-01-10T12:00:00Z")
test.each([
  ["2026-01-10T11:59:40Z", "just now"],
  ["2026-01-10T12:00:30Z", "just now"],
  ["2026-01-10T11:59:00Z", "1 min ago"],
  ["2026-01-10T11:01:00Z", "59 min ago"],
  ["2026-01-10T11:00:00Z", "1 h ago"],
  ["2026-01-09T13:00:00Z", "23 h ago"],
  ["2026-01-09T12:00:00Z", "1 d ago"],
  ["2026-01-03T12:00:00Z", "7 d ago"],
])("time: %s", (at, want) => {
  expect(timeAgo(at, now)).toBe(want)
})

test("save writes the slug and project through the target route, then the text without them", async () => {
  const f = stub({ draft })
  expect(await saveDraft({ ...req, slug: "s", keep_edited_at: true })).toEqual({
    kind: "ok",
    draft,
  })
  expect(f.mock.calls.map((c) => c[0])).toEqual([
    "/v1/designs/drafts/target",
    "/v1/designs/drafts/save",
  ])
  const body = (i: number) => JSON.parse((f.mock.calls[i][1] as RequestInit).body as string)
  expect(body(0)).toEqual({ id: "d1", project: "gx", slug: "s", keep_edited_at: true })
  expect(body(1)).toEqual({ id: "d1", idea: "x", revision: 1, keep_edited_at: true })

  // A draft that is gone stops at the target write; the text save is not sent.
  const g = stub({ refused: true, reason: "draft-gone" })
  expect(await saveDraft(req)).toEqual({ kind: "gone" })
  expect(g).toHaveBeenCalledTimes(1)

  // A new draft is created by the text save first, then given its slug and project.
  const h = stub({ draft })
  await saveDraft({ ...req, id: undefined })
  expect(h.mock.calls.map((c) => c[0])).toEqual([
    "/v1/designs/drafts/save",
    "/v1/designs/drafts/target",
  ])
  expect(JSON.parse((h.mock.calls[1][1] as RequestInit).body as string).id).toBe("d1")
})

test("save maps ok, deleted, conflict and gone to typed results", async () => {
  stub({ draft })
  expect(await saveDraft(req)).toEqual({ kind: "ok", draft })

  stub({ deleted: true })
  expect(await saveDraft(req)).toEqual({ kind: "deleted" })
  stub({ refused: true, reason: "revision-conflict" })
  expect(await saveDraft(req)).toEqual({ kind: "conflict" })
  stub({ refused: true, reason: "draft-gone" })
  expect(await saveDraft(req)).toEqual({ kind: "gone" })
})

test("read and delete use their routes and map refusals", async () => {
  const f = stub({ refused: true, reason: "draft-gone" })
  expect(await readDraft("a b")).toEqual({ kind: "gone" })
  expect(f.mock.calls[0][0]).toBe("/v1/designs/drafts/read?id=a%20b")

  const g = stub({ deleted: true })
  expect(await deleteDraft("d1")).toEqual({ kind: "deleted" })
  expect(g.mock.calls[0][0]).toBe("/v1/designs/drafts/delete")
  expect(JSON.parse((g.mock.calls[0][1] as RequestInit).body as string)).toEqual({ id: "d1" })
})

test("list and last project", async () => {
  stub([{ id: "d1" }])
  expect(await listDrafts()).toEqual([{ id: "d1" }])
  stub({ project: "gx" })
  expect(await getLastProject()).toBe("gx")
  const f = stub({ project: "other" })
  await setLastProject("other")
  expect(f.mock.calls[0][0]).toBe("/v1/designs/drafts/last-project")
  expect(JSON.parse((f.mock.calls[0][1] as RequestInit).body as string)).toEqual({
    project: "other",
  })
})

test("a 500 body throws its text", async () => {
  stub("disk full\n", 500)
  await expect(saveDraft(req)).rejects.toThrow("disk full")
  await expect(listDrafts()).rejects.toThrow("disk full")
  await expect(setLastProject("x")).rejects.toThrow("disk full")
})

describe.each([
  ["round", "round-7"],
  ["claude-dialog", "claude-dialog:abc123"],
  ["codex-question", "codex-question:thread-1:call-2"],
  ["permission", "permission:tool-use-9"],
  ["ticket-ask", "ticket-ask:gx/epic/12:3"],
])("round draft for a %s id", (_, id) => {
  afterEach(() => localStorage.clear())

  test("kept while the id stays the same", () => {
    writeRoundDraft("s1", id, { answers: { "1": { choice: "a" } } })
    expect(readRoundDraft("s1", id)).toEqual({ answers: { "1": { choice: "a" } } })
  })

  test("dropped when the id changes", () => {
    writeRoundDraft("s1", id, { answers: { "1": { choice: "a" } } })
    expect(readRoundDraft("s1", `${id}-next`)).toBeNull()
  })
})
