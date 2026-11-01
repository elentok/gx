import { Badge } from "../ui/badge/Badge";

interface LongRunning {
  long_ticket_turns?: number;
  long_ticket_threshold?: number;
}

// Shown only on a ticket the long-ticket alert marked; the tooltip has the turn count and the threshold.
export function LongRunningBadge({ of }: { of: LongRunning }) {
  const turns = of.long_ticket_turns;
  if (!turns) return null;
  const threshold = of.long_ticket_threshold;
  const title = `${turns} assistant turns${threshold ? ` (threshold ${threshold})` : ""}`;
  return (
    <Badge tone="paused" title={title} flexShrink={0} data-long-running>
      Long-running
    </Badge>
  );
}
