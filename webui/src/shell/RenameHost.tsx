import { chakra, Text } from "@chakra-ui/react"
import { useCallback, useState, type FormEvent } from "react"
import { postRenameSession, type RenameSession } from "../design/open"
import type { SessionInfo } from "../generated/server"
import { Button } from "../ui/button/Button"
import { BottomSheet } from "../ui/modal/BottomSheet"
import { useFocusFirstField } from "../ui/modal/useFocusFirstField"
import { useWide } from "../useWide"
import { refusalText, useWrite } from "../useWrite"
import { closeRename, useRenameRequest } from "./renameOpen"

/** The server checks the same rules; the page must not send a name it would refuse. */
const nameOk = (text: string) => text.trim() !== "" && !/[\r\n]/.test(text)

/** Renders the rename popup for the open request. */
export function RenameHost() {
  const request = useRenameRequest()
  if (!request) return null
  // The session id keys the form, so a new request starts from that chat's name.
  return <RenameForm key={request.session.id} session={request.session} rename={request.rename} />
}

function RenameForm({
  session,
  rename = postRenameSession,
}: {
  session: SessionInfo
  rename?: RenameSession
}) {
  const [text, setText] = useState(session.name ?? "")
  const { run, pending, refusal } = useWrite({ toastRefusal: false })
  const wide = useWide()
  const focusFirst = useFocusFirstField()
  // Stable, so a re-render does not run the focus or the selection again.
  const fieldRef = useCallback(
    (el: HTMLInputElement | null) => {
      focusFirst(el)
      if (el && wide) el.select()
    },
    [focusFirst, wide],
  )
  const valid = nameOk(text)
  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!valid || pending) return
    run(() => rename({ id: session.id, name: text.trim() }), { onDone: closeRename })
  }
  return (
    <BottomSheet
      label="Rename chat"
      onClose={closeRename}
      as="form"
      onSubmit={submit}
      desktop={{ maxW: "sm" }}
      bg="surface.raised"
      p="4"
      gap="3"
    >
      <chakra.input
        aria-label="Name"
        autoComplete="off"
        spellCheck={false}
        ref={fieldRef}
        fontSize="16px"
        color="text"
        bg="surface"
        borderRadius="8px"
        h="40px"
        px="3"
        value={text}
        onChange={(e) => setText(e.target.value)}
      />
      {session.auto_name && (
        <Text fontSize="sm" color="text.dim">
          Automatic name from the terminal title
        </Text>
      )}
      {refusal && (
        <Text role="alert" fontSize="sm" color="tone.paused.fg">
          {refusalText(refusal)}
        </Text>
      )}
      <chakra.div display="flex" gap="2" justifyContent="flex-end">
        <Button type="button" emphasis="secondary" onClick={closeRename}>
          Cancel
        </Button>
        <Button type="submit" tone="running" disabled={!valid || pending}>
          Rename
        </Button>
      </chakra.div>
    </BottomSheet>
  )
}
