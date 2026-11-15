import { Box, Text, chakra } from "@chakra-ui/react"
import type { ReactNode } from "react"
import {
  EpicListReady,
  type EpicListRow,
  type TriageResult,
  type TriageResultGroup,
} from "../generated/server"
import { EpicActionButton } from "../epics/EpicActionButton"
import { isCurrent, useEpicList, type FetchEpicList } from "../epics/list"
import { postQueueAdd, type PostEpicWrite } from "../epics/queue"
import { useConnectionContext } from "../connection"
import { epicHref, ticketHref } from "../hrefs"
import { Chip } from "../ui/chip/Chip"

const TITLES: Record<string, string> = {
  ready: "Ready epic",
  area: "Map epics",
  merge: "Merged",
  close: "Closed",
  keep: "Kept for later",
}
const ORDER = Object.keys(TITLES)

const link = (href: string, text: string) => (
  <chakra.a href={href} color="text.dim" textDecoration="underline">
    {text}
  </chakra.a>
)

/** Closed rows counted by reason. */
function closedCounts(group: TriageResultGroup): string {
  const counts = new Map<string, number>()
  for (const i of group.items)
    counts.set(i.reason ?? "no reason", (counts.get(i.reason ?? "no reason") ?? 0) + 1)
  return [...counts].map(([reason, n]) => `${reason} ${n}`).join(" · ")
}

/** Moved rows counted by the area (epic name) they went to. */
function areaCounts(group: TriageResultGroup): string {
  const counts = new Map<string, number>()
  for (const i of group.items) {
    const area = (i.target ?? "").replace(/\/[^/]*$/, "").replace(/^[^:]*:/, "")
    counts.set(area, (counts.get(area) ?? 0) + 1)
  }
  return [...counts].map(([area, n]) => `${area} ${n}`).join(" · ")
}

/** The line text of a group: its count and what happened to the rows. */
function GroupLine({ group, readyEpic }: { group: TriageResultGroup; readyEpic?: string }) {
  const n = group.items.length
  const count = (word: string) => (
    <Text as="span" fontWeight="semibold">
      {n} {word}
    </Text>
  )
  switch (group.action) {
    case "ready":
      return (
        <>
          {count("ready")}
          {readyEpic && <> in {link(epicHref(readyEpic), readyEpic)}</>}
        </>
      )
    case "area":
      return (
        <>
          {count("to areas")} · {areaCounts(group)}
        </>
      )
    case "merge":
      return count("merged")
    case "close":
      return (
        <>
          {count("closed")} · {closedCounts(group)}
        </>
      )
    default:
      return count("kept for later in follow-ups")
  }
}

function GroupRows({ group }: { group: TriageResultGroup }) {
  return (
    <Box as="ul" listStyleType="none" display="flex" flexDirection="column" gap="1" pl="6">
      {group.items.map((i) => (
        <Box as="li" key={i.address} display="flex" gap="2" flexWrap="wrap">
          {link(ticketHref(i.address), i.address)}
          <Text as="span" color="text.muted">
            {i.title}
          </Text>
          {i.target &&
            (group.action === "area" ? (
              link(ticketHref(i.target), `→ ${i.target}`)
            ) : (
              <Text as="span" color="text.dim">
                into {i.target}
              </Text>
            ))}
        </Box>
      ))}
    </Box>
  )
}

/** The finished Triage's result card, one line per outcome. `queue` sits on the ready line. */
export function DoneView({ result, queue }: { result: TriageResult; queue?: ReactNode }) {
  const groups = ORDER.flatMap(
    (a) => result.groups?.find((g) => g.action === a && g.items.length > 0) ?? [],
  )
  return (
    <Box
      as="section"
      aria-label="Triage result"
      display="flex"
      flexDirection="column"
      p="4"
      borderWidth="1px"
      borderColor="border"
      borderRadius="lg"
    >
      <Text fontWeight="semibold" pb="2">
        Result
      </Text>
      {groups.map((g) => (
        <Box
          as="section"
          key={g.action}
          aria-label={TITLES[g.action]}
          display="flex"
          flexDirection="column"
          gap="1"
          py="3"
          borderTopWidth="1px"
          borderColor="border"
        >
          <Box display="flex" alignItems="center" gap="2" flexWrap="wrap">
            <Text as="span" color="tone.done.fg" aria-hidden>
              ✓
            </Text>
            <GroupLine group={g} readyEpic={result.ready_epic} />
            {g.action === "ready" && queue && <Box ml="auto">{queue}</Box>}
          </Box>
          <GroupRows group={g} />
        </Box>
      ))}
    </Box>
  )
}

/** The ready epic's Queue button. What it shows derives from the epic's list row, never from the click. */
export function DoneQueue({
  epic,
  fetchEpics,
  queue = postQueueAdd,
}: {
  epic: string
  fetchEpics?: FetchEpicList
  queue?: PostEpicWrite
}) {
  const conn = useConnectionContext()
  const { list, retry } = useEpicList(fetchEpics, conn)
  const row: EpicListRow | undefined = list?.epics.find((r) => isCurrent(r, epic))
  if (!row) return null
  if (row.state === EpicListReady)
    return (
      <EpicActionButton
        address={epic}
        state={row.state}
        post={queue}
        label="Queue it"
        tone="running"
        pendingLabel="Queueing…"
        onDone={retry}
      />
    )
  return <Chip tone="running">{row.state}</Chip>
}
