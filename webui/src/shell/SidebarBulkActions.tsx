import { Box } from "@chakra-ui/react"
import { useRef, useState, type MouseEvent } from "react"
import { bulkRequest } from "../actions/bulkPlan"
import { isDraftEmpty } from "../actions/draftEmpty"
import { openConfirm, type ConfirmBody } from "../actions/confirmRequest"
import { bulkConfirms, bulkDecl } from "../actions/registry"
import { useConnectionContext } from "../connection"
import { TargetDone, TargetRefused, type SessionInfo, type TargetResult } from "../generated/server"
import { isExited } from "../agents/sessionWrites"
import {
  bulkArchive,
  bulkHold,
  bulkMove,
  bulkRemove,
  bulkRemoveDrafts,
  bulkStop,
  bulkUnhold,
  unarchiveWithToasts,
  type DraftTarget,
} from "../select/bulkWrites"
import {
  entityOfRow,
  skipLines,
  visibleActions,
  type ActionView,
  type SelectAction,
} from "../select/selectModel"
import { Button } from "../ui/button/Button"
import { GxMenu } from "../ui/menu/Menu"
import { showToast, toaster } from "../ui/toaster"
import type { Tone } from "../ui/tone"
import { WriteError } from "../useWrite"
import { useWorkspaces } from "../workspaces/useActiveWorkspace"
import type { SelectApi } from "./SidebarSelect"

const BUSY_TOAST = "bulk-action-busy"

const SIDEBAR_ACTIONS: SelectAction[] = ["move", "hold", "unhold", "stop", "archive", "remove"]

/** The buttons of the bar. `move` is a menu and sits outside this table. */
const BUTTON_TONE: Partial<Record<SelectAction, Tone>> = {
  hold: "paused",
  unhold: "running",
  stop: "paused",
  remove: "paused",
  archive: "paused",
  unarchive: "running",
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

  const views = visibleActions(sel.state, sel.rows).filter((v) => shown.includes(v.action))
  if (views.length === 0) return null

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

  const rowOf = (key: string) => sel.rows.find((r) => r.key === key)
  /** The sessions an action ends: a row's own, or the one live session of a Design. */
  const sessionsOf = (keys: string[]): SessionInfo[] =>
    keys.flatMap((k) => {
      const r = rowOf(k)
      return r?.session ? [r.session] : (r?.liveSessions ?? [])
    })
  const draftsOf = (keys: string[]): DraftTarget[] =>
    keys.flatMap((key) => {
      const r = rowOf(key)
      return r?.draftId ? [{ key, id: r.draftId, chat: r.kind === "chat-draft" }] : []
    })

  /** The confirm of a bulk run: a popover on the clicked button, listing every row the run covers. */
  const request = (v: ActionView, anchor: HTMLElement, onConfirm: () => void) => {
    return bulkRequest(
      bulkDecl(v.action),
      v.fit.fits.map(sel.nameOf),
      { mode: "anchor", anchor },
      onConfirm,
    )
  }

  // A dry run first: live sessions are the only thing worth a question.
  const hold = (v: ActionView, anchor: HTMLElement) =>
    run(v, async () => {
      const keys = v.fit.fits
      const dry = await bulkHold(keys, { dryRun: true })
      const sessions = dry.flatMap((r) => r.sessions ?? [])
      if (sessions.length === 0) return bulkHold(keys)
      const req = request(v, anchor, () => void run(v, () => bulkHold(keys, { yes: true })))
      openConfirm({
        ...req,
        body: { ...req.body, lines: [`Holding ends these live sessions: ${sessions.join(", ")}.`] },
      })
      return undefined
    })

  // Archive always asks, from a dry run: the rows, the live sessions it stops, and epics gx cannot see on trunk.
  const archive = (v: ActionView, anchor: HTMLElement) => {
    let fits: TargetResult[] = []
    let refused: TargetResult[] = []
    const req = request(v, anchor, () => {
      const n = fits.length
      // Rows the dry run refused do not run; they join the results so they stay selected with their reasons.
      void run(
        v,
        async () => [
          ...refused,
          ...(n > 0
            ? await bulkArchive(
                fits.map((r) => r.target),
                { yes: true },
              )
            : []),
        ],
        n > 0 ? `Archiving ${n} ${n === 1 ? "epic" : "epics"}…` : undefined,
      )
    })
    openConfirm({
      ...req,
      prepare: async (): Promise<ConfirmBody> => {
        const dry = await bulkArchive(v.fit.fits, { dryRun: true })
        fits = dry.filter((r) => r.status !== TargetRefused)
        refused = dry.filter((r) => r.status === TargetRefused)
        const live = fits.flatMap((r) => r.sessions ?? [])
        const offTrunk = fits.filter((r) => r.on_trunk === false)
        const n = fits.length
        return {
          lines: [
            `Archive ${n} ${n === 1 ? "epic" : "epics"}? Live agents stop and the epics leave the sidebar. Unarchive brings them back.`,
            ...(live.length > 0 ? [`Live sessions will stop: ${live.join(", ")}.`] : []),
            ...refused.map(
              (r) => `${sel.nameOf(r.target)} will not run: ${r.message || r.reason || "refused"}.`,
            ),
          ],
          warnings:
            offTrunk.length > 0
              ? [
                  `⚠ Not merged to trunk: ${offTrunk.map((r) => sel.nameOf(r.target)).join(", ")}. Archiving keeps their branches, but the work is not on trunk.`,
                ]
              : [],
          rows: fits.map((r) => sel.nameOf(r.target)),
        }
      },
    })
  }

  // Remove asks when a chat or a draft with something in it is selected; a selection of empty drafts runs at once.
  const remove = (v: ActionView, anchor: HTMLElement) =>
    run(v, async () => {
      const keys = v.fit.fits
      const sessions = sessionsOf(keys)
      const drafts = draftsOf(keys)
      const write = async () => [
        ...(sessions.length > 0 ? await bulkRemove(sessions) : []),
        ...(await bulkRemoveDrafts(drafts)),
      ]
      const empty = new Map(
        await Promise.all(
          drafts.map(
            async (d) =>
              [d.key, await isDraftEmpty(d.chat ? "chat-draft" : "design-draft", d.id)] as const,
          ),
        ),
      )
      const entities = keys.flatMap((k) => {
        const r = rowOf(k)
        return r ? [{ ...entityOfRow(r, sel.nameOf(k)), draftEmpty: empty.get(k) }] : []
      })
      if (!bulkConfirms(bulkDecl(v.action), entities)) return write()
      const live = sessions.filter((s) => !isExited(s))
      const req = request(v, anchor, () => void run(v, write))
      openConfirm({
        ...req,
        body: {
          ...req.body,
          lines: [
            ...(sessions.length > 0 ? ["Their sessions are deleted."] : []),
            ...(live.length > 0
              ? [`Live and stopped first: ${live.map((s) => sel.nameOf(s.id)).join(", ")}.`]
              : []),
            ...(drafts.length > 0 ? ["A draft's prompt and images are deleted."] : []),
          ],
        },
      })
      return undefined
    })

  const onClick: Partial<Record<SelectAction, (v: ActionView, anchor: HTMLElement) => void>> = {
    hold: (v, anchor) => void hold(v, anchor),
    unhold: (v) => void run(v, () => bulkUnhold(v.fit.fits)),
    stop: (v, anchor) => {
      const keys = v.fit.fits
      openConfirm(request(v, anchor, () => void run(v, () => bulkStop(sessionsOf(keys)))))
    },
    remove: (v, anchor) => void remove(v, anchor),
    archive: (v, anchor) => archive(v, anchor),
    unarchive: (v) => void run(v, () => unarchiveWithToasts(v.fit.fits, sel.nameOf)),
  }

  const move = views.find((v) => v.action === "move")

  return (
    <Box display="flex" flexWrap="wrap" gap="1.5">
      {views.map((v) => {
        if (v === move)
          return (
            <GxMenu
              key={v.action}
              placement="top-start"
              trigger={
                <Button
                  size="sm"
                  emphasis="secondary"
                  tone="running"
                  title={tip(v)}
                  disabled={pending}
                >
                  {v.label}
                </Button>
              }
              items={workspaces.map((w) => ({
                value: `move-${w.name}`,
                label: w.name,
                disabled: pending,
                onSelect: () => void run(v, () => bulkMove(v.fit.fits, w.name)),
              }))}
            />
          )
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
            onClick={(e: MouseEvent<HTMLButtonElement>) => onClick[v.action]?.(v, e.currentTarget)}
          >
            {v.label}
          </Button>
        )
      })}
    </Box>
  )
}
