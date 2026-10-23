import { Box, Link, Text } from "@chakra-ui/react";
import { useState } from "react";
import type { EpicDetail } from "../epic/detail";
import { idOf, isCancelled, isFolded } from "../epic/rows";
import type { EpicDetailRow, MapStep } from "../generated/server";
import { epicHref, ticketHref } from "../hrefs";
import { Button } from "../ui/button/Button";
import { SegmentedBar, type Segment } from "../ui/progress-bar/SegmentedBar";
import type { Tone } from "../ui/tone";

// The generated file is imported for types only: its tygo constants are not runtime values.
const RESOLVE_TICKETS = "resolve-tickets";

const STEP_MARKS: Record<string, { glyph: string; tone: Tone }> = {
  done: { glyph: "✓", tone: "done" },
  current: { glyph: "▶", tone: "running" },
  todo: { glyph: "○", tone: "muted" },
};
const markOf = (s: MapStep) => STEP_MARKS[s.state] ?? STEP_MARKS.todo;

const ticketsOf = (steps: MapStep[], rows: EpicDetailRow[]) =>
  steps.some((s) => s.step === RESOLVE_TICKETS && s.state === "current") ? rows : [];

function Tickets({ rows }: { rows: EpicDetailRow[] }) {
  const closed = rows.filter(isFolded);
  const cancelled = closed.filter(isCancelled).length;
  return (
    <Box ml="6" mt="1" mb="2" display="flex" flexDirection="column" gap="1" fontSize="sm">
      {closed.length > 0 && (
        <Text color="tone.done.fg">
          ✓ {closed.length} closed{cancelled > 0 && ` · ${cancelled} cancelled`}
        </Text>
      )}
      {rows
        .filter((r) => !isFolded(r))
        .map((r) => (
          <Box key={r.address} data-current={r.current || undefined} display="flex" gap="2" alignItems="baseline">
            <Text as="span" fontFamily="condensed" color={r.current ? "tone.running.fg" : "text.muted"}>
              {idOf(r.address)}
            </Text>
            <Link href={ticketHref(r.address)} color={r.current ? "tone.running.fg" : "text"} fontWeight={r.current ? "700" : undefined} display="inline">
              {r.title}
            </Link>
            {r.blocked && (
              <Text as="span" role="img" aria-label="Blocked" title="Blocked" color="text.muted">
                🔒
              </Text>
            )}
          </Box>
        ))}
    </Box>
  );
}

function Steps({ steps, rows }: { steps: MapStep[]; rows: EpicDetailRow[] }) {
  const tickets = ticketsOf(steps, rows);
  return (
    <Box as="ol" listStyleType="none" m="0" p="0" borderLeftWidth="2px" borderColor="border" pl="3">
      {steps.map((s) => {
        const { glyph, tone } = markOf(s);
        return (
          <Box as="li" key={s.step} data-step={s.step} data-state={s.state}>
            <Text color={s.state === "todo" ? "text.muted" : "text"} fontWeight={s.state === "current" ? "700" : undefined}>
              <Text as="span" aria-hidden color={`tone.${tone}.fg`} mr="2">
                {glyph}
              </Text>
              {s.label}
            </Text>
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
      <Button variant="ghost" tone="muted" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
        {summaryLine(steps, detail.rows)} {open ? "▴" : "▾"}
      </Button>
      {open && <Steps steps={steps} rows={detail.rows} />}
      {detail.impl_address && <ImplLink address={detail.impl_address} />}
    </Box>
  );
}

/** The Design's Progress: its eight steps, and the map's tickets under Resolve tickets. On desktop the impl link sits in the chat header. */
export function Progress({ detail, wide }: { detail: EpicDetail; wide: boolean }) {
  const steps = detail.steps ?? [];
  return wide ? <Steps steps={steps} rows={detail.rows} /> : <MobileProgress detail={detail} steps={steps} />;
}
