import { Box, Text } from "@chakra-ui/react"
import { useEffect, useRef, type KeyboardEvent } from "react"
import { Button } from "../ui/button/Button"
import { Modal } from "../ui/modal/Modal"
import { closeWorkspacePicker, useWorkspacePickerRequest } from "./workspacePickerOpen"

/** The Move to workspace chord's picker: every workspace, the current one marked; a pick posts the move. */
export function WorkspacePickerHost() {
  const request = useWorkspacePickerRequest()
  const list = useRef<HTMLDivElement>(null)
  // Focus the current workspace (else the first) so arrows and Enter work at once.
  useEffect(() => {
    if (!request) return
    const buttons = list.current?.querySelectorAll<HTMLElement>("button")
    const start = list.current?.querySelector<HTMLElement>("[aria-current]") ?? buttons?.[0]
    start?.focus()
  }, [request])
  if (!request) return null

  // Enter and Space already click the focused button; arrows move focus, wrapping at the ends.
  const onKeyDown = (e: KeyboardEvent) => {
    const by = e.key === "ArrowDown" ? 1 : e.key === "ArrowUp" ? -1 : 0
    if (!by) return
    const buttons = Array.from(list.current?.querySelectorAll<HTMLElement>("button") ?? [])
    if (buttons.length === 0) return
    const at = buttons.indexOf(document.activeElement as HTMLElement)
    buttons[(at + by + buttons.length) % buttons.length].focus()
    e.preventDefault()
  }

  return (
    <Modal label="Move to workspace" onClose={closeWorkspacePicker} p="4" maxW="360px">
      <Text fontWeight="600" mb="2">
        Move {request.name} to
      </Text>
      <Box ref={list} display="grid" gap="1" onKeyDown={onKeyDown}>
        {request.items.map((item) => (
          <Button
            key={item.label}
            emphasis="ghost"
            tone="muted"
            aria-current={item.current ? "true" : undefined}
            onClick={() => {
              closeWorkspacePicker()
              if (!item.current) item.run()
            }}
          >
            {item.label}
            {item.current ? " (current)" : ""}
          </Button>
        ))}
      </Box>
    </Modal>
  )
}
