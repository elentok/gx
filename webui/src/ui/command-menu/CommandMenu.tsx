import { Box, Text } from "@chakra-ui/react";
import type { Tone } from "../tone";

export interface CommandMenuSegment {
  text: string;
  match: boolean;
}

export interface CommandMenuRow {
  name: string;
  hint?: string;
  description: string;
  source: string;
  segments: CommandMenuSegment[];
}

export type CommandMenuStatus = "ready" | "loading" | "failed" | "no-match";

interface CommandMenuProps {
  rows: CommandMenuRow[];
  selected: number;
  status: CommandMenuStatus;
  // What the person typed, with its sigil, for the no-match line.
  typed: string;
  compact?: boolean;
  placement?: "above" | "below";
  tone?: Tone;
  onPick: (index: number) => void;
}

// Presentational only: the field keeps focus, so rows swallow mousedown instead of taking it.
export function CommandMenu({ rows, selected, status, typed, compact = false, placement = "above", tone = "running", onPick }: CommandMenuProps) {
  const showRows = status === "ready" || status === "failed";
  return (
    <Box
      data-tone={tone}
      data-placement={placement}
      position="absolute"
      left="0"
      right="0"
      {...(placement === "above" ? { bottom: "calc(100% + 6px)" } : { top: "calc(100% + 6px)" })}
      zIndex="6"
      bg="surface.raised"
      borderWidth="1px"
      borderColor="border"
      borderRadius="lg"
      boxShadow="0 12px 30px rgba(0,0,0,.5)"
      display="flex"
      flexDirection="column"
      overflow="hidden"
    >
      {status === "failed" && <Note warn>Skills could not load</Note>}
      {status === "loading" && <Note>Loading commands</Note>}
      {status === "no-match" && (
        <Note>{compact ? "Send sends it as typed." : `No command matches ${typed}. Cmd/Ctrl+Enter sends it as typed.`}</Note>
      )}
      {showRows && rows.length > 0 && (
        <Box role="listbox" aria-label="Commands" display="flex" flexDirection="column" p="1.5" gap="1px">
          {rows.map((row, i) => (
            <Row key={`${row.source}:${row.name}`} row={row} active={i === selected} compact={compact} tone={tone} onPick={() => onPick(i)} />
          ))}
        </Box>
      )}
      {showRows && !compact && (
        <Box display="flex" gap="3.5" alignItems="center" px="3" py="1.5" borderTopWidth="1px" borderColor="hairline" fontSize="xs" color="text.dim">
          <span>↑↓ select</span>
          <span>Enter fills</span>
          <span>Cmd/Ctrl+Enter sends</span>
          <span>Esc closes</span>
        </Box>
      )}
    </Box>
  );
}

function Note({ warn = false, children }: { warn?: boolean; children: string }) {
  return (
    <Text px="3.5" py="2" fontSize="xs" color={warn ? "tone.waiting.fg" : "text.muted"} borderBottomWidth={warn ? "1px" : "0"} borderColor="hairline">
      {children}
    </Text>
  );
}

function Row({ row, active, compact, tone, onPick }: { row: CommandMenuRow; active: boolean; compact: boolean; tone: Tone; onPick: () => void }) {
  return (
    <Box
      role="option"
      aria-selected={active}
      onMouseDown={(e) => e.preventDefault()}
      onClick={onPick}
      cursor="pointer"
      display="grid"
      gridTemplateColumns={compact ? "minmax(0,1fr) auto" : "minmax(0,max-content) minmax(0,1fr) auto"}
      columnGap="3"
      rowGap="1"
      alignItems="baseline"
      px="2.5"
      py={compact ? "2" : "1.5"}
      borderRadius="md"
      fontSize="sm"
      bg={active ? "surface" : "transparent"}
      boxShadow={active ? `inset 2px 0 0 var(--chakra-colors-tone-${tone}-fg)` : "none"}
    >
      <Text as="span" fontFamily="mono" fontSize="xs" whiteSpace="nowrap" gridColumn="1" gridRow="1">
        {row.segments.map((s, i) =>
          s.match ? (
            <Text as="mark" key={i} bg="transparent" color={`tone.${tone}.fg`} fontWeight="700">
              {s.text}
            </Text>
          ) : (
            <span key={i}>{s.text}</span>
          ),
        )}
        {row.hint && <Text as="i" fontStyle="normal" color="text.dim">{` ${row.hint}`}</Text>}
      </Text>
      <Text
        as="span"
        color="text.dim"
        fontSize="xs"
        minW="0"
        overflow="hidden"
        textOverflow="ellipsis"
        whiteSpace={compact ? "normal" : "nowrap"}
        gridColumn={compact ? "1 / -1" : "2"}
        gridRow={compact ? "2" : "1"}
        lineClamp={compact ? 1 : undefined}
      >
        {row.description}
      </Text>
      <Text
        as="span"
        fontSize="11px"
        color="text.dim"
        borderWidth="1px"
        borderColor="hairline"
        borderRadius="full"
        px="1.5"
        whiteSpace="nowrap"
        gridColumn={compact ? "2" : "3"}
        gridRow="1"
      >
        {row.source}
      </Text>
    </Box>
  );
}
