import { cleanup, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { renderThemed } from "../ui/render";
import { BackLink } from "./BackLink";

afterEach(cleanup);

const show = (waiting?: number) =>
  renderThemed(
    <MemoryRouter>
      <BackLink to="/epics" label="← Epics" waiting={waiting} />
    </MemoryRouter>,
  );

test("the count sits inside the link and the name includes it", () => {
  show(7);
  const link = screen.getByRole("link", { name: "← Epics · 7 more need you" });
  expect(link.getAttribute("href")).toBe("/epics");
  expect(link.textContent).toContain("· 7 more need you");
});

test("one reads singular", () => {
  show(1);
  expect(screen.getByRole("link", { name: "← Epics · 1 more needs you" })).toBeTruthy();
});

test("zero reads plain", () => {
  show(0);
  expect(screen.getByRole("link").textContent).toBe("← Epics");
  cleanup();
  show();
  expect(screen.getByRole("link", { name: "← Epics" })).toBeTruthy();
});

test("on a phone the count shortens but the name stays full", () => {
  vi.stubGlobal("matchMedia", () => ({ matches: false, addEventListener() {}, removeEventListener() {} }));
  try {
    show(7);
    const link = screen.getByRole("link", { name: "← Epics · 7 more need you" });
    expect(link.textContent).toBe("← Epics· 7 more");
  } finally {
    vi.unstubAllGlobals();
  }
});
