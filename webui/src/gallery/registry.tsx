import { Box } from "@chakra-ui/react";
import type { ReactNode } from "react";
import canned from "../canned-snapshot.json";
import { Arc } from "../ui/arc/Arc";
import { StatusIcon, STATUS_LABELS, type StatusKind } from "../ui/status-icon/StatusIcon";
import { Badge } from "../ui/badge/Badge";
import { Button } from "../ui/button/Button";
import { Card } from "../ui/card/Card";
import { Chip } from "../ui/chip/Chip";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { LiveDot } from "../ui/live-dot/LiveDot";
import { ProgressBar } from "../ui/progress-bar/ProgressBar";
import { SegmentedBar } from "../ui/progress-bar/SegmentedBar";
import { Spinner } from "../ui/spinner/Spinner";
import { Strip } from "../ui/strip/Strip";
import { TicketLine } from "../ui/ticket-line/TicketLine";
import { TopBar } from "../ui/top-bar/TopBar";
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
  { name: "Spinner", cases: perTone((tone) => <Spinner label={`Loading ${tone}`} tone={tone} />) },
  { name: "Arc", cases: perTone((tone) => <Arc tone={tone} label={tone} />) },
  {
    name: "StatusIcon",
    cases: perTone((tone) => (
      <Box display="flex" gap="2">
        {(Object.keys(STATUS_LABELS) as StatusKind[]).map((kind) => (
          <StatusIcon key={kind} kind={kind} tone={tone} />
        ))}
      </Box>
    )),
  },
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
      { label: "live", node: <TicketLine id={ticket.address} title={ticket.title} live /> },
      { label: "live, inline reason", node: <TicketLine id={ticket.address} title={ticket.title} reason="claimed 4m ago" tone="running" inlineReason live /> },
      { label: "with reason", node: <TicketLine id={ticket.address} title={ticket.title} reason="Blocked by 01" /> },
      { label: "clamped reason", node: <TicketLine id={ticket.address} title={ticket.title} reason={longReason} /> },
      {
        label: "answerable",
        node: (
          <TicketLine id={ticket.address} title={ticket.title} tone="paused" reason={longReason} unclipped href="#/t/gx:demo/02" />
        ),
      },
      {
        label: "needs repair",
        node: <TicketLine id={ticket.address} title={ticket.title} tone="paused" reason="Worktree is dirty" hint="Fix it in the TUI" />,
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
      {
        label: "live",
        node: (
          <Card title="running" tone="running" live>
            {ticket.title}
          </Card>
        ),
      },
    ],
  },
  {
    name: "Button",
    cases: [
      ...perTone((tone) => <Button tone={tone}>{tone}</Button>),
      ...(["solid", "tinted", "outline", "ghost"] as const).map((variant) => ({
        label: variant,
        node: <Button variant={variant}>{variant}</Button>,
      })),
      { label: "md", node: <Button size="md">md</Button> },
      { label: "disabled", node: <Button disabled>disabled</Button> },
    ],
  },
  {
    name: "TopBar",
    cases: [
      ...perTone((tone) => <TopBar active="queue" tone={tone} build="v1.2.3" connected />),
      { label: "Queue active", node: <TopBar active="queue" build="v1.2.3" connected /> },
      { label: "Epics active", node: <TopBar active="epics" build="v1.2.3" connected /> },
      { label: "none active", node: <TopBar build="v1.2.3" connected /> },
      { label: "Paused", node: <TopBar active="queue" build="v1.2.3" mode="paused" connected /> },
      { label: "Draining", node: <TopBar active="queue" build="v1.2.3" mode="draining" connected /> },
      { label: "Offline", node: <TopBar active="queue" build="v1.2.3" connected={false} /> },
      { label: "no build string", node: <TopBar active="queue" connected /> },
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
