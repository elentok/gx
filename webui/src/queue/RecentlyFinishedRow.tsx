import { Box, Link, Text } from "@chakra-ui/react";
import { epicHref, ticketHref } from "../hrefs";
import type { FinishedView } from "../view";

export function RecentlyFinishedRow({ row }: { row: FinishedView }) {
  const meta = [row.project, row.oneOff ? "one-off" : `${row.done}/${row.total}`, row.finished];
  return (
    <Box py="1" fontSize="sm">
      <Link href={(row.oneOff ? ticketHref : epicHref)(row.address)} color="text" fontWeight="600">
        {row.name}
      </Link>
      <Text as="span" color="text.dim">
        {" · "}
        {meta.join(" · ")}
      </Text>
    </Box>
  );
}
