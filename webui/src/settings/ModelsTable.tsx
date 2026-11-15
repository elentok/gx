import { Box, chakra, Text } from "@chakra-ui/react"
import { useState } from "react"
import type { SettingsEntry } from "../generated/server"
import { useWrite } from "../useWrite"
import { useDevice } from "../useWide"
import { removeChange, setChange } from "./settingsApi"
import { serverText, TextControl, writeErrorText, type WriteSetting } from "./SettingsRow"

// The registry lists the key without a tier first; the table reads weakest to strongest. A tier not named here sorts last.
const TIER_ORDER = ["light", "standard", "strongest"]

const capitalized = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)
const unique = (values: (string | undefined)[]) => [
  ...new Set(values.filter((v): v is string => !!v)),
]

/** The key pattern a table row stands for: its keys' shared path with a wildcard last segment. */
const patternOf = (e: SettingsEntry) => `${e.path.slice(0, -1).join(".")}.*`

function ModelCell({
  entry,
  agent,
  onWrite,
}: {
  entry: SettingsEntry
  agent: string
  onWrite: WriteSetting
}) {
  const write = useWrite({ toastRefusal: false })
  // A draft means the cell is open; closing drops it.
  const [draft, setDraft] = useState<string>()
  const current = serverText(entry)
  const { refused, text: errorText } = writeErrorText(write, entry.key)
  const run = (change: Parameters<WriteSetting>[0][number]) =>
    void write.run(() => onWrite([change]), { onDone: () => setDraft(undefined) })
  return (
    <Box
      as="td"
      data-cell={entry.key}
      data-set={entry.set ? "true" : undefined}
      verticalAlign="top"
      px="3"
      py="2"
      minW="140px"
    >
      {draft === undefined ? (
        <Box
          display="flex"
          alignItems="center"
          gap="2"
          minH="36px"
          px="3"
          borderWidth="1px"
          borderLeftWidth={entry.set ? "3px" : "1px"}
          borderColor={entry.set ? "tone.running.fg" : "border"}
          borderRadius="8px"
          bg="surface"
        >
          <chakra.button
            type="button"
            textAlign="left"
            flex="1"
            minW="0"
            fontFamily="mono"
            fontSize="13px"
            fontStyle={current === "" ? "italic" : undefined}
            color={current === "" ? "text.dim" : "text"}
            onClick={() => setDraft(current)}
          >
            {current === "" ? `${agent} default` : current}
          </chakra.button>
          {entry.set && (
            <chakra.button
              type="button"
              aria-label={`Reset ${entry.key}`}
              title="Reset to default"
              disabled={write.pending}
              color="tone.running.fg"
              onClick={() => run(removeChange(entry.key))}
            >
              ↺
            </chakra.button>
          )}
        </Box>
      ) : (
        <Box display="flex" gap="2" alignItems="center" flexWrap="wrap">
          <TextControl
            entry={entry}
            draft={draft}
            dirty={draft !== current}
            invalid={refused}
            pending={write.pending}
            locked={false}
            cancellable
            setDraft={setDraft}
            save={(value) => run(setChange(entry.key, value))}
            remove={() => run(removeChange(entry.key))}
          />
        </Box>
      )}
      {errorText && (
        <Text role="alert" fontSize="xs" color="tone.deadlocked.fg">
          {errorText}
        </Text>
      )}
    </Box>
  )
}

/** One agent's entries as a grid: a row per tier, a column per column the entries name. */
export function ModelsTable({
  agent,
  entries,
  onWrite,
}: {
  agent: string
  entries: SettingsEntry[]
  onWrite: WriteSetting
}) {
  const phone = useDevice() === "phone"
  const tiers = unique(entries.map((e) => e.tier)).sort(
    (a, b) => TIER_ORDER.indexOf(a) - TIER_ORDER.indexOf(b),
  )
  const columns = unique(entries.map((e) => e.column))
  return (
    <Box overflowX="auto">
      <chakra.table w="100%" borderCollapse="collapse">
        <thead>
          <tr>
            <th />
            {columns.map((c) => (
              <chakra.th
                key={c}
                textAlign="left"
                px="3"
                py="1.5"
                fontSize="11px"
                fontWeight="600"
                letterSpacing="0.06em"
                textTransform="uppercase"
                color="text.muted"
              >
                {capitalized(c)}
              </chakra.th>
            ))}
          </tr>
        </thead>
        <tbody>
          {tiers.map((tier) => {
            const row = entries.filter((e) => e.tier === tier)
            return (
              <chakra.tr key={tier} borderTopWidth="1px" borderTopColor="hairline">
                <chakra.th scope="row" textAlign="left" px="3.5" py="2" verticalAlign="top">
                  <Text fontWeight="600">{capitalized(tier)}</Text>
                  {!phone && row[0] && (
                    <Text fontFamily="mono" fontSize="11px" fontWeight="400" color="text.dim">
                      {patternOf(row[0])}
                    </Text>
                  )}
                </chakra.th>
                {columns.map((c) => {
                  const e = row.find((x) => x.column === c)
                  return e ? (
                    <ModelCell key={c} entry={e} agent={agent} onWrite={onWrite} />
                  ) : (
                    <td key={c} />
                  )
                })}
              </chakra.tr>
            )
          })}
        </tbody>
      </chakra.table>
    </Box>
  )
}
