import { Box, Text } from "@chakra-ui/react";
import { useState } from "react";
import type { EpicDetail } from "./detail";
import { statPreview, statRows } from "./stats";

// A collapsed card that opens onto the epic's stat rows; absent when there are none.
export function MoreStats({ detail }: { detail: EpicDetail }) {
  const [open, setOpen] = useState(false);
  const rows = statRows(detail);
  if (rows.length === 0) return null;
  return (
    <Box mt="4" borderWidth="1px" borderColor="border" borderRadius="md">
      <Box
        as="button"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        display="flex"
        w="full"
        alignItems="center"
        gap="2"
        px="3"
        py="2"
        textAlign="left"
        cursor="pointer"
      >
        <Text as="span" aria-hidden>
          {open ? "▾" : "▸"}
        </Text>
        <Text as="span">More stats</Text>
        <Text as="span" ml="auto" fontSize="sm" color="text.muted">
          {statPreview(detail)}
        </Text>
      </Box>
      {open && (
        <Box as="dl" display="grid" gridTemplateColumns="auto 1fr" columnGap="4" rowGap="1" px="3" pb="3">
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
      )}
    </Box>
  );
}
