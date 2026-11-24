import type { NotificationChannel, SettingsEntry } from "../generated/server"

/** The value a setting reads as: what is written, else the registry default. */
export const currentValue = (e: SettingsEntry | undefined) => (e?.set ? e.value : e?.default)

/** Whether a channel's switch is on. A missing entry is on, like the registry default. */
export function channelOn(
  entries: readonly SettingsEntry[],
  channel: Pick<NotificationChannel, "switch_key">,
): boolean {
  return currentValue(entries.find((e) => e.key === channel.switch_key)) !== false
}
