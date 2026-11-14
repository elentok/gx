import { postStopSession, rowToken, type StopSession } from "../design/open"
import type { SessionInfo } from "../generated/server"
import { openRemoveConfirm } from "../shell/removeConfirmOpen"
import { useWrite } from "../useWrite"
import type { MenuEntry } from "../ui/menu/Menu"
import { canManage, isExited } from "./sessionWrites"

/** Stop (while live) and Remove menu items for a session a person manages. Remove opens the shared confirm, rendered by RemoveConfirmHost. Shared by the agent page and the sidebar's chat rows. */
export function useSessionManage(
  session: SessionInfo | undefined,
  onRemoved: () => void,
  { stop = postStopSession }: { stop?: StopSession } = {},
): { items: MenuEntry[] } {
  const { run, pending } = useWrite()
  if (!session || !canManage(session)) return { items: [] }
  const items: MenuEntry[] = []
  if (!isExited(session))
    items.push({
      value: "stop",
      label: "Stop",
      danger: true,
      disabled: pending,
      onSelect: () => run(() => stop({ id: session.id, state: rowToken(session) })),
    })
  items.push({
    value: "remove",
    label: "Remove",
    danger: true,
    disabled: pending,
    onSelect: () => openRemoveConfirm({ session, onRemoved }),
  })
  return { items }
}
