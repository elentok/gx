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
  // Links the title to this ticket's page.
  address?: string;
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
  address,
  href,
  hint,
  state,
  inlineReason,
  linkLabel = "Answer →",
  live,
  icon,
  toneTitle,
}: TicketLineProps) {
  const titleText = (
    <Text as="span" fontSize="md" color={toneTitle ? `tone.${tone}.fg` : "text"}>
      {address ? (
        // Chakra's Link is inline-flex, which would move a long title to its own line instead of wrapping it.
        <Link href={`#/t/${address}`} color="inherit" display="inline">
          {title}
        </Link>
      ) : (
        title
      )}
    </Text>
  );
  const stateText = state && (
    <Text as="span" data-state fontSize="sm" color={`tone.${tone}.fg`} ml={inlineReason ? "2" : undefined}>
      {state}
    </Text>
  );
  return (
    <Box data-tone={tone}>
      <Box display="flex" gap="2" alignItems="baseline">
        {(live || icon) && (
          // An inline-reason row can wrap, so the icon sits on its first line instead of the row's middle.
          <Box as="span" alignSelf={inlineReason ? "flex-start" : "center"} flexShrink={0} position="relative" top="-1px" display="inline-flex" w="4" h={inlineReason ? "1lh" : "4"} fontSize="md" alignItems="center" justifyContent="center">
            {live ? <Arc label="Running" /> : <StatusIcon kind={icon!} tone={tone} />}
          </Box>
        )}
        <Text
          as="span"
          fontFamily="condensed"
          fontSize="sm"
          color={`tone.${tone}.fg`}
          flexShrink={0}
          // Ids differ in width, so a tone-titled list gives them one column to start the titles at the same x.
          display={toneTitle ? "inline-block" : undefined}
          minW={toneTitle ? "5" : undefined}
        >
          {id}
        </Text>
        {inlineReason ? (
          // One text block after the id, so a long title wraps under itself rather than under the id.
          <Box as="span" minW="0">
            {titleText}
            {stateText}
            {reason && (
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
        ) : (
          <>
            {titleText}
            {stateText}
          </>
        )}
      </Box>
      {!inlineReason && reason && (
        <Text
          data-reason
          fontSize="sm"
          color={tone === "paused" ? "tone.paused.fg" : "text.muted"}
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
        <Link href={href} fontSize="sm" color="tone.paused.fg">
          {linkLabel}
        </Link>
      )}
    </Box>
  );
}
