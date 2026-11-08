import { Box, Text } from "@chakra-ui/react";
import type { TicketDetail } from "../generated/server";

function runningFor(startedAt: string, now: number): string {
  const mins = Math.max(0, Math.floor((now - Date.parse(startedAt)) / 60000));
  if (mins < 60) return `${mins}m`;
  return `${Math.floor(mins / 60)}h ${mins % 60}m`;
}

// What the background-task gate shows of the live task. Without one, the pause text above names the real cause.
export function TaskCard({ detail }: { detail: TicketDetail }) {
  const task = detail.task;
  if (!task) return null;
  return (
    <Box mt="2" role="group" aria-label="Background task">
      <Text fontWeight="medium">A background task is still running.</Text>
      <Text fontFamily="mono" fontSize="sm" wordBreak="break-all">
        {task.label}
      </Text>
      <Text color="text.muted" fontSize="sm">
        Running for {runningFor(task.started_at, Date.now())} · process {task.alive ? "alive" : "not found"}
        {task.last_output_at ? ` · last output ${new Date(task.last_output_at).toLocaleTimeString()}` : " · no output file found"}
      </Text>
      {task.last_lines.length > 0 && (
        <Text as="pre" mt="1" fontFamily="mono" fontSize="sm" whiteSpace="pre-wrap" aria-label="Last output">
          {task.last_lines.join("\n")}
        </Text>
      )}
    </Box>
  );
}
