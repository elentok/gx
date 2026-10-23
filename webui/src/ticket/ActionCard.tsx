import { Box, Text } from "@chakra-ui/react";
import { ActionAnswer, ActionResume, type TicketDetail } from "../generated/server";
import type { Sent } from "./answer";
import { AnswerForm } from "./AnswerForm";
import { AnswerResult } from "./AnswerResult";
import { PermissionCard } from "./PermissionCard";
import { ResumeForm } from "./ResumeForm";
import { TicketReading } from "./TicketReading";

const hasOpenQuestion = (detail: TicketDetail) => detail.latest?.outcome === "open";

/** Whether anything on the page asks a person to act, or shows what they just did. */
export function needsActionCard(detail: TicketDetail, sent: Sent | null, resumed: string): boolean {
  return (
    !!detail.error_reason ||
    hasOpenQuestion(detail) ||
    detail.answered ||
    !!detail.permission ||
    !!detail.pause_text ||
    detail.actions.includes(ActionAnswer) ||
    detail.actions.includes(ActionResume) ||
    !!sent ||
    resumed !== ""
  );
}

const cardProps = {
  role: "group",
  "aria-label": "Action card",
  mt: "4",
  p: "3",
  borderWidth: "1px",
  borderColor: "border",
  rounded: "md",
  bg: "surface.raised",
} as const;

// Everything the page asks a person to act on, right under the header. The old rounds live here
// only, so a ticket nobody needs to act on shows no old questions.
export function ActionCard({
  detail,
  sent,
  resumed,
  onSent,
  onResumed,
  refetch,
}: {
  detail: TicketDetail;
  sent: Sent | null;
  resumed: string;
  onSent: (sent: Sent) => void;
  onResumed: (words: string) => void;
  refetch: () => void;
}) {
  if (!needsActionCard(detail, sent, resumed)) return null;

  // A broken ticket has nothing to answer or resume, and the parser error is the only file text shown.
  if (detail.error_reason) {
    return (
      <Box {...cardProps}>
        <Text color="tone.deadlocked.fg" whiteSpace="pre-wrap">
          {detail.error_reason}
        </Text>
        {detail.parse_error && (
          <Text color="tone.deadlocked.fg" mt="2" fontFamily="mono" fontSize="sm" whiteSpace="pre-wrap">
            {detail.parse_error}
          </Text>
        )}
        {detail.path && (
          <Text mt="2" color="text.muted" fontFamily="mono" fontSize="sm" wordBreak="break-all">
            {detail.path}
          </Text>
        )}
      </Box>
    );
  }

  return (
    <Box {...cardProps}>
      {/* A self-reported pause shows its question below, so its text is left out. */}
      {detail.pause_text && !hasOpenQuestion(detail) && (
        <Text color="tone.paused.fg" whiteSpace="pre-wrap">
          {detail.pause_text}
        </Text>
      )}
      <TicketReading detail={detail} />
      <PermissionCard detail={detail} onDone={refetch} onStale={refetch} />
      <AnswerResult detail={detail} sent={sent} />
      <AnswerForm
        detail={detail}
        onSent={(s) => {
          onSent(s);
          refetch();
        }}
        onStale={refetch}
      />
      {resumed && (
        <Text role="status" aria-label="Resume result" mt="4">
          Resumed ·{resumed}
        </Text>
      )}
      <ResumeForm
        detail={detail}
        onDone={(words) => {
          onResumed(words);
          refetch();
        }}
        onStale={refetch}
      />
    </Box>
  );
}
