import { Box, Link, Text } from "@chakra-ui/react";
import type { Tone } from "../tone";

interface TicketLineProps {
  id: string;
  title: string;
  reason?: string;
  tone?: Tone;
  // Shows the reason in full instead of clamping it to two lines.
  unclipped?: boolean;
  href?: string;
  hint?: string;
  state?: string;
  // The link's words; "Answer →" unless the row asks for something else.
  linkLabel?: string;
}

export function TicketLine({
  id,
  title,
  reason,
  tone = "muted",
  unclipped,
  href,
  hint,
  state,
  linkLabel = "Answer →",
}: TicketLineProps) {
  return (
    <Box data-tone={tone}>
      <Box display="flex" gap="2" alignItems="baseline">
        <Text as="span" fontFamily="condensed" fontSize="sm" color={`tone.${tone}.fg`}>
          {id}
        </Text>
        <Text as="span" fontSize="md" color="text">
          {title}
        </Text>
        {state && (
          <Text as="span" data-state fontSize="sm" color={`tone.${tone}.fg`}>
            {state}
          </Text>
        )}
      </Box>
      {reason && (
        <Text
          data-reason
          fontSize="sm"
          color={tone === "parked" ? "tone.parked.fg" : "text.muted"}
          lineClamp={unclipped ? undefined : "2"}
        >
          {reason}
        </Text>
      )}
      {hint && (
        <Text data-hint fontSize="sm" color="text.muted">
          {hint}
        </Text>
      )}
      {href && (
        <Link href={href} fontSize="sm" color="tone.parked.fg">
          {linkLabel}
        </Link>
      )}
    </Box>
  );
}
