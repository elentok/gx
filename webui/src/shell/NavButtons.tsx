import { Box } from "@chakra-ui/react"
import { LuChevronLeft, LuChevronRight } from "react-icons/lu"
import { useNavigate } from "react-router"
import { HeaderButton } from "./HeaderButton"
import { useHistoryState } from "./useHistoryState"

/** Its own component, so a history change re-renders only these two buttons. */
export function NavButtons() {
  const { supported, canGoBack, canGoForward } = useHistoryState()
  const navigate = useNavigate()
  if (!supported) return null
  const buttons = [
    { label: "Back", Icon: LuChevronLeft, delta: -1, enabled: canGoBack },
    { label: "Forward", Icon: LuChevronRight, delta: 1, enabled: canGoForward },
  ]
  return (
    <Box data-mock-part="nav-buttons" display="flex" alignItems="center" flex="none" gap="2px">
      {buttons.map(({ label, Icon, delta, enabled }) => (
        <HeaderButton
          key={label}
          aria-label={label}
          aria-disabled={enabled ? undefined : true}
          onClick={() => enabled && navigate(delta)}
        >
          <Icon size={20} strokeWidth={1.92} aria-hidden />
        </HeaderButton>
      ))}
    </Box>
  )
}
