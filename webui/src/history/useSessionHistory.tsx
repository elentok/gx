import { useEffect, useRef, useState, type RefObject } from "react"
import { LuClock } from "react-icons/lu"
import type { ComposerImages } from "../design/useComposerImages"
import type { ComposerPickRequest, ComposerPickResult, DraftHistoryEntry } from "../generated/ops"
import { Button } from "../ui/button/Button"
import { PROMPT_BOX_SELECTOR } from "../ui/expandable-text/look"
import { refusalText } from "../useWrite"
import { useWide } from "../useWide"
import { postComposerPick, type postHistoryList } from "./api"
import { HistoryHost } from "./HistoryButton"
import { useHistoryOpener } from "./usePickHistory"

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
  const { list, pick = postComposerPick } = calls
  const [shown, setShown] = useState(false)
  const [notice, setNotice] = useState("")
  const [focusTick, setFocusTick] = useState(0)

  const open = () => setShown(true)
  useHistoryOpener(open, enabled)

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
      setShown(false)
      setNotice(err instanceof Error ? err.message : "Could not restore")
      return
    }
    setShown(false)
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
    enabled && shown ? (
      <HistoryHost
        list="sent"
        pending={images.uploading > 0}
        listHistory={list}
        onRestore={(e) => void restore(e)}
        onClose={() => setShown(false)}
      />
    ) : null

  return { button, popup, notice }
}
