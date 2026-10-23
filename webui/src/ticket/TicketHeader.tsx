import { Box, Link } from "@chakra-ui/react";
import type { TicketDetail } from "../generated/server";
import { epicHref } from "../hrefs";
import { StatLine } from "../page/StatLine";
import { Arc } from "../ui/arc/Arc";
import { Badge } from "../ui/badge/Badge";
import { badgeOf, isRunning, statValues } from "./header";

// The line under the h1: epic, badge (and arc while running), then the stat line.
export function TicketHeader({ detail, now }: { detail: TicketDetail; now: number }) {
  const { tone, label } = badgeOf(detail);
  const values = statValues(detail, now);
  return (
    <>
      <Box mt="2" display="flex" alignItems="center" gap="2">
        <Link href={epicHref(detail.epic)} fontSize="sm">
          {detail.epic.slice(detail.epic.indexOf(":") + 1)}
        </Link>
        <Badge tone={tone}>{label}</Badge>
        {isRunning(detail) && <Arc label="Running" />}
      </Box>
      {values.length > 0 && <StatLine values={values} />}
    </>
  );
}
