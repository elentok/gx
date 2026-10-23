import { Text } from "@chakra-ui/react";

// One dim line of values joined by " · ".
export function StatLine({ values }: { values: string[] }) {
  return (
    <Text mt="2" fontSize="sm" color="text.muted">
      {values.join(" · ")}
    </Text>
  );
}
