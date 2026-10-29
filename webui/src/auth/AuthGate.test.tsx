import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { signOut } from "../apiFetch";
import { renderThemed } from "../ui/render";
import { AuthGate } from "./AuthGate";
import type { AuthState } from "./authState";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  sessionStorage.clear();
});
beforeEach(() => {
  window.location.hash = "";
});

const stateOf = (state: AuthState["state"]): AuthState => ({ state, totp_on: true, setup_code_required: false });

function renderGate(fetchState: () => Promise<AuthState>) {
  return renderThemed(
    <AuthGate fetchState={fetchState}>
      <p>the app</p>
    </AuthGate>,
  );
}

test("shows setup for a pending setup", async () => {
  renderGate(async () => stateOf("setup"));
  expect(await screen.findByRole("button", { name: "Finish setup" })).toBeTruthy();
});

test("shows login when signed out", async () => {
  renderGate(async () => stateOf("signed-out"));
  expect(await screen.findByRole("button", { name: "Sign in" })).toBeTruthy();
});

test("shows the app when signed in", async () => {
  renderGate(async () => stateOf("signed-in"));
  expect(await screen.findByText("the app")).toBeTruthy();
});

test("login asks for the setup code only when the state says so", async () => {
  renderGate(async () => ({ ...stateOf("signed-out"), setup_code_required: true }));
  expect(await screen.findByLabelText("Setup code")).toBeTruthy();
});

test("a 401 in the app switches to login and keeps the hash", async () => {
  window.location.hash = "#/stats";
  renderGate(async () => stateOf("signed-in"));
  await screen.findByText("the app");
  act(() => signOut());
  expect(await screen.findByRole("button", { name: "Sign in" })).toBeTruthy();
  expect(window.location.hash).toBe("#/stats");
});

test("a successful login re-reads the state and keeps the hash", async () => {
  window.location.hash = "#/stats";
  let current = stateOf("signed-out");
  vi.stubGlobal("fetch", vi.fn(async () => new Response("{}")));
  renderGate(async () => current);
  fireEvent.change(await screen.findByLabelText("Password"), { target: { value: "pw" } });
  current = stateOf("signed-in");
  fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
  expect(await screen.findByText("the app")).toBeTruthy();
  expect(window.location.hash).toBe("#/stats");
});

test("sign out clears the page caches", async () => {
  sessionStorage.setItem("gx.page.stats", "{}");
  renderGate(async () => stateOf("signed-in"));
  await screen.findByText("the app");
  act(() => signOut());
  await waitFor(() => expect(sessionStorage.getItem("gx.page.stats")).toBeNull());
});
