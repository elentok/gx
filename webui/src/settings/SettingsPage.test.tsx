import { fireEvent } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { renderThemed } from "../ui/render";
import { endsInDays } from "./logins";
import { SettingsHomeView, SettingsStubView, WebLoginsView } from "./SettingsPage";
import type { WebLoginEntry } from "./logins";

const NOW = Date.parse("2026-10-30T12:00:00Z");
const DAY = 24 * 60 * 60 * 1000;
const login = (over: Partial<WebLoginEntry> = {}): WebLoginEntry => ({
  id: "a",
  name: "Safari on iPhone",
  last_used: new Date(NOW - 60_000 * 30).toISOString(),
  last_ip: "192.168.1.23",
  this_device: false,
  ...over,
});
const usedDaysAgo = (days: number) => new Date(NOW - days * DAY).toISOString();

describe("SettingsHomeView", () => {
  it("lists the Security rows with the device count and signs out through the callback", () => {
    const onSignOut = vi.fn();
    const { getByRole, getByText } = renderThemed(<SettingsHomeView loginCount={4} totpOn onSignOut={onSignOut} />);
    expect(getByRole("link", { name: /Web logins/ }).textContent).toContain("4 devices");
    expect(getByRole("link", { name: /Web logins/ }).getAttribute("href")).toBe("#/settings/logins");
    for (const name of ["Change password", "Reset authenticator", "Certificate"]) expect(getByRole("link", { name })).toBeTruthy();
    fireEvent.click(getByText("Sign out of this device"));
    expect(onSignOut).toHaveBeenCalledOnce();
  });

  it("has no Reset authenticator row while TOTP is off, and counts one device in the singular", () => {
    const { queryByRole, getByRole } = renderThemed(<SettingsHomeView loginCount={1} totpOn={false} onSignOut={() => {}} />);
    expect(queryByRole("link", { name: "Reset authenticator" })).toBeNull();
    expect(getByRole("link", { name: /Web logins/ }).textContent).toContain("1 device");
    expect(getByRole("link", { name: /Web logins/ }).textContent).not.toContain("devices");
  });
});

describe("WebLoginsView", () => {
  it("shows the 90-day note, the back link, the name, last used, IP and the This device chip", () => {
    const { getByRole, getByText } = renderThemed(<WebLoginsView logins={[login({ this_device: true })]} now={NOW} />);
    expect(getByRole("link", { name: "‹ Settings" }).getAttribute("href")).toBe("#/settings");
    expect(getByText("A web login ends after 90 days without use.")).toBeTruthy();
    expect(getByText("Safari on iPhone")).toBeTruthy();
    expect(getByText("30m ago · 192.168.1.23")).toBeTruthy();
    expect(getByText("This device")).toBeTruthy();
  });

  it("warns with 29 days left and not with 31", () => {
    const { queryByText } = renderThemed(
      <WebLoginsView logins={[login({ id: "a", name: "Soon", last_used: usedDaysAgo(61) }), login({ id: "b", name: "Later", last_used: usedDaysAgo(59) })]} now={NOW} />,
    );
    expect(queryByText("Ends in 29 days unless used")).toBeTruthy();
    expect(queryByText(/Ends in 31/)).toBeNull();
    expect(queryByText(/Ends in/)?.textContent).toBe("Ends in 29 days unless used");
  });

  it("counts the days left at the 30-day edge", () => {
    expect(endsInDays(usedDaysAgo(60), NOW)).toBeUndefined();
    expect(endsInDays(usedDaysAgo(61), NOW)).toBe(29);
  });
});

describe("SettingsStubView", () => {
  it("renders only the back link", () => {
    expect(renderThemed(<SettingsStubView />).getByRole("link", { name: "‹ Settings" })).toBeTruthy();
  });
});
