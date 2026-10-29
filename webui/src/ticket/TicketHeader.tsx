import { Box } from "@chakra-ui/react";
import type { TicketDetail } from "../generated/server";
import { epicHref } from "../hrefs";
import { slugOf } from "../slug";
import { StatLine } from "../page/StatLine";
import { Arc } from "../ui/arc/Arc";
import { Badge } from "../ui/badge/Badge";
import { Button } from "../ui/button/Button";
import { badgeOf, isRunning, statValues } from "./header";

// The line under the h1: a button to the epic, badge (and arc while running), then the stat line.
export function TicketHeader({ detail, now }: { detail: TicketDetail; now: number }) {
  const { tone, label } = badgeOf(detail);
  const values = statValues(detail, now);
  return (
    <>
      <Box mt="2" display="flex" alignItems="center" gap="2">
        <Button asChild emphasis="secondary" size="sm">
          <a href={epicHref(detail.epic)}>Epic: {slugOf(detail.epic)}</a>
        </Button>
        <Badge tone={tone}>{label}</Badge>
        {isRunning(detail) && <Arc label="Running" />}
      </Box>
      {values.length > 0 && <StatLine values={values} />}
    </>
  );
}
