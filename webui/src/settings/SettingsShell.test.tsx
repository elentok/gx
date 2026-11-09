import { fireEvent, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { describe, expect, it, vi } from "vitest";
import type { SettingsEntry } from "../generated/server";
import { renderThemed } from "../ui/render";
import { GLOBAL_SECTIONS, SettingsShellView } from "./SettingsShell";

const entry = (over: Partial<SettingsEntry>): SettingsEntry => ({
  key: "execution-queue.max-agents",
  path: ["execution-queue", "max-agents"],
  type: "number",
  section: "Execution queue",
  description: "Live agents across every project.",
  set: false,
  ...over,
});

const entries = [
  entry({ set: true, value: 6, default: 4 }),
  entry({ key: "execution-queue.poll", path: ["execution-queue", "poll"], description: "Poll.", set: true, value: "5s" }),
  entry({ key: "budget.soft", path: ["budget", "soft"], section: "Budget", description: "Soft limit.", default: 100 }),
  entry({ key: "tui.theme", path: ["tui", "theme"], section: "TUI", description: "Theme." }),
];

const renderShell = (section?: string, onSignOut = () => {}, totpOn = true) =>
  renderThemed(
    <MemoryRouter initialEntries={["/settings"]}>
      <Routes>
        <Route
          path="*"
          element={<SettingsShellView entries={entries} projects={["gx", "ripple"]} section={section} totpOn={totpOn} onSignOut={onSignOut} onWrite={async () => ({})} />}
        />
      </Routes>
    </MemoryRouter>,
  );

describe("SettingsShellView", () => {
  it("lists exactly the seven sections, with no TUI or Global section", () => {
    const { getByRole, queryByRole } = renderShell();
    const nav = getByRole("navigation");
    for (const s of GLOBAL_SECTIONS) expect(getByRole("link", { name: new RegExp(`^${s}`) })).toBeTruthy();
    expect(nav.textContent).toContain("Global");
    expect(queryByRole("link", { name: /^TUI/ })).toBeNull();
    expect(queryByRole("link", { name: /^Global/ })).toBeNull();
  });

  it("shows a changed chip with the count of keys set in the file", () => {
    const { getByRole } = renderShell();
    expect(getByRole("link", { name: /^Execution queue\s*2$/ })).toBeTruthy();
    expect(getByRole("link", { name: /^Budget$/ })).toBeTruthy();
  });

  it("shows the section's rows read-only: label, description, key path and value", () => {
    const { getByText, getByLabelText, queryByText } = renderShell("Execution queue");
    expect(getByText("Max agents")).toBeTruthy();
    expect(getByText("Live agents across every project.")).toBeTruthy();
    expect(getByText("execution-queue.max-agents")).toBeTruthy();
    expect((getByLabelText("Max agents") as HTMLInputElement).value).toBe("6");
    expect(queryByText("Soft limit.")).toBeNull();
  });

  it("shows the default for a key the file does not hold", () => {
    const { getByLabelText } = renderShell("Budget");
    expect((getByLabelText("Soft") as HTMLInputElement).value).toBe("100");
  });

  it("lists the scope picker's projects after Global settings", () => {
    const { getByLabelText } = renderShell();
    const options = Array.from(getByLabelText("Settings scope").querySelectorAll("option")).map((o) => o.textContent);
    expect(options).toEqual(["Global settings", "Project: gx", "Project: ripple"]);
  });

  it("keeps the Account rows and their hrefs, and signs out", () => {
    const onSignOut = vi.fn();
    const { getByRole, getByText } = renderShell(undefined, onSignOut);
    for (const [name, href] of [
      ["Web logins", "#/settings/logins"],
      ["Password", "#/settings/password"],
      ["Authenticator", "#/settings/authenticator"],
      ["Certificate", "#/settings/certificate"],
    ])
      expect(getByRole("link", { name: new RegExp(name) }).getAttribute("href")).toBe(href);
    fireEvent.click(getByText("Sign out of this device"));
    expect(onSignOut).toHaveBeenCalledOnce();
  });

  describe("search", () => {
    const type = (getByLabelText: (s: string) => HTMLElement, v: string) => fireEvent.change(getByLabelText("Search settings"), { target: { value: v } });

    it("finds a key by name, key path or description word", () => {
      const { getByLabelText, getByText, queryByText } = renderShell("Budget");
      type(getByLabelText, "max agents");
      expect(getByText("execution-queue.max-agents")).toBeTruthy();
      type(getByLabelText, "BUDGET.SOFT");
      expect(getByText("budget.soft")).toBeTruthy();
      expect(queryByText("execution-queue.max-agents")).toBeNull();
      type(getByLabelText, "theme");
      expect(getByText("tui.theme")).toBeTruthy();
    });

    it("groups hits under section headings, replaces the page, and clearing returns to the section", () => {
      const { container, getByLabelText, getByText, queryByText, getByRole } = renderShell("Budget");
      type(getByLabelText, "o");
      expect(getByRole("heading", { name: "Search" })).toBeTruthy();
      const page = within(container.querySelector("[data-mock-part=page]") as HTMLElement);
      expect(page.getByText("Execution queue")).toBeTruthy();
      expect(page.getByText("Budget")).toBeTruthy();
      expect(getByText("execution-queue.poll")).toBeTruthy();
      type(getByLabelText, "");
      expect(getByRole("heading", { name: "Budget" })).toBeTruthy();
      expect(queryByText("execution-queue.poll")).toBeNull();
      expect(getByText("budget.soft")).toBeTruthy();
    });

    it("shows an empty state when nothing matches", () => {
      const { getByLabelText, getByText } = renderShell();
      type(getByLabelText, "zzz");
      expect(getByText("No settings match")).toBeTruthy();
    });
  });

  it("has no Authenticator row while TOTP is off", () => {
    const { queryByRole } = renderShell(undefined, () => {}, false);
    expect(queryByRole("link", { name: /Authenticator/ })).toBeNull();
  });
});
