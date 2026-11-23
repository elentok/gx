import { Box, chakra, Text } from "@chakra-ui/react"
import { useState } from "react"
import { DetailPage } from "../page/DetailPage"
import { PageHeader } from "../page/PageHeader"
import { PageIcon } from "../page/PageIcon"
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
import { useReady } from "../useReady"
import { BackLink, Chevron, Frame, rowProps } from "./parts"

export const SWITCH_LABEL = "Notifications on this device"
export const SWITCH_OFF_TEXT = "Off. Turning it on asks this browser for permission."
export const TEST_OFF_TEXT = "Turn notifications on first."
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
    borderColor={tone === "ok" ? "#1f5a35" : "#6b5316"}
    bg="surface.raised"
    color={tone === "ok" ? "tone.done.fg" : "tone.waiting.fg"}
  >
    {children}
  </Text>
)

export function NotificationsView({
  device,
  busy,
  error,
  testMessage,
  onToggle,
  onTest,
}: {
  device: PushDevice
  busy: boolean
  error?: string
  testMessage?: { ok: boolean; text: string }
  onToggle: () => void
  onTest: () => void
}) {
  const { on } = device
  const reason = disabledReason(device)
  const failedAt = device.failureAt ? new Date(device.failureAt) : undefined
  return (
    <Frame>
      <BackLink />
      <PageHeader title="Notifications" icon={<PageIcon kind="settings" />} />
      <Text mt="1" fontSize="sm" color="text.muted">
        Push alerts for sessions that need you, on this device only.
      </Text>
      <Card part="switch-card">
        <Box {...rowProps} cursor="default" _hover={{}}>
          <Box flex="1" minW="0">
            <Text fontWeight="600">{SWITCH_LABEL}</Text>
            <Text fontSize="sm" color="text.muted">
              {on ? `On for “${device.loginName ?? ""}”.` : (reason?.line ?? SWITCH_OFF_TEXT)}
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
            disabled={busy || !!reason?.disabled || !device.supported}
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
        <chakra.button type="button" {...rowProps} disabled={!on || busy} onClick={onTest}>
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
    </Frame>
  )
}

export function NotificationsPage() {
  useReady()
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
    <DetailPage title={{ kind: "Settings", name: "Notifications" }}>
      <NotificationsView
        device={device}
        busy={busy}
        error={error}
        testMessage={testMessage}
        onToggle={() => void run(device.on ? turnOff : turnOn)}
        onTest={() => void test()}
      />
    </DetailPage>
  )
}
