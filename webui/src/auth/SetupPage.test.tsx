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

test("an accepted code moves on to the next step", async () => {
  stubStart({});
  renderThemed(<SetupPage auth={authOf(false)} onDone={() => {}} />);
  fireEvent.change(screen.getByLabelText("Setup code"), { target: { value: "K7QM-3XPD" } });
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "Finish setup" })).toBeTruthy());
});
