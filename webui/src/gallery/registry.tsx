import type { ReactNode } from "react";
import canned from "../canned-snapshot.json";
import { Badge } from "../ui/badge/Badge";
import { Card } from "../ui/card/Card";
import { Chip } from "../ui/chip/Chip";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { LiveDot } from "../ui/live-dot/LiveDot";
import { ProgressBar } from "../ui/progress-bar/ProgressBar";
import { SegmentedBar } from "../ui/progress-bar/SegmentedBar";
import { Strip } from "../ui/strip/Strip";
import { TicketLine } from "../ui/ticket-line/TicketLine";
import { TONES, type Tone } from "../ui/tone";

export interface GalleryCase {
  label: string;
  node: ReactNode;
}

export interface GalleryEntry {
  name: string;
  cases: GalleryCase[];
}

const ticket = canned.tickets[0];
const longReason = "Waiting on a long chain of upstream tickets that all have to land first. ".repeat(4);

const perTone = (render: (tone: Tone) => ReactNode): GalleryCase[] =>
  TONES.map((tone) => ({ label: tone, node: render(tone) }));

// Add one entry per primitive; the gallery page renders every case.
export const galleryEntries: GalleryEntry[] = [
  { name: "Badge", cases: perTone((tone) => <Badge tone={tone}>{tone}</Badge>) },
  {
    name: "ProgressBar",
    cases: [
      ...perTone((tone) => <ProgressBar value={6} max={10} tone={tone} />),
      { label: "empty", node: <ProgressBar value={0} max={10} /> },
      { label: "full", node: <ProgressBar value={10} max={10} /> },
    ],
  },
  {
    name: "SegmentedBar",
    cases: perTone((tone) => (
      <SegmentedBar
        label={tone}
        value={4}
        segments={[
          { key: "done", tone: "done", count: 4 },
          { key: tone, tone, count: 3 },
          { key: "empty", tone: "waiting", count: 0 },
        ]}
      />
    )),
  },
  {
    name: "TicketLine",
    cases: [
      ...perTone((tone) => <TicketLine id={ticket.address} title={ticket.title} tone={tone} />),
      { label: "with reason", node: <TicketLine id={ticket.address} title={ticket.title} reason="Blocked by 01" /> },
      { label: "clamped reason", node: <TicketLine id={ticket.address} title={ticket.title} reason={longReason} /> },
      {
        label: "answerable",
        node: (
          <TicketLine id={ticket.address} title={ticket.title} tone="parked" reason={longReason} unclipped href="#/t/gx:demo/02" />
        ),
      },
      {
        label: "needs repair",
        node: <TicketLine id={ticket.address} title={ticket.title} tone="parked" reason="Worktree is dirty" hint="Fix it in the TUI" />,
      },
      {
        label: "answered",
        node: <TicketLine id={ticket.address} title={ticket.title} tone="waiting" state="Answered · #2 in queue" />,
      },
    ],
  },
  {
    name: "Card",
    cases: [
      ...perTone((tone) => (
        <Card title={tone} tone={tone}>
          {ticket.title}
        </Card>
      )),
      { label: "no title", node: <Card>{ticket.title}</Card> },
    ],
  },
  { name: "Chip", cases: perTone((tone) => <Chip tone={tone}>{tone}</Chip>) },
  { name: "LiveDot", cases: perTone((tone) => <LiveDot tone={tone} label={tone} />) },
  { name: "Strip", cases: perTone((tone) => <Strip tone={tone}>{tone}</Strip>) },
  {
    name: "EmptyState",
    cases: [
      ...perTone((tone) => <EmptyState tone={tone} title="All quiet" />),
      { label: "with body", node: <EmptyState title="All quiet">Nothing is running.</EmptyState> },
    ],
  },
];
