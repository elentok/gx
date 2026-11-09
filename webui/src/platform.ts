/** True on macOS, where the shortcut modifier is Cmd instead of Ctrl. */
export function isMac(): boolean {
  const platform =
    (navigator as Navigator & { userAgentData?: { platform?: string } }).userAgentData?.platform ?? navigator.platform;
  return /mac/i.test(platform ?? "");
}

/** True on iPhone, iPad and iPod in every browser. iPadOS Safari sends a Mac user agent, so touch tells it from a Mac. */
export function isIOS(): boolean {
  return /iPhone|iPad|iPod/.test(navigator.userAgent) || (isMac() && navigator.maxTouchPoints > 1);
}

/** True when the app runs as an installed app (a home-screen app on iOS, a standalone window elsewhere). */
export function isStandalone(): boolean {
  return (
    (navigator as Navigator & { standalone?: boolean }).standalone === true ||
    window.matchMedia?.("(display-mode: standalone)").matches === true
  );
}
