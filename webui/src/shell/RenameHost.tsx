import { chakra, Text } from "@chakra-ui/react"
import { useCallback, useState, type FormEvent, type ReactNode } from "react"
import { slugProblem } from "../design/draftSlug"
import { saveDraftFields, type SaveDraftFields } from "../design/drafts"
import { postRenameSession, type RenameSession } from "../design/open"
import type { SessionInfo } from "../generated/server"
import { Button } from "../ui/button/Button"
import { BottomSheet } from "../ui/modal/BottomSheet"
import { useFocusFirstField } from "../ui/modal/useFocusFirstField"
import { TextBox } from "../ui/text-box/TextBox"
import { useWide } from "../useWide"
import { refusalText, useWrite } from "../useWrite"
import { realChatDrafts, type ChatDraftsClient } from "../design/chatDrafts"
import {
  closeRename,
  useRenameRequest,
  type ChatDraftIdentity,
  type DraftIdentity,
} from "./renameOpen"

/** The server checks the same rules; the page must not send a name it would refuse. */
const nameOk = (text: string) => text.trim() !== "" && !/[\r\n]/.test(text)

/** Renders the rename popup for the open request. */
export function RenameHost() {
  const request = useRenameRequest()
  if (!request) return null
  // The id keys the form, so a new request starts from that chat's name or draft's slug.
  if ("draft" in request) {
    return (
      <DraftRenameForm
        key={request.draft.id}
        draft={request.draft}
        saveFields={request.saveFields}
      />
    )
  }
  if ("chatDraft" in request) {
    return (
      <ChatDraftRenameForm
        key={request.chatDraft.id}
        draft={request.chatDraft}
        drafts={request.drafts}
      />
    )
  }
  return <RenameForm key={request.session.id} session={request.session} rename={request.rename} />
}

function useFirstField() {
  const wide = useWide()
  const focusFirst = useFocusFirstField()
  // Stable, so a re-render does not run the focus or the selection again.
  return useCallback(
    (el: HTMLInputElement | null) => {
      focusFirst(el)
      if (el && wide) el.select()
    },
    [focusFirst, wide],
  )
}

function RenameSheet({
  label,
  onSubmit,
  disabled,
  children,
}: {
  label: string
  onSubmit: (e: FormEvent) => void
  disabled: boolean
  children: ReactNode
}) {
  return (
    <BottomSheet
      label={label}
      onClose={closeRename}
      as="form"
      onSubmit={onSubmit}
      desktop={{ maxW: "sm" }}
      bg="surface.raised"
      p="4"
      gap="3"
    >
      {children}
      <chakra.div display="flex" gap="2" justifyContent="flex-end">
        <Button type="button" emphasis="secondary" onClick={closeRename}>
          Cancel
        </Button>
        <Button type="submit" tone="running" disabled={disabled}>
          Rename
        </Button>
      </chakra.div>
    </BottomSheet>
  )
}

function Alert({ children }: { children: ReactNode }) {
  return (
    <Text role="alert" fontSize="sm" color="tone.paused.fg">
      {children}
    </Text>
  )
}

function DraftRenameForm({
  draft,
  saveFields = saveDraftFields,
}: {
  draft: DraftIdentity
  saveFields?: SaveDraftFields
}) {
  const [text, setText] = useState(draft.slug)
  const { run, pending, refusal } = useWrite({ toastRefusal: false })
  // The page's slug field uses the same check; an empty slug is allowed and clears it.
  const problem = slugProblem(text)
  const [tried, setTried] = useState(false)
  const fieldRef = useFirstField()
  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (pending) return
    if (problem) return setTried(true)
    // The slug write, never the text save, so the revision does not move.
    void run(
      async () => {
        const res = await saveFields({ id: draft.id, slug: text })
        if (res.kind === "gone") return { refused: true, message: "This draft no longer exists." }
        if (res.kind === "conflict")
          return { refused: true, message: "The draft changed. Try again." }
        return {}
      },
      { onDone: closeRename },
    )
  }
  return (
    <RenameSheet label="Rename draft" onSubmit={submit} disabled={pending}>
      <TextBox
        kind="line"
        verbatim
        label="Slug"
        hideLabel
        name="draft-slug"
        ref={fieldRef}
        value={text}
        onChange={(e) => {
          setText(e.currentTarget.value)
          setTried(false)
        }}
      />
      {tried && problem ? (
        <Alert>{problem}</Alert>
      ) : (
        refusal && <Alert>{refusalText(refusal)}</Alert>
      )}
    </RenameSheet>
  )
}

function ChatDraftRenameForm({
  draft,
  drafts = realChatDrafts,
}: {
  draft: ChatDraftIdentity
  drafts?: ChatDraftsClient
}) {
  const [text, setText] = useState(draft.name)
  const { run, pending, refusal } = useWrite({ toastRefusal: false })
  const valid = nameOk(text)
  const fieldRef = useFirstField()
  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!valid || pending) return
    // The save needs the brief and revision, so read the draft first.
    void run(
      async () => {
        const cur = await drafts.read(draft.id)
        if (cur.kind !== "ok") return { refused: true, message: "This draft no longer exists." }
        const res = await drafts.save({
          id: draft.id,
          brief: cur.draft.brief,
          revision: cur.draft.revision,
          name: text.trim(),
        })
        if (res.kind === "gone") return { refused: true, message: "This draft no longer exists." }
        if (res.kind === "conflict")
          return { refused: true, message: "The draft changed. Try again." }
        return {}
      },
      { onDone: closeRename },
    )
  }
  return (
    <RenameSheet label="Rename draft" onSubmit={submit} disabled={!valid || pending}>
      <TextBox
        kind="line"
        label="Title"
        hideLabel
        placeholder="Title"
        name="draft-title"
        ref={fieldRef}
        value={text}
        onChange={(e) => setText(e.currentTarget.value)}
      />
      {refusal && <Alert>{refusalText(refusal)}</Alert>}
    </RenameSheet>
  )
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
  const valid = nameOk(text)
  const fieldRef = useFirstField()
  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!valid || pending) return
    run(() => rename({ id: session.id, name: text.trim() }), { onDone: closeRename })
  }
  return (
    <RenameSheet label="Rename chat" onSubmit={submit} disabled={!valid || pending}>
      <TextBox
        kind="line"
        label="Title"
        hideLabel
        placeholder="Title"
        name="chat-title"
        ref={fieldRef}
        value={text}
        onChange={(e) => setText(e.currentTarget.value)}
      />
      {session.auto_name && (
        <Text fontSize="sm" color="text.dim">
          Automatic name from the terminal title
        </Text>
      )}
      {refusal && <Alert>{refusalText(refusal)}</Alert>}
    </RenameSheet>
  )
}
