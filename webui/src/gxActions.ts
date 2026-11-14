// Action names reported with a poor INP sample. A control opts in with data-gx-action="<name>";
// a name missing from this list, or a control without the attribute, is reported as "other".
export const ACTION_ATTRIBUTE = "data-gx-action"

export const ACTIONS = ["sidebar-row", "palette", "jump-menu", "page-button"] as const

const ALLOWED: ReadonlySet<string> = new Set(ACTIONS)

/** The allowlisted action of the closest marked ancestor, else "other". */
export function actionName(el: Node | null): string {
  const target = el instanceof Element ? el.closest(`[${ACTION_ATTRIBUTE}]`) : null
  const name = target?.getAttribute(ACTION_ATTRIBUTE) ?? ""
  return ALLOWED.has(name) ? name : "other"
}
