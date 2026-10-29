import { Box, Text } from "@chakra-ui/react";
import { useEffect, useMemo, useRef, type ReactNode } from "react";
import { createMemoryRouter, MemoryRouter, RouterProvider } from "react-router";
import { AgentSidebar } from "../agents/AgentSidebar";
import canned from "../canned-snapshot.json";
import { ConnectionContext, type Connection } from "../connection";
import { NewDesignForm } from "../design/NewDesignForm";
import { WideContext } from "../useWide";
import { EpicPage } from "../epic/EpicPage";
import { EpicCard } from "../epics/EpicCard";
import type { EpicDetail, EpicDetailRow, EpicListRow, FixAgentResult, SessionInfo } from "../generated/server";
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
import { BottomSheet } from "../ui/modal/BottomSheet";
import { Modal } from "../ui/modal/Modal";
import { ProgressBar } from "../ui/progress-bar/ProgressBar";
import { ProgressRing } from "../ui/progress-ring/ProgressRing";
import { SegmentedBar } from "../ui/progress-bar/SegmentedBar";
import { Sidebar } from "../ui/sidebar/Sidebar";
import { SidebarItem } from "../ui/sidebar/SidebarItem";
import { SidebarSection } from "../ui/sidebar/SidebarSection";
import { Spinner } from "../ui/spinner/Spinner";
import { StatCard } from "../ui/stat-card/StatCard";
import { DiskStrip } from "../ui/disk-strip/DiskStrip";
import { Strip } from "../ui/strip/Strip";
import { TicketLine } from "../ui/ticket-line/TicketLine";
import { TopBar } from "../ui/top-bar/TopBar";
import { Tray } from "../ui/tray/Tray";
import type { Health, HealthState } from "../generated/server";
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
  section: "implementing",
  status_words: "Implementing...",
  ring_progress: 40,
  actions: [],
  ...over,
});

const idleConnection = { refresh: () => {} } as Connection;

// A transform makes the modal's fixed overlay fill this frame instead of the page.
const ModalFrame = ({ children, h = "600px", w = "800px" }: { children: ReactNode; h?: string; w?: string }) => (
  <ConnectionContext.Provider value={idleConnection}>
    <Box position="relative" h={h} w={w} transform="translateZ(0)" overflow="hidden" borderWidth="1px" borderColor="border">
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

const epicSteps = (current: string, done: string[] = []): EpicDetail["steps"] =>
  ["chart", "spec", "create-tickets", "done"].map((step) => ({
    step,
    label: step,
    state: done.includes(step) ? "done" : step === current ? "current" : "todo",
  }));

const epicRow = (id: string, over: Partial<EpicDetailRow> = {}): EpicDetailRow =>
  ({ address: `demo:blog-impl/${id}`, title: `Ticket ${id}`, rendered_status: "open", ...over }) as EpicDetailRow;

const epicDetail = (over: Partial<EpicDetail> = {}): EpicDetail => ({
  address: "demo:blog-impl",
  kind: "loop",
  summary: "Move the blog to the new layout.",
  done: 0,
  total: 0,
  agent_time: 300,
  cost_total: 1.5,
  wall_clock: 125,
  stats: {},
  rows: [],
  queueable: false,
  queued: false,
  ...over,
});

const withRows = (over: Partial<EpicDetail>, rows: EpicDetailRow[]) =>
  epicDetail({ ...over, rows, total: rows.length, done: rows.filter((r) => r.rendered_status === "done").length });

const claimed = new Date().toISOString();
const doneRows = [epicRow("01", { rendered_status: "done", elapsed: 120 }), epicRow("02", { rendered_status: "done", elapsed: 300 })];

const agentSession = (id: string, over: Partial<SessionInfo>): SessionInfo => ({
  id,
  name: id,
  project: "gx",
  dir: "/x",
  harness: "claude",
  status: { state: "idle" },
  status_changed_at: claimed,
  run_started_at: claimed,
  unread: {},
  actionable: {},
  auto_advance: true,
  ...over,
});
const AGENT_SESSIONS: SessionInfo[] = [
  agentSession("s-1", { status: { state: "starting" } }),
  agentSession("s-2", { status: { state: "working", background: true }, design: true, ticket: "gx:map/01" }),
  agentSession("s-3", { unread: { unread: true } }),
  agentSession("s-4", { status: { state: "blocked" }, actionable: { reason: "Pick a layout" } }),
  agentSession("s-5", { status: { state: "rate-limited" } }),
  agentSession("s-6", { status: { state: "exited" }, exit: "clean" }),
  agentSession("s-7", { project: "other", status: { state: "exited" }, exit: "crashed" }),
];

// getState must return the same object each call, or useSyncExternalStore loops.
const idleEpicState = { snapshot: { sessions: [] }, connected: true, loadedOnce: true, tick: 0 };
const idleEpicConnection = {
  getState: () => idleEpicState,
  subscribe: () => () => {},
  onEvent: () => () => {},
  refresh: () => {},
} as unknown as Connection;

// The page needs a router and a connection; the transform keeps the fix dialog's fixed overlay inside the frame.
const EpicPageFrame = ({ detail, fixing }: { detail: EpicDetail; fixing?: boolean }) => {
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = root.current;
    if (!fixing || !el) return;
    // The detail loads after mount, so click the button once it shows up.
    const click = () => {
      const fix = [...el.querySelectorAll("button")].find((b) => b.textContent === "Fix it with an agent");
      fix?.click();
      return !!fix;
    };
    if (click()) return;
    const watch = new MutationObserver(() => click() && watch.disconnect());
    watch.observe(el, { childList: true, subtree: true });
    return () => watch.disconnect();
  }, [fixing]);
  const router = useMemo(
    () =>
      createMemoryRouter(
        [
          {
            path: "*",
            element: (
              <EpicPage
                address={detail.address}
                fetchDetail={async () => ({ kind: "ok", detail })}
                fetchList={async () => ({ epics: [], follow_ups: [] })}
                fixAgent={async () => ({}) as FixAgentResult}
              />
            ),
          },
        ],
        { initialEntries: [`/e/${detail.address}`] },
      ),
    [detail],
  );
  return (
    <ConnectionContext.Provider value={idleEpicConnection}>
      <Box ref={root} position="relative" w="800px" minH="500px" transform="translateZ(0)" overflow="hidden" borderWidth="1px" borderColor="border">
        <RouterProvider router={router} />
      </Box>
    </ConnectionContext.Provider>
  );
};

const toneState: Record<Tone, HealthState> = {
  running: "normal",
  done: "normal",
  waiting: "high",
  paused: "critical",
  deadlocked: "critical",
  muted: "unknown",
};
const stateLevel: Record<HealthState, number> = { normal: 1, high: 3, critical: 4, unknown: 0 };
const mixedHealth = (cpu: HealthState, memory: HealthState, disk: HealthState): Health => {
  const meterOf = (state: HealthState) => ({ level: stateLevel[state], state, read_at: "" });
  return { cpu: meterOf(cpu), memory: meterOf(memory), disk: meterOf(disk) };
};
const trayHealth = (state: HealthState) => mixedHealth(state, state, state);

const chartDays =["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

const perTone =(render: (tone: Tone) => ReactNode): GalleryCase[] =>
  TONES.map((tone) => ({ label: tone, node: render(tone) }));

// Add one entry per primitive; the gallery page renders every case.
export const galleryEntries: GalleryEntry[] = [
  { name: "Spinner", cases: perTone((tone) => <Spinner label={`Loading ${tone}`} tone={tone} />) },
  { name: "Arc", cases: perTone((tone) => <Arc tone={tone} label={tone} />) },
  {
    name: "Modal",
    cases: perTone((tone) => (
      <ModalFrame>
        <Modal label={tone} onClose={() => {}}>
          {tone}
        </Modal>
      </ModalFrame>
    )),
  },
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
        node: <TicketLine id={ticket.address} title={ticket.title} tone="paused" reason="Worktree is dirty" hint="Open the ticket to fix it" />,
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
      { label: "Agents active", node: <TopBar active="agents" build="v1.2.3" connected actionable={2} /> },
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
    name: "Tray",
    // The Tray has no tone of its own: its states are normal, high, critical and unknown, so the tone
    // cases show the nearest state (waiting is high, paused and deadlocked are critical, muted is unknown).
    cases: [
      ...perTone((tone) => <Tray health={trayHealth(toneState[tone])} />),
      { label: "calm", node: <Tray health={trayHealth("normal")} /> },
      { label: "busy", node: <Tray health={mixedHealth("high", "normal", "high")} /> },
      { label: "critical", node: <Tray health={mixedHealth("critical", "critical", "critical")} /> },
      { label: "unknown", node: <Tray health={trayHealth("unknown")} /> },
    ],
  },
  {
    name: "DiskStrip",
    // The strip has no tone of its own: it is "paused" when held and "waiting" otherwise, so the other tones show the unheld look.
    cases: [
      ...perTone((tone) => <DiskStrip name="Macintosh HD" free="4.2 GB" held={tone === "paused"} cleaning={false} pending={false} onClean={() => {}} />),
      { label: "warned", node: <DiskStrip name="Macintosh HD" free="4.2 GB" held={false} cleaning={false} pending={false} onClean={() => {}} /> },
      { label: "held", node: <DiskStrip name="Macintosh HD" free="900 MB" held cleaning={false} pending={false} onClean={() => {}} /> },
      { label: "cleaning", node: <DiskStrip name="Macintosh HD" free="4.2 GB" held={false} cleaning pending={false} onClean={() => {}} /> },
      { label: "pending", node: <DiskStrip name="Macintosh HD" free="4.2 GB" held={false} cleaning={false} pending onClean={() => {}} /> },
      { label: "refused", node: <DiskStrip name="Macintosh HD" free="4.2 GB" held={false} cleaning={false} pending={false} problem="A clean already runs on this disk." onClean={() => {}} /> },
      { label: "failed", node: <DiskStrip name="Macintosh HD" free="900 MB" held cleaning={false} pending={false} problem="failed: exit status 1" onClean={() => {}} /> },
    ],
  },
  {
    name: "Modal",
    // The Modal has no tone of its own; the tone cases show it holding a button in that tone.
    cases: [
      ...perTone((tone) => (
        <ModalFrame h="200px" w="400px">
          <Modal label={tone} onClose={() => {}}>
            <Text>{ticket.title}</Text>
            <Box display="flex" justifyContent="flex-end">
              <Button emphasis="main" tone={tone}>
                {tone}
              </Button>
            </Box>
          </Modal>
        </ModalFrame>
      )),
    ],
  },
  {
    name: "BottomSheet",
    // A bottom sheet on a phone and a centered dialog on a wide screen; the tone cases show it holding a button in that tone.
    cases: [
      ...perTone((tone) => (
        <WideContext.Provider value={false}>
          <ModalFrame h="200px" w="400px">
            <BottomSheet label={tone} onClose={() => {}}>
              <Text>{ticket.title}</Text>
              <Box display="flex" justifyContent="flex-end">
                <Button emphasis="main" tone={tone}>
                  {tone}
                </Button>
              </Box>
            </BottomSheet>
          </ModalFrame>
        </WideContext.Provider>
      )),
      {
        label: "wide",
        node: (
          <ModalFrame h="200px" w="400px">
            <BottomSheet label="wide" onClose={() => {}}>
              <Text>{ticket.title}</Text>
            </BottomSheet>
          </ModalFrame>
        ),
      },
    ],
  },
  {
    name: "EmptyState",
    cases: [
      ...perTone((tone) => <EmptyState tone={tone} title="All quiet" />),
      { label: "with body", node: <EmptyState title="All quiet">Nothing is running.</EmptyState> },
    ],
  },
  {
    name: "AgentSidebar",
    cases: [
      {
        label: "one row per state",
        node: (
          <MemoryRouter>
            <AgentSidebar selected="s-2" sessions={AGENT_SESSIONS} />
          </MemoryRouter>
        ),
      },
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
    name: "EpicPage",
    cases: [
      {
        label: "running",
        node: (
          <EpicPageFrame
            detail={withRows({ state: "running" }, [...doneRows, epicRow("03", { rendered_status: "claimed", claimed_at: claimed }), epicRow("04")])}
          />
        ),
      },
      { label: "finished", node: <EpicPageFrame detail={withRows({ state: "finished" }, doneRows)} /> },
      {
        label: "Design",
        node: (
          <EpicPageFrame
            detail={withRows({ address: "demo:blog", kind: "map", steps: epicSteps("spec", ["chart"]), stats: { cost: 2.5 } }, [
              epicRow("01", { rendered_status: "done", elapsed: 120 }),
            ])}
          />
        ),
      },
      { label: "draft -impl", node: <EpicPageFrame detail={withRows({ state: "waiting", draft: true }, [epicRow("01"), epicRow("02")])} /> },
      {
        label: "merge-blocked",
        node: (
          <EpicPageFrame
            detail={withRows({ state: "paused", land_block_reason: "Conflicts with main in server/land.go" }, doneRows)}
          />
        ),
      },
      {
        label: "dirty-trunk",
        node: (
          <EpicPageFrame
            detail={withRows(
              {
                state: "paused",
                land_block_kind: "dirty-trunk",
                land_block_reason: "Trunk has local changes: a.go, b/c.go and 1 more",
                land_block_files: ["a.go", "b/c.go", "d.go"],
                actions: ["stash-merge", "merge"],
                action_info: {
                  "stash-merge": { label: "Stash, merge and restore", tooltip: "Set your changes aside." },
                  merge: { label: "Merge again", tooltip: "Merge the ticket's commits again." },
                },
              },
              doneRows,
            )}
          />
        ),
      },
      { label: "cancelled card", node: <EpicPageFrame detail={withRows({ state: "running" }, [...doneRows, epicRow("03", { rendered_status: "cancelled" }), epicRow("04")])} /> },
      {
        label: "running fork",
        node: (
          <EpicPageFrame
            detail={withRows({ state: "running" }, [
              ...doneRows,
              epicRow("03", { rendered_status: "claimed", claimed_at: claimed }),
              epicRow("04", { rendered_status: "claimed", claimed_at: claimed, parent: "demo:blog-impl/03" }),
            ])}
          />
        ),
      },
      { label: "empty", node: <EpicPageFrame detail={epicDetail({ state: "waiting" })} /> },
      {
        label: "needs-repair",
        node: (
          <EpicPageFrame
            detail={withRows({ state: "paused" }, [...doneRows, epicRow("03", { rendered_status: "needs-repair", reason: "dirty tree", fix_prompt: "/gx-investigate demo:blog/03 is paused for repair: dirty tree" })])}
          />
        ),
      },
      {
        label: "needs-repair dialog",
        node: (
          <EpicPageFrame
            fixing
            detail={withRows({ state: "paused" }, [...doneRows, epicRow("03", { rendered_status: "needs-repair", reason: "dirty tree", fix_prompt: "/gx-investigate demo:blog/03 is paused for repair: dirty tree" })])}
          />
        ),
      },
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
