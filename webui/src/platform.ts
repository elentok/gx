/** True on macOS, where the shortcut modifier is Cmd instead of Ctrl. */
export function isMac(): boolean {
  const platform =
    (navigator as Navigator & { userAgentData?: { platform?: string } }).userAgentData?.platform ?? navigator.platform;
  return /mac/i.test(platform ?? "");
}
