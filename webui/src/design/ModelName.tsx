import type { SessionInfo } from "../generated/server"
import { openModelPicker } from "../shell/modelPickerOpen"
import { LinkButton } from "./LinkButton"
import { OutdatedModel } from "./OutdatedModel"

/** The footer model name: the outdated popover, the picker button, or plain text. */
export function ModelName({
  session,
  text,
  switching,
}: {
  session: SessionInfo
  text: string
  switching: boolean
}) {
  if (switching) return <>{text}</>
  if (session.latest_model) return <OutdatedModel session={session} text={text} />
  if (session.design) return <>{text}</>
  return (
    <LinkButton
      data-testid="model-name-button"
      font="inherit"
      color="inherit"
      _hover={{ textDecoration: "underline" }}
      onClick={() => openModelPicker({ session })}
    >
      {text}
    </LinkButton>
  )
}
