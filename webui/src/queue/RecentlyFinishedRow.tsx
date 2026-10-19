import { Box, Text } from "@chakra-ui/react";
import type { FinishedView } from "../view";

export function RecentlyFinishedRow({ row }: { row: FinishedView }) {
  return (
    <Box display="flex" gap="2" alignItems="baseline" py="1" color="text.dim" fontSize="sm">
      <Text as="span" fontFamily="condensed" fontWeight="600" flex="1">
        {row.name}
      </Text>
      <Text as="span">{row.project}</Text>
      <Text as="span" fontFamily="condensed">
        {row.done}/{row.total}
      </Text>
      <Text as="span">{row.finished}</Text>
    </Box>
  );
}
