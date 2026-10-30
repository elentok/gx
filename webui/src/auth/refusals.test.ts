import { describe, expect, it } from "vitest";
import {
  PASSWORD_TOO_SHORT,
  RATE_LIMITED,
  reasonText,
  SETUP_CODE_WRONG,
  TOTP_MISMATCH,
  TOTP_MISMATCH_TEXT,
  TOTP_OFF,
  WRONG_CREDENTIALS,
} from "./refusals";

describe("reasonText", () => {
  it("maps each reason to one text", () => {
    expect(reasonText({ reason: WRONG_CREDENTIALS })).toBe("Wrong password or code");
    expect(reasonText({ reason: SETUP_CODE_WRONG })).toBe("That code is wrong or expired. Run gx server status for a new one.");
    expect(reasonText({ reason: TOTP_MISMATCH })).toBe(TOTP_MISMATCH_TEXT);
    expect(reasonText({ reason: PASSWORD_TOO_SHORT })).toBe("The password needs at least 12 characters.");
    expect(reasonText({ reason: TOTP_OFF })).toBe("The authenticator is turned off.");
  });

  it("rounds the wait up to whole minutes", () => {
    expect(reasonText({ reason: RATE_LIMITED, wait_seconds: 181 })).toBe("Too many tries. Try again in 4 minutes.");
    expect(reasonText({ reason: RATE_LIMITED, wait_seconds: 5 })).toBe("Too many tries. Try again in 1 minute.");
  });

  it("shows an unknown reason as it came", () => {
    expect(reasonText({ reason: "odd" })).toBe("odd");
    expect(reasonText({})).toBe("The request was refused.");
  });
});
