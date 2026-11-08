import { Box, Text, chakra } from "@chakra-ui/react";
import { EpicListReady, type EpicListRow, type TriageResult, type TriageResultGroup } from "../generated/server";
import { EpicActionButton } from "../epics/EpicActionButton";
import { isCurrent, useEpicList, type FetchEpicList } from "../epics/list";
import { postQueueAdd, type PostEpicWrite } from "../epics/queue";
import { useConnectionContext } from "../connection";
import { epicHref, ticketHref } from "../hrefs";
import { Chip } from "../ui/chip/Chip";

const TITLES: Record<string, string> = {
  ready: "Ready epic",
  map: "Map epics",
  merge: "Merged",
  close: "Closed",
  keep: "Kept for later",
};
const ORDER = Object.keys(TITLES);

const link = (href: string, text: string) => (
  <chakra.a href={href} color="text.dim" textDecoration="underline">
    {text}
  </chakra.a>
);

/** Closed rows counted by reason. */
function closedCounts(group: TriageResultGroup): string {
  const counts = new Map<string, number>();
  for (const i of group.items) counts.set(i.reason ?? "no reason", (counts.get(i.reason ?? "no reason") ?? 0) + 1);
  return [...counts].map(([reason, n]) => `${reason} ${n}`).join(" · ");
}

function GroupBody({ group, readyEpic }: { group: TriageResultGroup; readyEpic?: string }) {
  switch (group.action) {
    case "close":
      return <Text color="text.muted">{closedCounts(group)}</Text>;
    case "keep":
      return <Text color="text.muted">Stay in follow-ups, the Triage still ends done.</Text>;
  }
  return (
    <Box as="ul" listStyleType="none" display="flex" flexDirection="column" gap="1">
      {group.action === "ready" && readyEpic && <li>{link(epicHref(readyEpic), readyEpic)}</li>}
      {group.items.map((i) => (
        <Box as="li" key={i.address} display="flex" gap="2" flexWrap="wrap">
          {link(ticketHref(i.address), i.address)}
          <Text as="span" color="text.muted">
            {i.title}
          </Text>
          {i.target && (group.action === "map" ? link(epicHref(i.target.replace(/\/[^/]*$/, "")), `→ ${i.target}`) : <Text as="span" color="text.dim">into {i.target}</Text>)}
        </Box>
      ))}
    </Box>
  );
}

/** The finished Triage's result, grouped by outcome. */
export function DoneView({ result }: { result: TriageResult }) {
  const groups = ORDER.flatMap((a) => result.groups?.find((g) => g.action === a && g.items.length > 0) ?? []);
  return (
    <Box as="section" aria-label="Triage result" display="flex" flexDirection="column" gap="4">
      {groups.map((g) => (
        <Box as="section" key={g.action} aria-label={TITLES[g.action]} display="flex" flexDirection="column" gap="1" pb="4" borderBottomWidth="1px" borderColor="border">
          <Box display="flex" alignItems="center" gap="2" fontWeight="semibold">
            {TITLES[g.action]}
            <Chip fontSize="xs">{g.items.length}</Chip>
          </Box>
          <GroupBody group={g} readyEpic={result.ready_epic} />
        </Box>
      ))}
    </Box>
  );
}

/** The ready epic's Queue button. What it shows derives from the epic's list row, never from the click. */
export function DoneQueue({ epic, fetchEpics, queue = postQueueAdd }: { epic: string; fetchEpics?: FetchEpicList; queue?: PostEpicWrite }) {
  const conn = useConnectionContext();
  const { list, retry } = useEpicList(fetchEpics, conn);
  const row: EpicListRow | undefined = list?.epics.find((r) => isCurrent(r, epic));
  if (!row) return null;
  if (row.state === EpicListReady)
    return <EpicActionButton address={epic} state={row.state} post={queue} label="Queue epic" tone="done" pendingLabel="Queueing…" onDone={retry} />;
  return <Chip tone="running">{row.state}</Chip>;
}
