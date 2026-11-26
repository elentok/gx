import { describe, expect, it } from "vitest"
import {
  CardKindDesign,
  CardKindImplementation,
  SectionImplementing,
  SectionOnHold,
  type ChatDraftRow,
  type DesignDraftRow,
  type EpicListRow,
  type SessionPlacement,
} from "../generated/server"
import { session } from "../agents/agentsTestKit"
import { buildSidebar, type Sidebar } from "./sidebarModel"
import { agentNavId, nextPrev, rowNavId, type NextPrevInput } from "./nextPrev"

const row = (address: string, over: Partial<EpicListRow> = {}): EpicListRow =>
  ({
    address,
    project: "p",
    name: address,
    kind: "loop",
    state: "idle",
    done: 1,
    total: 4,
    queued: false,
    queueable: false,
    workspace: "Default",
    card_kind: CardKindImplementation,
    section: SectionImplementing,
    status_words: "Idle",
    ...over,
  }) as EpicListRow

const place = (
  session: string,
  kind: string,
  extra: Partial<SessionPlacement> = {},
): SessionPlacement => ({ session, kind, ...extra })

// Sorted by name, so the model order is the order written here.
const model = buildSidebar({
  workspace: "Default",
  sort: "name",
  rows: [
    row("p:a-design", {
      name: "a",
      card_kind: CardKindDesign,
      addresses: ["p:a-design-impl", "p:a-design"],
    }),
    row("p:b-epic", { name: "b" }),
    row("p:h-held", { name: "h", section: SectionOnHold }),
  ],
  sessions: [
    session("n1", "p", { name: "n1" }),
    session("n2", "p", { name: "n2" }),
    session("c1", "p", { name: "c" }),
    session("o1", "p", { name: "o1" }),
    session("t1", "p", { name: "t1", flow: "triage", flow_key: "p" }),
  ],
  placements: [
    place("n1", "epic", { epic: "p:b-epic" }),
    place("n2", "epic", { epic: "p:b-epic" }),
    place("c1", "chat", { workspace: "Default" }),
    place("o1", "other"),
    place("t1", "other"),
  ],
  drafts: [
    { id: "dd", member_key: "draft:dd", label: "d", project: "p", workspace: "Default" },
  ] as DesignDraftRow[],
  chatDrafts: [
    { id: "cd", member_key: "chat-draft:cd", label: "e", project: "p", workspace: "Default" },
  ] as ChatDraftRow[],
})

const design = rowNavId("p:a-design")
const epic = rowNavId("p:b-epic")
const n1 = agentNavId("n1")
const n2 = agentNavId("n2")
const chat = rowNavId("c1")
const draft = rowNavId("draft:dd")
const chatDraft = rowNavId("chat-draft:cd")
const held = rowNavId("p:h-held")
const o1 = agentNavId("o1")
const triage = agentNavId("t1")
const all = [design, epic, n1, n2, chat, draft, chatDraft, held, o1, triage]

const at = (url: string, over: Partial<NextPrevInput> = {}) => {
  const [pathname, query] = url.split("?")
  return nextPrev({
    pathname,
    search: query ? `?${query}` : "",
    model,
    rendered: all,
    lastRow: undefined,
    ...over,
  })
}

const step = (url: string, over: Partial<NextPrevInput> = {}) => {
  const r = at(url, over)
  return [r.prev?.id, r.next?.id]
}

describe("nextPrev page ownership", () => {
  it.each([
    ["epic page", "/e/p:b-epic", epic],
    ["ticket page", "/t/p:b-epic/03", epic],
    ["answer page", "/answer/p:b-epic/03", epic],
    ["held epic page", "/e/p:h-held", held],
    ["Design page", "/d/p:a-design", design],
    ["epic page of the -impl epic", "/e/p:a-design-impl", design],
    ["ticket page of the -impl epic", "/t/p:a-design-impl/01", design],
    ["answer page of the -impl epic", "/answer/p:a-design-impl/01", design],
    ["Design page of the -impl epic", "/d/p:a-design-impl", design],
    ["Diff range", "/diff?project=p&base=x&tip=y&epic=p:b-epic", epic],
    ["Diff range of the -impl epic", "/diff?project=p&epic=p:a-design-impl", design],
    ["Diff range narrowed to a ticket", "/diff?project=p&epic=p:b-epic&ticket=p:b-epic/03", epic],
    ["chat worktree of a chat row", "/diff?project=p&worktree=chat:c1", chat],
    ["chat worktree of a nested agent", "/diff?project=p&worktree=chat:n2", n2],
    ["Design worktree", "/diff?project=p&worktree=design:a-design", design],
    ["ticket worktree", "/diff?project=p&worktree=ticket:p:b-epic/03", epic],
    [
      "ticket worktree of the -impl epic",
      "/diff?project=p&worktree=ticket:p:a-design-impl/1",
      design,
    ],
    ["worktree URL with another epic", "/diff?project=p&worktree=chat:c1&epic=p:b-epic", chat],
    ["chat page", "/agents/c1", chat],
    ["nested agent page", "/agents/n1", n1],
    ["Other agent page", "/agents/o1", o1],
    ["Triage page", "/triage/p", triage],
    ["Design draft page", "/drafts/dd", draft],
    ["chat draft page", "/chat-drafts/cd", chatDraft],
  ])("the %s belongs to its row", (_, url, owner) => {
    expect(at(url).pageRow).toBe(owner)
  })

  it.each([
    ["an archive page", "/archive/p:b-epic"],
    ["a file viewer page", "/files/p/b-epic/notes.md"],
    ["the Settings page", "/settings"],
    ["a Diff of two refs", "/diff?project=p&base=x&tip=y"],
    ["a worktree of an unknown chat with a known epic", "/diff?worktree=chat:zz&epic=p:b-epic"],
  ])("%s belongs to no row", (_, url) => {
    expect(at(url).pageRow).toBeUndefined()
  })

  it("takes the first of two owners from the top", () => {
    const twice: Sidebar = {
      ...model,
      other: [...model.other, { ...model.rows[1].agents[0], id: "again" }],
    }
    expect(at("/agents/n1", { model: twice }).pageRow).toBe(n1)
  })

  it("names the owner even when it is not rendered", () => {
    expect(at("/agents/n1", { rendered: [design] }).pageRow).toBe(n1)
  })
})

describe("nextPrev targets", () => {
  it("steps to the neighbours, with the model's links", () => {
    expect(at("/e/p:b-epic")).toEqual({
      pageRow: epic,
      prev: { id: design, href: "#/d/p:a-design" },
      next: { id: n1, href: "#/agents/n1" },
    })
  })

  it.each([
    ["the first row", "/d/p:a-design", [undefined, epic]],
    ["the last row", "/triage/p", [o1, undefined]],
  ])("does not wrap at %s", (_, url, want) => {
    expect(step(url)).toEqual(want)
  })

  it("ignores rendered values that are no row", () => {
    const rendered = ["section:other", design, "section:held", epic, "row:gone"]
    expect(step("/e/p:b-epic", { rendered })).toEqual([design, undefined])
    expect(step("/settings", { rendered })).toEqual([epic, design])
  })

  it("gives no targets when nothing is rendered", () => {
    expect(step("/e/p:b-epic", { rendered: [] })).toEqual([undefined, undefined])
  })

  it("never targets the rows of a collapsed row or a closed section", () => {
    const rendered = [design, epic, chat]
    expect(step("/e/p:b-epic", { rendered })).toEqual([design, chat])
    expect(step("/agents/c1", { rendered })).toEqual([epic, undefined])
  })

  it.each([
    ["between rendered rows", "/e/p:h-held", [chat, o1], [design, epic, chat, o1]],
    ["before all rendered rows", "/d/p:a-design", [undefined, chat], [chat, o1]],
    ["after all rendered rows", "/triage/p", [chat, undefined], [design, chat]],
    ["on a hidden nested agent", "/agents/n1", [epic, chat], [design, epic, chat]],
    ["on a second hidden nested agent", "/agents/n2", [epic, chat], [design, epic, chat]],
  ])("starts at the place of a hidden current row %s", (_, url, want, rendered) => {
    expect(step(url, { rendered })).toEqual(want)
  })
})

describe("nextPrev last row", () => {
  it("steps from the last row on a page with no row", () => {
    expect(step("/settings", { lastRow: chat })).toEqual([n2, draft])
  })

  it("steps from a hidden last row", () => {
    expect(step("/settings", { lastRow: n1, rendered: [design, epic, chat] })).toEqual([epic, chat])
  })

  it("loses to the open page's row, which is the one to remember", () => {
    const r = at("/agents/c1", { lastRow: design })
    expect([r.pageRow, r.prev?.id, r.next?.id]).toEqual([chat, n2, draft])
  })

  it.each([
    ["there is none", undefined],
    ["the model no longer holds it", "row:gone"],
  ])("goes to the first or last row when %s", (_, lastRow) => {
    const r = at("/settings", { lastRow })
    expect([r.pageRow, r.next?.id, r.prev?.id]).toEqual([undefined, design, triage])
  })
})
