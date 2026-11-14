import { Box, Text } from "@chakra-ui/react"
import { forgetLast } from "../agents/lastAgent"
import { isExited, postRemoveSession, removeSession } from "../agents/sessionWrites"
import { deleteDraft, type DeleteDraft } from "../design/drafts"
import { Button } from "../ui/button/Button"
import { Modal } from "../ui/modal/Modal"
import { useWrite } from "../useWrite"
import {
  closeRemoveConfirm,
  useRemoveConfirmRequest,
  type DraftRemoveTarget,
} from "./removeConfirmOpen"

type Request = NonNullable<ReturnType<typeof useRemoveConfirmRequest>>

/** Renders the Remove confirmation for the open request. */
export function RemoveConfirmHost() {
  const request = useRemoveConfirmRequest()
  if (!request) return null
  if ("draft" in request) return <DraftRemove key={request.draft.id} {...request} />
  return <SessionRemove {...request} />
}

function SessionRemove({ session, onRemoved }: Extract<Request, { session: unknown }>) {
  const { run, pending } = useWrite()
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

function DraftRemove({
  draft,
  onRemoved,
  remove = deleteDraft,
}: {
  draft: DraftRemoveTarget
  onRemoved: () => void
  remove?: DeleteDraft
}) {
  const { run, pending } = useWrite()
  const removeNow = () => {
    closeRemoveConfirm()
    void run(
      async () => {
        await remove(draft.id)
        return {}
      },
      { onDone: onRemoved },
    )
  }
  return (
    <Modal label={`Remove ${draft.label}`} onClose={closeRemoveConfirm} maxW="sm">
      <Text>{`Remove ${draft.label}? Its prompt and images are deleted.`}</Text>
      <Box display="flex" gap="2" justifyContent="flex-end">
        <Button emphasis="secondary" onClick={closeRemoveConfirm} autoFocus>
          Keep
        </Button>
        <Button tone="paused" disabled={pending} onClick={removeNow}>
          Remove
        </Button>
      </Box>
    </Modal>
  )
}
