import { Box, chakra, Text } from "@chakra-ui/react"
import { useCallback, useEffect, useRef, useState } from "react"
import { useLatest } from "../hooks"
import { Button } from "../ui/button/Button"
import { withKeys } from "../uniqueKeys"
import { Modal } from "../ui/modal/Modal"
import { refusalText, useWrite } from "../useWrite"
import {
  OWNER_TEXT,
  postTakeOver,
  tabClientId,
  subscribeOwner,
  useInputOwner,
  type SubscribeOwner,
  type TakeOver,
} from "./inputOwner"
import { ATTACH_KEYS, postInput, type PostInput } from "./keys"
import { fetchScreen, type FetchScreen } from "./screen"
import { changedRows, sameScreen } from "./screenDiff"

const FADE_MS = 3000

// Label on the button, name the keys route takes.
const KEYS: [label: string, key: string][] = [
  ["Esc", "esc"],
  ["Enter", "enter"],
  ["Tab", "tab"],
  ["↑", "up"],
  ["↓", "down"],
  ["←", "left"],
  ["→", "right"],
  ["Ctrl+C", "ctrl+c"],
]

const keyLabel = (label: string) =>
  ({ "↑": "Up", "↓": "Down", "←": "Left", "→": "Right" })[label] ?? label

/**
 * A screen as plain text rows; the target is whatever the fetcher takes. It reads once on open and again on Refresh,
 * and after each key sent. `keys` adds the key bar and Type field, only for a session a person may type into.
 */
export function ScreenModal({
  target,
  fetcher = fetchScreen,
  keys = false,
  poster = postInput,
  subscribe = subscribeOwner,
  takeOver: postTakeOverFn = postTakeOver,
  blockedHintMs,
  onClose,
}: {
  target: string
  fetcher?: FetchScreen
  keys?: boolean
  poster?: PostInput
  subscribe?: SubscribeOwner
  takeOver?: TakeOver
  blockedHintMs?: number
  onClose: () => void
}) {
  const input = useInputOwner({
    target,
    enabled: keys,
    subscribe,
    takeOver: postTakeOverFn,
    hintMs: blockedHintMs,
  })
  const [rows, setRows] = useState<string[]>()
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(false)
  const [text, setText] = useState("")
  const { run, pending, refusal, error: sendError } = useWrite({ toastRefusal: false })
  // The rows a key is compared against, kept in a ref so a Refresh between keys is the baseline too.
  const rowsRef = useRef<string[] | undefined>(undefined)
  const fadeRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const [marked, setMarked] = useState<number[]>([])
  const [unchanged, setUnchanged] = useState(false)
  useEffect(() => () => clearTimeout(fadeRef.current), [])

  const latestFetcher = useLatest(fetcher)
  // `diff` marks what a key changed; `show` is declared below but only called after render.
  const load = useCallback(
    async (diff = false) => {
      setLoading(true)
      setError("")
      try {
        const res = await latestFetcher.current(target)
        if (res.kind === "ok") {
          if (diff) showRef.current(res.detail.rows)
          else {
            rowsRef.current = res.detail.rows
            setRows(res.detail.rows)
          }
        } else setError("This session has no screen.")
      } catch {
        setError("Could not load the screen. Try Refresh.")
      } finally {
        setLoading(false)
      }
    },
    [target],
  )
  // Only a different target reloads (`load` changes with it); Refresh is the other way in.
  useEffect(() => {
    void load()
  }, [load])

  const show = (next: string[]) => {
    const prev = rowsRef.current
    rowsRef.current = next
    setRows(next)
    setMarked(prev ? changedRows(prev, next) : [])
    setUnchanged(!!prev && sameScreen(prev, next))
    clearTimeout(fadeRef.current)
    fadeRef.current = setTimeout(() => {
      setMarked([])
      setUnchanged(false)
    }, FADE_MS)
  }
  const showRef = useLatest(show)

  // Through the tab's own attach, so the input owner rules apply to these keys.
  const send = (body: { key?: string; text?: string }, after?: () => void) =>
    run(
      () =>
        poster({
          id: target,
          client: tabClientId(),
          ...(body.key ? { key: ATTACH_KEYS[body.key] } : { paste: body.text }),
        }),
      {
        onDone: (res) => {
          after?.()
          if (res.rows) show(res.rows)
          else void load(true)
        },
      },
    )

  const problem = refusal ? refusalText(refusal) : sendError

  return (
    <Modal
      label="Screen"
      onClose={onClose}
      w={{ md: "min(1200px, 92vw)" }}
      px={{ md: "5" }}
      py={{ md: "4.5" }}
      gap={{ md: "3" }}
    >
      <Box display="flex" gap="2" alignItems="center" justifyContent="space-between">
        <Text
          as="h3"
          m="0"
          fontFamily="condensed"
          fontSize="22px"
          textTransform="uppercase"
          letterSpacing="0.02em"
        >
          Screen
        </Text>
        <Box display="flex" gap="2">
          <Button emphasis="secondary" disabled={loading} onClick={() => void load()}>
            Refresh
          </Button>
          <Button emphasis="ghost" tone="muted" onClick={onClose}>
            Close
          </Button>
        </Box>
      </Box>
      {error && <Text role="alert">{error}</Text>}
      {rows && (
        <Box
          as="pre"
          aria-label="Screen rows"
          fontSize="xs"
          overflow="auto"
          maxH={{ md: "70vh" }}
          m="0"
          p="3"
          borderWidth="1px"
          borderRadius="8px"
          bg="surface.raised"
        >
          {withKeys(rows, (row) => row).map(({ item: row, index: line, key }) => (
            <Box
              key={key}
              as="span"
              display="block"
              color={marked.includes(line) ? "tone.waiting.fg" : undefined}
              fontWeight={marked.includes(line) ? "700" : undefined}
              transition="color 0.6s, font-weight 0.6s"
            >
              {row || " "}
            </Box>
          ))}
        </Box>
      )}
      <Text role="status" fontSize="xs" color="text.dim" m="0">
        {unchanged ? "Screen unchanged" : ""}
      </Text>
      {keys && (
        <>
          {input.owner && (
            <Box
              display="flex"
              gap="2"
              alignItems="center"
              flexWrap="wrap"
              aria-label="Input owner"
              role="group"
            >
              <Text fontSize="xs" color="text.dim" m="0" role="status">
                {OWNER_TEXT[input.owner]}
              </Text>
              {input.owner !== "self" && (
                <Button emphasis="secondary" onClick={() => void input.takeOver()}>
                  Take over
                </Button>
              )}
              {input.blocked && (
                <Text fontSize="xs" color="tone.waiting.fg" m="0" role="status">
                  Your key was not sent: another owner has the input. Take over to type.
                </Text>
              )}
            </Box>
          )}
          <Box display="flex" flexDirection="column" gap="1.5" aria-label="Keys" role="group">
            <Text
              fontSize="xs"
              color="text.dim"
              textTransform="uppercase"
              letterSpacing="0.06em"
              fontWeight="600"
            >
              Keys
            </Text>
            <Box display="flex" flexWrap="wrap" gap="1.5">
              {KEYS.map(([label, key]) => (
                <Button
                  key={key}
                  emphasis="secondary"
                  aria-label={keyLabel(label)}
                  disabled={pending}
                  h={{ base: "44px", md: "40px" }}
                  minW="44px"
                  flex={{ base: "1 0 20%", md: "0 0 auto" }}
                  fontFamily="mono"
                  fontWeight="400"
                  fontSize="13px"
                  onClick={() => send({ key })}
                >
                  {label}
                </Button>
              ))}
            </Box>
          </Box>
          <Box
            as="form"
            display="flex"
            flexDirection="column"
            gap="1.5"
            onSubmit={(e: React.FormEvent) => {
              e.preventDefault()
              if (text) void send({ text }, () => setText(""))
            }}
          >
            <Text
              fontSize="xs"
              color="text.dim"
              textTransform="uppercase"
              letterSpacing="0.06em"
              fontWeight="600"
            >
              Type text
            </Text>
            <Box display="flex" gap="2">
              <chakra.input
                aria-label="Text to type"
                placeholder="Text is typed as is, without Enter"
                value={text}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => setText(e.target.value)}
                flex="1"
                minW="0"
                h="44px"
                px="3"
                borderWidth="1px"
                borderRadius="8px"
                bg="surface.raised"
              />
              <Button
                type="submit"
                emphasis="secondary"
                disabled={pending}
                h={{ base: "44px", md: "40px" }}
                fontWeight="400"
                fontSize="13px"
              >
                Type
              </Button>
            </Box>
          </Box>
          {problem && <Text role="alert">{problem}</Text>}
        </>
      )}
    </Modal>
  )
}
