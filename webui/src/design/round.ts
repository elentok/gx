import type { SessionRoundCard } from "../generated/server";
import { choicesOf, isAnswered, nameOf, type Answer, type Answers } from "../ui/question-sheet/round";

export const HUH_MESSAGE = "I could not follow this question. Could you explain it another way?";

/** The cards of the round the person answered with Huh?, for the conversation record. */
export const huhOf = (round: SessionRoundCard[], answers: Answers) =>
  round.filter((c) => answers[c.label]?.huh).map((c) => ({ question: c.body, title: c.title }));

/** The answer as the prompt words it: the Huh? message, the picked option's label, or the typed text. */
export const answerText = (c: SessionRoundCard, a: Answer) => {
  if (a.huh) return HUH_MESSAGE;
  if (a.choice) {
    if (c.kind === "yes-no") {
      const [label, line] = a.choice === "yes" ? ["Yes", c.yes] : ["No", c.no];
      return line ? `${label} - ${line}` : label;
    }
    return choicesOf(c).find((o) => o.id === a.choice)?.label ?? a.choice;
  }
  return a.text?.trim() ?? "";
};

/** The cards of the round the person has not answered yet. */
export const unansweredOf = (round: SessionRoundCard[], answers: Answers) => round.filter((c) => !isAnswered(answers[c.label]));

/** One line per question, a per-question note on the next line, then the composer's notes. */
export function roundPrompt(round: SessionRoundCard[], answers: Answers, notes: string) {
  const lines = round.flatMap((c) => {
    const a = answers[c.label] ?? {};
    const out = [`${nameOf(c)} - ${c.title}: ${isAnswered(a) ? answerText(c, a) : "(unanswered)"}`];
    if (a.note?.trim()) out.push(`  Note: ${a.note.trim()}`);
    return out;
  });
  if (notes.trim()) lines.push("", "Note:", notes.trim());
  return lines.join("\n");
}
