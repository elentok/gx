import { cleanup, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { installClientErrorReporting } from "./clientErrors";
import { ErrorBoundary } from "./ErrorBoundary";
import { renderThemed } from "./ui/render";

const fetchMock = vi.fn(async (_url: string, _init: RequestInit) => new Response("{}"));

beforeEach(() => {
  vi.stubGlobal("fetch", fetchMock);
  vi.spyOn(console, "error").mockImplementation(() => {});
});
afterEach(() => {
  cleanup();
  fetchMock.mockClear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

function Boom(): never {
  throw new Error("kaboom");
}

const sent = () => fetchMock.mock.calls.map(([url, init]) => ({ url, body: JSON.parse(init.body as string) }));

test("a throwing child shows the error card with Reload and posts one report", () => {
  renderThemed(
    <ErrorBoundary>
      <Boom />
    </ErrorBoundary>,
  );
  expect(screen.getByText("Something went wrong")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Reload" })).toBeTruthy();
  expect(sent()).toHaveLength(1);
  expect(sent()[0].url).toBe("/v1/client-errors");
  expect(sent()[0].body.message).toBe("kaboom");
  expect(sent()[0].body.user_agent).toBe(navigator.userAgent);
});

test("an error event and an unhandled rejection each post one report", () => {
  installClientErrorReporting();
  window.dispatchEvent(new ErrorEvent("error", { error: new Error("thrown"), message: "thrown" }));
  const rejection = new Event("unhandledrejection") as Event & { reason: unknown };
  rejection.reason = new Error("rejected");
  window.dispatchEvent(rejection);
  expect(sent().map((r) => r.body.message)).toEqual(["thrown", "rejected"]);
});

test("the posted page URL drops the hash query", () => {
  window.location.hash = "#/setup?code=K7QM-3XPD";
  renderThemed(
    <ErrorBoundary>
      <Boom />
    </ErrorBoundary>,
  );
  const { url } = sent()[0].body;
  expect(url).toContain("#/setup");
  expect(url).not.toContain("K7QM");
  expect(url).not.toContain("?");
});
