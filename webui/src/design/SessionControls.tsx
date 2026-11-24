import { Box, Spinner, Text } from "@chakra-ui/react"
import { copyText } from "../clipboard"
import { useConnectionContext } from "../connection"
import type { DesignOpenRequest, SessionInfo } from "../generated/server"
import { Button } from "../ui/button/Button"
import { GxMenu, menuGroups, type MenuEntry } from "../ui/menu/Menu"
import { MoreActionsButton } from "../ui/menu/MoreActionsButton"
import { useWide } from "../useWide"
import { agentDetails } from "./agentDetails"
import { useWrite } from "../useWrite"
import {
  postOpenDesign,
  postStopSession,
  rowToken,
  stateToken,
  type OpenFlow,
  type StopSession,
} from "./open"

/** The shared write hook, with a connection refresh after every accepted write. */
export function useDesignWrite() {
  const conn = useConnectionContext()
  const { run, pending } = useWrite()
  const runThenRefresh: typeof run = (call, opts) =>
    run(call, {
      ...opts,
      onDone: (res) => {
        conn.refresh()
        opts?.onDone?.(res)
      },
    })
  return { run: runThenRefresh, pending }
}

export const Failure = ({ children }: { children: string }) =>
  children ? (
    <Text role="alert" fontSize="sm" color="tone.deadlocked.fg">
      {children}
    </Text>
  ) : null

/** Starts the flow's session. `onStarted` runs once it exists, e.g. to go to the Design page. */
export function StartDesignButton({
  address,
  open = postOpenDesign,
  onStarted,
  label = "Start design session",
}: {
  address: string
  open?: OpenFlow
  onStarted?: () => void
  label?: string
}) {
  const { run, pending } = useDesignWrite()
  return (
    <Box display="flex" alignItems="center" gap="2">
      <Button
        tone="running"
        disabled={pending}
        onClick={() => run(() => open({ epic: address }), { onDone: onStarted })}
      >
        {pending && <Spinner size="sm" />}
        {label}
      </Button>
    </Box>
  )
}

/** Opens a new session for the Design; asks first, and sends the state token, when the shown one is live. */
export function startNew(
  run: ReturnType<typeof useDesignWrite>["run"],
  address: string,
  shown: SessionInfo,
  open: OpenFlow = postOpenDesign,
) {
  const isLive = shown.status.state !== "exited"
  if (isLive && !window.confirm("Stop the current session and start a new one?")) return
  const body: DesignOpenRequest = {
    epic: address,
    new: true,
    ...(isLive ? { state: stateToken(shown) } : {}),
  }
  run(() => open(body))
}

/** The live sessions: pick one to show it, or stop one that is not shown. */
export function liveSessionEntries(
  live: SessionInfo[],
  shown: SessionInfo,
  pending: boolean,
  onSelect: (id: string) => void,
  onStop: (s: SessionInfo) => void,
  /** The Design page's Stop opens a confirm, so its menu closes; Triage's runs at once and stays open. */
  closeOnStop = false,
): MenuEntry[] {
  const heading: MenuEntry = {
    kind: "content",
    value: "live-sessions",
    content: (
      <Text
        px="3"
        pt="2"
        pb="0.5"
        fontFamily="mono"
        fontSize="11px"
        letterSpacing="0.07em"
        textTransform="uppercase"
        color="text.muted"
      >
        Live sessions · {live.length}
      </Text>
    ),
  }
  return [
    heading,
    ...live.flatMap((s): MenuEntry[] => {
      const name = s.name || s.id
      const isShown = s.id === shown.id
      return [
        {
          value: `session-${s.id}`,
          label: name,
          hint: isShown ? "✓" : undefined,
          inlineHint: true,
          current: isShown,
          onSelect: () => onSelect(s.id),
        },
        ...(isShown
          ? []
          : [
              {
                value: `stop-${s.id}`,
                label: `Stop ${name}`,
                danger: true,
                closeOnSelect: closeOnStop,
                disabled: pending,
                onSelect: () => onStop(s),
              },
            ]),
      ]
    }),
  ]
}

/**
 * The Triage page's "⋯" menu: the Agent block, New session, the live sessions (with 2 or more), Copy triage ID
 * and the flow's own items. The Design page reads the registry instead (`DesignPageMenu`).
 */
export function DesignActionsMenu({
  address,
  shown,
  live,
  onSelect,
  open = postOpenDesign,
  stop = postStopSession,
  extraItems = [],
  onOpenChange,
}: {
  /** True while the menu is open; the page holds its keyboard layout then. */
  onOpenChange?: (open: boolean) => void
  address: string
  /** Items another flow adds beside Copy ID, such as the Triage's Abandon. */
  extraItems?: MenuEntry[]
  shown?: SessionInfo
  /** Live sessions, newest first. */
  live: SessionInfo[]
  onSelect: (id: string) => void
  open?: OpenFlow
  stop?: StopSession
}) {
  const { run, pending } = useDesignWrite()
  const wide = useWide()

  const groups: MenuEntry[][] = []
  const agent = shown && agentSection(shown, wide)
  if (agent) groups.push(agent)
  const sessionItems: MenuEntry[] = []
  if (shown)
    sessionItems.push({
      value: "new-session",
      disabled: pending,
      label: "New session",
      hint:
        shown.status.state === "exited"
          ? "Start a fresh session"
          : "Stop this session and start a fresh one",
      onSelect: () => startNew(run, address, shown, open),
    })
  if (shown && live.length > 1)
    sessionItems.push(
      ...liveSessionEntries(live, shown, pending, onSelect, (s) =>
        run(() => stop({ id: s.id, state: rowToken(s) })),
      ),
    )
  if (sessionItems.length > 0) groups.push(sessionItems)
  groups.push([
    {
      value: "copy-design-id",
      label: "Copy triage ID",
      hint: address,
      inlineHint: true,
      onSelect: () => void copyText(address, null),
    },
    ...extraItems,
  ])
  return (
    <GxMenu
      trigger={<MoreActionsButton />}
      items={menuGroups(groups)}
      onOpenChange={onOpenChange}
    />
  )
}

/**
 * The menu's Agent block for what the footer line leaves out (Tier, the switched note, and Effort on a phone); none when it has no rows.
 */
export function agentSection(session: SessionInfo, wide: boolean): MenuEntry[] | undefined {
  const agent = agentDetails(session)
  const rows: AgentRow[] = []
  if (agent.tier) rows.push({ key: "Tier", value: agent.tier })
  if (!wide && agent.effort) rows.push({ key: "Effort", value: agent.effort })
  if (agent.switched) rows.push({ note: "switched by hand" })
  return rows.length > 0
    ? [{ kind: "content", value: "agent", content: <AgentBlock rows={rows} /> }]
    : undefined
}

type AgentRow = { key: string; value: string } | { note: string }

function AgentBlock({ rows }: { rows: AgentRow[] }) {
  return (
    <Box>
      <Text
        fontFamily="mono"
        fontSize="11px"
        letterSpacing="0.07em"
        textTransform="uppercase"
        color="text.muted"
        px="3"
        pt="2"
        pb="0.5"
      >
        Agent
      </Text>
      {rows.map((row) => (
        <Box
          key={"note" in row ? row.note : row.key}
          display="flex"
          // The model name is long; it sits under its label rather than wrapping beside it.
          flexDirection={"key" in row && row.key === "Model" ? "column" : "row"}
          justifyContent="space-between"
          gap="3"
          px="3"
          py="5px"
          fontSize="13.5px"
          color="text.dim"
        >
          {"key" in row && <span>{row.key}</span>}
          <Text as="b" fontFamily="mono" fontSize="13px" fontWeight="500" color="text">
            {"note" in row ? row.note : row.value}
          </Text>
        </Box>
      ))}
    </Box>
  )
}
