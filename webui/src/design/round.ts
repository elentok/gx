import type { SessionRoundCard } from "../generated/server"
import { isAnswered, type Answers } from "../ui/question-sheet/round"

/** The cards of the round the person has not answered yet. */
export const unansweredOf = (round: SessionRoundCard[], answers: Answers) =>
  round.filter((c) => !isAnswered(answers[c.label], c))
