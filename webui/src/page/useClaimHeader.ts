import { useEffect } from "react"
import { useShell } from "../shell/ShellContext"

/** Tells the shell this page draws its own header (with the burger), so the shell skips its toggle row. Released on unmount. */
export function useClaimHeader() {
  const claimHeader = useShell()?.claimHeader
  useEffect(() => claimHeader?.(), [claimHeader])
}
