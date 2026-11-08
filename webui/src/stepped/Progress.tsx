import { Box, Text, chakra } from "@chakra-ui/react";
import type { ReactNode } from "react";
import { idOf } from "../epic/rows";
import type { MapStep } from "../generated/server";
import { Button } from "../ui/button/Button";
import { BottomSheet } from "../ui/modal/BottomSheet";
import type { Tone } from "../ui/tone";
import { RESOLVE_TICKETS, type Flow, type SteppedModel } from "./model";

const STEP_MARKS: Record<string, { glyph: string; tone: Tone }> = {
  done: { glyph: "✓", tone: "done" },
  current: { glyph: "▶", tone: "running" },
  todo: { glyph: "○", tone: "muted" },
};
const markOf = (s: MapStep) => STEP_MARKS[s.state] ?? STEP_MARKS.todo;

const SHEET_LABELS: Record<Flow, string> = {
  design: "Design progress steps",
  triage: "Triage progress steps",
};

// The mark column is one fixed width, so every label in the tree starts at the same x.
export const MARK_W = "16px";
export const GAP = "2";

export function Row({ glyph, tone, children }: { glyph: string; tone: Tone; children: ReactNode }) {
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

/** `rows` is a flow's own sub-tree, shown under the Resolve tickets step (Design only). */
function Steps({ model, rows }: { model: SteppedModel; rows?: ReactNode }) {
  return (
    <Box as="ol" fontSize="sm" listStyleType="none" m="0" p="0" display="flex" flexDirection="column" gap="1">
      {model.steps.map((s) => {
        const { glyph, tone } = markOf(s);
        return (
          <Box as="li" key={s.step} data-step={s.step} data-state={s.state}>
            <Row glyph={glyph} tone={tone}>
              <Text as="span" color={s.state === "todo" ? "text.muted" : "text"} fontWeight={s.state === "current" ? "700" : undefined}>
                {s.label}
              </Text>
            </Row>
            {s.step === RESOLVE_TICKETS && rows}
          </Box>
        );
      })}
    </Box>
  );
}

/** The current step's label and, while tickets resolve, the position of its current ticket. */
function progressOf(model: SteppedModel) {
  const step = model.current ?? model.steps.filter((s) => s.state === "done").at(-1);
  if (!step) return undefined;
  return { label: step.label, rows: model.tickets, at: model.tickets.findIndex((r) => r.current) };
}

/** "Resolve tickets · 03 Chat content (2 of 7)": the current step, and its current ticket while tickets resolve. */
export function summaryLine(model: SteppedModel): string {
  const p = progressOf(model);
  if (!p) return "";
  if (p.at < 0) return p.label;
  return `${p.label} · ${idOf(p.rows[p.at].address)} ${p.rows[p.at].title} (${p.at + 1} of ${p.rows.length})`;
}

/** A phone's summary, without the ticket: "Resolve tickets · 2 of 7". */
export function shortSummaryLine(model: SteppedModel): string {
  const p = progressOf(model);
  if (!p) return "";
  if (p.at < 0) return p.label;
  return `${p.label} · ${p.at + 1} of ${p.rows.length}`;
}

/** The phone's step line under the page name; it and the header ring open the steps sheet. */
export function StepLine({ model, onOpen }: { model: SteppedModel; onOpen: () => void }) {
  return (
    <chakra.button
      type="button"
      aria-haspopup="dialog"
      onClick={onOpen}
      display="flex"
      gap="1"
      maxW="full"
      fontSize="inherit"
      color="inherit"
      cursor="pointer"
      textAlign="left"
    >
      <Text as="span" truncate>
        {shortSummaryLine(model)}
      </Text>
      <Text as="span" aria-hidden flexShrink="0">
        ▾
      </Text>
    </chakra.button>
  );
}

/** The phone's bottom sheet with the step tree. `ring` is a small ProgressRing. */
export function StepsSheet({
  model,
  ring,
  rows,
  onClose,
}: {
  model: SteppedModel;
  ring: ReactNode;
  rows?: ReactNode;
  onClose: () => void;
}) {
  return (
    <BottomSheet label={SHEET_LABELS[model.flow]} onClose={onClose} bg="surface.raised">
      <Box display="flex" alignItems="center" gap="2">
        {ring}
        <Text fontWeight="700" flex="1">
          Progress · {model.ringPercent}%
        </Text>
        <Button emphasis="ghost" tone="muted" onClick={onClose}>
          Close
        </Button>
      </Box>
      <Steps model={model} rows={rows} />
    </BottomSheet>
  );
}

/** The desktop Progress sidebar: the flow's steps, and `rows` under Resolve tickets. */
export function Progress({ model, rows }: { model: SteppedModel; rows?: ReactNode }) {
  // A panel like the Designs sidebar, only as tall as its steps.
  return (
    <Box bg="surface.raised" px="3" py="4">
      <Steps model={model} rows={rows} />
    </Box>
  );
}
