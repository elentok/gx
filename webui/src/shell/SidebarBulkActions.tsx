import { Box, Text } from "@chakra-ui/react"
import { useRef, useState, type ReactNode } from "react"
import { useConnectionContext } from "../connection"
import { TargetDone, TargetRefused, type TargetResult } from "../generated/server"
import { isExited } from "../agents/sessionWrites"
import type { SessionInfo } from "../generated/server"
import {
  bulkArchive,
  bulkHold,
  bulkMove,
  bulkRemove,
  bulkStop,
  bulkUnhold,
  unarchiveWithToasts,
} from "../select/bulkWrites"
import {
  skipLines,
  visibleActions,
  type ActionView,
  type SelectAction,
} from "../select/selectModel"
import { Button } from "../ui/button/Button"
import { GxMenu } from "../ui/menu/Menu"
import { Modal } from "../ui/modal/Modal"
import { showToast, toaster } from "../ui/toaster"
import type { Tone } from "../ui/tone"
import { WriteError } from "../useWrite"
import { useWorkspaces } from "../workspaces/useActiveWorkspace"
import type { SelectApi } from "./SidebarSelect"

const BUSY_TOAST = "bulk-action-busy"

const SIDEBAR_ACTIONS: SelectAction[] = ["move", "hold", "unhold", "stop", "remove", "archive"]

/** The question a bulk action is waiting on. Each kind carries the view it opened from, so confirming runs that same view. */
type Confirm =
  | { kind: "stop" | "remove"; view: ActionView; sessions: SessionInfo[] }
  | { kind: "hold"; view: ActionView; keys: string[]; sessions: string[] }
  /** An archive dry run: the rows that would run and the ones it refused. */
  | { kind: "archive"; view: ActionView; fits: TargetResult[]; refused: TargetResult[] }

/** The buttons of the bar, in order. `move` is a menu and sits outside this table. */
const BUTTON_TONE: Partial<Record<SelectAction, Tone>> = {
  hold: "paused",
  unhold: "running",
  stop: "paused",
  remove: "paused",
  archive: "paused",
  unarchive: "running",
}

function ConfirmModal({
  label,
  maxW,
  cancelLabel = "Cancel",
  confirmLabel,
  tone,
  confirmDisabled,
  onCancel,
  onConfirm,
  children,
}: {
  label: string
  maxW: "sm" | "md"
  cancelLabel?: string
  confirmLabel: string
  tone: Tone
  confirmDisabled?: boolean
  onCancel: () => void
  onConfirm: () => void
  children: ReactNode
}) {
  return (
    <Modal label={label} onClose={onCancel} maxW={maxW}>
      {children}
      <Box display="flex" gap="2" justifyContent="flex-end">
        <Button emphasis="secondary" onClick={onCancel} autoFocus>
          {cancelLabel}
        </Button>
        <Button tone={tone} disabled={confirmDisabled} onClick={onConfirm}>
          {confirmLabel}
        </Button>
      </Box>
    </Modal>
  )
}

/** The actions of a select bar: each runs on the selected rows it fits. `shown` limits which actions the bar offers. */
export function SidebarBulkActions({
  sel,
  shown = SIDEBAR_ACTIONS,
  onResults,
}: {
  sel: SelectApi
  shown?: SelectAction[]
  onResults?: (results: TargetResult[]) => void
}) {
  const { workspaces } = useWorkspaces(useConnectionContext())
  const [pending, setPending] = useState(false)
  const inFlight = useRef(false)
  const [confirm, setConfirm] = useState<Confirm>()

  const views = visibleActions(sel.state, sel.rows).filter((v) => shown.includes(v.action))
  if (views.length === 0 && !confirm) return null

  const tip = (v: ActionView) =>
    [`Runs on ${v.fit.fits.map(sel.nameOf).join(", ")}`, ...skipLines(v.fit, sel.nameOf)].join("\n")

  /** Runs one write; reports refusals and the rows the action did not fit, and ends the mode when nothing is left. */
  async function run(
    v: ActionView,
    call: () => Promise<TargetResult[] | undefined>,
    busy?: string,
  ) {
    if (inFlight.current) return
    inFlight.current = true
    setPending(true)
    // A slow write shows a toast that stays until it ends.
    if (busy) showToast({ type: "info", title: busy, id: BUSY_TOAST, persistent: true })
    try {
      const results = await call()
      // undefined: the call stopped to ask a question; nothing ran yet.
      if (!results) return
      // Unarchive reports its own results.
      if (v.action !== "unarchive") {
        for (const r of results) {
          if (r.status !== TargetDone)
            showToast({
              type: "error",
              title: `${sel.nameOf(r.target)}: ${r.message || r.reason || "refused"}`,
            })
        }
        for (const r of results) {
          if (r.status === TargetDone && r.new_address)
            showToast({ type: "info", title: `Archived as ${r.new_address.split(":").pop()}` })
        }
      }
      onResults?.(results)
      if (v.fit.skipped.length > 0)
        showToast({
          type: "info",
          title: "Not run on some rows",
          description: skipLines(v.fit, sel.nameOf).join("; "),
        })
      sel.afterRun()
    } catch (err) {
      showToast({
        type: "error",
        title: err instanceof WriteError ? err.message : "Could not reach gx. Try again.",
      })
    } finally {
      if (busy) toaster.remove(BUSY_TOAST)
      inFlight.current = false
      setPending(false)
    }
  }

  // A dry run first: live sessions are the only thing worth a question.
  const hold = (v: ActionView) =>
    run(v, async () => {
      const keys = v.fit.fits
      const dry = await bulkHold(keys, { dryRun: true })
      const sessions = dry.flatMap((r) => r.sessions ?? [])
      if (sessions.length === 0) return bulkHold(keys)
      setConfirm({ kind: "hold", view: v, keys, sessions })
      return undefined
    })

  // Archive always asks, from a dry run: the rows, the live sessions it stops, and epics gx cannot see on trunk.
  const archive = (v: ActionView) =>
    run(v, async () => {
      const dry = await bulkArchive(v.fit.fits, { dryRun: true })
      setConfirm({
        kind: "archive",
        view: v,
        fits: dry.filter((r) => r.status !== TargetRefused),
        refused: dry.filter((r) => r.status === TargetRefused),
      })
      return undefined
    })

  const nameOfSession = (s: SessionInfo) => sel.nameOf(s.id)
  const sessionsOf = (keys: string[]) =>
    keys.flatMap((k) => sel.rows.find((r) => r.key === k)?.session ?? [])

  // Stop asks once, listing the rows; Exited rows were already skipped "not live" by the fit.
  // Remove asks once and says which rows are live.
  const ask = (kind: "stop" | "remove", v: ActionView) =>
    setConfirm({ kind, view: v, sessions: sessionsOf(v.fit.fits) })

  const onClick: Partial<Record<SelectAction, (v: ActionView) => void>> = {
    hold: (v) => void hold(v),
    unhold: (v) => void run(v, () => bulkUnhold(v.fit.fits)),
    stop: (v) => ask("stop", v),
    remove: (v) => ask("remove", v),
    archive: (v) => void archive(v),
    unarchive: (v) => void run(v, () => unarchiveWithToasts(v.fit.fits, sel.nameOf)),
  }

  const move = views.find((v) => v.action === "move")

  const close = () => setConfirm(undefined)
  /** Closes the question, then runs the confirmed write. */
  const confirmed = (v: ActionView, call: () => Promise<TargetResult[]>, busy?: string) => {
    close()
    void run(v, call, busy)
  }

  const modal = (c: Confirm) => {
    switch (c.kind) {
      case "stop":
        return (
          <ConfirmModal
            label="Stop selected"
            maxW="sm"
            confirmLabel="Stop"
            tone="paused"
            onCancel={close}
            onConfirm={() => confirmed(c.view, () => bulkStop(c.sessions))}
          >
            <Text>Stop {c.sessions.map(nameOfSession).join(", ")}?</Text>
          </ConfirmModal>
        )
      case "remove":
        return (
          <ConfirmModal
            label="Remove selected"
            maxW="sm"
            cancelLabel="Keep"
            confirmLabel="Remove"
            tone="paused"
            onCancel={close}
            onConfirm={() => confirmed(c.view, () => bulkRemove(c.sessions))}
          >
            <Text>
              Remove {c.sessions.map(nameOfSession).join(", ")}? Their sessions are deleted.
              {c.sessions.some((s) => !isExited(s)) &&
                ` Live and stopped first: ${c.sessions
                  .filter((s) => !isExited(s))
                  .map(nameOfSession)
                  .join(", ")}.`}
            </Text>
          </ConfirmModal>
        )
      case "hold":
        return (
          <ConfirmModal
            label="Hold selected"
            maxW="sm"
            confirmLabel="Hold and end sessions"
            tone="paused"
            onCancel={close}
            onConfirm={() => confirmed(c.view, () => bulkHold(c.keys, { yes: true }))}
          >
            <Text>Holding ends these live sessions: {c.sessions.join(", ")}.</Text>
          </ConfirmModal>
        )
      case "archive": {
        const { fits, refused } = c
        const live = fits.flatMap((r) => r.sessions ?? [])
        const offTrunk = fits.filter((r) => r.on_trunk === false)
        const n = fits.length
        return (
          <ConfirmModal
            label="Archive selected"
            maxW="md"
            confirmLabel="Archive"
            tone="running"
            confirmDisabled={n === 0}
            onCancel={close}
            // Rows the dry run refused do not run; they join the results so they stay selected with their reasons.
            onConfirm={() =>
              confirmed(
                c.view,
                async () => [
                  ...refused,
                  ...(await bulkArchive(
                    fits.map((r) => r.target),
                    { yes: true },
                  )),
                ],
                `Archiving ${n} ${n === 1 ? "epic" : "epics"}…`,
              )
            }
          >
            {/* The list scrolls on a short screen; the buttons below stay in view. */}
            <Box display="flex" flexDirection="column" gap="2" overflowY="auto" minH="0" flex="1">
              <Text fontWeight="bold">
                Archive {n} {n === 1 ? "epic" : "epics"}? Live agents stop and the epics leave the
                sidebar. Unarchive brings them back.
              </Text>
              <Box as="ul" pl="5" listStyleType="disc">
                {fits.map((r) => (
                  <li key={r.target}>{sel.nameOf(r.target)}</li>
                ))}
              </Box>
              {live.length > 0 && <Text>Live sessions will stop: {live.join(", ")}.</Text>}
              {offTrunk.length > 0 && (
                <Text color="tone.waiting.fg">
                  ⚠ Not merged to trunk: {offTrunk.map((r) => sel.nameOf(r.target)).join(", ")}.
                  Archiving keeps their branches, but the work is not on trunk.
                </Text>
              )}
              {refused.map((r) => (
                <Text key={r.target}>
                  {sel.nameOf(r.target)} will not run: {r.message || r.reason || "refused"}.
                </Text>
              ))}
            </Box>
          </ConfirmModal>
        )
      }
    }
  }

  return (
    <Box display="flex" flexWrap="wrap" gap="1.5">
      {move && (
        <GxMenu
          placement="top-start"
          trigger={
            <Button
              size="sm"
              emphasis="secondary"
              tone="running"
              title={tip(move)}
              disabled={pending}
            >
              {move.label}
            </Button>
          }
          items={workspaces.map((w) => ({
            value: `move-${w.name}`,
            label: w.name,
            disabled: pending,
            onSelect: () => void run(move, () => bulkMove(move.fit.fits, w.name)),
          }))}
        />
      )}
      {views.map((v) => {
        const tone = BUTTON_TONE[v.action]
        if (!tone) return null
        return (
          <Button
            key={v.action}
            size="sm"
            emphasis="secondary"
            tone={tone}
            title={tip(v)}
            disabled={pending}
            onClick={() => onClick[v.action]?.(v)}
          >
            {v.label}
          </Button>
        )
      })}
      {confirm && modal(confirm)}
    </Box>
  )
}
