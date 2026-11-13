import { Box, Menu, Text } from "@chakra-ui/react"
import { useEffect, useMemo, useRef, type ComponentProps, type ReactNode } from "react"
import { PageHeader } from "../page/PageHeader"
import { PageIcon } from "../page/PageIcon"
import { createMemoryRouter, MemoryRouter, RouterProvider } from "react-router"
import canned from "../canned-snapshot.json"
import { authEntries } from "./authEntries"
import { designFooterEntries } from "./designFooterEntries"
import { questionEntries } from "./questionEntries"
import { shellEntries } from "./shellEntries"
import { ConnectionContext, type Connection } from "../connection"
import { DraftSwitcher } from "../design/DraftSwitcher"
import { DraftImages } from "../design/DraftImages"
import { ContextBadge } from "../design/ContextBadge"
import { ImageStrip } from "../design/ImageStrip"
import type { DraftImages as DraftImagesState } from "../design/useDraftImages"
import { NewDesignForm } from "../design/NewDesignForm"
import type { DraftsClient } from "../design/drafts"
import type { ComposerImage } from "../design/useComposerImages"
import { CoarsePointerContext, type Device, DeviceContext, WideContext } from "../useWide"
import { EpicPage } from "../epic/EpicPage"
import { ErrorCard } from "../ErrorBoundary"
import { EpicCard } from "../epics/EpicCard"
import { JumpRow } from "../nav/JumpMenu"
import { Arc } from "../ui/arc/Arc"
import { StatusIcon, STATUS_LABELS, type StatusKind } from "../ui/status-icon/StatusIcon"
import { Badge } from "../ui/badge/Badge"
import { Button } from "../ui/button/Button"
import { Card } from "../ui/card/Card"
import { Chip } from "../ui/chip/Chip"
import { CommandMenu, type CommandMenuRow } from "../ui/command-menu/CommandMenu"
import { modelName } from "../modelName"
import { HarnessLabel } from "../ui/harness-label/HarnessLabel"
import { DayChart } from "../ui/day-chart/DayChart"
import { LineChart } from "../ui/day-chart/LineChart"
import { EmptyState } from "../ui/empty-state/EmptyState"
import { InfoTip } from "../ui/info-tip/InfoTip"
import { PromptBox } from "../ui/expandable-text/PromptBox"
import { LargeEditor } from "../ui/expandable-text/LargeEditor"
import { ModeText } from "../ui/expandable-text/ModeText"
import { LiveDot } from "../ui/live-dot/LiveDot"
import { BottomSheet } from "../ui/modal/BottomSheet"
import { GxMenu } from "../ui/menu/Menu"
import { MoreActionsButton } from "../ui/menu/MoreActionsButton"
import { Modal } from "../ui/modal/Modal"
import { ProgressBar } from "../ui/progress-bar/ProgressBar"
import { ProgressRing } from "../ui/progress-ring/ProgressRing"
import { SegmentedBar } from "../ui/progress-bar/SegmentedBar"
import { FullScreenLoader } from "../ui/loader/FullScreenLoader"
import { Spinner } from "../ui/spinner/Spinner"
import { StatCard } from "../ui/stat-card/StatCard"
import { DiskStrip } from "../ui/disk-strip/DiskStrip"
import { Strip } from "../ui/strip/Strip"
import { TicketLine } from "../ui/ticket-line/TicketLine"
import { StickyTopContainer } from "../ui/top-bar/StickyTopContainer"
import { Tray } from "../ui/tray/Tray"
import type {
  DesignDraftItem,
  EpicDetail,
  EpicDetailRow,
  EpicListRow,
  FixAgentResult,
  Health,
  HealthState,
  LimitWindow,
  UsageLimits,
  UsageLimitWindow,
  UsageSpend,
  UsageSummary,
} from "../generated/server"
import { UsageModeLimits, UsageModeSpending } from "../generated/server"
import { LimitsBlock } from "../usage/LimitsBlock"
import { LimitsTray } from "../usage/LimitsTray"
import { SpendBlock } from "../usage/SpendBlock"
import { KINDS, TONES, type Tone } from "../ui/tone"

export interface GalleryCase {
  label: string
  node: ReactNode
}

export interface GalleryEntry {
  name: string
  cases: GalleryCase[]
}

const stripImages: ComposerImage[] = [
  { key: "a", state: "ready", canRetry: false },
  { key: "b", state: "uploading", canRetry: false },
  { key: "c", state: "failed", message: "Too large", canRetry: true },
  { key: "d", state: "failed", message: "Upload cut off", canRetry: false },
]
const draftImagesState = (images: ComposerImage[]): DraftImagesState => ({
  images,
  readyIds: images.filter((i) => i.state === "ready").map((i) => i.key),
  uploading: images.filter((i) => i.state === "uploading").length,
  failed: images.filter((i) => i.state === "failed").length,
  error: "",
  count: images.length,
  addFiles: async () => {},
  retry: () => {},
  remove: () => {},
  reset: () => {},
  urlOf: () => "",
})
const ticket = canned.tickets[0]
const longReason =
  "Waiting on a long chain of upstream tickets that all have to merge first. ".repeat(4)

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
})

const idleState = { connected: true, loadedOnce: true, tick: 0 }
const idleConnection = {
  refresh: () => {},
  getState: () => idleState,
  subscribe: () => () => {},
  onEvent: () => () => {},
} as unknown as Connection
/** A draft client with no drafts, so the New Design form renders blank without a server. */
const noDrafts: DraftsClient = {
  list: async () => [],
  read: async () => ({ kind: "gone" }),
  save: async () => ({ kind: "deleted" }),
  remove: async () => ({ kind: "deleted" }),
  getLastProject: async () => "",
  setLastProject: async () => {},
  uploadImage: async () => "gallery",
  removeImage: async () => ({}),
}

// A transform makes the modal's fixed overlay fill this frame instead of the page.
const sampleDrafts = (n: number): DesignDraftItem[] =>
  Array.from({ length: n }, (_, i) => ({
    id: `d${i}`,
    project: ["gx", "blog", "dotfiles"][i % 3],
    slug: `epic-${i + 1}`,
    idea: "",
    image_count: i % 4,
    edited_at: new Date(Date.now() - i * 5_400_000).toISOString(),
  }))

const SwitcherFrame = ({ drafts, open }: { drafts: DesignDraftItem[]; open?: boolean }) => (
  <Box position="relative" w="360px" h="380px" pt="2">
    <DraftSwitcher
      label="epic-1"
      currentId={drafts[0]?.id}
      load={async () => drafts}
      onPick={() => {}}
      onNew={() => {}}
      onRename={() => {}}
      onDelete={async () => {}}
      defaultOpen={open}
    />
  </Box>
)

const ModalFrame = ({
  children,
  h = "600px",
  w = "800px",
}: {
  children: ReactNode
  h?: string
  w?: string
}) => (
  <ConnectionContext.Provider value={idleConnection}>
    <MemoryRouter>
      <Box
        position="relative"
        h={h}
        w={w}
        transform="translateZ(0)"
        overflow="hidden"
        borderWidth="1px"
        borderColor="border"
      >
        {children}
      </Box>
    </MemoryRouter>
  </ConnectionContext.Provider>
)

// Opens a menu the way a user does, by clicking its trigger once mounted.
const OpenMenu = ({ children }: { children: ReactNode }) => {
  const root = useRef<HTMLDivElement>(null)
  useEffect(() => root.current?.querySelector<HTMLElement>("button[aria-label=Menu]")?.click(), [])
  return <div ref={root}>{children}</div>
}

const limitsNow = Date.UTC(2026, 9, 31, 12, 0, 0)
const limitWin = (pct: number, resetsInSec: number, seenAgoSec: number): LimitWindow => ({
  used_percentage: pct,
  resets_at: limitsNow / 1000 + resetsInSec,
  received_at: limitsNow / 1000 - seenAgoSec,
})
const weeklyWin = (pct: number, extra: Partial<UsageLimitWindow>): UsageLimitWindow => ({
  ...limitWin(pct, 187200, 60),
  ...extra,
})
const limitsStates: [string, UsageLimits, UsageLimits?][] = [
  ["limits fresh", { five_hour: limitWin(34, 7800, 60), seven_day: limitWin(81, 187200, 60) }],
  [
    "limits weekly delta + Codex",
    {
      five_hour: limitWin(34, 7800, 60),
      seven_day: weeklyWin(81, {
        chip: { today: 7, yesterday: 5, direction: "up" },
        forecast: {
          status: "projected",
          pace: 30,
          at_reset: 130,
          runs_out_at: limitsNow / 1000 + 86400,
        },
      }),
    },
    {
      five_hour: limitWin(12, 13200, 60),
      seven_day: weeklyWin(40, {
        chip: { today: 3, yesterday: 4, direction: "down" },
        forecast: { status: "projected", pace: 10, at_reset: 72 },
      }),
    },
  ],
  [
    "limits few days",
    { seven_day: weeklyWin(22, { forecast: { status: "needs_data", pace: 0, at_reset: 0 } }) },
  ],
  [
    "limits stale",
    { five_hour: limitWin(34, 7800, 10800), seven_day: limitWin(52, 187200, 10800) },
  ],
  [
    "limits past reset",
    { five_hour: limitWin(34, -60, 21600), seven_day: limitWin(52, 187200, 21600) },
  ],
  ["limits one window", { seven_day: limitWin(12, 464400, 60) }],
]
// The tray reads the real clock, so its windows reset relative to now, not to limitsNow.
function LimitsTrayDemo({
  claude,
  codex,
  spending,
}: {
  claude: number
  codex?: number
  spending?: boolean
}) {
  const win = (pct: number): LimitWindow => ({
    used_percentage: pct,
    resets_at: Date.now() / 1000 + 7800,
    received_at: Date.now() / 1000 - 60,
  })
  const summary = {
    mode: spending ? UsageModeSpending : UsageModeLimits,
    limits: { five_hour: win(claude), seven_day: win(claude) },
    codex: codex === undefined ? undefined : { five_hour: win(codex), seven_day: win(codex) },
    spend: { today: 4.82, since_sunday: 21.5, since_first: 96.1, first_day: "2026-09-01" },
  } as UsageSummary
  return <LimitsTray conn={idleEpicConnection} fetcher={async () => summary} />
}
const spendStates: [string, UsageSpend][] = [
  ["spend normal", { today: 4.82, since_sunday: 21.5, since_first: 96.1, first_day: "2026-09-01" }],
  [
    "spend short ledger",
    {
      today: 1.2,
      since_sunday: 3.4,
      since_sunday_short: true,
      since_first: 3.4,
      since_first_short: true,
      first_day: "2026-10-30",
    },
  ],
  ["spend empty", { today: 0, since_sunday: 0, since_first: 0 }],
]

// Opens a tooltip the way a user does, by clicking its marker once mounted.
const OpenTip = ({ children }: { children: ReactNode }) => {
  const root = useRef<HTMLDivElement>(null)
  useEffect(() => root.current?.querySelector<HTMLElement>("button")?.click(), [])
  return <div ref={root}>{children}</div>
}

const epicSteps = (current: string, done: string[] = []): EpicDetail["steps"] =>
  ["chart", "spec", "create-tickets", "done"].map((step) => ({
    step,
    label: step,
    state: done.includes(step) ? "done" : step === current ? "current" : "todo",
  }))

const epicRow = (id: string, over: Partial<EpicDetailRow> = {}): EpicDetailRow =>
  ({
    address: `demo:blog-impl/${id}`,
    title: `Ticket ${id}`,
    rendered_status: "open",
    ...over,
  }) as EpicDetailRow

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
})

const withRows = (over: Partial<EpicDetail>, rows: EpicDetailRow[]) =>
  epicDetail({
    ...over,
    rows,
    total: rows.length,
    done: rows.filter((r) => r.rendered_status === "done").length,
  })

const claimed = new Date().toISOString()
const doneRows = [
  epicRow("01", { rendered_status: "done", elapsed: 120 }),
  epicRow("02", { rendered_status: "done", elapsed: 300 }),
]

// getState must return the same object each call, or useSyncExternalStore loops.
const idleEpicState = { snapshot: { sessions: [] }, connected: true, loadedOnce: true, tick: 0 }
const idleEpicConnection = {
  getState: () => idleEpicState,
  subscribe: () => () => {},
  onEvent: () => () => {},
  refresh: () => {},
} as unknown as Connection

// The page needs a router and a connection; the transform keeps the fix dialog's fixed overlay inside the frame.
const EpicPageFrame = ({ detail, fixing }: { detail: EpicDetail; fixing?: boolean }) => {
  const root = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const el = root.current
    if (!fixing || !el) return
    // The detail loads after mount, so click the button once it shows up.
    const click = () => {
      const fix = [...el.querySelectorAll("button")].find(
        (b) => b.textContent === "Fix it with an agent",
      )
      fix?.click()
      return !!fix
    }
    if (click()) return
    const watch = new MutationObserver(() => click() && watch.disconnect())
    watch.observe(el, { childList: true, subtree: true })
    return () => watch.disconnect()
  }, [fixing])
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
  )
  return (
    <ConnectionContext.Provider value={idleEpicConnection}>
      <Box
        ref={root}
        position="relative"
        w="800px"
        minH="500px"
        transform="translateZ(0)"
        overflow="hidden"
        borderWidth="1px"
        borderColor="border"
      >
        <RouterProvider router={router} />
      </Box>
    </ConnectionContext.Provider>
  )
}

const toneState: Record<Tone, HealthState> = {
  running: "normal",
  done: "normal",
  waiting: "high",
  paused: "critical",
  deadlocked: "critical",
  muted: "unknown",
}
const stateLevel: Record<HealthState, number> = { normal: 1, high: 3, critical: 4, unknown: 0 }
const mixedHealth = (cpu: HealthState, memory: HealthState, disk: HealthState): Health => {
  const meterOf = (state: HealthState) => ({ level: stateLevel[state], state, read_at: "" })
  return { cpu: meterOf(cpu), memory: meterOf(memory), disk: meterOf(disk) }
}
const trayHealth = (state: HealthState) => mixedHealth(state, state, state)

const chartDays = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"]

const perTone = (render: (tone: Tone) => ReactNode): GalleryCase[] =>
  TONES.map((tone) => ({ label: tone, node: render(tone) }))

const menuRows: CommandMenuRow[] = [
  {
    name: "/clear",
    description: "Start a new conversation",
    source: "built-in",
    segments: [
      { text: "/cl", match: true },
      { text: "ear", match: false },
    ],
  },
  {
    name: "/compact",
    hint: "[instructions]",
    description: "Summarize the conversation to free context",
    source: "built-in",
    segments: [
      { text: "/c", match: true },
      { text: "ompact", match: false },
    ],
  },
  {
    name: "/review",
    description: "Review the current branch",
    source: "skill",
    segments: [{ text: "/review", match: false }],
  },
]
const menuBase = {
  rows: menuRows,
  selected: 0,
  status: "ready" as const,
  typed: "/c",
  onPick: () => {},
}
const MenuFrame = ({ children }: { children: ReactNode }) => (
  <Box position="relative" w="480px" h="260px" pt="220px">
    {children}
    <Box borderWidth="1px" borderColor="border" borderRadius="md" px="3" py="2" color="text.dim">
      /c
    </Box>
  </Box>
)

// Add one entry per primitive; the gallery page renders every case.
const headerLine =
  "Speed up the Go test suite: split the slow packages, share one fixture cache, and fail CI on any test over 5 seconds."
const headerCase = (
  label: string,
  device: Device,
  over: Partial<ComponentProps<typeof PageHeader>> = {},
) => ({
  label,
  node: (
    <DeviceContext.Provider value={device}>
      <WideContext.Provider value={device !== "phone"}>
        <PageHeader
          title="slow-tests-impl"
          icon={<PageIcon kind="epic" />}
          subtitle={headerLine}
          meta="Running · 3 of 9 done · 42m · 118k tokens"
          actions={
            <>
              <ProgressRing progress={0.33} kind="implementation" label="33%" size="sm" />
              <span>⋯</span>
            </>
          }
          {...over}
        />
      </WideContext.Provider>
    </DeviceContext.Provider>
  ),
})

export const galleryEntries: GalleryEntry[] = [
  {
    name: "PageHeader",
    cases: [
      headerCase("desktop", "desktop"),
      headerCase("tablet", "tablet"),
      headerCase("phone", "phone"),
      headerCase("no subtitle", "desktop", { subtitle: undefined }),
      headerCase("no meta bar", "desktop", { meta: undefined }),
    ],
  },
  {
    name: "MoreActionsButton",
    cases: (["rest", "hover", "focus", "open"] as const).map((state) => ({
      label: state,
      node: (
        <GxMenu
          trigger={<MoreActionsButton pinned={state === "rest" ? undefined : state} />}
          items={[{ value: "copy", label: "Copy ID", onSelect: () => {} }]}
        />
      ),
    })),
  },
  {
    name: "Menu",
    // Open on each device type: a popover beside the button on tablet and desktop, an anchored popover with in-place submenus on a phone.
    cases: (["phone", "tablet", "desktop"] as const).flatMap((device) =>
      [false, true].map((openMenu) => {
        const menu = (
          <GxMenu
            trigger={
              <button type="button" aria-label="Menu">
                ⋯
              </button>
            }
            items={[
              { value: "rename", label: "Rename", onSelect: () => {} },
              {
                kind: "submenu",
                value: "move",
                label: "Move to workspace",
                items: [{ value: "work", label: "Work", onSelect: () => {} }],
              },
              { kind: "separator" },
              { value: "remove", label: "Remove", danger: true, onSelect: () => {} },
            ]}
          />
        )
        return {
          label: `${device} ${openMenu ? "open" : "closed"}`,
          node: (
            <DeviceContext.Provider value={device}>
              <WideContext.Provider value={device !== "phone"}>
                {openMenu ? <OpenMenu>{menu}</OpenMenu> : menu}
              </WideContext.Provider>
            </DeviceContext.Provider>
          ),
        }
      }),
    ),
  },
  { name: "Spinner", cases: perTone((tone) => <Spinner label={`Loading ${tone}`} tone={tone} />) },
  {
    name: "FullScreenLoader",
    cases: [{ label: "default", node: <FullScreenLoader label="Loading things" /> }],
  },
  { name: "Arc", cases: perTone((tone) => <Arc tone={tone} label={tone} />) },
  {
    name: "StickyTopContainer",
    cases: perTone((tone) => <StickyTopContainer ruled>{tone}</StickyTopContainer>),
  },
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
        {
          label: `${kind} live`,
          node: <ProgressRing progress={0.6} kind={kind} label="60%" live />,
        },
      ]),
      {
        label: "compact",
        node: <ProgressRing progress={0.6} kind="design" size="compact" label="3" />,
      },
      {
        label: "compact live",
        node: <ProgressRing progress={0.6} kind="design" size="compact" live />,
      },
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
    name: "ContextBadge",
    cases: [
      {
        label: "usage popover",
        node: (
          <ContextBadge
            tokens={70_000}
            level="green"
            ceiling={200_000}
            usage={{
              input_tokens: 5_000,
              cache_read_tokens: 60_000,
              cache_creation_tokens: 5_000,
              total_tokens: 1_840_000,
              estimated_cost: 4.72,
              compactions: 2,
            }}
          />
        ),
      },
      {
        label: "over smart zone",
        node: (
          <ContextBadge
            tokens={185_375}
            level="red"
            ceiling={130_000}
            usage={{ total_tokens: 15_800_000 }}
          />
        ),
      },
    ],
  },
  {
    name: "PromptBox",
    cases: [
      {
        label: "closed",
        node: (
          <PromptBox
            label="Closed"
            name="gallery-note"
            title="Note"
            rows={3}
            placeholder="Note"
            value=""
            onChange={() => {}}
          />
        ),
      },
      {
        label: "disabled",
        node: (
          <PromptBox
            label="Disabled"
            name="gallery-note"
            title="Note"
            rows={3}
            placeholder="Note"
            disabled
            value=""
            onChange={() => {}}
          />
        ),
      },
      {
        label: "with text",
        node: (
          <PromptBox
            label="With text"
            name="gallery-note"
            title="Note"
            rows={3}
            value={"A note\nover two lines"}
            onChange={() => {}}
          />
        ),
      },
    ],
  },
  ...questionEntries,
  {
    name: "LargeEditor",
    cases: [
      { wide: true, coarse: false, label: "wide, mouse" },
      { wide: true, coarse: true, label: "wide, touch" },
      { wide: false, coarse: false, label: "narrow, mouse" },
      { wide: false, coarse: true, label: "narrow, touch" },
    ].map(({ wide, coarse, label }) => ({
      label,
      node: (
        <DeviceContext.Provider value={wide ? "desktop" : "phone"}>
          <WideContext.Provider value={wide}>
            <CoarsePointerContext.Provider value={coarse}>
              <ModalFrame>
                <LargeEditor
                  title="Note"
                  value={"A note\nover two lines"}
                  placeholder="Note"
                  selection={{ start: 0, end: 0 }}
                  onChange={() => {}}
                  onClose={() => {}}
                />
              </ModalFrame>
            </CoarsePointerContext.Provider>
          </WideContext.Provider>
        </DeviceContext.Provider>
      ),
    })),
  },
  {
    name: "ModeText",
    cases: (["INSERT", "NORMAL", "VISUAL"] as const).flatMap((mode) => [
      { label: `${mode} pill`, node: <ModeText mode={mode} pill /> },
      { label: `${mode} footer`, node: <ModeText mode={mode} /> },
    ]),
  },
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
      {
        label: "live, inline reason",
        node: (
          <TicketLine
            id={ticket.address}
            title={ticket.title}
            reason="claimed 4m ago"
            tone="running"
            inlineReason
            live
          />
        ),
      },
      {
        label: "with reason",
        node: <TicketLine id={ticket.address} title={ticket.title} reason="Blocked by 01" />,
      },
      {
        label: "clamped reason",
        node: <TicketLine id={ticket.address} title={ticket.title} reason={longReason} />,
      },
      {
        label: "answerable",
        node: (
          <TicketLine
            id={ticket.address}
            title={ticket.title}
            tone="paused"
            reason={longReason}
            unclipped
            href="#/t/gx:demo/02"
          />
        ),
      },
      {
        label: "attention",
        node: (
          <TicketLine
            id={ticket.address}
            title={ticket.title}
            tone="paused"
            reason="Worktree is dirty"
            hint="Open the ticket to fix it"
          />
        ),
      },
      {
        label: "answered",
        node: (
          <TicketLine
            id={ticket.address}
            title={ticket.title}
            tone="waiting"
            state="Answered · #2 in queue"
          />
        ),
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
      {
        label: "main disabled",
        node: (
          <Button emphasis="main" disabled>
            disabled
          </Button>
        ),
      },
    ],
  },
  {
    name: "DayChart",
    cases: [
      ...perTone((tone) => (
        <DayChart
          days={chartDays}
          series={[{ key: tone, label: tone, tone, values: [3, 5, null, 0, 4, 2, 6] }]}
        />
      )),
      {
        label: "stacked",
        node: (
          <DayChart
            days={chartDays}
            kind="stacked"
            series={[
              { key: "a", label: "Design", tone: "waiting", values: [2, 3, null, 0, 2, 1, 3] },
              {
                key: "b",
                label: "Implementation",
                tone: "running",
                values: [1, 2, null, 0, 2, 1, 3],
              },
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
            series={[
              { key: "a", label: "Tickets", tone: "running", values: [3, 5, null, 0, 4, 2, 6] },
            ]}
            folded={[{ key: "x", label: "Other", values: [1, 0, null, 0, 1, 0, 2] }]}
          />
        ),
      },
    ],
  },
  {
    name: "LineChart",
    cases: [
      ...perTone((tone) => (
        <LineChart days={chartDays} tone={tone} values={[3, 5, null, 0, 4, 2, 6]} />
      )),
    ],
  },
  {
    name: "StatCard",
    cases: [
      ...perTone((tone) => (
        <StatCard
          title={tone}
          days={chartDays}
          series={[{ key: tone, label: tone, tone, values: [3, 5, 2, 0, 4, 2, 6] }]}
        />
      )),
      {
        label: "partial",
        node: (
          <StatCard
            title="Tickets merged"
            days={chartDays}
            series={[
              { key: "a", label: "Tickets", tone: "running", values: [3, 5, null, 0, 4, 2, 6] },
            ]}
          />
        ),
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
        node: (
          <StatCard
            title="Waiting for design"
            days={chartDays}
            chart="line"
            series={[
              { key: "a", label: "Waiting", tone: "waiting", values: [3, 5, null, 0, 4, 2, 6] },
            ]}
          />
        ),
      },
    ],
  },
  { name: "Chip", cases: perTone((tone) => <Chip tone={tone}>{tone}</Chip>) },
  {
    name: "CommandMenu",
    cases: [
      ...perTone((tone) => (
        <MenuFrame>
          <CommandMenu {...menuBase} tone={tone} />
        </MenuFrame>
      )),
      {
        label: "filtered",
        node: (
          <MenuFrame>
            <CommandMenu {...menuBase} rows={menuRows.slice(0, 1)} typed="/cl" />
          </MenuFrame>
        ),
      },
      {
        label: "keyboard selection",
        node: (
          <MenuFrame>
            <CommandMenu {...menuBase} selected={1} />
          </MenuFrame>
        ),
      },
      {
        label: "no match",
        node: (
          <MenuFrame>
            <CommandMenu {...menuBase} rows={[]} status="no-match" typed="/zz" />
          </MenuFrame>
        ),
      },
      {
        label: "no match phone",
        node: (
          <MenuFrame>
            <CommandMenu {...menuBase} rows={[]} status="no-match" typed="/zz" compact />
          </MenuFrame>
        ),
      },
      {
        label: "loading",
        node: (
          <MenuFrame>
            <CommandMenu {...menuBase} rows={[]} status="loading" />
          </MenuFrame>
        ),
      },
      {
        label: "list failed",
        node: (
          <MenuFrame>
            <CommandMenu {...menuBase} status="failed" />
          </MenuFrame>
        ),
      },
      {
        label: "phone",
        node: (
          <MenuFrame>
            <CommandMenu {...menuBase} compact />
          </MenuFrame>
        ),
      },
      {
        label: "below",
        node: (
          <MenuFrame>
            <CommandMenu {...menuBase} placement="below" />
          </MenuFrame>
        ),
      },
    ],
  },
  {
    name: "HarnessLabel",
    cases: [
      {
        label: "claude full",
        node: (
          <HarnessLabel
            harness="claude"
            tierLabel="Worker"
            model={modelName("claude-sonnet-4-6", "claude")}
            effort="high"
          />
        ),
      },
      {
        label: "codex full",
        node: (
          <HarnessLabel
            harness="codex"
            tierLabel="Strongest"
            model={modelName("gpt-5.5", "codex")}
            effort="xhigh"
          />
        ),
      },
      {
        label: "no reply yet",
        node: (
          <HarnessLabel
            harness="codex"
            tierLabel="Worker"
            model={modelName("", "codex")}
            effort=""
          />
        ),
      },
    ],
  },
  {
    name: "InfoTip",
    cases: [
      {
        label: "chip",
        node: (
          <InfoTip
            variant="chip"
            chipColor="tone.waiting.fg"
            label="Burn"
            content="Burn today 7 pts so far, yesterday 5"
          >
            ▲ 7 vs 5 pts
          </InfoTip>
        ),
      },
      { label: "closed", node: <InfoTip label="Reset time" content="Resets Friday 09:00" /> },
      {
        label: "open",
        node: (
          <OpenTip>
            <InfoTip label="Reset time" content="Resets Friday 09:00" />
          </OpenTip>
        ),
      },
    ],
  },
  { name: "LiveDot", cases: perTone((tone) => <LiveDot tone={tone} label={tone} />) },
  { name: "Strip", cases: perTone((tone) => <Strip tone={tone}>{tone}</Strip>) },
  {
    name: "ImageStrip",
    cases: [
      { label: "editable", node: <ImageStrip images={stripImages} /> },
      { label: "readOnly", node: <ImageStrip images={stripImages} readOnly onOpen={() => {}} /> },
    ],
  },
  {
    name: "DraftImages",
    cases: [
      { label: "empty", node: <DraftImages state={draftImagesState([])} /> },
      {
        label: "ready",
        node: <DraftImages state={draftImagesState(stripImages.slice(0, 1))} />,
      },
      {
        label: "uploading",
        node: <DraftImages state={draftImagesState(stripImages.slice(0, 2))} />,
      },
      {
        label: "failed",
        node: <DraftImages state={draftImagesState(stripImages)} />,
      },
    ],
  },
  {
    name: "Tray",
    // The Tray has no tone of its own: its states are normal, high, critical and unknown, so the tone
    // cases show the nearest state (waiting is high, paused and deadlocked are critical, muted is unknown).
    cases: [
      ...perTone((tone) => <Tray health={trayHealth(toneState[tone])} />),
      { label: "calm", node: <Tray health={trayHealth("normal")} /> },
      { label: "busy", node: <Tray health={mixedHealth("high", "normal", "high")} /> },
      {
        label: "critical",
        node: <Tray health={mixedHealth("critical", "critical", "critical")} />,
      },
      { label: "unknown", node: <Tray health={trayHealth("unknown")} /> },
    ],
  },
  {
    name: "LimitsTray",
    cases: [
      { label: "fresh", node: <LimitsTrayDemo claude={34} codex={12} /> },
      { label: "past 80%", node: <LimitsTrayDemo claude={85} codex={100} /> },
      {
        label: "popover",
        node: (
          <OpenTip>
            <LimitsTrayDemo claude={34} codex={12} />
          </OpenTip>
        ),
      },
      {
        label: "spending popover",
        node: (
          <OpenTip>
            <LimitsTrayDemo claude={34} spending />
          </OpenTip>
        ),
      },
    ],
  },
  {
    name: "DiskStrip",
    // The strip has no tone of its own: it is "paused" when held and "waiting" otherwise, so the other tones show the unheld look.
    cases: [
      ...perTone((tone) => (
        <DiskStrip
          name="Macintosh HD"
          free="4.2 GB"
          held={tone === "paused"}
          cleaning={false}
          pending={false}
          onClean={() => {}}
        />
      )),
      {
        label: "warned",
        node: (
          <DiskStrip
            name="Macintosh HD"
            free="4.2 GB"
            held={false}
            cleaning={false}
            pending={false}
            onClean={() => {}}
          />
        ),
      },
      {
        label: "held",
        node: (
          <DiskStrip
            name="Macintosh HD"
            free="900 MB"
            held
            cleaning={false}
            pending={false}
            onClean={() => {}}
          />
        ),
      },
      {
        label: "cleaning",
        node: (
          <DiskStrip
            name="Macintosh HD"
            free="4.2 GB"
            held={false}
            cleaning
            pending={false}
            onClean={() => {}}
          />
        ),
      },
      {
        label: "pending",
        node: (
          <DiskStrip
            name="Macintosh HD"
            free="4.2 GB"
            held={false}
            cleaning={false}
            pending
            onClean={() => {}}
          />
        ),
      },
      {
        label: "refused",
        node: (
          <DiskStrip
            name="Macintosh HD"
            free="4.2 GB"
            held={false}
            cleaning={false}
            pending={false}
            problem="A clean already runs on this disk."
            onClean={() => {}}
          />
        ),
      },
      {
        label: "failed",
        node: (
          <DiskStrip
            name="Macintosh HD"
            free="900 MB"
            held
            cleaning={false}
            pending={false}
            problem="failed: exit status 1"
            onClean={() => {}}
          />
        ),
      },
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
        <DeviceContext.Provider value="phone">
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
        </DeviceContext.Provider>
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
    name: "EpicCard",
    cases: [
      { label: "running", node: <EpicCard row={cardRow()} /> },
      {
        label: "queued",
        node: (
          <EpicCard
            row={cardRow({
              state: "queued",
              status_words: "Queued #2",
              reason: "Starts after agent-runner-phase2",
            })}
          />
        ),
      },
      {
        label: "merging",
        node: (
          <EpicCard
            row={cardRow({
              state: "merging",
              status_words: "Merging...",
              ring_progress: 100,
              done: 5,
            })}
          />
        ),
      },
      {
        label: "finished",
        node: (
          <EpicCard
            row={cardRow({
              state: "finished",
              section: "finished",
              status_words: "Merged",
              ring_progress: 100,
              done: 5,
            })}
          />
        ),
      },
      {
        label: "draft",
        node: (
          <EpicCard
            row={cardRow({
              state: "draft",
              section: "ready-to-design",
              status_words: "Draft",
              done: 0,
              ring_progress: 0,
            })}
          />
        ),
      },
      {
        label: "ready with Queue",
        node: (
          <EpicCard
            row={cardRow({
              state: "ready",
              section: "ready-to-queue",
              status_words: "Implementing...",
              actions: [{ kind: "queue", label: "Queue" }],
            })}
          />
        ),
      },
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
        node: (
          <EpicCard
            row={cardRow({
              card_kind: "design",
              state: "waiting-to-design",
              section: "ready-to-design",
              status_words: "Not started",
              ring_progress: 0,
              actions: [{ kind: "design", label: "Design", target: "demo:blog" }],
            })}
          />
        ),
      },
      {
        label: "joined card",
        node: (
          <EpicCard
            row={cardRow({
              card_kind: "implementation",
              addresses: ["demo:blog-impl", "demo:blog"],
              name: "blog",
            })}
          />
        ),
      },
      {
        label: "one-off",
        node: (
          <EpicCard
            row={cardRow({
              card_kind: "one-off",
              state: "queued",
              status_words: "One-off",
              ring_progress: 0,
            })}
          />
        ),
      },
      {
        label: "follow-ups",
        node: (
          <EpicCard
            row={cardRow({
              card_kind: "follow-ups",
              state: "follow-ups",
              section: "ready-to-design",
              status_words: "5 bugs, 2 ideas",
              ring_progress: 0,
              actions: [{ kind: "triage", label: "Triage", target: "demo" }],
            })}
          />
        ),
      },
      {
        label: "two in a row, one footer",
        node: (
          <Box display="grid" gridTemplateColumns="repeat(2, 360px)" gap="20px">
            <EpicCard row={cardRow({ name: "no buttons" })} />
            <EpicCard
              row={cardRow({
                state: "ready",
                section: "ready-to-queue",
                name: "with Queue",
                actions: [{ kind: "queue", label: "Queue" }],
              })}
            />
          </Box>
        ),
      },
    ],
  },
  {
    name: "JumpRow",
    cases: [
      {
        label: "design",
        node: (
          <JumpRow
            row={cardRow({
              card_kind: "design",
              name: "blog",
              status_words: "Designing",
              ring_progress: 30,
            })}
          />
        ),
      },
      { label: "implementation", node: <JumpRow row={cardRow()} /> },
      {
        label: "one-off",
        node: (
          <JumpRow
            row={cardRow({
              card_kind: "one-off",
              name: "fix-it",
              state: "queued",
              status_words: "One-off",
              ring_progress: 0,
            })}
          />
        ),
      },
      {
        label: "follow-ups",
        node: (
          <JumpRow
            row={cardRow({
              card_kind: "follow-ups",
              name: "follow-ups",
              section: "ready-to-design",
              status_words: "5 bugs, 2 ideas",
              ring_progress: 0,
            })}
          />
        ),
      },
      {
        label: "Needs you",
        node: <JumpRow row={cardRow({ section: "needs-you", state_pill: "Question" })} />,
      },
      { label: "current", node: <JumpRow row={cardRow()} current /> },
      { label: "cursor", node: <JumpRow row={cardRow()} active /> },
      {
        label: "finished",
        node: (
          <JumpRow
            row={cardRow({
              state: "finished",
              section: "finished",
              status_words: "Merged",
              ring_progress: 100,
            })}
          />
        ),
      },
    ],
  },
  {
    name: "EpicPage",
    cases: [
      {
        label: "running",
        node: (
          <EpicPageFrame
            detail={withRows({ state: "running" }, [
              ...doneRows,
              epicRow("03", { rendered_status: "claimed", claimed_at: claimed }),
              epicRow("04"),
            ])}
          />
        ),
      },
      {
        label: "finished",
        node: <EpicPageFrame detail={withRows({ state: "finished" }, doneRows)} />,
      },
      {
        label: "Design",
        node: (
          <EpicPageFrame
            detail={withRows(
              {
                address: "demo:blog",
                kind: "map",
                steps: epicSteps("spec", ["chart"]),
                stats: { cost: 2.5 },
              },
              [epicRow("01", { rendered_status: "done", elapsed: 120 })],
            )}
          />
        ),
      },
      {
        label: "draft -impl",
        node: (
          <EpicPageFrame
            detail={withRows({ state: "waiting", draft: true }, [epicRow("01"), epicRow("02")])}
          />
        ),
      },
      {
        label: "merge-blocked",
        node: (
          <EpicPageFrame
            detail={withRows(
              { state: "paused", land_block_reason: "Conflicts with main in server/land.go" },
              doneRows,
            )}
          />
        ),
      },
      {
        label: "CI failing",
        node: (
          <EpicPageFrame
            detail={withRows(
              {
                state: "paused",
                pr: {
                  number: 412,
                  url: "https://github.com/demo/blog/pull/412",
                  state: "draft",
                  chip: "#412 · draft",
                },
                pr_notice:
                  "CI failing: go-test failed 3 times (limit 3 per check). Fix it by hand and push, or press Try again to give the agent 3 more attempts.",
                pr_actions: [
                  {
                    kind: "open-pr",
                    label: "Open on GitHub",
                    link: "https://github.com/demo/blog/pull/412",
                  },
                  { kind: "try-again", label: "Try again" },
                ],
                pr_activity: [
                  {
                    time: "2026-01-02T14:52:00Z",
                    kind: "warning",
                    text: "Paused: go-test failed 3 times",
                    detail: "Limit reached for this check.",
                  },
                  {
                    time: "2026-01-02T14:50:00Z",
                    kind: "bad",
                    text: "CI failed: go-test",
                    detail: "TestSearchIndex: nil pointer in index.go:88",
                  },
                  {
                    time: "2026-01-02T14:41:00Z",
                    kind: "info",
                    text: "Fix pushed",
                    ticket: "demo:blog/11",
                    detail: "Third attempt: guarded the nil case.",
                  },
                  { time: "2026-01-02T14:02:00Z", kind: "good", text: "PR #412 opened as draft" },
                ],
              },
              doneRows,
            )}
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
                  "stash-merge": {
                    label: "Stash, merge and restore",
                    tooltip: "Set your changes aside.",
                  },
                  merge: { label: "Merge again", tooltip: "Merge the ticket's commits again." },
                },
              },
              doneRows,
            )}
          />
        ),
      },
      {
        label: "cancelled card",
        node: (
          <EpicPageFrame
            detail={withRows({ state: "running" }, [
              ...doneRows,
              epicRow("03", { rendered_status: "cancelled" }),
              epicRow("04"),
            ])}
          />
        ),
      },
      {
        label: "running fork",
        node: (
          <EpicPageFrame
            detail={withRows({ state: "running" }, [
              ...doneRows,
              epicRow("03", { rendered_status: "claimed", claimed_at: claimed }),
              epicRow("04", {
                rendered_status: "claimed",
                claimed_at: claimed,
                parent: "demo:blog-impl/03",
              }),
            ])}
          />
        ),
      },
      { label: "empty", node: <EpicPageFrame detail={epicDetail({ state: "waiting" })} /> },
      {
        label: "needs-repair",
        node: (
          <EpicPageFrame
            detail={withRows({ state: "paused" }, [
              ...doneRows,
              epicRow("03", {
                rendered_status: "needs-repair",
                reason: "dirty tree",
                fix_prompt: "/gx-investigate demo:blog/03 is paused for repair: dirty tree",
              }),
            ])}
          />
        ),
      },
      {
        label: "needs-repair dialog",
        node: (
          <EpicPageFrame
            fixing
            detail={withRows({ state: "paused" }, [
              ...doneRows,
              epicRow("03", {
                rendered_status: "needs-repair",
                reason: "dirty tree",
                fix_prompt: "/gx-investigate demo:blog/03 is paused for repair: dirty tree",
              }),
            ])}
          />
        ),
      },
    ],
  },
  {
    name: "ErrorCard",
    cases: [{ label: "render error", node: <ErrorCard /> }],
  },
  ...authEntries,
  ...shellEntries,
  ...designFooterEntries,
  {
    name: "DraftSwitcher",
    cases: [
      { label: "menu closed", node: <SwitcherFrame drafts={sampleDrafts(3)} /> },
      { label: "menu open", node: <SwitcherFrame drafts={sampleDrafts(3)} open /> },
      { label: "empty list", node: <SwitcherFrame drafts={[]} open /> },
      { label: "many drafts", node: <SwitcherFrame drafts={sampleDrafts(30)} open /> },
    ],
  },
  {
    name: "NewDesignForm",
    cases: [
      {
        label: "several projects",
        node: (
          <ModalFrame>
            <NewDesignForm
              drafts={noDrafts}
              projects={["gx", "dotfiles", "blog"]}
              onStarted={() => {}}
              onClose={() => {}}
            />
          </ModalFrame>
        ),
      },
      {
        label: "one project",
        node: (
          <ModalFrame>
            <NewDesignForm
              drafts={noDrafts}
              projects={["gx"]}
              onStarted={() => {}}
              onClose={() => {}}
            />
          </ModalFrame>
        ),
      },
      {
        label: "phone bottom sheet",
        node: (
          <DeviceContext.Provider value="phone">
            <WideContext.Provider value={false}>
              <ModalFrame>
                <NewDesignForm
                  drafts={noDrafts}
                  projects={["gx", "dotfiles", "blog"]}
                  onStarted={() => {}}
                  onClose={() => {}}
                />
              </ModalFrame>
            </WideContext.Provider>
          </DeviceContext.Provider>
        ),
      },
    ],
  },
]
