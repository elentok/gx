import { Box, Text, chakra } from "@chakra-ui/react"
import { useState } from "react"
import type { TriageRow, TriageView } from "../generated/server"
import { ticketHref } from "../hrefs"

const SHOWN = 3
const AREA_ACTIONS = ["area", "map"]

type Tone = "done" | "running" | "muted"
type Group = {
  key: string
  count: number
  title: string
  sub?: string
  tone?: Tone
  rows: TriageRow[]
}

const byAction = (rows: TriageRow[], ...actions: string[]) =>
  rows.filter((r) => actions.includes(r.action ?? ""))

function closedSub(rows: TriageRow[]): string {
  const counts = new Map<string, number>()
  for (const r of rows) {
    const reason = r.destination || "no reason"
    counts.set(reason, (counts.get(reason) ?? 0) + 1)
  }
  return [...counts].map(([reason, n]) => `${n} ${reason}`).join(" · ")
}

/** The count lines, then one line per area, in the order the card shows them. */
function groupsOf(view: Pick<TriageView, "rows" | "ready_epic">): {
  lines: Group[]
  areas: Group[]
} {
  const live = view.rows.filter((r) => !r.gone)
  const lines: Group[] = []
  const add = (g: Omit<Group, "count">) =>
    g.rows.length > 0 && lines.push({ ...g, count: g.rows.length })
  const area = byAction(live, ...AREA_ACTIONS)
  const names = new Set(area.map((r) => r.destination ?? ""))
  const closed = byAction(live, "close")
  add({
    key: "ready",
    title: "Ready",
    sub: view.ready_epic && `→ ${view.ready_epic}`,
    tone: "done",
    rows: byAction(live, "ready"),
  })
  add({
    key: "area",
    title: "To an area",
    sub: `→ ${names.size} ${names.size === 1 ? "area" : "areas"}`,
    tone: "running",
    rows: area,
  })
  add({ key: "close", title: "Closed", sub: closedSub(closed), tone: "muted", rows: closed })
  add({ key: "merge", title: "Merged", tone: "muted", rows: byAction(live, "merge") })
  add({ key: "keep", title: "Kept for later", tone: "muted", rows: byAction(live, "keep") })

  const perArea = new Map<string, TriageRow[]>()
  for (const r of area)
    perArea.set(r.destination ?? "", [...(perArea.get(r.destination ?? "") ?? []), r])
  const areas = [...perArea].map(([name, rows]) => ({
    key: `area:${name}`,
    count: rows.length,
    title: name || "(no area)",
    rows,
  }))
  return { lines, areas }
}

/** The plan has a saved action for every row. */
export function planSaved(view: Pick<TriageView, "rows">): boolean {
  const live = view.rows.filter((r) => !r.gone)
  return live.length > 0 && live.every((r) => !!r.action)
}

const heading = (text: string) => (
  <Text fontSize="xs" color="text.dim" textTransform="uppercase" letterSpacing="wider" pt="2">
    {text}
  </Text>
)

function GroupList({ group }: { group: Group }) {
  const [all, setAll] = useState(false)
  const rows = all ? group.rows : group.rows.slice(0, SHOWN)
  const more = group.rows.length - rows.length
  return (
    <Box role="group" aria-label={`Group: ${group.title} ${group.count}`} fontSize="sm">
      {heading(`Group: ${group.title} ${group.count}`)}
      {rows.map((r) => (
        <Box
          key={r.address}
          display="flex"
          flexWrap="wrap"
          columnGap="2"
          minW="0"
          py="1"
          borderTopWidth="1px"
          borderColor="border"
        >
          <chakra.a href={ticketHref(r.address)} color="text.dim" textDecoration="underline">
            {r.address}
          </chakra.a>
          <Text as="span" minW="0" overflowWrap="anywhere">
            {r.title}
          </Text>
          {r.destination && (
            <Text as="span" color="tone.running.fg">
              → {r.destination}
            </Text>
          )}
        </Box>
      ))}
      {more > 0 && (
        <chakra.button
          type="button"
          color="text.dim"
          borderTopWidth="1px"
          borderColor="border"
          w="full"
          textAlign="left"
          py="1"
          onClick={() => setAll(true)}
        >
          {more} more
        </chakra.button>
      )}
    </Box>
  )
}

/** The saved plan as counts. Read-only: the person corrects a group by typing in the composer. */
export function PlanSummary({ view }: { view: Pick<TriageView, "rows" | "ready_epic"> }) {
  const [open, setOpen] = useState<string>()
  const { lines, areas } = groupsOf(view)
  const total = lines.reduce((n, g) => n + g.count, 0)
  const toggle = (g: Group) => setOpen(open === g.key ? undefined : g.key)
  const openGroup = [...lines, ...areas].find((g) => g.key === open)
  return (
    <Box
      role="region"
      aria-label="Plan summary"
      borderWidth="1px"
      borderColor="border"
      borderRadius="md"
      bg="surface.raised"
      px="3"
      py="3"
      fontSize="sm"
      display="flex"
      flexDirection="column"
      gap="2"
      minW="0"
    >
      <Text fontWeight="semibold">
        Plan · {total} follow-ups{" "}
        <Text as="span" color="text.dim" fontWeight="normal" fontSize="xs">
          read-only
        </Text>
      </Text>
      {lines.map((g) => (
        <chakra.button
          key={g.key}
          type="button"
          aria-expanded={open === g.key}
          display="flex"
          alignItems="center"
          gap="3"
          textAlign="left"
          onClick={() => toggle(g)}
        >
          <Text as="span" w="10" fontSize="2xl" fontWeight="bold" color={`tone.${g.tone}.fg`}>
            {g.count}
          </Text>
          <Box minW="0">
            <Text fontWeight="semibold">{g.title}</Text>
            {g.sub && (
              <Text fontSize="xs" color="text.dim">
                {g.sub}
              </Text>
            )}
          </Box>
        </chakra.button>
      ))}
      {areas.length > 0 && heading("Areas")}
      {areas.map((g) => (
        <chakra.button
          key={g.key}
          type="button"
          aria-expanded={open === g.key}
          display="flex"
          justifyContent="space-between"
          gap="3"
          onClick={() => toggle(g)}
        >
          <span>{g.title}</span>
          <Text as="span" fontWeight="bold">
            {g.count}
          </Text>
        </chakra.button>
      ))}
      {openGroup && <GroupList group={openGroup} />}
    </Box>
  )
}
