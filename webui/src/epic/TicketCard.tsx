import { Box, chakra, Link, Text } from "@chakra-ui/react";
import { useState } from "react";
import { useNavigate } from "react-router";
import { ticketHref } from "../hrefs";
import { useWide } from "../design/useWide";
import { Button } from "../ui/button/Button";
import { StatusIcon } from "../ui/status-icon/StatusIcon";
import { type EpicDetailRow } from "./detail";
import { FixAgentDialog } from "./FixAgentDialog";
import type { FixAgent } from "./fixAgent";
import { cardLook, type CardLook, idOf, isRunning, toneOf } from "./rows";

const RUNNING_BLUE = "#6cb4ff";

// A 30% arc on a faint track, spinning.
function Spinner() {
  return (
    <chakra.svg
      role="img"
      aria-label="Running"
      data-tone="running"
      viewBox="0 0 10 10"
      w="10px"
      h="10px"
      animation="spin 1.05s linear infinite"
      _motionReduce={{ animation: "none" }}
    >
      <circle
        cx="5"
        cy="5"
        r="4"
        fill="none"
        stroke="rgba(108,180,255,0.3)"
        strokeWidth="2"
      />
      <circle
        cx="5"
        cy="5"
        r="4"
        fill="none"
        stroke={RUNNING_BLUE}
        strokeWidth="2"
        strokeLinecap="round"
        strokeDasharray="7.54 25.13"
      />
    </chakra.svg>
  );
}

function Dot({ row }: { row: EpicDetailRow }) {
  const tone = toneOf(row);
  let mark;
  if (isRunning(row)) mark = <Spinner />;
  else if (tone === "done") mark = <StatusIcon kind="done" tone={tone} />;
  else
    mark = (
      <Box
        as="span"
        data-dot
        w="8px"
        h="8px"
        borderRadius="full"
        bg={`tone.${tone}.fg`}
      />
    );
  return (
    <Box
      as="span"
      flexShrink={0}
      display="inline-flex"
      w="16px"
      h="16px"
      alignItems="center"
      justifyContent="center"
    >
      {mark}
    </Box>
  );
}

const metaColor = (row: EpicDetailRow) =>
  isRunning(row) ? RUNNING_BLUE : cardLook(row).kind === "note" ? "#ff7066" : "text.dim";

// Wide: one row. Phone: the meta drops to a second line, indented under the title.
function Line({
  row,
  meta,
  wide,
  titleLink,
  bold,
}: {
  row: EpicDetailRow;
  meta: string;
  wide: boolean;
  titleLink?: boolean;
  bold?: boolean;
}) {
  const title = titleLink ? (
    <Link href={ticketHref(row.address)} color="inherit" display="inline">
      {row.title}
    </Link>
  ) : (
    row.title
  );
  const metaNode = meta && (
    <Text
      as="span"
      data-meta
      fontSize="13px"
      color={metaColor(row)}
      whiteSpace="nowrap"
      flexShrink={0}
      pl={wide ? undefined : "54px"}
    >
      {meta}
    </Text>
  );
  return (
    <Box
      display="flex"
      flexDirection={wide ? "row" : "column"}
      alignItems={wide ? "center" : "stretch"}
      gap={wide ? "12px" : "2px"}
      minW="0"
    >
      <Box
        display="flex"
        alignItems="center"
        gap={wide ? "12px" : "10px"}
        minW="0"
        flex={wide ? "1 1 0" : undefined}
      >
        <Dot row={row} />
        <Text
          as="span"
          fontFamily="ui-monospace, monospace"
          fontSize="12px"
          color="text.dim"
          flexShrink={0}
          w={wide ? "28px" : "26px"}
        >
          {idOf(row.address)}
        </Text>
        <Text
          as="span"
          color="white"
          fontWeight={bold ? 500 : undefined}
          minW="0"
          flex="1 1 0"
          overflow="hidden"
          textOverflow="ellipsis"
          whiteSpace="nowrap"
        >
          {title}
        </Text>
      </Box>
      {metaNode}
    </Box>
  );
}

// On phones the card's buttons span the row at touch height.
const phoneButton = (wide: boolean) => (wide ? {} : { w: "100%", h: "44px" });

function CardFrame({
  row,
  wide,
  children,
}: {
  row: EpicDetailRow;
  wide: boolean;
  children: React.ReactNode;
}) {
  return (
    <Box
      bg="#141920"
      borderWidth="1px"
      borderColor={isRunning(row) ? "rgba(108,180,255,0.40)" : "#252c36"}
      borderRadius="12px"
      h={wide ? "48px" : undefined}
      px={wide ? "16px" : "14px"}
      py={wide ? undefined : "10px"}
      display="flex"
      alignItems="center"
    >
      <Box flex="1" minW="0">
        {children}
      </Box>
    </Box>
  );
}

// A needs-you or needs-repair ticket: the title is the link and its question or hint sits in a second row.
function NoteCard({
  row,
  meta,
  wide,
  note,
  fixAgent,
}: {
  row: EpicDetailRow;
  meta: string;
  wide: boolean;
  note: Extract<CardLook, { kind: "note" }>;
  fixAgent?: FixAgent;
}) {
  const navigate = useNavigate();
  const [fixing, setFixing] = useState(false);
  const { answerPath } = note;
  return (
    <Box
      bg="rgba(255,112,102,0.07)"
      borderWidth="1px"
      borderColor="rgba(255,112,102,0.35)"
      borderRadius="12px"
      px="16px"
      py="12px"
      display="flex"
      flexDirection="column"
      gap="10px"
    >
      <Line row={row} meta={meta} wide={wide} titleLink bold />
      <Box
        data-note-row
        display="flex"
        flexDirection={wide ? "row" : "column"}
        alignItems={wide ? "center" : "stretch"}
        gap={wide ? "12px" : "10px"}
        pl={wide ? "60px" : "54px"}
      >
        {note.text && (
          <Text
            data-hint
            fontSize="14px"
            color="text.muted"
            flex={wide ? "1 1 0" : undefined}
            minW="0"
          >
            {note.text}
          </Text>
        )}
        {answerPath && (
          <Button
            tone="paused"
            size="sm"
            onClick={() => navigate(answerPath)}
            {...phoneButton(wide)}
          >
            Answer
          </Button>
        )}
        {note.fix && (
          <Button
            tone="paused"
            size="sm"
            onClick={() => setFixing(true)}
            {...phoneButton(wide)}
          >
            Fix it with an agent
          </Button>
        )}
      </Box>
      {fixing && (
        <FixAgentDialog
          row={row}
          fixAgent={fixAgent}
          onClose={() => setFixing(false)}
        />
      )}
    </Box>
  );
}

// A live ticket as one anchor to its page, or a NoteCard when it needs a person.
export function TicketCard({
  row,
  meta,
  indent,
  fixAgent,
}: {
  row: EpicDetailRow;
  meta: string;
  indent: number;
  fixAgent?: FixAgent;
}) {
  const look = cardLook(row);
  const wide = useWide();
  return (
    <Box
      data-indent={indent}
      data-tone={toneOf(row)}
      ml={`${indent * (wide ? 28 : 20)}px`}
    >
      {look.kind === "note" ? (
        <NoteCard row={row} meta={meta} wide={wide} note={look} fixAgent={fixAgent} />
      ) : (
        <Link
          href={ticketHref(row.address)}
          display="block"
          color="inherit"
          _hover={{ textDecoration: "none" }}
        >
          <CardFrame row={row} wide={wide}>
            <Line row={row} meta={meta} wide={wide} />
          </CardFrame>
        </Link>
      )}
    </Box>
  );
}
