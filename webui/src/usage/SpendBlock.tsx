import { Text } from "@chakra-ui/react";
import type { UsageSummary } from "../generated/server";

// Title only; the rows arrive in a later ticket.
export function SpendBlock(_props: { summary: UsageSummary; now: number }) {
  return (
    <Text px="3" py="1" fontSize="xs" fontWeight="medium" color="text.muted">
      gx spend (est.)
    </Text>
  );
}
