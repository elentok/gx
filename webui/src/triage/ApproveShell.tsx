import { Box, chakra } from "@chakra-ui/react";
import { useState, type ReactNode } from "react";
import { postApproveTriage, type ApproveTriage } from "./api";
import { ReasonTriageAlreadyApproved, type TriageView } from "../generated/server";
import { Row } from "../stepped/Progress";
import { showToast } from "../ui/toaster";
import { refusalText, useWrite } from "../useWrite";
import { ApproveBar, approveBlock } from "./ApproveBar";

export type ApproveViewName = "questions" | "review";

// The wire's refusal for an out-of-date revision (server/wire ReasonStale); the generated types carry no constant for it.
const REASON_STALE = "stale";

/**
 * The view shown at Approve. Nothing is stored beyond the reader's toggle: the page opens on Questions while a
 * question is open and on Review once none is, and a toggle holds for the record revision it was made on.
 * `carry` keeps a Review jump across the reload a stale refusal causes.
 */
export function useApproveView(revision: number | undefined, openQuestions: number) {
  const [toggle, setToggle] = useState<{ revision?: number; view: ApproveViewName; carry?: boolean }>();
  // The reload after a stale refusal brings a new revision: adopt the carried Review for it, once.
  if (toggle?.carry && toggle.revision !== revision) setToggle({ revision, view: toggle.view });
  const held = toggle && toggle.revision === revision ? toggle.view : undefined;
  const view: ApproveViewName = held ?? (openQuestions > 0 ? "questions" : "review");
  return {
    view,
    setView: (v: ApproveViewName) => setToggle({ revision, view: v }),
    carryReview: () => setToggle({ revision, view: "review", carry: true }),
  };
}

/** The Approve action: posts the revision the page read and answers each way it can end. */
export function useApprove({
  project,
  view,
  refetch,
  approve = postApproveTriage,
  carryReview,
}: {
  project: string;
  view: TriageView;
  refetch: () => void;
  approve?: ApproveTriage;
  carryReview: () => void;
}) {
  const { run, pending } = useWrite({ toastRefusal: false });
  const go = () =>
    run(async () => {
      const res = await approve({ project, revision: view.revision ?? 0 });
      if (res.refused) {
        refetch();
        if (res.reason === ReasonTriageAlreadyApproved) showToast({ type: "info", title: "Already approved. Showing the write." });
        else if (res.reason === REASON_STALE) {
          carryReview();
          showToast({ type: "warning", title: "The plan changed. Review it again." });
        } else showToast({ type: "error", title: refusalText(res) });
        return {};
      }
      refetch();
      return res;
    });
  return { approve: go, pending, blocked: approveBlock(view) };
}

/** Questions and Review under Approve in the Progress tree; the shown one is bold. */
export function approveSubRows(view: TriageView, shown: ApproveViewName): ReactNode {
  const items: [ApproveViewName, string][] = [
    ["questions", "Questions"],
    ["review", "Review"],
  ];
  return (
    <Box ml="4" display="flex" flexDirection="column" gap="1">
      {items.map(([name, label]) => (
        <Row key={name} glyph={name === "questions" && view.open_questions === 0 ? "✓" : "○"} tone={name === shown ? "running" : "muted"}>
          <chakra.span fontWeight={name === shown ? "700" : undefined}>{label}</chakra.span>
        </Row>
      ))}
    </Box>
  );
}

function Segment({ on, onClick, children }: { on: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <chakra.button
      type="button"
      aria-pressed={on}
      onClick={onClick}
      px="3"
      py="1"
      fontSize="sm"
      cursor="pointer"
      bg={on ? "surface.raised" : "transparent"}
      fontWeight={on ? "700" : undefined}
    >
      {children}
    </chakra.button>
  );
}

/** The Approve step: the Questions | Review switch, the shown view's body, and one Approve bar under both. */
export function ApproveShell({
  view,
  shown,
  onShow,
  approval,
  questions,
  review,
}: {
  view: TriageView;
  shown: ApproveViewName;
  onShow: (v: ApproveViewName) => void;
  approval: Pick<ReturnType<typeof useApprove>, "approve" | "pending">;
  /** The views' bodies; later tickets fill them. */
  questions?: ReactNode;
  review?: ReactNode;
}) {
  return (
    <Box display="flex" flexDirection="column" gap="3">
      <Box role="group" aria-label="View" display="inline-flex" alignSelf="flex-start" borderWidth="1px" borderColor="border" borderRadius="md" overflow="hidden">
        <Segment on={shown === "questions"} onClick={() => onShow("questions")}>
          Questions {view.open_questions === 0 ? "✓" : view.open_questions}
        </Segment>
        <Segment on={shown === "review"} onClick={() => onShow("review")}>
          Review {view.rows.length} rows
        </Segment>
      </Box>
      {shown === "questions" ? questions : review}
      <ApproveBar view={view} pending={approval.pending} onApprove={approval.approve} onAnswer={() => onShow("questions")} />
    </Box>
  );
}
