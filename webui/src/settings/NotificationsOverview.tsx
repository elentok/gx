import { Box, chakra, Text } from "@chakra-ui/react"
import { useState } from "react"
import type { NotificationChannel, NotificationKind, SettingsEntry } from "../generated/server"
import { Chip } from "../ui/chip/Chip"
import { useDevice } from "../useWide"
import { useWrite } from "../useWrite"
import { BoolControl, writeErrorText, type WriteSetting } from "./SettingsRow"
import { channelOn, currentValue } from "./channelOn"
import { setChange } from "./settingsApi"

const card = {
  borderWidth: "1px",
  borderColor: "border",
  borderRadius: "8px",
  bg: "surface.raised",
} as const

/** The Notifications section: one switch per channel, then one row per kind with a chip per channel. */
export function NotificationsOverview({
  entries,
  kinds,
  channels,
  onWrite,
}: {
  entries: SettingsEntry[]
  kinds: NotificationKind[]
  channels: NotificationChannel[]
  onWrite: WriteSetting
}) {
  const phone = useDevice() === "phone"
  const write = useWrite({ toastRefusal: false })
  const [lastKey, setLastKey] = useState("")
  const byKey = new Map(entries.map((e) => [e.key, e]))
  const enabled = (c: NotificationChannel) => channelOn(entries, c)
  const listOf = (c: NotificationChannel): string[] => {
    const v = currentValue(byKey.get(c.list_key))
    return Array.isArray(v) ? v.map(String) : []
  }
  const save = (key: string, value: unknown) => {
    setLastKey(key)
    void write.run(() => onWrite([setChange(key, value)]))
  }
  const toggle = (c: NotificationChannel, kindId: string) => {
    const list = listOf(c)
    // The whole list goes in catalog order, so one click changes one cell.
    save(
      c.list_key,
      kinds
        .map((k) => k.id)
        .filter((id) => (id === kindId ? !list.includes(id) : list.includes(id))),
    )
  }
  const { text: errorText } = writeErrorText(write, lastKey)
  return (
    <Box display="grid" gap="3" mt="2">
      <Box
        data-mock-part="channel-switches"
        display="flex"
        flexWrap="wrap"
        gap="10px"
        px="14px"
        py="12px"
        {...card}
      >
        {channels.map((c) => {
          const entry = byKey.get(c.switch_key)
          const on = enabled(c)
          return (
            <Box
              key={c.id}
              data-testid={`channel-switch-${c.id}`}
              flex={phone ? "1 1 45%" : "1"}
              minW="130px"
              display="flex"
              alignItems="center"
              justifyContent="space-between"
              gap="2"
              borderWidth="1px"
              borderColor="border"
              borderRadius="8px"
              px="10px"
              py="8px"
              opacity={on ? 1 : 0.55}
            >
              <Text fontWeight="700">{c.label}</Text>
              {entry && (
                <BoolControl
                  entry={{ ...entry, label: c.label }}
                  on={on}
                  pending={write.pending}
                  save={(value) => save(c.switch_key, value)}
                />
              )}
            </Box>
          )
        })}
      </Box>
      <Box data-mock-part="kind-rows" {...card}>
        {kinds.map((k) => (
          <Box
            key={k.id}
            data-testid={`kind-row-${k.id}`}
            display="flex"
            alignItems="center"
            justifyContent="space-between"
            flexWrap="wrap"
            gap="2"
            px="14px"
            py="8px"
            _notFirst={{ borderTopWidth: "1px", borderTopColor: "hairline" }}
          >
            <Box flex="1 1 0" minW="0">
              <Text fontWeight="500">{k.name}</Text>
              {k.hint && (
                <Text fontSize="12px" color="text.muted">
                  {k.hint}
                </Text>
              )}
            </Box>
            <Box
              display="flex"
              flexWrap="wrap"
              flex={phone ? "1 1 100%" : undefined}
              flexShrink={phone ? undefined : "0"}
              gap="1.5"
            >
              {channels.map((c) => {
                const checked = listOf(c).includes(k.id)
                return (
                  <chakra.button
                    key={c.id}
                    type="button"
                    aria-pressed={checked}
                    aria-label={`${k.name} on ${c.label}`}
                    disabled={write.pending}
                    onClick={() => toggle(c, k.id)}
                    opacity={enabled(c) ? 1 : 0.4}
                    cursor="pointer"
                  >
                    <Chip tone={checked ? "running" : "muted"} fontSize="12px">
                      {c.label}
                    </Chip>
                  </chakra.button>
                )
              })}
            </Box>
          </Box>
        ))}
      </Box>
      {errorText && (
        <Text role="alert" fontSize="sm" color="tone.deadlocked.fg">
          {errorText}
        </Text>
      )}
    </Box>
  )
}
