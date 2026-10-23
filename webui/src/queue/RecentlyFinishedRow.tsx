import { Box, Link, Text } from "@chakra-ui/react";
import { epicHref, ticketHref } from "../hrefs";
import type { FinishedView } from "../view";

export function RecentlyFinishedRow({ row }: { row: FinishedView }) {
  return (
    <Box display="flex" gap="2" alignItems="baseline" py="1" color="text.dim" fontSize="sm">
      <Text as="span" fontFamily="condensed" fontWeight="600" flex="1">
        <Link href={(row.oneOff ? ticketHref : epicHref)(row.address)} color="inherit">
          {row.name}
        </Link>
      </Text>
      <Text as="span">{row.project}</Text>
      <Text as="span" fontFamily="condensed">
        {row.oneOff ? "one-off" : `${row.done}/${row.total}`}
      </Text>
      <Text as="span">{row.finished}</Text>
    </Box>
  );
}
