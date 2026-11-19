import { Box, Input, Text } from "@chakra-ui/react"
import { useEffect, useMemo, useState } from "react"
import { LuImage, LuSearch } from "react-icons/lu"
import type { DraftHistoryEntry } from "../generated/ops"
import { Button } from "../ui/button/Button"
import { BottomSheet } from "../ui/modal/BottomSheet"
import { useWide } from "../useWide"
import type { HistoryList } from "./api"
import { historyTime } from "./historyTime"

const LISTS: Record<HistoryList, { name: string; placeholder: string }> = {
  "chat-drafts": { name: "Chat drafts", placeholder: "Filter chat drafts…" },
  "design-drafts": { name: "Design drafts", placeholder: "Filter design drafts…" },
  sent: { name: "Sent messages", placeholder: "Filter sent messages…" },
}

type Tone = "running" | "done" | "chat"

function pillOf(list: HistoryList, e: DraftHistoryEntry): { label: string; tone: Tone } {
  if (list === "design-drafts") return { label: e.slug, tone: "running" }
  if (list === "chat-drafts") return { label: `${e.project} / ${e.name}`, tone: "chat" }
  const label = e.composer === "design" ? `Design · ${e.slug}` : `Chat · ${e.name}`
  return { label, tone: "done" }
}

function detailsOf(list: HistoryList, e: DraftHistoryEntry): string {
  if (list !== "chat-drafts") return e.project
  return [e.harness, e.model, e.effort].filter(Boolean).join(" · ")
}

const pillColors: Record<Tone, { color: string; bg: string }> = {
  running: { color: "tone.running.fg", bg: "tone.running.bg" },
  done: { color: "tone.done.fg", bg: "tone.done.bg" },
  chat: { color: "chat", bg: "chat/16" },
}

function matches(e: DraftHistoryEntry, q: string): boolean {
  const needle = q.toLowerCase()
  return [e.text, e.slug, e.project, e.name].some((s) => s.toLowerCase().includes(needle))
}

function Row({
  entry,
  list,
  now,
  active,
  pending,
  onPick,
  id,
}: {
  entry: DraftHistoryEntry
  list: HistoryList
  now: Date
  active: boolean
  pending: boolean
  onPick: () => void
  id: string
}) {
  const pill = pillOf(list, entry)
  const colors = pillColors[pill.tone]
  const hasImages = entry.imageCount > 0 || entry.imagesDropped
  return (
    <Box
      id={id}
      role="option"
      aria-selected={active}
      px="10px"
      py="8px"
      borderRadius="8px"
      cursor="pointer"
      bg={active ? "sidebar.hover" : undefined}
      boxShadow={active ? "inset 2px 0 0 {colors.tone.running.fg}" : undefined}
      _hover={{ bg: "sidebar.hover" }}
      onClick={onPick}
    >
      <Text lineClamp="1">{entry.text || (hasImages ? "(images only)" : "")}</Text>
      <Box display="flex" alignItems="center" gap="2" mt="1px" fontSize="12px" color="text.dim">
        <Text
          as="span"
          fontSize="11px"
          fontWeight="600"
          letterSpacing="0.04em"
          borderRadius="full"
          px="2"
          flexShrink="0"
          whiteSpace="nowrap"
          color={colors.color}
          bg={colors.bg}
        >
          {pill.label}
        </Text>
        <Text as="span" lineClamp="1">
          {detailsOf(list, entry)}
        </Text>
        <Text as="span" whiteSpace="nowrap">
          {pending ? "Saving…" : historyTime(new Date(entry.savedAt), now)}
        </Text>
        {entry.imagesDropped ? (
          <Text as="span" whiteSpace="nowrap">
            images not kept
          </Text>
        ) : (
          entry.imageCount > 0 && (
            <Box as="span" display="inline-flex" alignItems="center" gap="1">
              <LuImage size={12} aria-hidden />
              {entry.imageCount}
            </Box>
          )
        )}
      </Box>
    </Box>
  )
}

/** The History popup: a filterable list of past entries. The host owns what a restore does. */
export function HistoryPopup({
  list,
  entries,
  restoreDisabled = false,
  max = 50,
  now,
  onRestore,
  onClose,
}: {
  list: HistoryList
  /** Newest first. */
  entries: DraftHistoryEntry[]
  /** A save is pending: rows show "Saving…" and nothing restores. */
  restoreDisabled?: boolean
  max?: number
  now?: Date
  onRestore: (entry: DraftHistoryEntry) => void
  onClose: () => void
}) {
  const wide = useWide()
  const { name, placeholder } = LISTS[list]
  const [filter, setFilter] = useState("")
  // On a wide screen the cursor starts on the first row; on a phone nothing is marked until a tap.
  const [cursor, setCursor] = useState(wide ? 0 : -1)
  const shown = useMemo(() => {
    const q = filter.trim()
    return q ? entries.filter((e) => matches(e, q)) : entries
  }, [entries, filter])
  const [clock] = useState(() => now ?? new Date())
  const rowId = (i: number) => `history-row-${i}`
  const activeId = shown[cursor] ? rowId(cursor) : undefined

  useEffect(() => {
    document.getElementById(activeId ?? "")?.scrollIntoView?.({ block: "nearest" })
  }, [activeId])

  const restore = (e: DraftHistoryEntry | undefined) => {
    if (e && !restoreDisabled) onRestore(e)
  }

  function onKeyDown(e: React.KeyboardEvent) {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault()
      if (shown.length === 0) return
      const step = e.key === "ArrowDown" ? 1 : shown.length - 1
      setCursor((c) => (c < 0 ? (step === 1 ? 0 : shown.length - 1) : (c + step) % shown.length))
    } else if (e.key === "Enter") {
      e.preventDefault()
      restore(shown[cursor])
    }
  }

  return (
    <BottomSheet
      label={`${name} history`}
      onClose={onClose}
      fillViewport
      topGap={90}
      p="0"
      gap="0"
      bg="surface.raised"
      borderColor="hairline"
      desktop={{
        w: "640px",
        maxH: "480px",
        p: "0",
        gap: "0",
        bg: "surface.raised",
        borderColor: "hairline",
        borderRadius: "14px",
        boxShadow: "lg",
        overflow: "hidden",
      }}
      desktopBackdrop={{ alignItems: "flex-start", pt: "70px" }}
    >
      <Text px="3" pt="10px" fontWeight="700" data-testid="history-title">
        {name}{" "}
        <Text as="span" fontSize="xs" fontWeight="400" color="text.dim">
          · newest first
        </Text>
      </Text>
      <Box
        display="flex"
        alignItems="center"
        gap="2"
        px="3"
        py="10px"
        borderBottomWidth="1px"
        borderColor="hairline"
      >
        <Box color="text.faint" display="inline-flex">
          <LuSearch size={16} aria-hidden />
        </Box>
        <Input
          autoFocus
          type="search"
          role="combobox"
          aria-label={placeholder}
          aria-expanded
          aria-controls="history-list"
          aria-activedescendant={activeId}
          placeholder={placeholder}
          value={filter}
          variant="flushed"
          borderWidth="0"
          px="0"
          h="auto"
          fontSize="16px"
          color="text"
          _placeholder={{ color: "text.faint" }}
          onChange={(e) => {
            setFilter(e.target.value)
            setCursor(wide ? 0 : -1)
          }}
          onKeyDown={onKeyDown}
        />
      </Box>
      <Box id="history-list" role="listbox" overflowY="auto" p="6px" flex="1" minH="0">
        {shown.length === 0 ? (
          <Text px="10px" py="8px" color="text.dim">
            No match for "{filter}"
          </Text>
        ) : (
          shown.map((entry, i) => (
            <Row
              key={entry.id}
              id={rowId(i)}
              entry={entry}
              list={list}
              now={clock}
              active={i === cursor}
              pending={restoreDisabled}
              onPick={() => (wide ? restore(entry) : setCursor(i))}
            />
          ))
        )}
      </Box>
      {wide ? (
        <Box
          display="flex"
          gap="3"
          px="3"
          py="6px"
          borderTopWidth="1px"
          borderColor="hairline"
          fontSize="12px"
          color="text.faint"
        >
          <span>↑↓ pick</span>
          <span>⏎ restore</span>
          <span>esc close</span>
          <Text as="span" ml="auto">
            {shown.length} of {max}
          </Text>
        </Box>
      ) : (
        <Box display="flex" gap="2" px="3" py="10px" borderTopWidth="1px" borderColor="hairline">
          <Button emphasis="secondary" h="44px" flex="1" onClick={onClose}>
            Close
          </Button>
          <Button
            emphasis="ghost"
            tone="running"
            h="44px"
            flex="2"
            bg="tone.running.bg"
            disabled={cursor < 0 || !shown[cursor] || restoreDisabled}
            onClick={() => restore(shown[cursor])}
          >
            Restore
          </Button>
        </Box>
      )}
    </BottomSheet>
  )
}
