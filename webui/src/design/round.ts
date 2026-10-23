import type { SessionRoundCard } from "../generated/server";

export type Answer = { choice?: string; text?: string; note?: string };
export type Answers = Record<number, Answer>;

export const choicesOf = (c: SessionRoundCard) =>
  c.kind === "yes-no"
    ? [
        { id: "yes", label: c.yes || "Yes" },
        { id: "no", label: c.no || "No" },
      ]
    : (c.options ?? []);

export const isAnswered = (a?: Answer) => !!a?.choice || !!a?.text?.trim();

const answerText = (c: SessionRoundCard, a: Answer) => {
  if (a.choice) {
    if (c.kind === "yes-no") return a.choice === "yes" ? "Yes" : "No";
    return choicesOf(c).find((o) => o.id === a.choice)?.label ?? a.choice;
  }
  return a.text?.trim() ?? "";
};

/** The cards of the round the person has not answered yet. */
export const unansweredOf = (round: SessionRoundCard[], answers: Answers) => round.filter((c) => !isAnswered(answers[c.number]));

/** One line per question, a per-question note on the next line, then the composer's notes. */
export function roundPrompt(round: SessionRoundCard[], answers: Answers, notes: string) {
  const lines = round.flatMap((c) => {
    const a = answers[c.number] ?? {};
    const out = [`Q${c.number} - ${c.title}: ${isAnswered(a) ? answerText(c, a) : "(unanswered)"}`];
    if (a.note?.trim()) out.push(`  Note: ${a.note.trim()}`);
    return out;
  });
  if (notes.trim()) lines.push("", "Note:", notes.trim());
  return lines.join("\n");
}
