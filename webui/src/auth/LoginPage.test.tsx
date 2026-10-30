import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { renderThemed } from "../ui/render";
import type { AuthResult, AuthState } from "./authState";
import { CertContext } from "./cert";
import { LoginPage } from "./LoginPage";
import { TEST_CERT } from "./testCert";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const authOf = (totp_on: boolean): AuthState => ({ ...TEST_CERT, state: "signed-out", totp_on, setup_code_required: false });

function stubLogin(result: AuthResult) {
  const fetchMock = vi.fn(async () => new Response(JSON.stringify(result)));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function sentBody(fetchMock: ReturnType<typeof stubLogin>) {
  const init = (fetchMock.mock.calls[0] as unknown as [string, RequestInit])[1];
  return JSON.parse(init.body as string);
}

test("the code input is numeric and offers one-time-code autofill", () => {
  renderThemed(<LoginPage auth={authOf(true)} onDone={() => {}} />);
  const code = screen.getByLabelText("Code from your authenticator app");
  expect(code.getAttribute("inputmode")).toBe("numeric");
  expect(code.getAttribute("autocomplete")).toBe("one-time-code");
});

test("with TOTP off the code field is absent and the password alone signs in", async () => {
  const fetchMock = stubLogin({});
  const onDone = vi.fn();
  renderThemed(<LoginPage auth={authOf(false)} onDone={onDone} />);
  expect(screen.queryByLabelText("Code from your authenticator app")).toBeNull();
  fireEvent.change(screen.getByLabelText("Password"), { target: { value: "pw" } });
  fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
  await waitFor(() => expect(onDone).toHaveBeenCalled());
  expect(sentBody(fetchMock)).toEqual({ password: "pw" });
});

test("a wrong credentials refusal shows one message and does not call the gate", async () => {
  stubLogin({ refused: true, reason: "wrong-credentials" });
  const onDone = vi.fn();
  renderThemed(<LoginPage auth={authOf(true)} onDone={onDone} />);
  fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
  expect((await screen.findByRole("alert")).textContent).toBe("Wrong password or code");
  expect(onDone).not.toHaveBeenCalled();
});

test("a rate limited refusal shows the wait in minutes and disables Sign in until it ends", async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  try {
    stubLogin({ refused: true, reason: "rate-limited", wait_seconds: 181 });
    renderThemed(<LoginPage auth={authOf(true)} onDone={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
    expect((await screen.findByRole("alert")).textContent).toBe("Too many tries. Try again in 4 minutes.");
    expect((screen.getByRole("button", { name: "Sign in" }) as HTMLButtonElement).disabled).toBe(true);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(181_000);
    });
    expect((screen.getByRole("button", { name: "Sign in" }) as HTMLButtonElement).disabled).toBe(false);
    expect(screen.queryByRole("alert")).toBeNull();
  } finally {
    vi.useRealTimers();
  }
});

test("success sends the password and code and calls the gate", async () => {
  const fetchMock = stubLogin({});
  const onDone = vi.fn();
  renderThemed(<LoginPage auth={authOf(true)} onDone={onDone} />);
  fireEvent.change(screen.getByLabelText("Password"), { target: { value: "pw" } });
  fireEvent.change(screen.getByLabelText("Code from your authenticator app"), { target: { value: "123456" } });
  fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
  await waitFor(() => expect(onDone).toHaveBeenCalled());
  expect(sentBody(fetchMock)).toEqual({ password: "pw", totp_code: "123456" });
});

const URI = "otpauth://totp/gx?secret=JBSWY3DPEHPK3PXP&issuer=gx";

test("the Setup code field shows only when the auth state asks and is sent with the password", async () => {
  renderThemed(<LoginPage auth={authOf(true)} onDone={() => {}} />);
  expect(screen.queryByLabelText("Setup code")).toBeNull();
  cleanup();
  const fetchMock = stubLogin({});
  const onDone = vi.fn();
  renderThemed(<LoginPage auth={{ ...authOf(true), setup_code_required: true }} onDone={onDone} />);
  fireEvent.change(screen.getByLabelText("Setup code"), { target: { value: "K7QM-3XPD" } });
  fireEvent.change(screen.getByLabelText("Password"), { target: { value: "pw" } });
  fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
  await waitFor(() => expect(onDone).toHaveBeenCalled());
  expect(sentBody(fetchMock)).toEqual({ password: "pw", totp_code: "", setup_code: "K7QM-3XPD" });
});

test("a wrong Setup code shows the wrong-or-expired message", async () => {
  stubLogin({ refused: true, reason: "setup-code-wrong" });
  renderThemed(<LoginPage auth={{ ...authOf(true), setup_code_required: true }} onDone={() => {}} />);
  fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
  expect((await screen.findByRole("alert")).textContent).toBe(
    "That code is wrong or expired. Run gx server status for a new one.",
  );
});

test("an authenticator-needed answer shows the add step; a mismatch shows the clock message; success signs in", async () => {
  const answers: AuthResult[] = [
    { authenticator_needed: true, pending_id: "p1", secret: "JBSWY3DPEHPK3PXP", uri: URI },
    { refused: true, reason: "totp-mismatch" },
    {},
  ];
  const fetchMock = vi.fn(async () => new Response(JSON.stringify(answers.shift())));
  vi.stubGlobal("fetch", fetchMock);
  const onDone = vi.fn();
  const { container } = renderThemed(<LoginPage auth={authOf(true)} onDone={onDone} />);
  fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
  const link = await screen.findByRole("link", { name: "Add to authenticator app" });
  expect(link.getAttribute("href")).toBe(URI);
  expect(container.querySelector("[data-mock-part=totp-qr] svg")).not.toBeNull();
  expect(screen.getByText("JBSW Y3DP EHPK 3PXP")).toBeTruthy();
  fireEvent.change(screen.getByLabelText("Enter the code it shows"), { target: { value: "000000" } });
  fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
  expect((await screen.findByRole("alert")).textContent).toBe(
    "That code doesn't match. Check the clock on your phone and try again.",
  );
  expect(onDone).not.toHaveBeenCalled();
  fireEvent.change(screen.getByLabelText("Enter the code it shows"), { target: { value: "123456" } });
  fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
  await waitFor(() => expect(onDone).toHaveBeenCalled());
  const calls = fetchMock.mock.calls as unknown as [string, RequestInit][];
  expect(calls[2][0]).toContain("/v1/web/auth/add-authenticator");
  expect(JSON.parse(calls[2][1].body as string)).toEqual({ pending_id: "p1", totp_code: "123456" });
});

test("the certificate help unfolds to both links, the trust step and the fingerprint", () => {
  const cert = { fingerprint: "3F9A1C7E 52B0D488", profile_path: "/gx-root.mobileconfig", cer_path: "/gx-root.cer" };
  renderThemed(
    <CertContext.Provider value={{ cert }}>
      <LoginPage auth={authOf(true)} onDone={() => {}} />
    </CertContext.Provider>,
  );
  expect(screen.getByRole("link", { name: "iPhone or iPad profile" }).getAttribute("href")).toBe("/gx-root.mobileconfig");
  expect(screen.getByRole("link", { name: "Mac certificate" }).getAttribute("href")).toBe("/gx-root.cer");
  expect(screen.getByText(/Certificate Trust Settings/)).toBeTruthy();
  expect(screen.getByText("3F9A1C7E")).toBeTruthy();
  expect(screen.getByText("52B0D488")).toBeTruthy();
});

test("the reset hint and the folded certificate help are shown", () => {
  renderThemed(<LoginPage auth={authOf(true)} onDone={() => {}} />);
  expect(screen.getByText("gx server web-reset")).toBeTruthy();
  expect(screen.getByText("Browser warns about this page?")).toBeTruthy();
});
