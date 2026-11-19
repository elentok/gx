import { useCallback, useEffect, useRef, useState, type RefObject } from "react"
import { LuClock } from "react-icons/lu"
import type { ComposerImages } from "../design/useComposerImages"
import type { ComposerPickRequest, ComposerPickResult, DraftHistoryEntry } from "../generated/ops"
import { Button } from "../ui/button/Button"
import { PROMPT_BOX_SELECTOR } from "../ui/expandable-text/look"
import { refusalText } from "../useWrite"
import { useWide } from "../useWide"
import { postComposerPick, postHistoryList } from "./api"
import { registerHistoryOpener } from "./historyOpener"
import { HistoryPopup } from "./HistoryPopup"

export interface HistoryCalls {
  list?: typeof postHistoryList
  pick?: (body: ComposerPickRequest) => Promise<ComposerPickResult>
}

/**
 * The History button, its popup and the restore of a Sent messages entry into a composer.
 * `button` goes after Add; `popup` and `notice` are for the composer to place. Pass `enabled` false for a session the rule does not offer History.
 */
export function useSessionHistory({
  enabled,
  sessionId,
  text,
  setText,
  images,
  box,
  calls = {},
}: {
  enabled: boolean
  sessionId: string
  text: string
  setText: (text: string) => void
  images: ComposerImages
  box: RefObject<HTMLElement | null>
  calls?: HistoryCalls
}) {
  const wide = useWide()
  const { list = postHistoryList, pick = postComposerPick } = calls
  const [entries, setEntries] = useState<DraftHistoryEntry[] | null>(null)
  const [max, setMax] = useState(50)
  const [notice, setNotice] = useState("")
  const [focusTick, setFocusTick] = useState(0)

  const open = useCallback(() => {
    list({ list: "sent" }).then(
      (res) => {
        setMax(res.max)
        setEntries(res.entries)
      },
      (err) => setNotice(err instanceof Error ? err.message : "Could not load History"),
    )
  }, [list])

  const openRef = useRef(open)
  openRef.current = open
  useEffect(() => {
    if (!enabled) return
    return registerHistoryOpener(() => openRef.current())
  }, [enabled])

  // After the popup closes: the prompt box takes focus.
  useEffect(() => {
    if (focusTick > 0) box.current?.querySelector<HTMLElement>(PROMPT_BOX_SELECTOR)?.focus()
  }, [focusTick, box])

  // The text and images a pick left behind: an edit away from them, or a send, makes its notice stale.
  const pickState = useRef<{ text: string; ids: string } | null>(null)
  const ids = images.readyIds.join(",")
  useEffect(() => {
    const picked = pickState.current
    if (!picked || (picked.text === text && picked.ids === ids)) return
    pickState.current = null
    setNotice("")
  }, [text, ids])

  const restore = async (entry: DraftHistoryEntry) => {
    let res: ComposerPickResult
    try {
      res = await pick({
        session_id: sessionId,
        entry_id: entry.id,
        text,
        images: images.readyIds,
      })
    } catch (err) {
      setEntries(null)
      setNotice(err instanceof Error ? err.message : "Could not restore")
      return
    }
    setEntries(null)
    if (res.refused) {
      setNotice(refusalText(res))
      return
    }
    pickState.current = { text: res.text, ids: res.images.join(",") }
    setText(res.text)
    images.replace(res.images)
    setNotice(res.notices.join(" · "))
    setFocusTick((n) => n + 1)
  }

  const button = enabled ? (
    <Button
      type="button"
      emphasis="secondary"
      aria-label="History"
      title={wide ? undefined : "History"}
      {...(wide ? {} : { w: "44px", h: "44px", px: "0", justifyContent: "center", flex: "none" })}
      onClick={open}
    >
      <LuClock size={wide ? undefined : 20} />
      {wide && " History"}
    </Button>
  ) : null

  const popup =
    enabled && entries ? (
      <HistoryPopup
        list="sent"
        entries={entries}
        max={max}
        restoreDisabled={images.uploading > 0}
        onRestore={(e) => void restore(e)}
        onClose={() => setEntries(null)}
      />
    ) : null

  return { button, popup, notice }
}
