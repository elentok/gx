import { Box } from "@chakra-ui/react";
import type { Tone } from "../tone";

export type StatusKind = "done" | "asking" | "waiting" | "children" | "attention";

export const STATUS_LABELS: Record<StatusKind, string> = {
  done: "Done",
  asking: "Asking",
  waiting: "Waiting",
  children: "Waiting for children",
  attention: "Needs attention",
};

// Every icon is a circle of the same size, so a row of mixed statuses lines up.
const MARKS: Record<StatusKind, string> = {
  done: "M5.2 8.2l2 2 3.6-4.2",
  asking: "M6.3 6.4a1.8 1.8 0 1 1 2.5 1.7c-.5.3-.8.6-.8 1.1M8 11.1v.1",
  waiting: "",
  children: "",
  attention: "M8 5v3.4M8 11v.1",
};

export function StatusIcon({ kind, tone }: { kind: StatusKind; tone: Tone }) {
  return (
    <Box as="span" role="img" aria-label={STATUS_LABELS[kind]} color={`tone.${tone}.fg`} display="inline-flex" alignItems="center" justifyContent="center" w="4" h="4">
      <svg viewBox="0 0 16 16" width="100%" height="100%" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
        <circle cx="8" cy="8" r="5.5" strokeDasharray={kind === "children" ? "2 2.4" : undefined} />
        {MARKS[kind] && <path d={MARKS[kind]} />}
      </svg>
    </Box>
  );
}
