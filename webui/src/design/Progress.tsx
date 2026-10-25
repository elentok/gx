import { Box, Link, Text } from "@chakra-ui/react";
import { useState, type ReactNode } from "react";
import type { EpicDetail } from "../epic/detail";
import { idOf, isCancelled, isFolded } from "../epic/rows";
import type { EpicDetailRow, MapStep } from "../generated/server";
import { epicHref, ticketHref } from "../hrefs";
import { Button } from "../ui/button/Button";
import { SegmentedBar, type Segment } from "../ui/progress-bar/SegmentedBar";
import type { Tone } from "../ui/tone";

// The generated file is imported for types only: its tygo constants are not runtime values.
const RESOLVE_TICKETS = "resolve-tickets";
const BRANCH = "branch";
const DONE = "done";

const STEP_MARKS: Record<string, { glyph: string; tone: Tone }> = {
  done: { glyph: "✓", tone: "done" },
  current: { glyph: "▶", tone: "running" },
  todo: { glyph: "○", tone: "muted" },
};
const markOf = (s: MapStep) => STEP_MARKS[s.state] ?? STEP_MARKS.todo;

const ticketsOf = (steps: MapStep[], rows: EpicDetailRow[]) =>
  steps.some((s) => s.step === RESOLVE_TICKETS && s.state === "current") ? rows : [];

// The mark column is one fixed width, so every label in the tree starts at the same x.
const MARK_W = "1.25em";
const GAP = "2";

function Row({ glyph, tone, children }: { glyph: string; tone: Tone; children: ReactNode }) {
  return (
    <Box display="flex" alignItems="baseline" gap={GAP}>
      <Text as="span" aria-hidden w={MARK_W} flexShrink="0" textAlign="center" color={`tone.${tone}.fg`}>
        {glyph}
      </Text>
      <Box as="span" minW="0">
        {children}
      </Box>
    </Box>
  );
}

function Tickets({ rows }: { rows: EpicDetailRow[] }) {
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

function Steps({ steps, rows }: { steps: MapStep[]; rows: EpicDetailRow[] }) {
  const tickets = ticketsOf(steps, rows);
  return (
    <Box as="ol" fontSize="sm" listStyleType="none" m="0" p="0" display="flex" flexDirection="column" gap="1">
      {steps.map((s) => {
        const { glyph, tone } = markOf(s);
        return (
          <Box as="li" key={s.step} data-step={s.step} data-state={s.state}>
            <Row glyph={glyph} tone={tone}>
              <Text as="span" color={s.state === "todo" ? "text.muted" : "text"} fontWeight={s.state === "current" ? "700" : undefined}>
                {s.label}
              </Text>
            </Row>
            {s.step === RESOLVE_TICKETS && tickets.length > 0 && <Tickets rows={tickets} />}
          </Box>
        );
      })}
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

/** "Resolve tickets · 03 Chat content (2 of 7)": the current step, and its current ticket while tickets resolve. */
function summaryLine(steps: MapStep[], rows: EpicDetailRow[]): string {
  const step = steps.find((s) => s.state === "current") ?? steps.filter((s) => s.state === "done").at(-1);
  if (!step) return "";
  const at = ticketsOf(steps, rows).findIndex((r) => r.current);
  if (at < 0) return step.label;
  return `${step.label} · ${idOf(rows[at].address)} ${rows[at].title} (${at + 1} of ${rows.length})`;
}

function MobileProgress({ detail, steps }: { detail: EpicDetail; steps: MapStep[] }) {
  const [open, setOpen] = useState(false);
  const segments: Segment[] = steps.map((s) => ({ key: s.step, tone: markOf(s).tone, count: 1 }));
  return (
    <Box display="flex" flexDirection="column" gap="2">
      <SegmentedBar segments={segments} value={steps.filter((s) => s.state === "done").length} label="Design progress" />
      <Button emphasis="ghost" tone="muted" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
        {summaryLine(steps, detail.rows)} {open ? "▴" : "▾"}
      </Button>
      {open && <Steps steps={steps} rows={detail.rows} />}
      {showsImplLink(detail) && <ImplLink address={detail.impl_address!} />}
    </Box>
  );
}

/** The Design's Progress: its eight steps, and the map's tickets under Resolve tickets. On desktop the impl link sits in the chat header. */
export function Progress({ detail, wide }: { detail: EpicDetail; wide: boolean }) {
  const steps = detail.steps ?? [];
  // On desktop a panel like the Designs sidebar, only as tall as its steps.
  return wide ? (
    <Box bg="surface.raised" px="3" py="4">
      <Steps steps={steps} rows={detail.rows} />
    </Box>
  ) : (
    <MobileProgress detail={detail} steps={steps} />
  );
}
