import { chakra, Kbd, Text } from "@chakra-ui/react"
import { useEffect, useMemo, useState, type FormEvent, type KeyboardEvent } from "react"
import { useConnectionContext, useConnectionState } from "../connection"
import { stateToken } from "../design/open"
import type { SessionInfo, SessionModelsResponse } from "../generated/server"
import { Button } from "../ui/button/Button"
import { Modal } from "../ui/modal/Modal"
import { useFocusFirstField } from "../ui/modal/useFocusFirstField"
import { refusalText, useWrite } from "../useWrite"
import { buildGroups, effortFor, filterGroups } from "./modelPickerList"
import {
  closeModelPicker,
  fetchSessionModels,
  postSwitchSession,
  useModelPickerRequest,
  type FetchModels,
  type SwitchModel,
} from "./modelPickerOpen"

/** How long to wait before asking again while the server still reads the list in the background. */
const REFRESH_MS = 1500

const KEY_HINTS = [
  ["↑↓", "model"],
  ["←→", "effort"],
  ["↵", "switch"],
  ["esc", "close"],
] as const

/** Renders the model picker for the open request. */
export function ModelPickerHost() {
  const request = useModelPickerRequest()
  if (!request) return null
  // The id keys the dialog, so a new request starts from that chat's own model and effort.
  return (
    <ModelPicker
      key={request.session.id}
      opened={request.session}
      fetchModels={request.fetchModels}
      switchModel={request.switchModel}
    />
  )
}

/** Why the picker cannot switch right now, or null when it can. */
function blockedNote(s: SessionInfo): string | null {
  if (s.status.state === "exited" || s.stopped) return "Resume the session to switch."
  if (s.status.state !== "idle") return "The agent is working. Wait until it is idle to switch."
  return null
}

function useModels(sessionId: string, fetchModels: FetchModels) {
  const [models, setModels] = useState<SessionModelsResponse | null>(null)
  const [refreshNo, setRefreshNo] = useState(0)
  useEffect(() => {
    let live = true
    let timer: ReturnType<typeof setTimeout> | undefined
    void fetchModels(sessionId).then((res) => {
      if (!live) return
      // A failed read keeps the answer already shown.
      if (res) setModels(res)
      if (res?.refreshing) timer = setTimeout(() => setRefreshNo((n) => n + 1), REFRESH_MS)
    })
    return () => {
      live = false
      clearTimeout(timer)
    }
  }, [sessionId, fetchModels, refreshNo])
  return models
}

function ModelPicker({
  opened,
  fetchModels = fetchSessionModels,
  switchModel = postSwitchSession,
}: {
  opened: SessionInfo
  fetchModels?: FetchModels
  switchModel?: SwitchModel
}) {
  const { snapshot } = useConnectionState(useConnectionContext())
  const session = snapshot?.sessions?.find((s) => s.id === opened.id) ?? opened
  const models = useModels(session.id, fetchModels)
  const [query, setQuery] = useState("")
  const [choice, setChoice] = useState<string>()
  const [wantedEffort, setWantedEffort] = useState(session.effort)
  const { run, pending, refusal } = useWrite({ toastRefusal: false })
  const focusFirst = useFocusFirstField()

  const groups = useMemo(() => buildGroups(models ?? undefined, session), [models, session])
  const shown = useMemo(() => filterGroups(groups, query), [groups, query])
  const flat = shown.flatMap((g) => g.rows)
  const current = groups.flatMap((g) => g.rows).find((r) => r.current)
  // The choice survives a later list answer and a filter; it falls to the first match only when
  // the filter hides it.
  const selected = flat.find((r) => r.id === (choice ?? current?.id)) ?? flat[0]
  const effort = selected ? effortFor(selected, wantedEffort) : ""
  const note = blockedNote(session)
  const blocked = note !== null
  const changed =
    !!selected &&
    (selected.id !== current?.id ||
      (selected.efforts.length > 0 && effort !== (session.effort ?? "")))
  const canSwitch = changed && !blocked && !pending

  const submit = (e?: FormEvent) => {
    e?.preventDefault()
    if (!canSwitch || !selected) return
    void run(
      () =>
        switchModel({
          id: session.id,
          state: stateToken(session),
          model: selected.id,
          ...(effort ? { effort } : {}),
        }),
      { onDone: closeModelPicker },
    )
  }

  const move = (by: number) => {
    if (!selected || flat.length === 0) return
    setChoice(flat[(flat.indexOf(selected) + by + flat.length) % flat.length].id)
  }
  const moveEffort = (by: number) => {
    if (!selected) return
    const at = selected.efforts.indexOf(effort)
    const next = selected.efforts[at + by]
    if (next) setWantedEffort(next)
  }
  const onKeyDown = (e: KeyboardEvent) => {
    if (blocked) return
    const step: Record<string, () => void> = {
      ArrowUp: () => move(-1),
      ArrowDown: () => move(1),
      ArrowLeft: () => moveEffort(-1),
      ArrowRight: () => moveEffort(1),
    }
    const act = step[e.key]
    if (!act) return
    e.preventDefault()
    act()
  }

  return (
    <Modal
      label="Switch model"
      onClose={closeModelPicker}
      as="form"
      onSubmit={submit}
      onKeyDown={onKeyDown}
      bg="surface.raised"
      p="0"
      gap="0"
      w="full"
      maxW="440px"
      overflow="hidden"
    >
      <chakra.div
        display="flex"
        justifyContent="space-between"
        alignItems="baseline"
        px="3.5"
        py="3"
        borderBottomWidth="1px"
        borderColor="border"
      >
        <Text fontWeight="600">Switch model</Text>
        <Text fontSize="xs" color="text.dim">
          {session.harness === "codex" ? "Codex" : "Claude"}
          {blocked ? "" : " · this chat only"}
        </Text>
      </chakra.div>
      <chakra.input
        aria-label="Filter models"
        placeholder="Filter models…"
        autoComplete="off"
        spellCheck={false}
        ref={focusFirst}
        disabled={blocked}
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        fontSize="16px"
        color="text"
        bg="surface"
        borderWidth="1px"
        borderColor="blue.500"
        borderRadius="8px"
        h="40px"
        mx="3.5"
        mt="2.5"
        px="3"
        // The blue border is the focus mark, as in the mock; the global ring would double it.
        data-focus-ring="none"
        _placeholder={{ color: "text.faint" }}
        _disabled={{ borderColor: "border", opacity: 0.5 }}
      />
      {note && (
        <Text
          role="status"
          mx="3.5"
          mt="2.5"
          px="2.5"
          py="2"
          borderRadius="6px"
          fontSize="12.5px"
          color="tone.waiting.fg"
          bg="rgba(242, 184, 69, 0.12)"
        >
          {note}
        </Text>
      )}
      <chakra.div role="radiogroup" aria-label="Model" overflowY="auto" px="1.5" py="2">
        {shown.length === 0 && (
          <Text px="2.5" py="2" fontSize="sm" color="text.dim">
            No model matches.
          </Text>
        )}
        {shown.map((g) => (
          <chakra.div key={g.family ?? "other"} pb="1">
            <Text
              px="2.5"
              py="1"
              fontSize="11px"
              letterSpacing="0.08em"
              textTransform="uppercase"
              color="text.faint"
            >
              {g.family ?? "Other"}
            </Text>
            {g.rows.map((r) => {
              const on = r.id === selected?.id
              return (
                <chakra.button
                  key={r.id}
                  type="button"
                  role="radio"
                  aria-checked={on}
                  disabled={blocked}
                  onClick={() => setChoice(r.id)}
                  display="flex"
                  alignItems="center"
                  gap="2.5"
                  w="full"
                  textAlign="left"
                  px="2.5"
                  py="1.5"
                  borderRadius="6px"
                  fontSize="sm"
                  color={r.outdated ? "text.faint" : "text"}
                  bg={on ? "tone.running.bg" : "transparent"}
                  _disabled={{ opacity: 0.5, cursor: "not-allowed" }}
                >
                  <chakra.span
                    aria-hidden
                    w="14px"
                    h="14px"
                    flexShrink="0"
                    borderRadius="full"
                    borderWidth="2px"
                    borderColor={on ? "tone.running.fg" : "border"}
                    bg={on ? "tone.running.fg" : "transparent"}
                    boxShadow={on ? "inset 0 0 0 2px var(--chakra-colors-surface-raised)" : "none"}
                  />
                  {r.label}
                  <chakra.span ml="auto" display="flex" gap="2" fontSize="11px">
                    {r.outdated && (
                      <chakra.span color="tone.paused.fg">newer available</chakra.span>
                    )}
                    {r.current && (
                      <chakra.span
                        color="text.dim"
                        borderWidth="1px"
                        borderColor="border"
                        borderRadius="4px"
                        px="1.5"
                      >
                        current
                      </chakra.span>
                    )}
                  </chakra.span>
                </chakra.button>
              )
            })}
          </chakra.div>
        ))}
      </chakra.div>
      {selected && selected.efforts.length > 0 && (
        <chakra.div
          display="flex"
          alignItems="center"
          gap="2.5"
          flexWrap="wrap"
          px="3.5"
          py="2.5"
          borderTopWidth="1px"
          borderColor="border"
        >
          <Text as="span" id="picker-effort" fontSize="xs" color="text.dim" w="48px">
            Effort
          </Text>
          <chakra.div
            role="radiogroup"
            aria-labelledby="picker-effort"
            display="flex"
            borderWidth="1px"
            borderColor="border"
            borderRadius="8px"
            overflow="hidden"
            opacity={blocked ? 0.5 : 1}
          >
            {selected.efforts.map((level) => (
              <chakra.button
                key={level}
                type="button"
                role="radio"
                aria-checked={level === effort}
                disabled={blocked}
                onClick={() => setWantedEffort(level)}
                px="2.5"
                py="1"
                fontSize="xs"
                color={level === effort ? "text" : "text.dim"}
                fontWeight={level === effort ? "600" : "400"}
                bg={level === effort ? "tone.running.bg" : "transparent"}
              >
                {level}
              </chakra.button>
            ))}
          </chakra.div>
        </chakra.div>
      )}
      {refusal && (
        <Text
          role="alert"
          mx="3.5"
          mb="2.5"
          px="2.5"
          py="2"
          borderRadius="6px"
          fontSize="sm"
          color="tone.paused.fg"
          bg="tone.paused.bg"
        >
          {refusalText(refusal)}
        </Text>
      )}
      <chakra.div
        display="flex"
        alignItems="center"
        justifyContent="flex-end"
        gap="2.5"
        px="3.5"
        py="2.5"
        borderTopWidth="1px"
        borderColor="border"
      >
        {!blocked && (
          <chakra.span
            display="flex"
            flexWrap="wrap"
            columnGap="2.5"
            fontSize="xs"
            color="text.faint"
            mr="auto"
          >
            {KEY_HINTS.map(([key, label]) => (
              <span key={key}>
                <Kbd
                  bg="transparent"
                  color="text.muted"
                  borderWidth="1px"
                  borderColor="border"
                  borderRadius="sm"
                  px="1"
                  fontSize="2xs"
                >
                  {key}
                </Kbd>{" "}
                {label}
              </span>
            ))}
          </chakra.span>
        )}
        <Button type="button" emphasis="secondary" onClick={closeModelPicker}>
          {blocked ? "Close" : "Cancel"}
        </Button>
        <Button type="submit" tone="running" disabled={!canSwitch}>
          Switch
        </Button>
      </chakra.div>
    </Modal>
  )
}
