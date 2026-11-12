import { Box, Text, chakra } from "@chakra-ui/react"
import type { TriageHistoryItem, TriageResult } from "../generated/server"

// The result groups' actions, with the word each count carries.
const COUNT_LABELS: [string, string][] = [
  ["ready", "ready"],
  ["map", "map"],
  ["merge", "merged"],
  ["close", "closed"],
  ["keep", "kept"],
]

const countsLine = (result: TriageResult) =>
  COUNT_LABELS.map(
    ([action, word]) =>
      `${word} ${result.groups?.find((g) => g.action === action)?.items.length ?? 0}`,
  ).join(" · ")

// A map row's target may be a ticket address; its epic is what the link opens.
const epicOf = (target: string) => target.replace(/\/[^/]*$/, "")

const mapEpics = (result: TriageResult) => [
  ...new Set(
    (result.groups ?? [])
      .filter((g) => g.action === "map")
      .flatMap((g) => g.items.flatMap((i) => (i.target ? [epicOf(i.target)] : []))),
  ),
]

const epicLink = (epic: string, label: string) => (
  <chakra.a
    key={`${label}-${epic}`}
    href={`#/e/${epic}`}
    color="text.dim"
    textDecoration="underline"
  >
    {label} {epic} ↗
  </chakra.a>
)

/** The past Triages, read-only and newest first (the server sends them in that order). */
export function TriageHistory({ items }: { items: TriageHistoryItem[] }) {
  if (items.length === 0) return null
  return (
    <Box
      as="section"
      aria-label="Past Triages"
      display="flex"
      flexDirection="column"
      gap="2"
      fontSize="sm"
    >
      <Text fontWeight="semibold">Past Triages</Text>
      <Box as="ul" listStyleType="none" display="flex" flexDirection="column" gap="2">
        {items.map((item) => (
          <Box
            as="li"
            key={item.id}
            data-history-item={item.id}
            display="flex"
            flexWrap="wrap"
            alignItems="center"
            columnGap="3"
            rowGap="1"
          >
            <time dateTime={item.started_at}>{new Date(item.started_at).toLocaleString()}</time>
            <Text as="span" color="text.dim">
              {countsLine(item.result)}
            </Text>
            <Text as="span" fontWeight="medium">
              {item.state}
            </Text>
            {item.result.ready_epic && epicLink(item.result.ready_epic, "ready epic")}
            {mapEpics(item.result).map((epic) => epicLink(epic, "map epic"))}
          </Box>
        ))}
      </Box>
    </Box>
  )
}
