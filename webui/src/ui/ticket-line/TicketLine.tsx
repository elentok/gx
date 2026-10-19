import { Box, Text } from "@chakra-ui/react";
import type { Tone } from "../tone";

export function TicketLine({ id, title, reason, tone = "muted" }: { id: string; title: string; reason?: string; tone?: Tone }) {
  return (
    <Box data-tone={tone}>
      <Box display="flex" gap="2" alignItems="baseline">
        <Text as="span" fontFamily="condensed" fontSize="sm" color={`tone.${tone}.fg`}>
          {id}
        </Text>
        <Text as="span" fontSize="md" color="text">
          {title}
        </Text>
      </Box>
      {reason && (
        <Text data-reason fontSize="sm" color={tone === "parked" ? "tone.parked.fg" : "text.muted"} lineClamp="2">
          {reason}
        </Text>
      )}
    </Box>
  );
}
