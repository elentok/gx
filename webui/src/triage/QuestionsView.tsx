import { Box, Text } from "@chakra-ui/react";
import { useEffect, useMemo, useRef, useState } from "react";
import { postSaveTriageRow, type SaveTriageRow } from "../design/open";
import { QuestionSheet } from "../design/QuestionSheet";
import type { TriageRow, TriageView } from "../generated/server";
import type { Answers } from "../ui/question-sheet/round";
import { showToast } from "../ui/toaster";
import { refusalText } from "../useWrite";
import { answersOf, changedRows, roundOf, rowAnswers } from "./questions";

// The wire's refusal for an out-of-date revision (server/wire ReasonStale); the generated types carry no constant for it.
const REASON_STALE = "stale";
// Typing waits this long before it saves; a pick saves at once.
const TYPING_MS = 600;

/** The row's person fields as the save replaces them wholesale: keep what the person already set, change the answers. */
function saveBody(project: string, revision: number, row: TriageRow, answers: Answers) {
  return {
    project,
    revision,
    address: row.address,
    action: row.person_changed && row.action !== "keep" ? row.action : undefined,
    destination: row.person_changed ? row.destination : undefined,
    keep_for_later: row.action === "keep" || undefined,
    answers: rowAnswers(row, answers),
  };
}

/** The open quick questions in the shared question component; answers save to their rows with the record revision. */
export function QuestionsView({
  project,
  view,
  refetch,
  save = postSaveTriageRow,
}: {
  project: string;
  view: TriageView;
  refetch: () => void;
  save?: SaveTriageRow;
}) {
  const round = useMemo(() => roundOf(view.rows), [view.rows]);
  const [answers, setAnswers] = useState<Answers>(() => answersOf(view.rows));
  // The revision our own saves produced: a view at any other revision changed elsewhere, so it replaces the draft.
  const ownRevision = useRef(view.revision);
  const latest = useRef({ rows: view.rows, answers, revision: view.revision ?? 0 });
  const saved = useRef(answers);
  const queue = useRef<Promise<void>>(Promise.resolve());
  const timer = useRef<number | undefined>(undefined);
  useEffect(() => {
    if (view.revision === ownRevision.current) return;
    ownRevision.current = view.revision;
    const fresh = answersOf(view.rows);
    setAnswers(fresh);
    saved.current = fresh;
    latest.current.revision = view.revision ?? 0;
  }, [view.revision, view.rows]);
  latest.current.rows = view.rows;
  useEffect(() => () => window.clearTimeout(timer.current), []);

  const flush = () => {
    const { rows, answers: now } = latest.current;
    const dirty = changedRows(rows, saved.current, now);
    saved.current = now;
    queue.current = queue.current.then(async () => {
      for (const row of dirty) {
        const res = await save(saveBody(project, latest.current.revision, row, now));
        if (res.refused) {
          if (res.reason !== REASON_STALE) showToast({ type: "error", title: refusalText(res) });
          else showToast({ type: "warning", title: "The plan changed. Your answers were reloaded." });
          refetch();
          return;
        }
        if (res.revision !== undefined) {
          latest.current.revision = res.revision;
          ownRevision.current = res.revision;
        }
      }
      refetch();
    });
  };

  const onAnswers = (update: (all: Answers) => Answers) => {
    const next = update(latest.current.answers);
    const typing = Object.keys(next).some((k) => next[k]?.text !== latest.current.answers[k]?.text);
    latest.current.answers = next;
    setAnswers(next);
    window.clearTimeout(timer.current);
    if (typing) timer.current = window.setTimeout(flush, TYPING_MS);
    else flush();
  };

  if (round.length === 0) return <Text color="text.muted">No open questions.</Text>;
  return (
    <Box display="flex" flexDirection="column" gap="2">
      <QuestionSheet key={view.rows.length} round={round} answers={answers} onAnswers={onAnswers} questions />
      <Text fontSize="xs" color="text.dim">
        Answers go into each ticket body when the Write step runs.
      </Text>
    </Box>
  );
}
