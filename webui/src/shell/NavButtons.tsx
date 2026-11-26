import { Box } from "@chakra-ui/react"
import { LuChevronLeft, LuChevronRight } from "react-icons/lu"
import { useNavigate } from "react-router"
import { useConnectionContext, useConnectionState } from "../connection"
import { leaderKeyOf } from "../leaderKey"
import { displayKey } from "./chords"
import { HeaderButton } from "./HeaderButton"
import { HeaderTooltip } from "./HeaderTooltip"
import { useHistoryState } from "./useHistoryState"

/** Its own component, so a history change re-renders only these two buttons. */
export function NavButtons() {
  const { supported, canGoBack, canGoForward } = useHistoryState()
  const { snapshot } = useConnectionState(useConnectionContext())
  const navigate = useNavigate()
  if (!supported) return null
  const leader = displayKey(leaderKeyOf(snapshot))
  const buttons = [
    { label: "Back", chord: ",", Icon: LuChevronLeft, delta: -1, enabled: canGoBack },
    { label: "Forward", chord: ".", Icon: LuChevronRight, delta: 1, enabled: canGoForward },
  ]
  return (
    <Box data-mock-part="nav-buttons" display="flex" alignItems="center" flex="none" gap="2px">
      {buttons.map(({ label, chord, Icon, delta, enabled }) => (
        <HeaderTooltip key={label} content={leader ? `${label} (${leader} ${chord})` : label}>
          <HeaderButton
            aria-label={label}
            aria-disabled={enabled ? undefined : true}
            onClick={() => enabled && navigate(delta)}
          >
            <Icon size={20} strokeWidth={1.92} aria-hidden />
          </HeaderButton>
        </HeaderTooltip>
      ))}
    </Box>
  )
}
