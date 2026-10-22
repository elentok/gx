import { Box, Link, Text } from "@chakra-ui/react";
import { Arc } from "../arc/Arc";
import { StatusIcon, type StatusKind } from "../status-icon/StatusIcon";
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
  // Shows a status icon before the id, and colors the title with the tone instead of plain text.
  icon?: StatusKind;
  toneTitle?: boolean;
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
  icon,
  toneTitle,
}: TicketLineProps) {
  return (
    <Box data-tone={tone}>
      <Box display={inlineReason ? "block" : "flex"} gap="2" alignItems="baseline">
        {(live || icon) && (
          <Box as="span" mr={inlineReason ? "2" : undefined} alignSelf="center" verticalAlign="middle" position="relative" top="-1px" display="inline-flex" w="4" h="4" alignItems="center" justifyContent="center">
            {live ? <Arc label="Running" /> : <StatusIcon kind={icon!} tone={tone} />}
          </Box>
        )}
        <Text
          as="span"
          fontFamily="condensed"
          fontSize="sm"
          color={`tone.${tone}.fg`}
          mr={inlineReason ? (toneTitle ? "1" : "2") : undefined}
          // Ids differ in width, so a tone-titled list gives them one column to start the titles at the same x.
          display={toneTitle ? "inline-block" : undefined}
          minW={toneTitle ? "5" : undefined}
        >
          {id}
        </Text>
        <Text as="span" fontSize="md" color={toneTitle ? `tone.${tone}.fg` : "text"}>
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
