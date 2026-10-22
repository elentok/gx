import { Box, Link, Text } from "@chakra-ui/react";
import { Arc } from "../arc/Arc";
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
  // Puts the reason right after the title, behind a dot, in one line that wraps as text does.
  inlineReason?: boolean;
  // The link's words; "Answer →" unless the row asks for something else.
  linkLabel?: string;
  // Puts a spinning arc before the id while an agent works the ticket.
  live?: boolean;
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
  inlineReason,
  linkLabel = "Answer →",
  live,
}: TicketLineProps) {
  return (
    <Box data-tone={tone}>
      <Box display={inlineReason ? "block" : "flex"} gap="2" alignItems="baseline">
        {live && (
          <Box as="span" mr={inlineReason ? "2" : undefined} alignSelf="center" display="inline-flex">
            <Arc label="Running" />
          </Box>
        )}
        <Text as="span" fontFamily="condensed" fontSize="sm" color={`tone.${tone}.fg`} mr={inlineReason ? "2" : undefined}>
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
        {inlineReason && reason && (
          <>
            <Text as="span" aria-hidden color="text.muted">
              {" · "}
            </Text>
            <Text as="span" data-reason fontSize="sm" color="text.muted">
              {reason}
            </Text>
          </>
        )}
      </Box>
      {!inlineReason && reason && (
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
