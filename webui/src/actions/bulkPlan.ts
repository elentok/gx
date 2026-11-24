import type { ConfirmRequest, SurfaceContext } from "./confirmRequest"
import type { Entity } from "./entity"
import { bulkFit, bulkLabel, type ActionDecl, type BulkFit } from "./registry"

export interface BulkPlan {
  fit: BulkFit
  /** "Hold 2 of 5", or "Hold 2" when every selected entity fits. */
  label: string
  /** The tooltip lines: one "name: reason" per skipped entity. */
  skipLines: string[]
  /** One merged confirm for the rows that run; absent when none of them needs it. */
  confirm?: ConfirmRequest
}

/** The bar's plan for one action over the selection. `onConfirm` runs the writes for the fitting rows. */
export function planBulk(
  action: ActionDecl,
  selected: Entity[],
  surface: SurfaceContext,
  onConfirm: (fits: Entity[]) => void,
): BulkPlan {
  const fit = bulkFit(action, selected)
  const byKey = new Map(selected.map((e) => [e.key, e]))
  const running = fit.fits.flatMap((k) => byKey.get(k) ?? [])
  const label = bulkLabel(action, fit)
  const skipLines = fit.skipped.map((s) => `${byKey.get(s.key)?.name ?? s.key}: ${s.reason}`)
  const needs = running.some((e) => action.confirm.rule(e))
  if (!needs) return { fit, label, skipLines }
  return {
    fit,
    label,
    skipLines,
    confirm: bulkRequest(
      action,
      running.map((e) => e.name),
      surface,
      () => onConfirm(running),
    ),
  }
}

/** The confirm of a bulk run over `rows`: the button carries the count and the body lists every row. */
export function bulkRequest(
  action: ActionDecl,
  rows: string[],
  surface: SurfaceContext,
  onConfirm: () => void,
): ConfirmRequest {
  const confirmLabel = `${action.barLabel ?? action.label} ${rows.length}`
  return {
    actionId: action.id,
    entityName: `${rows.length} items`,
    group: action.group,
    danger: action.danger,
    confirmLabel,
    dismissLabel: "Cancel",
    title: confirmLabel,
    body: { lines: [], rows },
    surface,
    onConfirm: () => {
      onConfirm()
    },
  }
}
