import { Box, Spinner, Text } from "@chakra-ui/react";
import { SessionExitCrashed, SessionWorking, type SessionInfo } from "../generated/server";
import { hasAttentionMarks, isActionable, statusMark } from "./rules";

/** The status glyph; Working is a spinner, as in the mock. */
export function StatusMark({ session: s }: { session: SessionInfo }) {
  const { glyph, tone, faded } = statusMark(s);
  const label = s.exit === SessionExitCrashed ? "crashed" : s.status.state;
  return (
    <Text as="span" role="img" aria-label={label} color={`tone.${tone}.fg`} opacity={faded ? 0.5 : 1} display="inline-flex" w="4" justifyContent="center" flex="none">
      {s.status.state === SessionWorking ? <Spinner size="xs" borderWidth="2px" /> : glyph}
    </Text>
  );
}

const actionableReason = (s: SessionInfo) => s.actionable.reason || s.status.message || "Blocked";

/** The status glyph and the Design diamond that lead a session row. */
export function SessionGlyphs({ session: s }: { session: SessionInfo }) {
  return (
    <>
      <StatusMark session={s} />
      {s.design && (
        <Text as="span" role="img" aria-label="Design" fontSize="xs" color="kind.design.fg">
          ◆
        </Text>
      )}
    </>
  );
}

/** The unread dot and the actionable star; `tooltip` shows the reason on hover. */
export function SessionBadges({ session: s, tooltip }: { session: SessionInfo; tooltip?: boolean }) {
  return (
    <>
      {hasAttentionMarks(s) && s.unread.unread && (
        <Text as="span" role="img" aria-label="Unread" color="kind.design.fg">
          ●
        </Text>
      )}
      {isActionable(s) &&
        (tooltip ? (
          <Box as="span" className="group" position="relative" color="tone.waiting.fg">
            <Text as="span" role="img" aria-label="Actionable">
              ★
            </Text>
            <Box
              as="span"
              role="tooltip"
              display="none"
              _groupHover={{ display: "block" }}
              position="absolute"
              zIndex="5"
              top="100%"
              right="0"
              mt="1"
              px="2.5"
              py="1.5"
              borderWidth="1px"
              borderColor="border"
              borderRadius="md"
              bg="surface.raised"
              boxShadow="0 6px 20px rgba(0,0,0,.5)"
              fontSize="xs"
              color="text"
              whiteSpace="nowrap"
            >
              <Box as="b" color="tone.waiting.fg">
                ★ Actionable:
              </Box>{" "}
              {actionableReason(s)}
            </Box>
          </Box>
        ) : (
          <Text as="span" role="img" aria-label="Actionable" color="tone.waiting.fg">
            ★
          </Text>
        ))}
    </>
  );
}
