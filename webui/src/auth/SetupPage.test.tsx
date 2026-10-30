import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { renderThemed } from "../ui/render";
import type { AuthResult, AuthState } from "./authState";
import { SetupPage } from "./SetupPage";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

const authOf = (totp_on: boolean): AuthState => ({ state: "setup", totp_on, setup_code_required: false });

function stubStart(result: AuthResult) {
  const fetchMock = vi.fn(async () => new Response(JSON.stringify(result)));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

test("the code from the hash fills the field and leaves the URL", () => {
  window.location.hash = "#/setup?code=K7QM-3XPD";
  renderThemed(<SetupPage auth={authOf(true)} onDone={() => {}} />);
  expect((screen.getByLabelText("Setup code") as HTMLInputElement).value).toBe("K7QM-3XPD");
  expect(window.location.hash).toBe("#/setup");
});

test("a wrong code shows the message that sends the person back to the CLI", async () => {
  stubStart({ refused: true, reason: "setup-code-wrong" });
  renderThemed(<SetupPage auth={authOf(true)} onDone={() => {}} />);
  fireEvent.change(screen.getByLabelText("Setup code"), { target: { value: "K7QM-3XPP" } });
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));
  expect((await screen.findByRole("alert")).textContent).toBe(
    "That code is wrong or expired. Run gx server status for a new one.",
  );
});

test("the step count follows TOTP", () => {
  renderThemed(<SetupPage auth={authOf(false)} onDone={() => {}} />);
  expect(screen.getByText(/step 1 of 2/)).toBeTruthy();
});

async function toPasswordStep(totpOn: boolean, onDone = () => {}) {
  renderThemed(<SetupPage auth={authOf(totpOn)} onDone={onDone} />);
  fireEvent.change(screen.getByLabelText("Setup code"), { target: { value: "K7QM-3XPD" } });
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));
  await screen.findByText("Choose a password");
}

function submitPasswords(password: string, again: string, button: string) {
  fireEvent.change(screen.getByLabelText(/^Password(?! again)/), { target: { value: password } });
  fireEvent.change(screen.getByLabelText("Password again"), { target: { value: again } });
  fireEvent.click(screen.getByRole("button", { name: button }));
}

test("an accepted code moves on to the password step", async () => {
  stubStart({});
  await toPasswordStep(false);
  expect(screen.getByText(/step 2 of 2/)).toBeTruthy();
  expect(screen.getByText("At least 12 characters.")).toBeTruthy();
});

test("a short password is refused", async () => {
  stubStart({});
  await toPasswordStep(true);
  submitPasswords("short", "short", "Continue");
  expect((await screen.findByRole("alert")).textContent).toBe("The password needs at least 12 characters.");
});

test("a mismatch is refused", async () => {
  stubStart({});
  await toPasswordStep(true);
  submitPasswords("correct horse battery", "correct horse batterz", "Continue");
  expect((await screen.findByRole("alert")).textContent).toBe("The two passwords don't match.");
});

test("a good password moves on to step 3 when TOTP is on", async () => {
  stubStart({});
  await toPasswordStep(true);
  submitPasswords("correct horse battery", "correct horse battery", "Continue");
  await waitFor(() => expect(screen.getByRole("button", { name: "Finish setup" })).toBeTruthy());
});

test("with TOTP off a good password finishes setup and goes to the app", async () => {
  const fetchMock = stubStart({});
  const onDone = vi.fn();
  await toPasswordStep(false, onDone);
  submitPasswords("correct horse battery", "correct horse battery", "Finish setup");
  await waitFor(() => expect(onDone).toHaveBeenCalled());
  const [url, init] = fetchMock.mock.calls.at(-1) as unknown as [string, RequestInit];
  expect(url).toMatch(/\/v1\/web\/auth\/setup-finish$/);
  expect(JSON.parse(init.body as string)).toEqual({ code: "K7QM-3XPD", password: "correct horse battery" });
  expect(window.location.hash).toBe("#/");
});

test("a refusal from setup finish is shown", async () => {
  stubStart({});
  await toPasswordStep(false);
  stubStart({ refused: true, reason: "setup-code-wrong" });
  submitPasswords("correct horse battery", "correct horse battery", "Finish setup");
  expect((await screen.findByRole("alert")).textContent).toBe(
    "That code is wrong or expired. Run gx server status for a new one.",
  );
});
