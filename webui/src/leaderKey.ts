import { useContext, useSyncExternalStore } from "react"
import { ConnectionContext, type ConnectionState } from "./connection"
import { isMac } from "./platform"

const noopSubscribe = () => () => {}
const noState = (): ConnectionState | undefined => undefined

// Reads this platform's Leader key (like "cmd+e"); empty means chords are off. A missing
// ConnectionContext reads as off instead of throwing, like useViEscape.
export function useLeaderKey(): string {
  const conn = useContext(ConnectionContext)
  const state = useSyncExternalStore(conn?.subscribe ?? noopSubscribe, conn?.getState ?? noState)
  const snap = state?.snapshot
  return (isMac() ? snap?.leader_key_mac : snap?.leader_key) ?? ""
}
