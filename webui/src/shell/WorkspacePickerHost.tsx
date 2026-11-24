import { Box, Text } from "@chakra-ui/react"
import { Button } from "../ui/button/Button"
import { Modal } from "../ui/modal/Modal"
import { closeWorkspacePicker, useWorkspacePickerRequest } from "./workspacePickerOpen"

/** The Move to workspace chord's picker: every workspace, the current one marked; a pick posts the move. */
export function WorkspacePickerHost() {
  const request = useWorkspacePickerRequest()
  if (!request) return null
  return (
    <Modal label="Move to workspace" onClose={closeWorkspacePicker} p="4" maxW="360px">
      <Text fontWeight="600" mb="2">
        Move {request.name} to
      </Text>
      <Box display="grid" gap="1">
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
