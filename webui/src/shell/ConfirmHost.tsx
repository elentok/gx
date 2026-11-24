import { Box, Popover, Portal, Spinner, Text, Textarea } from "@chakra-ui/react"
import { useEffect, useRef, useState, type RefObject } from "react"
import {
  closeConfirm,
  useConfirmRequest,
  type ConfirmBody,
  type ConfirmRequest,
} from "../actions/confirmRequest"
import { useLatest } from "../hooks"
import { isMac } from "../platform"
import { MENU_TRIGGER } from "../select/selectKeys"
import { Button } from "../ui/button/Button"
import { Modal } from "../ui/modal/Modal"
import { showToast } from "../ui/toaster"
import { refusalText } from "../useWrite"
import { focusMainHeadingAfterLanding } from "./focusMainHeading"

/** The one primary color for every confirm that is not a Danger-group action. */
const PRIMARY_TONE = "running"
const DANGER_TONE = "paused"

/** The mock's button: 29px, radius 6px, regular weight. */
const BUTTON_LOOK = {
  size: "sm",
  h: "29px",
  borderRadius: "6px",
  fontWeight: "400",
  _focus: { outline: "2px solid", outlineColor: "tone.running.fg", outlineOffset: "1px" },
} as const

const ids = new WeakMap<ConfirmRequest, number>()
let nextId = 0
const idOf = (r: ConfirmRequest) => {
  if (!ids.has(r)) ids.set(r, nextId++)
  return ids.get(r)
}

/** The shared confirm: a popover on the clicked button, or a modal when a chord or palette started the action. */
export function ConfirmHost() {
  const request = useConfirmRequest()
  return request ? <Confirm key={idOf(request)} request={request} /> : null
}

const isEditable = (t: EventTarget | null) =>
  t instanceof HTMLElement && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName))

function Confirm({ request }: { request: ConfirmRequest }) {
  const [body, setBody] = useState<ConfirmBody>(request.body)
  const [preparing, setPreparing] = useState(!!request.prepare)
  const [text, setText] = useState("")
  const done = useRef(false)
  const cancelRef = useRef<HTMLButtonElement>(null)
  const { surface } = request

  const close = (run: boolean) => {
    if (done.current) return
    done.current = true
    const from = window.location.pathname
    closeConfirm()
    if (run) request.onConfirm(request.body.input ? text : undefined)
    if (run && request.removesOpenPage) return focusMainHeadingAfterLanding(from)
    queueMicrotask(() => restoreFocus(request))
  }

  useEffect(() => {
    if (!request.prepare) return
    let live = true
    request.prepare().then(
      (res) => {
        if (!live) return
        if ("refused" in res) {
          showToast({ type: "error", title: refusalText(res) })
          return close(false)
        }
        setBody(res)
        setPreparing(false)
      },
      () => {
        if (!live) return
        showToast({ type: "error", title: "Could not reach gx. Try again." })
        close(false)
      },
    )
    return () => {
      live = false
    }
    // The request is fixed for this mount (the host keys on it).
  }, [])

  // The modal parks focus on its panel as it mounts; this runs after it, so Cancel ends up focused.
  useEffect(() => cancelRef.current?.focus(), [])

  const needsText = !!body.input?.required && text.trim() === ""
  const canConfirm = !preparing && !needsText
  const latest = useLatest({ close, canConfirm })

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const { close, canConfirm } = latest.current
      // A modal closes itself on Escape, top layer only; the popover has no layer of its own.
      if (e.key === "Escape" && surface.mode === "anchor") return close(false)
      const mod = isMac() ? e.metaKey : e.ctrlKey
      if (e.key === "Enter" && mod) {
        e.preventDefault()
        if (canConfirm) close(true)
        return
      }
      if (e.metaKey || e.ctrlKey || e.altKey || isEditable(e.target)) return
      if (e.key === "y") {
        e.preventDefault()
        if (canConfirm) close(true)
      } else if (e.key === "n") {
        e.preventDefault()
        close(false)
      }
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [])

  const card = (
    <ConfirmCard
      request={request}
      body={body}
      preparing={preparing}
      text={text}
      onText={setText}
      canConfirm={canConfirm}
      cancelRef={cancelRef}
      onCancel={() => close(false)}
      onConfirm={() => close(true)}
    />
  )

  if (surface.mode === "modal") {
    return (
      <Modal
        label={request.title}
        onClose={() => close(false)}
        w="min(374px, 100%)"
        p="0"
        borderRadius="12px"
        data-part="confirm-modal"
      >
        {card}
      </Modal>
    )
  }
  const { anchor } = surface
  return (
    <Popover.Root
      open
      onOpenChange={(e) => !e.open && close(false)}
      initialFocusEl={() => cancelRef.current}
      finalFocusEl={() => (anchor.isConnected ? anchor : null)}
      // The anchor's own click would otherwise count as outside and reopen what it closed.
      persistentElements={[() => anchor]}
      positioning={{
        placement: "bottom-end",
        gutter: 10,
        getAnchorRect: () => anchor.getBoundingClientRect(),
      }}
      unmountOnExit
    >
      <Portal>
        <Popover.Positioner>
          <Popover.Content
            role="alertdialog"
            aria-label={request.title}
            data-part="confirm-popover"
            bg="surface.raised"
            borderWidth="1px"
            borderColor="border"
            borderRadius="10px"
            boxShadow="lg"
            color="text"
            w={`min(${request.body.input ? 346 : 326}px, calc(100vw - 32px))`}
            css={{ "--popover-bg": "colors.surface.raised" }}
          >
            <Popover.Arrow>
              <Popover.ArrowTip borderColor="border" />
            </Popover.Arrow>
            {card}
          </Popover.Content>
        </Popover.Positioner>
      </Portal>
    </Popover.Root>
  )
}

function restoreFocus(request: ConfirmRequest) {
  const { surface } = request
  if (surface.mode === "anchor" && surface.anchor.isConnected) return surface.anchor.focus()
  const fallback = request.focusAfter?.() ?? document.querySelector<HTMLElement>(MENU_TRIGGER)
  fallback?.focus()
}

function Key({ children }: { children: string }) {
  return (
    <Text as="span" fontWeight="400" opacity={0.7}>
      {children}
    </Text>
  )
}

function ConfirmCard({
  request,
  body,
  preparing,
  text,
  onText,
  canConfirm,
  cancelRef,
  onCancel,
  onConfirm,
}: {
  request: ConfirmRequest
  body: ConfirmBody
  preparing: boolean
  text: string
  onText: (t: string) => void
  canConfirm: boolean
  cancelRef: RefObject<HTMLButtonElement | null>
  onCancel: () => void
  onConfirm: () => void
}) {
  return (
    <Box
      p={request.surface.mode === "modal" ? "4" : "3"}
      display="flex"
      flexDirection="column"
      gap="10px"
      lineHeight="normal"
    >
      <Box display="flex" flexDirection="column" gap="1">
        <Text fontWeight="700" fontSize="14px" data-part="title">
          {request.title}
        </Text>
        <Box data-part="body" fontSize="13px" color="text.dim" display="flex" flexDirection="column" gap="2">
          {preparing ? (
            <Box role="status" aria-label="Checking" display="flex" alignItems="center" gap="2">
              <Spinner size="sm" color="tone.running.fg" />
              <Text as="span">Checking…</Text>
            </Box>
          ) : (
            <>
              {body.lines.map((line) => (
                <Text key={line}>{line}</Text>
              ))}
              {body.warnings?.map((w) => (
                <Text key={w} color="tone.paused.fg">
                  {w}
                </Text>
              ))}
              {body.rows && (
                <Box as="ul" pl="18px" m="0" listStyleType="disc">
                  {body.rows.map((row) => (
                    <li key={row}>{row}</li>
                  ))}
                </Box>
              )}
            </>
          )}
          {body.input && !preparing && (
            <Textarea
              data-part="reason-field"
              aria-label={body.input.label}
              placeholder={body.input.required ? `${body.input.label} (required)` : body.input.label}
              value={text}
              onChange={(e) => onText(e.target.value)}
              rows={2}
              size="sm"
              borderRadius="6px"
              p="6px"
              bg="surface.raised"
              borderColor="border"
              minH="0"
              h="46px"
            />
          )}
        </Box>
      </Box>
      <Box display="flex" justifyContent="flex-end" gap="2">
        <Button
          ref={cancelRef}
          data-part="cancel-button"
          {...BUTTON_LOOK}
          emphasis="secondary"
          autoFocus
          onClick={onCancel}
        >
          {request.dismissLabel} <Key>n</Key>
        </Button>
        <Button
          data-part="confirm-button"
          {...BUTTON_LOOK}
          tone={request.group === "danger" ? DANGER_TONE : PRIMARY_TONE}
          disabled={!canConfirm}
          onClick={onConfirm}
        >
          {request.confirmLabel} <Key>y</Key>
        </Button>
      </Box>
    </Box>
  )
}
