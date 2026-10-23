import { Box, Text } from "@chakra-ui/react";
import { SectionHeading } from "../page/SectionHeading";
import type { EpicDetail } from "./detail";
import { statRows } from "./stats";

export function StatsSection({ detail }: { detail: EpicDetail }) {
  const rows = statRows(detail);
  if (rows.length === 0) return null;
  return (
    <Box as="section" mt="6" aria-labelledby="epic-stats">
      <SectionHeading id="epic-stats" tone="muted">
        Stats
      </SectionHeading>
      <Box as="dl" display="grid" gridTemplateColumns="auto 1fr" columnGap="4" rowGap="1">
        {rows.map((r) => (
          <Box key={r.label} display="contents" data-stat-row={r.label}>
            <Text as="dt" color="text.muted">
              {r.label}
            </Text>
            <Text as="dd">
              {r.value}
              {r.note && (
                <Text as="span" ml="2" fontSize="sm" color="text.muted">
                  {r.note}
                </Text>
              )}
            </Text>
          </Box>
        ))}
      </Box>
    </Box>
  );
}
