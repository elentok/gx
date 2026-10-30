export const MIN_PASSWORD = 6;
export const PASSWORD_HINT = `At least ${MIN_PASSWORD} characters.`;

export const WRONG_CREDENTIALS = "wrong-credentials";
export const RATE_LIMITED = "rate-limited";
export const SETUP_CODE_WRONG = "setup-code-wrong";
export const TOTP_MISMATCH = "totp-mismatch";
export const PASSWORD_TOO_SHORT = "password-too-short";
export const TOTP_OFF = "totp-off";

export const TOTP_MISMATCH_TEXT = "That code doesn't match. Check the clock on your phone and try again.";

const TEXTS: Record<string, string> = {
  [WRONG_CREDENTIALS]: "Wrong password or code",
  [SETUP_CODE_WRONG]: "That code is wrong or expired. Run gx server status for a new one.",
  [TOTP_MISMATCH]: TOTP_MISMATCH_TEXT,
  [PASSWORD_TOO_SHORT]: `The password needs at least ${MIN_PASSWORD} characters.`,
  [TOTP_OFF]: "The authenticator is turned off.",
};

export function rateLimitText(waitSeconds: number): string {
  const minutes = Math.max(1, Math.ceil(waitSeconds / 60));
  return `Too many tries. Try again in ${minutes} ${minutes === 1 ? "minute" : "minutes"}.`;
}

/** The one text a refusal reads as on every page. An unknown reason shows as it came. */
export function reasonText(res: { reason?: string; wait_seconds?: number }): string {
  if (res.reason === RATE_LIMITED) return rateLimitText(res.wait_seconds ?? 0);
  return (res.reason && TEXTS[res.reason]) || res.reason || "The request was refused.";
}
