import { fireEvent, waitFor } from "@testing-library/react";
import type { ComponentProps } from "react";
import { MemoryRouter, Route, Routes } from "react-router";
import { describe, expect, it, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import { renderThemed } from "../ui/render";
import { endsInDays } from "./logins";
import { SettingsHomeView, SettingsStubView, WebLoginPage, WebLoginsView } from "./SettingsPage";
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

  it("reads Add authenticator when TOTP is on and the saved secret was cleared", () => {
    const { getByRole, queryByRole } = renderThemed(<SettingsHomeView loginCount={1} totpOn authenticatorMissing onSignOut={() => {}} />);
    expect(getByRole("link", { name: "Add authenticator" }).getAttribute("href")).toBe("#/settings/authenticator");
    expect(queryByRole("link", { name: "Reset authenticator" })).toBeNull();
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

describe("WebLoginPage", () => {
  const state: ConnectionState = {
    snapshot: undefined as unknown as ConnectionState["snapshot"],
    connected: true,
    loadedOnce: true,
    liveSnapshot: true,
    tick: 0,
    streams: 0,
  };
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  const entries = [login({ id: "a", name: "Safari on iPhone", this_device: true }), login({ id: "b", name: "Firefox on Linux" })];
  const renderAt = (id: string, props: Partial<ComponentProps<typeof WebLoginPage>> = {}) =>
    renderThemed(
      <ConnectionContext.Provider value={conn}>
        <MemoryRouter initialEntries={[`/settings/logins/${id}`]}>
          <Routes>
            <Route path="/settings/logins/:id" element={<WebLoginPage fetchList={async () => entries} now={() => NOW} {...props} />} />
          </Routes>
        </MemoryRouter>
      </ConnectionContext.Provider>,
    );

  it("renames on leaving the name field and refreshes the list", async () => {
    const rename = vi.fn(async () => ({}));
    const fetchList = vi.fn(async () => entries);
    const { findByDisplayValue } = renderAt("b", { rename, fetchList });
    const input = await findByDisplayValue("Firefox on Linux");
    fireEvent.change(input, { target: { value: "Work laptop" } });
    fireEvent.blur(input);
    await waitFor(() => expect(rename).toHaveBeenCalledWith("b", "Work laptop"));
    await waitFor(() => expect(fetchList).toHaveBeenCalledTimes(2));
  });

  it("revokes another web login only after the confirm step", async () => {
    const revoke = vi.fn(async () => ({}));
    const { findByText, getByText, queryByText } = renderAt("b", { revoke });
    fireEvent.click(await findByText("Sign out this device"));
    expect(revoke).not.toHaveBeenCalled();
    expect(getByText("It has to sign in again with the password and a code.")).toBeTruthy();
    fireEvent.click(getByText("Cancel"));
    expect(queryByText("Cancel")).toBeNull();
    fireEvent.click(getByText("Sign out this device"));
    fireEvent.click(getByText("Sign out"));
    await waitFor(() => expect(revoke).toHaveBeenCalledWith("b"));
  });

  it("signs this device out through the gate instead of revoking", async () => {
    const revoke = vi.fn(async () => ({}));
    const signOut = vi.fn();
    const { findByText, getByText } = renderAt("a", { revoke, signOut });
    fireEvent.click(await findByText("Sign out this device"));
    fireEvent.click(getByText("Sign out"));
    await waitFor(() => expect(signOut).toHaveBeenCalledOnce());
    expect(revoke).not.toHaveBeenCalled();
  });
});

describe("SettingsStubView", () => {
  it("renders only the back link", () => {
    expect(renderThemed(<SettingsStubView />).getByRole("link", { name: "‹ Settings" })).toBeTruthy();
  });
});
