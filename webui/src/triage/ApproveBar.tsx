import { Box, Spinner as ChakraSpinner, Text, chakra } from "@chakra-ui/react";
import type { TriageView } from "../generated/server";
import { Button } from "../ui/button/Button";

type ApproveFacts = Pick<TriageView, "open_questions" | "action_counts" | "changed_rows" | "rows">;

const plural = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;

// The order the bar lists the actions in; a zero count is left out.
const COUNT_LABELS: [string, (n: number) => string][] = [
  ["ready", (n) => `${n} ready epic`],
  ["map", (n) => plural(n, "map epic")],
  ["merge", (n) => `${n} merge`],
  ["close", (n) => `${n} close`],
  ["keep", (n) => `${n} keep`],
];

/** Why Approve is refused right now, or undefined when it can go. The header button and the bar share it. */
export function approveBlock(view: ApproveFacts): string | undefined {
  if (view.open_questions > 0) return `${plural(view.open_questions, "question")} left.`;
  if (view.rows.some((r) => r.merge_orphan)) return "A merge target is gone. Pick a new one.";
  return undefined;
}

/** The one Approve bar under both views: counts and changed rows, or the reason it is disabled. */
export function ApproveBar({
  view,
  pending,
  onApprove,
  onAnswer,
}: {
  view: ApproveFacts;
  pending: boolean;
  onApprove: () => void;
  onAnswer: () => void;
}) {
  const blocked = approveBlock(view);
  const counts = COUNT_LABELS.filter(([key]) => view.action_counts[key] > 0).map(([key, label]) => label(view.action_counts[key]));
  return (
    <Box
      role="group"
      aria-label="Approve"
      display="flex"
      alignItems="center"
      flexWrap="wrap"
      gap="3"
      px="3"
      py="2"
      borderWidth="1px"
      borderColor="border"
      borderRadius="md"
      bg="surface.raised"
    >
      <Box flex="1" minW="0" fontSize="sm">
        {blocked ? (
          <>
            <Text as="span">{blocked}</Text>
            {view.open_questions > 0 && (
              <>
                {" "}
                <chakra.button type="button" textDecoration="underline" cursor="pointer" onClick={onAnswer}>
                  Answer them
                </chakra.button>
              </>
            )}
          </>
        ) : (
          <>
            <Text as="span">{counts.join(" · ")}</Text>
            {view.changed_rows > 0 && (
              <Text as="span" color="text.muted">
                {" · "}
                {view.changed_rows} changed by you
              </Text>
            )}
          </>
        )}
      </Box>
      <Button tone="done" disabled={!!blocked || pending} onClick={onApprove}>
        {pending && <ChakraSpinner size="sm" />}
        Approve and write
      </Button>
    </Box>
  );
}
