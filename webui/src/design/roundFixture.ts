import type { QuestionCard, SessionRoundCard } from "../generated/server";

/** Test fixture: a printed round as the one `round` card the server lists in `questions`, id built as the server builds it. */
export const roundCards = (round: readonly SessionRoundCard[] | null | undefined): QuestionCard[] =>
  !round || round.length === 0
    ? []
    : [
        {
          source: "round",
          id: round.map((c) => `${c.label}:${c.title}`).join("|"),
          questions: round.map((c) => ({ ...c, options: c.options?.map((o) => ({ ...o })) })),
        },
      ];
