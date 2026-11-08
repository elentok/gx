import { HUH_MESSAGE } from "../design/round";
import type { SessionRoundCard, TriageQuestion, TriageRow } from "../generated/server";
import { isAnswered, type Answer, type Answers } from "../ui/question-sheet/round";

const NOTE_MARK = "\nNote: ";

/** The rows whose questions the person still owns: a gone row's questions are not asked. */
const askedRows = (rows: TriageRow[]) => rows.filter((r) => !r.gone);

/** A record question as a round card. The label is the question id; the plan's default is the recommended mark. */
export function cardOf(q: TriageQuestion): SessionRoundCard {
  const card: SessionRoundCard = { label: q.id, title: q.title, body: q.body ?? "", kind: q.kind };
  if (q.kind === "options") card.options = (q.options ?? []).map((o) => ({ id: o, label: o }));
  if (q.recommended) card.recommended = q.recommended;
  return card;
}

/** The open questions' cards across the rows, in row order. */
export const roundOf = (rows: TriageRow[]): SessionRoundCard[] => askedRows(rows).flatMap((r) => (r.questions ?? []).map(cardOf));

/** The saved answers as the component holds them: a picked choice, Huh?, or typed text, plus the note. */
export function answersOf(rows: TriageRow[]): Answers {
  const out: Answers = {};
  for (const row of askedRows(rows)) {
    for (const q of row.questions ?? []) {
      const saved = row.answers?.[q.id];
      if (!saved) continue;
      const at = saved.lastIndexOf(NOTE_MARK);
      const [text, note] = at < 0 ? [saved, undefined] : [saved.slice(0, at), saved.slice(at + NOTE_MARK.length)];
      const a: Answer = {};
      const picks = q.kind === "yes-no" ? ["yes", "no"] : (q.options ?? []);
      if (text === HUH_MESSAGE) a.huh = true;
      else if (picks.includes(text)) a.choice = text;
      else a.text = text;
      if (note) a.note = note;
      out[q.id] = a;
    }
  }
  return out;
}

/** What the row stores for an answer; "" for one not given yet. A note rides on the answer it belongs to. */
export function encodeAnswer(a: Answer | undefined): string {
  if (!isAnswered(a)) return "";
  const base = a?.huh ? HUH_MESSAGE : (a?.choice ?? a?.text?.trim() ?? "");
  return a?.note?.trim() ? `${base}${NOTE_MARK}${a.note.trim()}` : base;
}

/** The row's answers map for a save: every answered question of the row. */
export function rowAnswers(row: TriageRow, answers: Answers): Record<string, string> {
  const out: Record<string, string> = {};
  for (const q of row.questions ?? []) {
    const text = encodeAnswer(answers[q.id]);
    if (text) out[q.id] = text;
  }
  return out;
}

/** The rows whose answers differ between two answer sets. */
export const changedRows = (rows: TriageRow[], before: Answers, after: Answers) =>
  askedRows(rows).filter((r) => (r.questions ?? []).some((q) => encodeAnswer(before[q.id]) !== encodeAnswer(after[q.id])));
