import { Box, Text } from "@chakra-ui/react";
import { useEffect, useMemo, useRef, useState } from "react";
import { postSaveTriageRow, postSendBackTriage, type SaveTriageRow, type SendBackTriage } from "./api";
import { QuestionSheet } from "../design/QuestionSheet";
import type { TriageRow, TriageView } from "../generated/server";
import { Button } from "../ui/button/Button";
import type { Answers } from "../ui/question-sheet/round";
import { showToast } from "../ui/toaster";
import { refusalText, useWrite } from "../useWrite";
import { answersOf, changedRows, roundOf, rowAnswers } from "./questions";

// The wire's refusal for an out-of-date revision (server/wire ReasonStale); the generated types carry no constant for it.
const REASON_STALE = "stale";
// Typing waits this long before it saves; a pick saves at once.
const TYPING_MS = 600;

/** The row's person fields as the save replaces them wholesale: keep what the person already set, change the answers. */
function saveBody(project: string, revision: number, rows: TriageRow[], row: TriageRow, answers: Answers) {
  return {
    project,
    revision,
    address: row.address,
    action: row.person_changed && row.action !== "keep" ? row.action : undefined,
    destination: row.person_changed ? row.destination : undefined,
    keep_for_later: row.action === "keep" || undefined,
    answers: rowAnswers(rows, row, answers),
  };
}

/** The open quick questions in the shared question component; answers save to their rows with the record revision. */
export function QuestionsView({
  project,
  view,
  refetch,
  save = postSaveTriageRow,
  sendBack = postSendBackTriage,
}: {
  project: string;
  view: TriageView;
  refetch: () => void;
  save?: SaveTriageRow;
  sendBack?: SendBackTriage;
}) {
  const round = useMemo(() => roundOf(view.rows), [view.rows]);
  const [answers, setAnswers] = useState<Answers>(() => answersOf(view.rows));
  // The revisions our own saves produced: a view at any other revision changed elsewhere, so it replaces the draft.
  const ownRevisions = useRef(new Set([view.revision]));
  // Edits typed but not saved yet, or saves still in flight: the screen is ahead of any view that arrives meanwhile.
  const inFlight = useRef(0);
  const latest = useRef({ rows: view.rows, answers, revision: view.revision ?? 0 });
  const saved = useRef(answers);
  const queue = useRef<Promise<void>>(Promise.resolve());
  const timer = useRef<number | undefined>(undefined);
  useEffect(() => {
    if (ownRevisions.current.has(view.revision)) return;
    // A view that crossed our edits on the way would erase what was just typed. The save that follows is refused
    // as stale if the record really changed elsewhere, and that refusal reloads it.
    if (inFlight.current > 0 || timer.current !== undefined) return;
    ownRevisions.current.add(view.revision);
    const fresh = answersOf(view.rows);
    setAnswers(fresh);
    // The next edit builds on these, not on the draft the change replaced.
    latest.current.answers = fresh;
    saved.current = fresh;
    latest.current.revision = view.revision ?? 0;
  }, [view.revision, view.rows]);
  latest.current.rows = view.rows;
  useEffect(() => () => window.clearTimeout(timer.current), []);

  const flush = () => {
    const { rows, answers: now } = latest.current;
    const dirty = changedRows(rows, saved.current, now);
    saved.current = now;
    timer.current = undefined;
    inFlight.current++;
    queue.current = queue.current.then(async () => {
      try {
        await saveRows();
      } finally {
        inFlight.current--;
      }
      refetch();
    });
    async function saveRows() {
      for (const row of dirty) {
        const res = await save(saveBody(project, latest.current.revision, rows, row, now));
        if (res.refused) {
          if (res.reason !== REASON_STALE) showToast({ type: "error", title: refusalText(res) });
          else showToast({ type: "warning", title: "The plan changed. Your answers were reloaded." });
          return;
        }
        if (res.revision !== undefined) {
          latest.current.revision = res.revision;
          ownRevisions.current.add(res.revision);
        }
      }
    }
  };

  const onAnswers = (update: (all: Answers) => Answers) => {
    const next = update(latest.current.answers);
    const typing = Object.keys(next).some((k) => next[k]?.text !== latest.current.answers[k]?.text || next[k]?.note !== latest.current.answers[k]?.note);
    latest.current.answers = next;
    setAnswers(next);
    window.clearTimeout(timer.current);
    if (typing) timer.current = window.setTimeout(flush, TYPING_MS);
    else flush();
  };

  const { run, pending } = useWrite({ toastRefusal: false });
  // Whatever is typed is saved first, so the agent reads the answers on screen, then the record goes back to Plan.
  const sendBackNow = () =>
    run(async () => {
      window.clearTimeout(timer.current);
      flush();
      await queue.current;
      const res = await sendBack({ project, revision: latest.current.revision });
      if (res.refused) {
        showToast({ type: "error", title: refusalText(res) });
        refetch();
        return res;
      }
      showToast({ type: "info", title: "Sent back. The agent is writing new questions." });
      refetch();
      return res;
    });

  if (round.length === 0) return <Text color="text.muted">No open questions.</Text>;
  return (
    <Box display="flex" flexDirection="column" gap="2">
      <QuestionSheet key={view.rows.length} round={round} answers={answers} onAnswers={onAnswers} questions />
      <Box display="flex" alignItems="center" flexWrap="wrap" gap="3">
        <Button emphasis="ghost" tone="muted" size="sm" disabled={view.open_questions > 0 || pending} onClick={sendBackNow}>
          Send back to the agent
        </Button>
        <Text fontSize="xs" color="text.dim">
          {view.open_questions > 0
            ? "Answer every question to send them back."
            : "The agent reads your answers and notes and asks again. Your answers stay."}
        </Text>
      </Box>
      <Text fontSize="xs" color="text.dim">
        Answers go into each ticket body when the Write step runs.
      </Text>
    </Box>
  );
}
