import { Box, Button, Input, Text } from "@chakra-ui/react"
import { useEffect, useMemo, useState, type KeyboardEvent as ReactKeyboardEvent } from "react"
import { createPortal } from "react-dom"
import { RestKindBedtime, RestKindBreak } from "./generated/server"
import { isUnlockSentence, postRestUnlock, useRestState } from "./rest"

// Above the modal token (1400) and the toast layer, so no sheet or modal shows over it.
const REST_Z_INDEX = 2000

// The screen is always dark, whatever theme the person chose.
const PAGE = "#0a0d11"
const FG = "#e6edf3"
const DIM = "#8b98a5"
const ACCENT = "#4da3ff"

const MASK = "radial-gradient(circle at center, #000 52%, transparent 71%)"

// Everything else in the body goes inert, which covers the app and any modal portalled to the body.
function useInertPage(screen: HTMLElement | null) {
  useEffect(() => {
    if (!screen) return
    const inerted: Element[] = []
    for (const el of Array.from(document.body.children)) {
      if (el === screen || el.hasAttribute("inert")) continue
      el.setAttribute("inert", "")
      inerted.push(el)
    }
    return () => inerted.forEach((el) => el.removeAttribute("inert"))
  }, [screen])
}

// Capture phase on the document, so a shortcut handler underneath never sees the key. Text fields
// on the screen itself keep working.
function useKeyGuard(screen: HTMLElement | null) {
  useEffect(() => {
    if (!screen) return
    const guard = (e: KeyboardEvent) => {
      if (e.target instanceof Node && screen.contains(e.target)) return
      e.preventDefault()
      e.stopPropagation()
    }
    document.addEventListener("keydown", guard, true)
    return () => document.removeEventListener("keydown", guard, true)
  }, [screen])
}

function BedtimeView() {
  const { period, rule } = useRestState()
  if (!period) return null
  return (
    <>
      <Message message={period.message} subMessage={period.sub_message} />
      <TimeLabel>Back at</TimeLabel>
      <Text
        data-part="time"
        m="6px 0"
        fontFamily="ui-monospace, monospace"
        fontWeight="700"
        fontSize="40px"
        lineHeight="1"
        color={ACCENT}
      >
        {rule?.bedtime_end}
      </Text>
      {rule && (
        <Text data-part="rule-note" color={DIM} fontSize="12px" m="0">
          Bedtime {rule.bedtime_start} – {rule.bedtime_end}
        </Text>
      )}
    </>
  )
}

// Whole seconds left until `end`, or undefined while off. The offset between the server clock and
// the browser clock is taken once per snapshot, so a wrong browser clock does not shift the count.
function useCountdown(end: string | undefined, serverNow: string | undefined, on: boolean) {
  const [, tick] = useState(0)
  useEffect(() => {
    if (!on) return
    const timer = setInterval(() => tick((n) => n + 1), 1000)
    return () => clearInterval(timer)
  }, [on])
  const offset = useMemo(
    () => (serverNow ? Date.parse(serverNow) - Date.now() : 0),
    // The snapshot's `now` changes with every snapshot, which is the point of re-taking it.
    [serverNow],
  )
  if (!on || !end) return undefined
  return Math.max(0, Math.ceil((Date.parse(end) - (Date.now() + offset)) / 1000))
}

function formatCountdown(seconds: number) {
  const mm = String(Math.floor(seconds / 60)).padStart(2, "0")
  const ss = String(seconds % 60).padStart(2, "0")
  return `${mm}:${ss}`
}

function Message({ message, subMessage }: { message: string; subMessage?: string }) {
  return (
    <>
      <Text data-part="message" fontSize="22px" lineHeight="1.3" fontWeight="600" m="0 0 10px">
        {message}
      </Text>
      {subMessage && (
        <Text data-part="submessage" color={DIM} m="0">
          {subMessage}
        </Text>
      )}
    </>
  )
}

function TimeLabel({ children }: { children: string }) {
  return (
    <Text
      data-part="time-label"
      mt="16px"
      fontSize="12px"
      textTransform="uppercase"
      letterSpacing="1.5px"
      color={DIM}
    >
      {children}
    </Text>
  )
}

function BreakView({ seconds }: { seconds: number }) {
  const { period, rule } = useRestState()
  if (!period) return null
  return (
    <>
      <Message message={period.message} subMessage={period.sub_message} />
      <TimeLabel>Break ends in</TimeLabel>
      <Text
        data-part="time"
        m="6px 0"
        fontFamily="ui-monospace, monospace"
        fontWeight="700"
        fontSize="56px"
        letterSpacing="2px"
        lineHeight="1"
      >
        {formatCountdown(seconds)}
      </Text>
      {rule && (
        <Text data-part="rule-note" color={DIM} fontSize="12px" m="0">
          Break · {rule.break_minutes} minutes after {rule.work_minutes} minutes of work
        </Text>
      )}
    </>
  )
}

function UnlockSection({ buttonLabel }: { buttonLabel: string }) {
  const [open, setOpen] = useState(false)
  const [text, setText] = useState("")
  const [refusal, setRefusal] = useState<string>()
  const [busy, setBusy] = useState(false)
  const enabled = isUnlockSentence(text)

  const submit = async () => {
    setBusy(true)
    try {
      setRefusal(await postRestUnlock(text))
    } catch (e) {
      setRefusal(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Box data-part="unlock" mt="22px">
      <Text
        as="a"
        data-part="unlock-link"
        role="button"
        tabIndex={0}
        color={DIM}
        fontSize="13px"
        textDecoration="underline"
        cursor="pointer"
        onClick={() => setOpen(true)}
        onKeyDown={(e: ReactKeyboardEvent) => e.key === "Enter" && setOpen(true)}
      >
        Unlock anyway
      </Text>
      {open && (
        <Box data-part="unlock-box" mt="10px" maxW="380px" textAlign="center">
          <Text color={DIM} fontSize="13px" m="0 0 6px">
            Type exactly: <i>I'm [anything] and I'm addicted to AI</i>
          </Text>
          <Input
            aria-label="Unlock sentence"
            value={text}
            onChange={(e) => setText(e.target.value)}
            autoFocus
            bg="#0d1117"
            borderColor="#30363d"
            borderRadius="6px"
            color={FG}
            h="auto"
            p="8px 10px"
            fontSize="inherit"
            textAlign="left"
          />
          {refusal && (
            <Text role="alert" color="#ff7b72" fontSize="12px" m="6px 0 0">
              {refusal}
            </Text>
          )}
          <Button
            mt="8px"
            onClick={submit}
            disabled={!enabled || busy}
            bg={enabled ? "#b4472f" : "#222a33"}
            color={enabled ? "#fff" : DIM}
            border="1px solid"
            borderColor={enabled ? "#b4472f" : "#30363d"}
            borderRadius="6px"
            h="auto"
            p="7px 12px"
            fontWeight="inherit"
            fontSize="inherit"
          >
            {buttonLabel}
          </Button>
        </Box>
      )}
    </Box>
  )
}

export function RestScreen() {
  const { period, now, rule } = useRestState()
  const [screen, setScreen] = useState<HTMLElement | null>(null)
  const isBreak = period?.kind === RestKindBreak
  const seconds = useCountdown(period?.end, now, isBreak)
  useInertPage(screen)
  useKeyGuard(screen)
  if (!period || (isBreak && !seconds)) return null
  return createPortal(
    <Box
      ref={setScreen}
      role="dialog"
      aria-modal="true"
      aria-label="Rest"
      position="fixed"
      inset="0"
      zIndex={REST_Z_INDEX}
      display="flex"
      alignItems="center"
      justifyContent="center"
      p="24px"
      bg={PAGE}
      color={FG}
      overflowY="auto"
    >
      <Box display="flex" flexDirection="column" alignItems="center" textAlign="center">
        <Box
          data-part="mascot-image"
          w={{ base: "260px", md: "300px" }}
          h={{ base: "260px", md: "300px" }}
          flex="none"
          backgroundImage={isBreak ? "url(/gx-rest-break.png)" : "url(/gx-rest-bedtime.png)"}
          backgroundSize="cover"
          backgroundPosition="center"
          style={{ maskImage: MASK, WebkitMaskImage: MASK }}
        />
        {period.kind === RestKindBedtime && <BedtimeView />}
        {isBreak && <BreakView seconds={seconds ?? 0} />}
        <UnlockSection
          buttonLabel={
            isBreak
              ? "Unlock until the end of this Break"
              : `Unlock until ${rule?.bedtime_end ?? ""}`
          }
        />
      </Box>
    </Box>,
    document.body,
  )
}
