import type { ConfirmRequest, SurfaceContext } from "./confirmRequest"
import type { ActionDecl } from "./registry"

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
