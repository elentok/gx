import { Box, Link, Text } from "@chakra-ui/react";
import type { EpicDetail } from "../epic/detail";
import { idOf, isCancelled, isFolded } from "../epic/rows";
import type { EpicDetailRow } from "../generated/server";
import { epicHref, ticketHref } from "../hrefs";
import { GAP, MARK_W, Row } from "../stepped/Progress";

const BRANCH = "branch";
const DONE = "done";

/** The Design-only slot of the step tree: the map's tickets under Resolve tickets. Empty rows render nothing. */
export function Tickets({ rows }: { rows: EpicDetailRow[] }) {
  if (rows.length === 0) return null;
  const closed = rows.filter(isFolded);
  const cancelled = closed.filter(isCancelled).length;
  return (
    <Box pl={`calc(${MARK_W} + var(--chakra-spacing-${GAP}))`} mt="1" mb="2" display="flex" flexDirection="column" gap="1">
      {closed.length > 0 && (
        <Row glyph="✓" tone="done">
          <Text as="span" color="text.muted">
            {closed.length} closed{cancelled > 0 && ` · ${cancelled} cancelled`}
          </Text>
        </Row>
      )}
      {rows
        .filter((r) => !isFolded(r))
        .map((r) => (
          <Box key={r.address} data-current={r.current || undefined}>
            <Row glyph={r.current ? "▸" : "·"} tone={r.current ? "running" : "muted"}>
              <Text as="span" color="text.dim" mr="2">
                {idOf(r.address)}
              </Text>
              <Link href={ticketHref(r.address)} color={r.current ? "tone.running.fg" : "text"} display="inline">
                {r.title}
              </Link>
              {r.blocked && (
                <Text as="span" role="img" aria-label="Blocked" title="Blocked" color="text.muted" ml="2">
                  🔒
                </Text>
              )}
            </Row>
          </Box>
        ))}
    </Box>
  );
}

export const ImplLink = ({ address }: { address: string }) => (
  <Link href={epicHref(address)} fontSize="sm" color="tone.running.fg">
    Open implementation epic →
  </Link>
);

/** Progress links to the implementation epic once the design is handed off: at Branch and Done. */
export const showsImplLink = (detail: EpicDetail) =>
  !!detail.impl_address && !!detail.steps?.some((s) => (s.step === BRANCH || s.step === DONE) && s.state !== "todo");
