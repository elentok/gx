import { chatEntity } from "../actions/entity"
import { useActions } from "../actions/useActions"
import type { SessionInfo } from "../generated/server"
import type { StopSession } from "../design/open"
import type { MenuEntry } from "../ui/menu/Menu"
import { canManage } from "./sessionWrites"

/** Stop (while live) and Remove menu items for a session a person manages, from the registry. Their confirms open in the shared host. Shared by the agent page and the sidebar's chat rows. */
export function useSessionManage(
  session: SessionInfo | undefined,
  onRemoved: () => void,
  { stop }: { stop?: StopSession } = {},
): { items: MenuEntry[] } {
  const { actions } = useActions(session && chatEntity(session), {
    surface: { mode: "modal" },
    onRemoved,
    writers: stop ? { stop } : undefined,
  })
  if (!session || !canManage(session)) return { items: [] }
  return {
    items: actions
      .filter((a) => a.id === "stop" || a.id === "remove")
      .map((a) => ({
        value: a.id,
        label: a.label,
        danger: true,
        disabled: a.disabled,
        onSelect: a.run,
      })),
  }
}
