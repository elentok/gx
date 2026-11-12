import { ticketHref } from "../hrefs"
import type { SessionRoundCard, TriageQuestion, TriageRow } from "../generated/server"
import { isAnswered, type Answer, type Answers } from "../ui/question-sheet/round"

/** The text a Huh? answer is stored as in a triage row's answers. */
export const HUH_MESSAGE = "I could not follow this question. Could you explain it another way?"

const NOTE_MARK = "\nNote: "

/** The rows whose questions the person still owns: a gone row's questions are not asked. */
const askedRows = (rows: TriageRow[]) => rows.filter((r) => !r.gone)

/** A card the shared question component shows: the server's round card plus what the question is about. */
export type TriageCard = SessionRoundCard & { context?: string; contextHref?: string }

/**
 * Each asked row's number, in row order: the digits that end its address ("gx:follow-ups/01" is "1"), or its
 * position when it has none. Rows are unique by address, so a number another row already took gets a prime.
 */
export function rowNumbers(rows: TriageRow[]): Map<string, string> {
  const out = new Map<string, string>()
  const used = new Set<string>()
  askedRows(rows).forEach((r, i) => {
    const digits = /(\d+)$/.exec(r.address)?.[1]
    let n = digits ? String(Number(digits)) : String(i + 1)
    while (used.has(n)) n += "'"
    used.add(n)
    out.set(r.address, n)
  })
  return out
}

/** A question's key and card label: its row's number, a dot, then its id without the q: "1.2" for q2 of row 01. */
export const labelOf = (nums: Map<string, string>, row: TriageRow, q: TriageQuestion) =>
  `${nums.get(row.address)}.${q.id.replace(/^q/i, "")}`

/**
 * The card kind a record question shows as. The agent tags a question quick or design; what the person can do
 * follows from its options: yes and no is a yes-no card, any other list a choice card, none a text box.
 */
function cardKind(q: TriageQuestion): string {
  const opts = q.options ?? []
  if (opts.length === 2 && opts.includes("yes") && opts.includes("no")) return "yes-no"
  return opts.length > 0 ? "options" : q.kind === "yes-no" ? "yes-no" : "text"
}

/** A record question as a round card. The label is its row-qualified name; the plan's default is the recommended mark. */
export function cardOf(
  q: TriageQuestion,
  label: string,
  context?: string,
  contextHref?: string,
): TriageCard {
  const kind = cardKind(q)
  const card: TriageCard = { label, title: q.title, body: q.body ?? "", kind }
  if (context) card.context = context
  if (contextHref) card.contextHref = contextHref
  if (kind === "options") card.options = (q.options ?? []).map((o) => ({ id: o, label: o }))
  if (q.recommended) card.recommended = q.recommended
  return card
}

/** The open questions' cards across the rows, in row order. */
export function roundOf(rows: TriageRow[]): TriageCard[] {
  const nums = rowNumbers(rows)
  return askedRows(rows).flatMap((r) =>
    (r.questions ?? []).map((q) =>
      cardOf(q, labelOf(nums, r, q), `${nums.get(r.address)} · ${r.title}`, ticketHref(r.address)),
    ),
  )
}

/** The saved answers as the component holds them: a picked choice, Huh?, or typed text, plus the note. */
export function answersOf(rows: TriageRow[]): Answers {
  const out: Answers = {}
  const nums = rowNumbers(rows)
  for (const row of askedRows(rows)) {
    for (const q of row.questions ?? []) {
      const saved = row.answers?.[q.id]
      if (!saved) continue
      const at = saved.lastIndexOf(NOTE_MARK)
      const [text, note] =
        at < 0 ? [saved, undefined] : [saved.slice(0, at), saved.slice(at + NOTE_MARK.length)]
      const a: Answer = {}
      const picks = cardKind(q) === "yes-no" ? ["yes", "no"] : (q.options ?? [])
      if (text === HUH_MESSAGE) a.huh = true
      else if (picks.includes(text)) a.choice = text
      else a.text = text
      if (note) a.note = note
      out[labelOf(nums, row, q)] = a
    }
  }
  return out
}

/** What the row stores for an answer; "" for one not given yet. A note rides on the answer it belongs to. */
export function encodeAnswer(a: Answer | undefined): string {
  if (!isAnswered(a)) return ""
  const base = a?.huh ? HUH_MESSAGE : (a?.choice ?? a?.text?.trim() ?? "")
  return a?.note?.trim() ? `${base}${NOTE_MARK}${a.note.trim()}` : base
}

/** The row's answers map for a save, stored under the record ids: every answered question of the row. */
export function rowAnswers(
  rows: TriageRow[],
  row: TriageRow,
  answers: Answers,
): Record<string, string> {
  const out: Record<string, string> = {}
  const nums = rowNumbers(rows)
  for (const q of row.questions ?? []) {
    const text = encodeAnswer(answers[labelOf(nums, row, q)])
    if (text) out[q.id] = text
  }
  return out
}

/** The rows whose answers differ between two answer sets. */
export function changedRows(rows: TriageRow[], before: Answers, after: Answers) {
  const nums = rowNumbers(rows)
  return askedRows(rows).filter((r) =>
    (r.questions ?? []).some((q) => {
      const key = labelOf(nums, r, q)
      return encodeAnswer(before[key]) !== encodeAnswer(after[key])
    }),
  )
}
