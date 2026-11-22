import { chakra } from "@chakra-ui/react"
import { useEffect, useState } from "react"
import { LuClock } from "react-icons/lu"
import type { DraftHistoryEntry } from "../generated/ops"
import { useLatest } from "../hooks"
import { useWide } from "../useWide"
import { HistoryPopup } from "./HistoryPopup"
import { postHistoryList, type HistoryList, type ListHistory } from "./api"

/** The History button of a title row: icon and text on a wide screen, the icon alone on a phone. */
export function HistoryButton({ onClick }: { onClick: () => void }) {
  const wide = useWide()
  return (
    <chakra.button
      type="button"
      aria-label={wide ? undefined : "History"}
      title={wide ? undefined : "History"}
      onClick={onClick}
      display="inline-flex"
      alignItems="center"
      justifyContent="center"
      gap="6px"
      h="32px"
      w={wide ? undefined : "32px"}
      px={wide ? "14px" : "0"}
      flexShrink="0"
      fontSize="13px"
      fontWeight="600"
      color="text"
      bg="surface.raised"
      borderWidth="1px"
      borderColor="hairline"
      borderRadius="8px"
    >
      <LuClock size={18} aria-hidden />
      {wide && "History"}
    </chakra.button>
  )
}

/** Loads one list and shows it in the popup. A failed load closes the popup. */
export function HistoryHost({
  list,
  pending,
  listHistory = postHistoryList,
  onRestore,
  onClose,
}: {
  list: HistoryList
  /** A save is pending: nothing restores. */
  pending: boolean
  listHistory?: ListHistory
  onRestore: (entry: DraftHistoryEntry) => void
  onClose: () => void
}) {
  const [shown, setShown] = useState<{ entries: DraftHistoryEntry[]; max: number }>()
  // Once per opening: the load reads the latest props without re-running.
  const latest = useLatest({ listHistory, onClose })
  useEffect(() => {
    let live = true
    latest.current
      .listHistory({ list })
      .then((res) => live && setShown({ entries: res.entries, max: res.max }))
      .catch(() => live && latest.current.onClose())
    return () => {
      live = false
    }
  }, [list])
  if (!shown) return null
  return (
    <HistoryPopup
      list={list}
      entries={shown.entries}
      max={shown.max}
      restoreDisabled={pending}
      onRestore={onRestore}
      onClose={onClose}
    />
  )
}
