import { afterEach, describe, expect, test, vi } from "vitest"
import { readRoundDraft, writeRoundDraft } from "../drafts"
import {
  createDraft,
  deleteDraft,
  getLastProject,
  readDraft,
  saveDraft,
  saveDraftFields,
  setLastProject,
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
const req = { id: "d1", idea: "x", revision: 1 }

const bodyOf = (f: ReturnType<typeof stub>) =>
  JSON.parse((f.mock.calls[0][1] as RequestInit).body as string)

test("the text save carries the revision and no slug or project", async () => {
  const f = stub({ draft })
  expect(await saveDraft(req)).toEqual({ kind: "ok", draft })
  expect(f.mock.calls[0][0]).toBe("/v1/designs/drafts/save")
  expect(bodyOf(f)).toEqual(req)
})

test("saveFields writes the slug and project through the target route with no revision", async () => {
  const f = stub({ draft })
  expect(await saveDraftFields({ id: "d1", project: "gx", slug: "s" })).toEqual({
    kind: "ok",
    draft,
  })
  expect(f).toHaveBeenCalledTimes(1)
  expect(f.mock.calls[0][0]).toBe("/v1/designs/drafts/target")
  expect(bodyOf(f)).toEqual({ id: "d1", project: "gx", slug: "s" })

  stub({ refused: true, reason: "draft-gone" })
  expect(await saveDraftFields({ id: "d1", project: "gx", slug: "" })).toEqual({ kind: "gone" })
})

test("createDraft saves an empty draft in the workspace and returns its id", async () => {
  const f = stub({ draft })
  expect(await createDraft({ workspace: "w1" })).toBe("d1")
  expect(bodyOf(f)).toEqual({ idea: "", revision: 0, workspace: "w1" })
  stub({ deleted: true })
  await expect(createDraft()).rejects.toThrow()
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

test("last project", async () => {
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
