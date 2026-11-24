import { describe, expect, it } from "vitest"
import { session } from "../agents/agentsTestKit"
import { ActionStartOver, HoldQueuedText } from "../generated/server"
import { chordTable } from "../shell/chords"
import type { Entity, EntityKind } from "./entity"
import {
  ACTIONS,
  GROUP_ORDER,
  actionById,
  bulkBar,
  bulkConfirms,
  bulkFit,
  canCancelRun,
  chordLetters,
  menuActions,
  type ActionDecl,
} from "./registry"
import { canCancel } from "../ticket/recover"

const e = (kind: EntityKind, over: Partial<Entity> = {}): Entity => ({
  kind,
  key: `${kind}-key`,
  name: kind,
  ...over,
})

const ONE_OFF = "p:one-offs/01"
const live = session("live", "p")
const exited = session("done", "p", { status: { state: "exited" } })
const idsOf = (x: Entity, actions = ACTIONS) => menuActions(x, actions).map((a) => a.id)

/** One fixture per entity and state the invariants run over. */
const FIXTURES: Record<string, Entity> = {
  epic: e("epic", { prUrl: "https://x/pr/1" }),
  heldEpic: e("epic", { held: true }),
  queuedEpic: e("epic", { queued: true }),
  archivedEpic: e("epic", { archived: true, prUrl: "https://x/pr/1" }),
  design: e("design", { surface: "row", liveSessions: 1 }),
  heldDesign: e("design", { held: true }),
  ticket: e("ticket", { serverActions: [ActionStartOver] }),
  oneOff: e("ticket", { key: ONE_OFF, status: "claimed" }),
  chat: e("chat", { session: live }),
  exitedChat: e("chat", { session: exited }),
  queueAgent: e("chat", { agent: "queue", session: live }),
  fixAgent: e("chat", { agent: "fix", session: live }),
  investigateAgent: e("chat", { agent: "investigate", session: live }),
  designDraft: e("design-draft", { draftEmpty: true }),
  chatDraft: e("chat-draft", { draftEmpty: false }),
}

const APP_CHORDS = ["c", "C", "d", "s", "b", "h"]

/** Every structural invariant, as the list of violations it finds. */
function violations(actions: ActionDecl[]): string[] {
  const out: string[] = []
  const dup = (what: string, keys: string[]) => {
    for (const k of keys.filter((x, i) => keys.indexOf(x) !== i)) out.push(`duplicate ${what} ${k}`)
  }
  dup(
    "id",
    actions.map((a) => a.id),
  )
  dup(
    "label",
    actions.map((a) => a.label),
  )
  dup(
    "group+order",
    actions.filter((a) => a.menu !== false).map((a) => `${a.group}/${a.order}`),
  )
  // The pair Archive and Unarchive shares one letter; their availability must exclude each other.
  const byLetter = new Map<string, ActionDecl[]>()
  for (const a of actions) if (a.chord) byLetter.set(a.chord, [...(byLetter.get(a.chord) ?? []), a])
  for (const [letter, owners] of byLetter) {
    if (APP_CHORDS.includes(letter)) out.push(`chord ${letter} clashes with an app chord`)
    for (const [i, a] of owners.entries())
      for (const b of owners.slice(i + 1))
        for (const x of Object.values(FIXTURES))
          if (a.available(x) && b.available(x)) out.push(`chord ${letter} clashes`)
  }
  for (const a of actions) {
    if (a.label === "Cancel") out.push("an action is labeled Cancel")
    if ((a.group === "danger") !== !!a.danger) out.push(`${a.id} danger flag`)
  }
  for (const id of DESTRUCTIVE)
    if (!actions.find((a) => a.id === id)?.danger) out.push(`${id} is destructive without danger`)
  return out
}

const DESTRUCTIVE = ["start-over", "cancel-run", "remove", "delete-design"]

describe("registry invariants", () => {
  it("holds for the shipped declarations", () => {
    expect(violations(ACTIONS)).toEqual([])
  })

  it("matches the app-level chord keys", () => {
    const noop = () => {}
    const keys = chordTable({ remove: noop, rename: noop, switchModel: noop, history: noop })
      .map((c) => c.key)
      .filter((k) => APP_CHORDS.includes(k))
    expect(keys.sort()).toEqual([...APP_CHORDS].sort())
    expect(chordLetters()).toEqual(["r", "w", "m", "a", "y", "x"])
  })

  const broken = (id: string, over: Partial<ActionDecl>): ActionDecl[] =>
    ACTIONS.map((a) => (a.id === id ? ({ ...a, ...over } as ActionDecl) : a))

  it.each([
    ["a chord clash", broken("rename", { chord: "w" }), "clashes"],
    ["an app chord clash", broken("rename", { chord: "d" }), "app chord"],
    ["a duplicated label", broken("hold", { label: "Stop" }), "duplicate label"],
    ["a duplicated order", broken("hold", { order: 4 }), "duplicate group+order"],
    ["a Cancel label", broken("cancel-run", { label: "Cancel" }), "labeled Cancel"],
    ["a destructive action without danger", broken("remove", { danger: false }), "remove"],
  ])("rejects %s", (_, actions, text) => {
    expect(violations(actions).join("\n")).toContain(text)
  })

  it("orders every entity's available actions by group then order", () => {
    for (const x of Object.values(FIXTURES)) {
      const ranks = menuActions(x).map(
        (a) => GROUP_ORDER.indexOf(a.group as never) * 1000 + a.order!,
      )
      expect(ranks).toEqual([...ranks].sort((a, b) => a - b))
    }
  })

  it("keeps the footer-only actions out of every menu", () => {
    const footer = ["queue", "merge", "create-pr", "start-design-session"]
    for (const id of footer) expect(actionById(id)?.menu).toBe(false)
    for (const x of Object.values(FIXTURES))
      for (const id of footer) expect(idsOf(x)).not.toContain(id)
  })
})

describe("availability", () => {
  const F = FIXTURES
  it("offers Hold xor Take off hold on an epic and a Design", () => {
    expect(idsOf(F.epic)).toContain("hold")
    expect(idsOf(F.epic)).not.toContain("unhold")
    expect(idsOf(F.heldEpic)).toContain("unhold")
    expect(idsOf(F.heldEpic)).not.toContain("hold")
    expect(idsOf(F.heldDesign)).toContain("unhold")
    expect(idsOf(F.design)).toContain("hold")
  })

  it("offers an archived epic Unarchive and Copy ID only", () => {
    expect(idsOf(F.archivedEpic)).toEqual(["unarchive", "copy-id"])
  })

  it("shows a queued epic's Hold disabled with the reason", () => {
    const hold = actionById("hold")!
    expect(hold.available(F.queuedEpic)).toBe(true)
    expect(hold.disabled?.(F.queuedEpic)).toBe(HoldQueuedText)
    expect(hold.disabled?.(F.epic)).toBeUndefined()
  })

  it("offers a plain chat the full chat set, an exited one without Stop", () => {
    expect(idsOf(F.chat)).toEqual([
      "rename",
      "move",
      "switch-model",
      "promote",
      "stop",
      "copy-id",
      "remove",
    ])
    expect(idsOf(F.exitedChat)).not.toContain("stop")
    expect(idsOf(F.exitedChat)).toContain("remove")
  })

  it("offers the Queue agent Copy ID only", () => {
    expect(idsOf(F.queueAgent)).toEqual(["copy-id"])
  })

  it("offers Fix and Investigate agents Copy ID, Stop and Remove", () => {
    for (const x of [F.fixAgent, F.investigateAgent])
      expect(idsOf(x)).toEqual(["stop", "copy-id", "remove"])
  })

  it("offers drafts Rename, Move to workspace and Remove, no Copy ID", () => {
    for (const x of [F.designDraft, F.chatDraft])
      expect(idsOf(x)).toEqual(["rename", "move", "remove"])
  })

  it("offers Start over only when the server list has it", () => {
    expect(idsOf(F.ticket)).toEqual(["copy-id", "start-over"])
    expect(idsOf(e("ticket"))).toEqual(["copy-id"])
  })

  it("offers Cancel run exactly where the page's canCancel does", () => {
    const statuses = ["open", "claimed", "needs-answer", "needs-repair", "done"] // oldword:ok
    for (const address of [ONE_OFF, "p:epic/01", "p:one-offs"])
      for (const status of statuses)
        expect(canCancelRun(e("ticket", { key: address, status }))).toBe(
          canCancel({ address, status }),
        )
    expect(idsOf(F.oneOff)).toContain("cancel-run")
    expect(idsOf(e("ticket", { status: "claimed" }))).not.toContain("cancel-run")
  })

  it("offers Open PR only with a PR URL", () => {
    expect(idsOf(F.epic)).toContain("open-pr")
    expect(idsOf(e("epic"))).not.toContain("open-pr")
  })

  describe("Stop on a Design", () => {
    it("on the page follows the shown session", () => {
      expect(idsOf(e("design", { surface: "page", session: live }))).toContain("stop")
      expect(idsOf(e("design", { surface: "page", session: exited }))).not.toContain("stop")
    })
    it("on a row needs exactly one live session", () => {
      for (const [n, want] of [
        [0, false],
        [1, true],
        [2, false],
      ] as const)
        expect(idsOf(e("design", { surface: "row", liveSessions: n })).includes("stop")).toBe(want)
    })
  })
})

describe("confirm rule", () => {
  const rule = (id: string, x: Entity) => actionById(id)!.confirm.rule(x)
  const F = FIXTURES

  it("always confirms Stop, chat Remove, Archive, Cancel run, Start over, Mark as merged, Delete design", () => {
    for (const [id, x] of [
      ["stop", F.chat],
      ["remove", F.chat],
      ["archive", F.epic],
      ["cancel-run", F.oneOff],
      ["start-over", F.ticket],
      ["mark-merged", F.epic],
      ["delete-design", F.design],
    ] as const)
      expect(rule(id, x), id).toBe(true)
  })

  it("confirms a draft's Remove only when it is not empty or unknown", () => {
    expect(rule("remove", F.designDraft)).toBe(false)
    expect(rule("remove", F.chatDraft)).toBe(true)
    expect(rule("remove", e("design-draft"))).toBe(true)
  })

  it("confirms Hold and New session only with a live session", () => {
    expect(rule("hold", e("epic", { liveSessions: 0 }))).toBe(false)
    expect(rule("hold", e("epic", { liveSessions: 2 }))).toBe(true)
    expect(rule("new-session", e("design", { liveSessions: 0 }))).toBe(false)
    expect(rule("new-session", e("design", { liveSessions: 1 }))).toBe(true)
  })

  it("never confirms the rest", () => {
    for (const id of [
      "move",
      "rename",
      "unarchive",
      "copy-id",
      "promote",
      "switch-model",
      "unhold",
    ])
      for (const x of Object.values(F)) expect(rule(id, x), id).toBe(false)
  })

  it("rejects a wrong rule", () => {
    const wrong: ActionDecl = { ...actionById("stop")!, confirm: { rule: () => false } }
    expect(wrong.confirm.rule(F.chat)).not.toBe(rule("stop", F.chat))
  })
})

describe("bulk rules", () => {
  const rows = [
    e("epic", { key: "epic" }),
    e("design", { key: "design", liveSessions: 1 }),
    e("design", { key: "design2", liveSessions: 2 }),
    e("epic", { key: "queued", queued: true }),
    e("epic", { key: "held", held: true }),
    e("chat", { key: "chat", session: live }),
    e("chat", { key: "exited", session: exited }),
    e("chat", { key: "queue", agent: "queue", session: live }),
    e("chat", { key: "fix", agent: "fix", session: live }),
    e("epic", { key: "arch", archived: true }),
    e("design-draft", { key: "dd", draftEmpty: true }),
    e("chat-draft", { key: "cd", draftEmpty: true }),
    e("ticket", { key: "ticket" }),
  ]
  const fits = (id: string) => bulkFit(actionById(id)!, rows).fits
  const reason = (id: string, key: string) =>
    bulkFit(actionById(id)!, rows).skipped.find((s) => s.key === key)?.reason

  it("keeps today's fit rules", () => {
    expect(fits("move")).toEqual([
      "epic",
      "design",
      "design2",
      "queued",
      "held",
      "chat",
      "exited",
      "dd",
      "cd",
    ])
    expect(fits("hold")).toEqual(["epic", "design", "design2"])
    expect(fits("unhold")).toEqual(["held"])
    expect(fits("archive")).toEqual(["epic", "design", "design2", "queued", "held"])
    expect(fits("unarchive")).toEqual(["arch"])
    expect(reason("hold", "queued")).toBe("queued")
    expect(reason("hold", "held")).toBe("already on hold")
    expect(reason("stop", "exited")).toBe("not live")
  })

  it("skips the Queue agent in every bar action", () => {
    for (const v of bulkBar(rows)) expect(v.fit.fits).not.toContain("queue")
    for (const id of ["move", "hold", "unhold", "stop", "archive", "unarchive", "remove"])
      expect(reason(id, "queue"), id).toBe("managed by the queue")
  })

  it("fits a Fix agent to Stop and Remove, skips it for Move and Hold", () => {
    expect(fits("stop")).toContain("fix")
    expect(fits("remove")).toContain("fix")
    expect(fits("move")).not.toContain("fix")
    expect(fits("hold")).not.toContain("fix")
  })

  it("fits a Design to Stop with one live session, skips two", () => {
    expect(fits("stop")).toContain("design")
    expect(reason("stop", "design2")).toBe("2 live sessions")
  })

  it("fits chat drafts to Remove and never offers a ticket", () => {
    expect(fits("remove")).toEqual(["chat", "exited", "fix", "dd", "cd"])
    for (const v of bulkBar(rows)) expect(v.fit.fits).not.toContain("ticket")
  })

  it("lists bar actions in registry order with count labels", () => {
    const bar = bulkBar(rows)
    expect(bar.map((v) => v.id)).toEqual([
      "move",
      "hold",
      "unhold",
      "stop",
      "archive",
      "unarchive",
      "remove",
    ])
    expect(bar.find((v) => v.id === "hold")!.label).toBe(`Hold 3 of ${rows.length}`)
  })

  it("counts without 'of' when every selected row fits", () => {
    expect(bulkBar([rows[0], rows[1]]).find((v) => v.id === "hold")!.label).toBe("Hold 2")
  })

  it("rejects a wrong bulk rule", () => {
    const wrong: ActionDecl = { ...actionById("stop")!, bulk: () => undefined }
    expect(bulkFit(wrong, rows).fits).not.toEqual(fits("stop"))
  })
})

describe("bulk confirm merge", () => {
  const remove = actionById("remove")!
  const empty = (k: string) => e("design-draft", { key: k, draftEmpty: true })

  it("needs no confirm to remove only empty drafts", () => {
    expect(bulkConfirms(remove, [empty("a"), empty("b")])).toBe(false)
  })

  it("confirms once a non-empty draft or a chat joins", () => {
    expect(bulkConfirms(remove, [empty("a"), e("chat-draft", { draftEmpty: false })])).toBe(true)
    expect(bulkConfirms(remove, [empty("a"), e("chat", { session: live })])).toBe(true)
  })
})
