import { fireEvent, waitFor } from "@testing-library/react";
import type { ComponentProps } from "react";
import { MemoryRouter, Route, Routes } from "react-router";
import { describe, expect, it, vi } from "vitest";
import type { AuthState } from "../auth/authState";
import { TEST_CERT } from "../auth/testCert";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import { renderThemed } from "../ui/render";
import { AuthenticatorPage, AuthenticatorView, ChangePasswordPage, ChangePasswordView } from "./AccountPages";
import type { WebLoginEntry } from "./logins";

const state: ConnectionState = {
  snapshot: undefined as unknown as ConnectionState["snapshot"],
  connected: true,
  loadedOnce: true,
  liveSnapshot: true,
  tick: 0,
  streams: 0,
};
const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
const entry = (id: string, this_device: boolean): WebLoginEntry => ({ id, name: id, last_used: "2026-10-30T12:00:00Z", last_ip: "", this_device });
const logins = async () => [entry("a", true), entry("b", false), entry("c", false)];
const auth = (over: Partial<AuthState> = {}) => async (): Promise<AuthState> => ({
  ...TEST_CERT,
  state: "signed-in",
  totp_on: true,
  setup_code_required: false,
  ...over,
});
const NEXT = { secret: "JBSWY3DPEHPK3PXP", uri: "otpauth://totp/gx?secret=JBSWY3DPEHPK3PXP" };

const type = (el: HTMLElement, value: string) => fireEvent.change(el, { target: { value } });
// The "New password" label also holds its hint, so look the inputs up by name.
const input = (container: HTMLElement, name: string) => container.querySelector(`input[name="${name}"]`) as HTMLElement;

describe("ChangePasswordView", () => {
  it("counts the other devices before submit, and says nothing with none", () => {
    for (const [count, text] of [
      [3, "The 3 other devices will be signed out."],
      [1, "The 1 other device will be signed out."],
    ] as const) {
      const { getByText, unmount } = renderThemed(<ChangePasswordView otherDevices={count} totpOn onSubmit={() => {}} />);
      expect(getByText(text)).toBeTruthy();
      unmount();
    }
    const { queryByText } = renderThemed(<ChangePasswordView otherDevices={0} totpOn onSubmit={() => {}} />);
    expect(queryByText(/will be signed out/)).toBeNull();
  });

  it("leaves the code field out when TOTP is off", () => {
    const off = renderThemed(<ChangePasswordView otherDevices={2} totpOn={false} onSubmit={() => {}} />);
    expect(off.queryByLabelText("Code from your authenticator app")).toBeNull();
    off.unmount();
    const on = renderThemed(<ChangePasswordView otherDevices={2} totpOn onSubmit={() => {}} />);
    expect(on.queryByLabelText("Code from your authenticator app")).toBeTruthy();
  });

  it("does not submit when the new passwords differ", () => {
    const onSubmit = vi.fn();
    const { container, getByLabelText, getByRole, getByText } = renderThemed(
      <ChangePasswordView otherDevices={2} totpOn={false} onSubmit={onSubmit} />,
    );
    type(getByLabelText("Current password"), "old");
    type(input(container, "new_password"), "correct horse battery");
    type(input(container, "new_password_again"), "correct horse batterX");
    fireEvent.click(getByRole("button", { name: "Change password" }));
    expect(getByText("The new passwords don't match.")).toBeTruthy();
    expect(onSubmit).not.toHaveBeenCalled();
  });
});

describe("ChangePasswordPage", () => {
  const renderPage = (props: Partial<ComponentProps<typeof ChangePasswordPage>> = {}) =>
    renderThemed(
      <ConnectionContext.Provider value={conn}>
        <MemoryRouter initialEntries={["/settings/password"]}>
          <Routes>
            <Route path="/settings/password" element={<ChangePasswordPage fetchAuth={auth()} fetchList={logins} {...props} />} />
          </Routes>
        </MemoryRouter>
      </ConnectionContext.Provider>,
    );

  it("counts the other devices from the list and shows a refusal's message", async () => {
    const change = vi.fn(async () => ({ refused: true, reason: "password-too-short", message: "The password needs at least 12 characters." }));
    const { container, findByText, getByLabelText, getByRole } = renderPage({ change });
    expect(await findByText("The 2 other devices will be signed out.")).toBeTruthy();
    type(getByLabelText("Current password"), "old");
    type(getByLabelText("Code from your authenticator app"), "123456");
    type(input(container, "new_password"), "short pass word");
    type(input(container, "new_password_again"), "short pass word");
    fireEvent.click(getByRole("button", { name: "Change password" }));
    expect(await findByText("The password needs at least 12 characters.")).toBeTruthy();
    expect(change).toHaveBeenCalledWith({ password: "old", totp_code: "123456", new_password: "short pass word" });
  });
});

describe("AuthenticatorView", () => {
  it("replace step 1 asks for the password and a code from the current app", () => {
    const { getByLabelText } = renderThemed(<AuthenticatorView mode="replace"otherDevices={3} onBegin={() => {}} onFinish={() => {}} />);
    expect(getByLabelText("Current password")).toBeTruthy();
    expect(getByLabelText("Code from the current app")).toBeTruthy();
  });

  it("replace step 2 shows the QR, link, key, code field and the keep note", () => {
    const onFinish = vi.fn();
    const { container, getByRole, getByText, getByLabelText } = renderThemed(
      <AuthenticatorView mode="replace"otherDevices={3} next={NEXT} onBegin={() => {}} onFinish={onFinish} />,
    );
    expect(container.querySelector('[data-mock-part="totp-qr"] svg')).toBeTruthy();
    expect(getByRole("link", { name: "Add to authenticator app" }).getAttribute("href")).toBe(NEXT.uri);
    expect(getByText("JBSW Y3DP EHPK 3PXP")).toBeTruthy();
    expect(getByText("The old app keeps working until this code checks out.")).toBeTruthy();
    type(getByLabelText("Code from the new app"), "654321");
    fireEvent.click(getByRole("button", { name: "Save new authenticator" }));
    expect(onFinish).toHaveBeenCalledWith({ totp_code: "654321" });
  });

  it("reads Add authenticator and asks for the password only when there is no saved secret", () => {
    const { getByRole, getByLabelText, queryByLabelText } = renderThemed(
      <AuthenticatorView mode="add" otherDevices={3} onBegin={() => {}} onFinish={() => {}} />,
    );
    expect(getByRole("heading", { name: "Add authenticator" })).toBeTruthy();
    expect(getByLabelText("Current password")).toBeTruthy();
    expect(queryByLabelText("Code from the current app")).toBeNull();
  });
});

describe("AuthenticatorPage", () => {
  const renderPage = (props: Partial<ComponentProps<typeof AuthenticatorPage>> = {}) =>
    renderThemed(
      <ConnectionContext.Provider value={conn}>
        <MemoryRouter initialEntries={["/settings/authenticator"]}>
          <Routes>
            <Route path="/settings/authenticator" element={<AuthenticatorPage fetchAuth={auth()} fetchList={logins} {...props} />} />
          </Routes>
        </MemoryRouter>
      </ConnectionContext.Provider>,
    );

  it("shows a refusal on step 1, then step 2 after the password and code check out, and finishes", async () => {
    const begin = vi
      .fn()
      .mockResolvedValueOnce({ refused: true, reason: "wrong-credentials", message: "Wrong password or code" })
      .mockResolvedValueOnce(NEXT);
    const finish = vi.fn(async () => ({}));
    const { findByLabelText, findByText, getByLabelText, getByRole } = renderPage({ begin, finish });
    type(await findByLabelText("Current password"), "pw");
    type(getByLabelText("Code from the current app"), "111111");
    fireEvent.click(getByRole("button", { name: "Next" }));
    expect(await findByText("Wrong password or code")).toBeTruthy();
    fireEvent.click(getByRole("button", { name: "Next" }));
    type(await findByLabelText("Code from the new app"), "222222");
    expect(begin).toHaveBeenLastCalledWith({ password: "pw", totp_code: "111111" });
    fireEvent.click(getByRole("button", { name: "Save new authenticator" }));
    await waitFor(() => expect(finish).toHaveBeenCalledWith({ totp_code: "222222" }));
  });

  it("asks for the password only when the saved secret was cleared", async () => {
    const begin = vi.fn(async () => NEXT);
    const { findByRole, getByLabelText, queryByLabelText } = renderPage({ fetchAuth: auth({ setup_code_required: true }), begin });
    expect(await findByRole("heading", { name: "Add authenticator" })).toBeTruthy();
    expect(queryByLabelText("Code from the current app")).toBeNull();
    type(getByLabelText("Current password"), "pw");
    fireEvent.click(await findByRole("button", { name: "Next" }));
    await waitFor(() => expect(begin).toHaveBeenCalledWith({ password: "pw" }));
  });
});
