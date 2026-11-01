import { Box, Spinner, Text } from "@chakra-ui/react";
import { useEffect, useRef, type ReactNode } from "react";
import type { Tone } from "../tone";
import { useMenuRoom } from "./useMenuRoom";

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
  /** False where the field has no send key (the New session form): the hints then say the menu only fills. */
  sends?: boolean;
  /** The row list's height cap. A field inside a scrolling dialog needs it short enough that the menu is not clipped. */
  listMaxH?: string;
  onPick: (index: number) => void;
}

// Presentational only: the field keeps focus, so rows swallow mousedown instead of taking it.
export function CommandMenu({ rows, selected, status, typed, compact = false, placement = "above", tone = "running", sends = true, listMaxH = "352px", onPick }: CommandMenuProps) {
  const showRows = status === "ready" || status === "failed";
  const ref = useRef<HTMLDivElement>(null);
  // Only the phone has an on-screen keyboard to make room for.
  const room = useMenuRoom(ref, placement, compact);
  return (
    <Box
      ref={ref}
      maxH={room === undefined ? undefined : `${room}px`}
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
      {status === "failed" && <Note warn>⚠ Skills could not load. Built-ins only.</Note>}
      {status === "loading" && (
        <Note>
          <Spinner size="xs" mr="2" verticalAlign="middle" color={`tone.${tone}.fg`} />
          Loading commands…
        </Note>
      )}
      {status === "no-match" && (
        <>
          <Note>
            No command matches{" "}
            <Text as="span" fontFamily="mono">
              {typed}
            </Text>
            {sends ? `. ${compact ? "Send" : "Cmd/Ctrl+Enter"} sends it as typed.` : "."}
          </Note>
          {!compact && <Footer>Esc closes</Footer>}
        </>
      )}
      {showRows && rows.length > 0 && (
        <Box role="listbox" aria-label="Commands" display="flex" flexDirection="column" p="1.5" gap="1px" maxH={listMaxH} minH="0" overflowY="auto">
          {rows.map((row, i) => (
            <Row key={`${row.source}:${row.name}`} row={row} active={i === selected} compact={compact} tone={tone} onPick={() => onPick(i)} />
          ))}
        </Box>
      )}
      {showRows && !compact && (
        <Footer>
          <span>↑↓ select</span>
          <span>Enter fills</span>
          <span>{sends ? "Cmd/Ctrl+Enter sends" : "fills the text, does not send"}</span>
          <span>Esc closes</span>
        </Footer>
      )}
    </Box>
  );
}

function Footer({ children }: { children: ReactNode }) {
  return (
    <Box display="flex" gap="3.5" alignItems="center" px="3" py="1.5" borderTopWidth="1px" borderColor="hairline" fontSize="xs" color="text.dim">
      {children}
    </Box>
  );
}

function Note({ warn = false, children }: { warn?: boolean; children: ReactNode }) {
  return (
    <Text px="3.5" py="2" fontSize="xs" color={warn ? "tone.waiting.fg" : "text.muted"} borderBottomWidth={warn ? "1px" : "0"} borderColor="hairline">
      {children}
    </Text>
  );
}

function Row({ row, active, compact, tone, onPick }: { row: CommandMenuRow; active: boolean; compact: boolean; tone: Tone; onPick: () => void }) {
  // The list scrolls inside the menu, so the arrow keys keep the selected row in view.
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (active) ref.current?.scrollIntoView?.({ block: "nearest" });
  }, [active]);
  return (
    <Box
      ref={ref}
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
      <Text as="span" fontFamily="mono" fontSize="xs" whiteSpace="nowrap" minW="0" overflow="hidden" textOverflow="ellipsis" gridColumn="1" gridRow="1">
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
