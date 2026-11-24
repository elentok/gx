import { Box, chakra, Text } from "@chakra-ui/react"
import { useState } from "react"
import {
  decodeKey,
  refreshPushDevice,
  setKeyChanged,
  subscriptionBody,
  subscriptionHasKey,
  usePushDevice,
  type PushDevice,
} from "../push/deviceState"
import { disabledReason } from "../push/reason"
import { fetchPushState, removePush, subscribePush, testPush } from "../push/pushApi"
import { Chevron, rowProps } from "./parts"

export const SWITCH_LABEL = "Notifications on this device"
export const SWITCH_OFF_TEXT = "Off. Turning it on asks this browser for permission."
export const TEST_OFF_TEXT = "Turn notifications on first."
export const SERVER_OFF_TEXT =
  "Push is off for the whole server. Turn it on in Settings › Notifications › Push."
export const TEST_ON_TEXT = "Sends one push to this device only."
export const TEST_SENT_TEXT =
  "Sent. It should arrive in a few seconds. If nothing shows, check the Focus mode and iOS notification settings."

/** Asks for permission, then makes (or reuses) the browser subscription and posts it. Returns the refusal text, if any. */
async function turnOn(): Promise<string | undefined> {
  if ((await Notification.requestPermission()) !== "granted") return undefined
  const publicKey = (await fetchPushState()).public_key
  const key = decodeKey(publicKey)
  const { pushManager } = await navigator.serviceWorker.ready
  let sub = await pushManager.getSubscription()
  if (sub && !subscriptionHasKey(sub, key)) {
    await sub.unsubscribe()
    sub = null
  }
  sub ??= await pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key })
  const out = await subscribePush(subscriptionBody(sub, publicKey))
  if (out.refused) return out.message
  setKeyChanged(false)
  return undefined
}

async function turnOff(): Promise<string | undefined> {
  const out = await removePush()
  void navigator.clearAppBadge?.()
  return out.refused ? out.message : undefined
}

const Card = ({ part, children }: { part: string; children: React.ReactNode }) => (
  <Box
    mt="4"
    data-mock-part={part}
    borderWidth="1px"
    borderColor="border"
    borderRadius="lg"
    bg="surface.raised"
    overflow="hidden"
  >
    {children}
  </Box>
)

function SwitchButton({
  on,
  disabled,
  onToggle,
}: {
  on: boolean
  disabled: boolean
  onToggle: () => void
}) {
  return (
    <chakra.button
      type="button"
      role="switch"
      aria-checked={on}
      aria-label={SWITCH_LABEL}
      disabled={disabled}
      onClick={onToggle}
      flex="none"
      w="46px"
      h="28px"
      borderRadius="full"
      position="relative"
      opacity={disabled ? 0.4 : 1}
      bg={on ? "tone.done.fg" : "border"}
    >
      <Box
        position="absolute"
        top="3px"
        left={on ? "21px" : "3px"}
        w="22px"
        h="22px"
        borderRadius="full"
        bg="white"
      />
    </chakra.button>
  )
}

const Message = ({
  part,
  tone,
  children,
}: {
  part: "test-result" | "reason"
  tone: "ok" | "warn"
  children: string
}) => (
  <Text
    data-mock-part={part}
    role="status"
    mt="3"
    px="3"
    py="2.5"
    fontSize="sm"
    borderWidth="1px"
    borderRadius="md"
    borderColor={tone === "ok" ? "tone.done.fg" : "tone.waiting.fg"}
    bg="surface.raised"
    color={tone === "ok" ? "tone.done.fg" : "tone.waiting.fg"}
  >
    {children}
  </Text>
)

/** The switch, the test row and their messages. Lives in the Push group of the Notifications section. */
export function PushControls({
  device,
  busy,
  error,
  testMessage,
  serverOn = true,
  onToggle,
  onTest,
}: {
  device: PushDevice
  busy: boolean
  serverOn?: boolean
  error?: string
  testMessage?: { ok: boolean; text: string }
  onToggle: () => void
  onTest: () => void
}) {
  const { on } = device
  const reason = disabledReason(device)
  const failedAt = device.failureAt ? new Date(device.failureAt) : undefined
  return (
    <Box px="3" pb="3" data-mock-part="push-controls">
      <Text mt="3" fontSize="sm" color="text.muted">
        Push alerts for sessions that need you, on this device only.
      </Text>
      <Card part="account-device-switch">
        <Box {...rowProps} cursor="default" _hover={{}}>
          <Box flex="1" minW="0">
            <Text fontWeight="600">{SWITCH_LABEL}</Text>
            <Text fontSize="sm" color="text.muted">
              {!serverOn
                ? SERVER_OFF_TEXT
                : on
                  ? `On for “${device.loginName ?? ""}”.`
                  : (reason?.line ?? SWITCH_OFF_TEXT)}
            </Text>
            {device.failureReason && (
              <Text fontSize="sm" color="tone.waiting.fg">
                Last push failed: {device.failureReason}
                {failedAt && `, ${failedAt.toLocaleString()}`}
              </Text>
            )}
          </Box>
          <SwitchButton
            on={on}
            disabled={!serverOn || busy || !!reason?.disabled || !device.supported}
            onToggle={onToggle}
          />
        </Box>
      </Card>
      {reason?.message && (
        <Message part="reason" tone="warn">
          {reason.message}
        </Message>
      )}
      {error && (
        <Text role="alert" mt="3" fontSize="sm" color="tone.deadlocked.fg">
          {error}
        </Text>
      )}
      <Card part="test-card">
        <chakra.button
          type="button"
          {...rowProps}
          disabled={!on || busy || !serverOn}
          onClick={onTest}
        >
          <Box flex="1" minW="0" opacity={on ? 1 : 0.4}>
            <Text fontWeight="600">Send test push</Text>
            <Text fontSize="sm" color="text.muted">
              {on ? TEST_ON_TEXT : TEST_OFF_TEXT}
            </Text>
          </Box>
          <Chevron />
        </chakra.button>
      </Card>
      {testMessage && (
        <Message part="test-result" tone={testMessage.ok ? "ok" : "warn"}>
          {testMessage.text}
        </Message>
      )}
    </Box>
  )
}

/** The push controls with their own state: the device, the busy flag, the last error and the test result. */
export function PushDevicePanel({ serverOn = true }: { serverOn?: boolean }) {
  const device = usePushDevice()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()
  const [testMessage, setTestMessage] = useState<{ ok: boolean; text: string }>()

  const run = async (work: () => Promise<string | undefined>) => {
    setBusy(true)
    setError(undefined)
    setTestMessage(undefined)
    try {
      setError(await work())
    } catch (err) {
      setError(err instanceof Error ? err.message : "Something went wrong.")
    } finally {
      await refreshPushDevice()
      setBusy(false)
    }
  }

  const test = async () => {
    setBusy(true)
    setError(undefined)
    try {
      const out = await testPush()
      if (out.refused) setTestMessage({ ok: false, text: out.message ?? "The test was refused." })
      else if (out.sent) setTestMessage({ ok: true, text: TEST_SENT_TEXT })
      else setTestMessage({ ok: false, text: out.failure ?? "The test push did not go out." })
    } catch (err) {
      setError(err instanceof Error ? err.message : "Something went wrong.")
    } finally {
      await refreshPushDevice()
      setBusy(false)
    }
  }

  return (
    <PushControls
      device={device}
      busy={busy}
      error={error}
      testMessage={testMessage}
      serverOn={serverOn}
      onToggle={() => void run(device.on ? turnOff : turnOn)}
      onTest={() => void test()}
    />
  )
}
