import { Box, Text } from "@chakra-ui/react"
import { forgetLast } from "../agents/lastAgent"
import { isExited, postRemoveSession, removeSession } from "../agents/sessionWrites"
import { Button } from "../ui/button/Button"
import { Modal } from "../ui/modal/Modal"
import { useWrite } from "../useWrite"
import { closeRemoveConfirm, useRemoveConfirmRequest } from "./removeConfirmOpen"

/** Renders the Remove confirmation for the open request. */
export function RemoveConfirmHost() {
  const request = useRemoveConfirmRequest()
  const { run, pending } = useWrite()
  if (!request) return null
  const { session, onRemoved } = request
  const name = session.name || session.id
  return (
    <Modal label={`Remove ${name}`} onClose={closeRemoveConfirm} maxW="sm">
      <Text>
        {isExited(session)
          ? `Remove ${name}? Its session is deleted.`
          : `Stop and remove ${name}? Its session is deleted.`}
      </Text>
      <Box display="flex" gap="2" justifyContent="flex-end">
        <Button emphasis="secondary" onClick={closeRemoveConfirm} autoFocus>
          Keep
        </Button>
        <Button
          tone="paused"
          disabled={pending}
          onClick={() => {
            closeRemoveConfirm()
            run(() => removeSession(session, { remove: postRemoveSession }), {
              onDone: () => {
                forgetLast(session.id)
                onRemoved()
              },
            })
          }}
        >
          Remove
        </Button>
      </Box>
    </Modal>
  )
}
