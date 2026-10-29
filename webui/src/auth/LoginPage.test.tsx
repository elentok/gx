import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { renderThemed } from "../ui/render";
import type { AuthResult, AuthState } from "./authState";
import { LoginPage } from "./LoginPage";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const authOf = (totp_on: boolean): AuthState => ({ state: "signed-out", totp_on, setup_code_required: false });

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

test("the reset hint and the folded certificate help are shown", () => {
  renderThemed(<LoginPage auth={authOf(true)} onDone={() => {}} />);
  expect(screen.getByText("gx server web-reset")).toBeTruthy();
  expect(screen.getByText("Browser warns about this page?")).toBeTruthy();
});
