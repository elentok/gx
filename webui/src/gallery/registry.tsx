import { Box } from "@chakra-ui/react";
import { useEffect, useRef, type ReactNode } from "react";
import canned from "../canned-snapshot.json";
import { ConnectionContext, type Connection } from "../connection";
import { NewDesignForm } from "../design/NewDesignForm";
import { WideContext } from "../design/useWide";
import { EpicCard } from "../epics/EpicCard";
import type { EpicListRow } from "../generated/server";
import { JumpRow } from "../nav/JumpMenu";
import { Arc } from "../ui/arc/Arc";
import { StatusIcon, STATUS_LABELS, type StatusKind } from "../ui/status-icon/StatusIcon";
import { Badge } from "../ui/badge/Badge";
import { Button } from "../ui/button/Button";
import { Card } from "../ui/card/Card";
import { Chip } from "../ui/chip/Chip";
import { DayChart } from "../ui/day-chart/DayChart";
import { LineChart } from "../ui/day-chart/LineChart";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { LiveDot } from "../ui/live-dot/LiveDot";
import { ProgressBar } from "../ui/progress-bar/ProgressBar";
import { ProgressRing } from "../ui/progress-ring/ProgressRing";
import { SegmentedBar } from "../ui/progress-bar/SegmentedBar";
import { Sidebar } from "../ui/sidebar/Sidebar";
import { SidebarItem } from "../ui/sidebar/SidebarItem";
import { SidebarSection } from "../ui/sidebar/SidebarSection";
import { Spinner } from "../ui/spinner/Spinner";
import { StatCard } from "../ui/stat-card/StatCard";
import { Strip } from "../ui/strip/Strip";
import { TicketLine } from "../ui/ticket-line/TicketLine";
import { TopBar } from "../ui/top-bar/TopBar";
import { KINDS, TONES, type Tone } from "../ui/tone";

export interface GalleryCase {
  label: string;
  node: ReactNode;
}

export interface GalleryEntry {
  name: string;
  cases: GalleryCase[];
}

const ticket = canned.tickets[0];
const longReason = "Waiting on a long chain of upstream tickets that all have to merge first. ".repeat(4);

const cardRow = (over: Partial<EpicListRow> = {}): EpicListRow => ({
  address: "demo:blog",
  project: "demo",
  name: "blog",
  kind: "loop",
  state: "running",
  done: 2,
  total: 5,
  queued: false,
  queueable: false,
  blocked_by_epics: [],
  addresses: ["demo:blog"],
  card_kind: "implementation",
  section: "in-progress",
  status_words: "Implementing...",
  ring_progress: 40,
  actions: [],
  ...over,
});

const idleConnection = { refresh: () => {} } as Connection;

// A transform makes the modal's fixed overlay fill this frame instead of the page.
const ModalFrame = ({ children }: { children: ReactNode }) => (
  <ConnectionContext.Provider value={idleConnection}>
    <Box position="relative" h="600px" w="800px" transform="translateZ(0)" overflow="hidden" borderWidth="1px" borderColor="border">
      {children}
    </Box>
  </ConnectionContext.Provider>
);

// Opens a menu the way a user does, by clicking its trigger once mounted.
const OpenMenu = ({ children }: { children: ReactNode }) => {
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => root.current?.querySelector<HTMLElement>("button[aria-label=Menu]")?.click(), []);
  return <div ref={root}>{children}</div>;
};

const chartDays =["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

const perTone =(render: (tone: Tone) => ReactNode): GalleryCase[] =>
  TONES.map((tone) => ({ label: tone, node: render(tone) }));

// Add one entry per primitive; the gallery page renders every case.
export const galleryEntries: GalleryEntry[] = [
  { name: "Spinner", cases: perTone((tone) => <Spinner label={`Loading ${tone}`} tone={tone} />) },
  { name: "Arc", cases: perTone((tone) => <Arc tone={tone} label={tone} />) },
  {
    name: "ProgressRing",
    cases: [
      ...perTone((tone) => <ProgressRing progress={0.6} tone={tone} kind="design" label="60%" />),
      { label: "n/m", node: <ProgressRing progress={0.6} kind="implementation" label="3/5" /> },
      { label: "pending", node: <ProgressRing progress={0} kind="implementation" label="…" /> },
      { label: "no label", node: <ProgressRing progress={0.3} kind="implementation" /> },
      ...KINDS.flatMap((kind) => [
        { label: `${kind} still`, node: <ProgressRing progress={0.6} kind={kind} label="60%" /> },
        { label: `${kind} live`, node: <ProgressRing progress={0.6} kind={kind} label="60%" live /> },
      ]),
      { label: "compact", node: <ProgressRing progress={0.6} kind="design" size="compact" label="3" /> },
      { label: "compact live", node: <ProgressRing progress={0.6} kind="design" size="compact" live /> },
    ],
  },
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
        label: "attention",
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
      ...(["main", "secondary", "ghost"] as const).flatMap((emphasis) =>
        TONES.flatMap((tone) =>
          (["sm", "md"] as const).map((size) => ({
            label: `${emphasis} ${tone} ${size}`,
            node: (
              <Button emphasis={emphasis} tone={tone} size={size}>
                {emphasis}
              </Button>
            ),
          })),
        ),
      ),
      { label: "disabled", node: <Button disabled>disabled</Button> },
      { label: "main disabled", node: <Button emphasis="main" disabled>disabled</Button> },
    ],
  },
  {
    name: "TopBar",
    cases: [
      ...perTone((tone) => <TopBar active="epics" tone={tone} build="v1.2.3" connected />),
      { label: "Epics active", node: <TopBar active="epics" build="v1.2.3" connected /> },
      { label: "Stats active", node: <TopBar active="stats" build="v1.2.3" connected /> },
      { label: "none active", node: <TopBar build="v1.2.3" connected /> },
      { label: "Paused", node: <TopBar active="epics" build="v1.2.3" mode="paused" connected /> },
      { label: "Offline", node: <TopBar active="epics" build="v1.2.3" connected={false} /> },
      { label: "menu closed", node: <TopBar active="epics" build="v1.2.3" connected onToggleAfk={() => {}} /> },
      { label: "menu open, AFK off", node: <OpenMenu><TopBar active="epics" build="v1.2.3" connected onToggleAfk={() => {}} /></OpenMenu> },
      { label: "menu open, AFK on", node: <OpenMenu><TopBar active="epics" build="v1.2.3" connected afk onToggleAfk={() => {}} /></OpenMenu> },
      { label: "no build string", node: <TopBar active="epics" connected /> },
    ],
  },
  {
    name: "DayChart",
    cases: [
      ...perTone((tone) => <DayChart days={chartDays} series={[{ key: tone, label: tone, tone, values: [3, 5, null, 0, 4, 2, 6] }]} />),
      {
        label: "stacked",
        node: (
          <DayChart
            days={chartDays}
            kind="stacked"
            series={[
              { key: "a", label: "Design", tone: "waiting", values: [2, 3, null, 0, 2, 1, 3] },
              { key: "b", label: "Implementation", tone: "running", values: [1, 2, null, 0, 2, 1, 3] },
            ]}
          />
        ),
      },
      {
        label: "side by side",
        node: (
          <DayChart
            days={chartDays}
            kind="side-by-side"
            series={[
              { key: "a", label: "Created", tone: "waiting", values: [2, 3, null, 0, 2, 1, 3] },
              { key: "b", label: "Resolved", tone: "running", values: [1, 2, null, 0, 2, 1, 3] },
            ]}
          />
        ),
      },
      {
        label: "added and removed",
        node: (
          <DayChart
            days={chartDays}
            kind="diverging"
            series={[
              { key: "a", label: "Added", tone: "done", values: [40, 10, null, 0, 25, 5, 30] },
              { key: "b", label: "Removed", tone: "paused", values: [10, 4, null, 0, 12, 2, 8] },
            ]}
          />
        ),
      },
      {
        label: "folded series",
        node: (
          <DayChart
            days={chartDays}
            series={[{ key: "a", label: "Tickets", tone: "running", values: [3, 5, null, 0, 4, 2, 6] }]}
            folded={[{ key: "x", label: "Other", values: [1, 0, null, 0, 1, 0, 2] }]}
          />
        ),
      },
    ],
  },
  {
    name: "LineChart",
    cases: [
      ...perTone((tone) => <LineChart days={chartDays} tone={tone} values={[3, 5, null, 0, 4, 2, 6]} />),
    ],
  },
  {
    name: "StatCard",
    cases: [
      ...perTone((tone) => (
        <StatCard title={tone} days={chartDays} series={[{ key: tone, label: tone, tone, values: [3, 5, 2, 0, 4, 2, 6] }]} />
      )),
      {
        label: "partial",
        node: <StatCard title="Tickets merged" days={chartDays} series={[{ key: "a", label: "Tickets", tone: "running", values: [3, 5, null, 0, 4, 2, 6] }]} />,
      },
      {
        label: "override",
        node: (
          <StatCard
            title="Cost"
            days={chartDays}
            headline="$12.40"
            small="of $50 budget"
            series={[{ key: "a", label: "Cost", tone: "done", values: [1, 2, 3, 1, 2, 1, 2] }]}
          />
        ),
      },
      {
        label: "line",
        node: <StatCard title="Waiting for design" days={chartDays} chart="line" series={[{ key: "a", label: "Waiting", tone: "waiting", values: [3, 5, null, 0, 4, 2, 6] }]} />,
      },
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
  {
    name: "Sidebar",
    cases: [
      ...perTone((tone) => (
        <Sidebar title={tone} tone={tone}>
          <SidebarItem label={ticket.title} tone={tone} current />
        </Sidebar>
      )),
      {
        label: "with action",
        node: (
          <Sidebar
            title="Designs"
            action={
              <Button emphasis="ghost" tone="running">
                + New
              </Button>
            }
          >
            <SidebarItem label={ticket.title} />
          </Sidebar>
        ),
      },
    ],
  },
  {
    name: "SidebarSection",
    cases: [
      ...perTone((tone) => (
        <SidebarSection title={tone} count={2} tone={tone}>
          <SidebarItem label={ticket.title} />
        </SidebarSection>
      )),
      {
        label: "collapsed",
        node: (
          <SidebarSection title="Finished" count={5} defaultCollapsed>
            <SidebarItem label={ticket.title} />
          </SidebarSection>
        ),
      },
      {
        label: "no count",
        node: (
          <SidebarSection title="Ready">
            <SidebarItem label={ticket.title} />
          </SidebarSection>
        ),
      },
    ],
  },
  {
    name: "SidebarItem",
    cases: [
      ...perTone((tone) => <SidebarItem label={ticket.title} tone={tone} current />),
      { label: "plain", node: <SidebarItem label={ticket.title} /> },
      { label: "current", node: <SidebarItem label={ticket.title} current /> },
      { label: "with marks", node: <SidebarItem label={ticket.title} marks={<LiveDot tone="running" label="live" />} /> },
    ],
  },
  {
    name: "EpicCard",
    cases: [
      { label: "running", node: <EpicCard row={cardRow()} /> },
      { label: "queued", node: <EpicCard row={cardRow({ state: "queued", status_words: "Queued #2", reason: "Starts after agent-runner-phase2" })} /> },
      { label: "merging", node: <EpicCard row={cardRow({ state: "merging", status_words: "Merging...", ring_progress: 100, done: 5 })} /> },
      { label: "finished", node: <EpicCard row={cardRow({ state: "finished", section: "finished", status_words: "Merged", ring_progress: 100, done: 5 })} /> },
      { label: "draft", node: <EpicCard row={cardRow({ state: "draft", section: "ready-to-design", status_words: "Draft", done: 0, ring_progress: 0 })} /> },
      { label: "ready with Queue", node: <EpicCard row={cardRow({ state: "ready", section: "ready-to-queue", status_words: "Implementing...", actions: [{ kind: "queue", label: "Queue" }] })} /> },
      {
        label: "Needs you with buttons",
        node: (
          <EpicCard
            row={cardRow({
              state: "paused",
              section: "needs-you",
              state_pill: "Paused",
              reason: "06 Retry the launch: zero commits after the iteration",
              actions: [{ kind: "open-ticket", label: "Open ticket", target: "demo:blog/06" }],
            })}
          />
        ),
      },
      {
        label: "merge blocked",
        node: (
          <EpicCard
            row={cardRow({
              state: "merge-blocked",
              section: "needs-you",
              state_pill: "Merge blocked",
              reason: "Conflicts with main in server/land.go",
              actions: [{ kind: "merge", label: "Merge" }],
            })}
          />
        ),
      },
      {
        label: "design not started",
        node: <EpicCard row={cardRow({ card_kind: "design", state: "waiting-to-design", section: "ready-to-design", status_words: "Not started", ring_progress: 0, actions: [{ kind: "design", label: "Design", target: "demo:blog" }] })} />,
      },
      {
        label: "joined card",
        node: <EpicCard row={cardRow({ card_kind: "implementation", addresses: ["demo:blog-impl", "demo:blog"], name: "blog" })} />,
      },
      { label: "one-off", node: <EpicCard row={cardRow({ card_kind: "one-off", state: "queued", status_words: "One-off", ring_progress: 0 })} /> },
      {
        label: "follow-ups",
        node: <EpicCard row={cardRow({ card_kind: "follow-ups", state: "follow-ups", section: "ready-to-design", status_words: "5 bugs, 2 ideas", ring_progress: 0, actions: [{ kind: "triage", label: "Triage", target: "demo" }] })} />,
      },
      {
        label: "two in a row, one footer",
        node: (
          <Box display="grid" gridTemplateColumns="repeat(2, 360px)" gap="20px">
            <EpicCard row={cardRow({ name: "no buttons" })} />
            <EpicCard row={cardRow({ state: "ready", section: "ready-to-queue", name: "with Queue", actions: [{ kind: "queue", label: "Queue" }] })} />
          </Box>
        ),
      },
    ],
  },
  {
    name: "JumpRow",
    cases: [
      { label: "design", node: <JumpRow row={cardRow({ card_kind: "design", name: "blog", status_words: "Designing", ring_progress: 30 })} /> },
      { label: "implementation", node: <JumpRow row={cardRow()} /> },
      { label: "one-off", node: <JumpRow row={cardRow({ card_kind: "one-off", name: "fix-it", state: "queued", status_words: "One-off", ring_progress: 0 })} /> },
      { label: "follow-ups", node: <JumpRow row={cardRow({ card_kind: "follow-ups", name: "follow-ups", section: "ready-to-design", status_words: "5 bugs, 2 ideas", ring_progress: 0 })} /> },
      { label: "Needs you", node: <JumpRow row={cardRow({ section: "needs-you", state_pill: "Question" })} /> },
      { label: "current", node: <JumpRow row={cardRow()} current /> },
      { label: "cursor", node: <JumpRow row={cardRow()} active /> },
      { label: "finished", node: <JumpRow row={cardRow({ state: "finished", section: "finished", status_words: "Merged", ring_progress: 100 })} /> },
    ],
  },
  {
    name: "NewDesignForm",
    cases: [
      { label: "several projects", node: <ModalFrame><NewDesignForm projects={["gx", "dotfiles", "blog"]} onStarted={() => {}} onClose={() => {}} /></ModalFrame> },
      { label: "one project", node: <ModalFrame><NewDesignForm projects={["gx"]} onStarted={() => {}} onClose={() => {}} /></ModalFrame> },
      { label: "phone bottom sheet", node: <WideContext.Provider value={false}><ModalFrame><NewDesignForm projects={["gx", "dotfiles", "blog"]} onStarted={() => {}} onClose={() => {}} /></ModalFrame></WideContext.Provider> },
    ],
  },
];
