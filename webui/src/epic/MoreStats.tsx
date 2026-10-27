import { Box, chakra, Text } from "@chakra-ui/react";
import { useState } from "react";
import { useWide } from "../design/useWide";
import type { EpicDetail } from "./detail";
import { statPreview, statRows } from "./stats";

function Chevron({ open }: { open: boolean }) {
  return (
    <chakra.svg
      aria-hidden
      viewBox="0 0 16 16"
      w="16px"
      h="16px"
      flexShrink="0"
      fill="none"
      stroke="currentColor"
      color="text.dim"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      transform={open ? "rotate(90deg)" : "none"}
      transition="transform 0.15s"
    >
      <path d="M6 3.5 10.5 8 6 12.5" />
    </chakra.svg>
  );
}

// A collapsed card that opens onto the epic's stat rows; absent when there are none.
export function MoreStats({ detail }: { detail: EpicDetail }) {
  const [open, setOpen] = useState(false);
  const wide = useWide();
  const rows = statRows(detail);
  if (rows.length === 0) return null;
  return (
    <Box mt="28px" bg="surface.raised" borderWidth="1px" borderColor="#252c36" borderRadius="12px">
      <Box
        as="button"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        display="flex"
        flexWrap={wide ? "nowrap" : "wrap"}
        w="full"
        alignItems="center"
        columnGap="2"
        rowGap="4px"
        px={wide ? "16px" : "14px"}
        py={wide ? "14px" : "12px"}
        textAlign="left"
        cursor="pointer"
      >
        <Chevron open={open} />
        <Text as="span" fontSize="15px" fontWeight="500">
          More stats
        </Text>
        <Text
          as="span"
          ml={wide ? "auto" : "0"}
          pl={wide ? "0" : "28px"}
          flexBasis={wide ? "auto" : "full"}
          textAlign={wide ? "right" : "left"}
          fontSize="13px"
          color="text.dim"
        >
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
