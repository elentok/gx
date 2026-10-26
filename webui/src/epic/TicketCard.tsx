import { Box, Link, Text } from "@chakra-ui/react";
import { useNavigate } from "react-router";
import { ticketHref } from "../hrefs";
import { Arc } from "../ui/arc/Arc";
import { Button } from "../ui/button/Button";
import { StatusIcon } from "../ui/status-icon/StatusIcon";
import { type EpicDetailRow } from "./detail";
import { cardNote, idOf, isExpanded, isRunning, toneOf } from "./rows";

function Dot({ row }: { row: EpicDetailRow }) {
  const tone = toneOf(row);
  let mark;
  if (isRunning(row)) mark = <Arc label="Running" />;
  else if (tone === "done") mark = <StatusIcon kind="done" tone={tone} />;
  else mark = <Box as="span" data-dot w="2" h="2" borderRadius="full" bg={`tone.${tone}.fg`} />;
  return (
    <Box as="span" flexShrink={0} display="inline-flex" w="4" h="4" alignItems="center" justifyContent="center">
      {mark}
    </Box>
  );
}

// On phones the meta wraps under the title: its basis is the full row there and its own width from md up.
function Line({ row, meta, titleLink }: { row: EpicDetailRow; meta: string; titleLink?: boolean }) {
  const tone = toneOf(row);
  const title = titleLink ? (
    <Link href={ticketHref(row.address)} color="inherit" display="inline">
      {row.title}
    </Link>
  ) : (
    row.title
  );
  return (
    <Box display="flex" flexWrap="wrap" alignItems="baseline" columnGap="2" rowGap="0">
      <Dot row={row} />
      <Text as="span" fontFamily="condensed" fontSize="sm" color={`tone.${tone}.fg`} flexShrink={0}>
        {idOf(row.address)}
      </Text>
      <Text as="span" fontSize="md" color={`tone.${tone}.fg`} minW="0" flex={{ base: "1 1 0", md: "0 1 auto" }}>
        {title}
      </Text>
      {meta && (
        <Text
          as="span"
          data-meta
          fontSize="sm"
          color="text.muted"
          flexBasis={{ base: "100%", md: "auto" }}
          pl={{ base: "6", md: "0" }}
        >
          {meta}
        </Text>
      )}
    </Box>
  );
}

// A live ticket as one anchor to its page; a needs-you or needs-repair ticket stays expanded, with the title as the
// link and its question or hint in a second row.
export function TicketCard({ row, meta, indent }: { row: EpicDetailRow; meta: string; indent: number }) {
  const navigate = useNavigate();
  const note = isExpanded(row) ? cardNote(row) : undefined;
  return (
    <Box data-indent={indent} data-tone={toneOf(row)} ml={indent * 6}>
      {note ? (
        <>
          <Line row={row} meta={meta} titleLink />
          <Box display="flex" flexWrap="wrap" alignItems="center" gap="3" pl="6" mt="1">
            {note.text && (
              <Text data-hint fontSize="sm" color="text.muted">
                {note.text}
              </Text>
            )}
            {note.answerHref && (
              <Button tone="paused" size="sm" onClick={() => navigate(note.answerHref!.slice(1))}>
                Answer
              </Button>
            )}
          </Box>
        </>
      ) : (
        <Link href={ticketHref(row.address)} display="block" color="inherit" _hover={{ textDecoration: "none" }}>
          <Line row={row} meta={meta} />
        </Link>
      )}
    </Box>
  );
}
