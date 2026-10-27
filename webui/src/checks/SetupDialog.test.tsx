import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { renderThemed } from "../ui/render";
import { SetupDialog } from "./SetupDialog";
import type { SaveChecks } from "./setup";
import type { ProjectChecks } from "./status";

afterEach(cleanup);

const defaults: Record<string, string> = {
  format: "gofmt / prettier on changed files",
  lint: "go vet / eslint on changed files",
  typecheck: "go build / tsc on changed packages",
  test: "go test / vitest on changed packages",
};

const project = (over: Partial<ProjectChecks> = {}): ProjectChecks => ({
  project: "demo",
  missing: true,
  trunk: "main",
  save_targets: ["repo", "project.json"],
  error: "",
  kinds: ["format", "lint", "typecheck", "test"].map((kind) => ({
    kind,
    state: "gx-default",
    default: defaults[kind],
    candidates: [],
    ...(kind === "test"
      ? {
          recommended: { command: "make test", source: "Makefile · test" },
          candidates: [
            { command: "make test", source: "Makefile · test" },
            { command: "pnpm test", source: "package.json · test" },
          ],
        }
      : {}),
  })),
  ...over,
});

function open(over: Partial<ProjectChecks> = {}) {
  const save = vi.fn<SaveChecks>(async () => ({}));
  const onSaved = vi.fn();
  const onClose = vi.fn();
  renderThemed(<SetupDialog project={project(over)} save={save} onSaved={onSaved} onClose={onClose} />);
  const row = (kind: string) => screen.getByRole("group", { name: kind[0].toUpperCase() + kind.slice(1) });
  const field = (kind: string) => within(row(kind)).getByRole<HTMLInputElement>("combobox");
  const pick = (kind: string, mode: string) => fireEvent.click(within(row(kind)).getByRole("radio", { name: mode }));
  const preview = () => screen.getByText(/^\/\/ /).textContent ?? "";
  return { save, onSaved, onClose, row, field, pick, preview };
}

test("a found command is preset to Use command with Recommended and its source", () => {
  const { row, field } = open();
  const test = row("test");
  expect(within(test).getByRole<HTMLInputElement>("radio", { name: "Use command" }).checked).toBe(true);
  expect(field("test").value).toBe("make test");
  expect(field("test").disabled).toBe(false);
  expect(within(test).getByText("Recommended")).toBeTruthy();
  expect(test.textContent).toContain("Found in Makefile · test");
});

test("a kind with nothing found is preset to gx default with an empty disabled field", () => {
  const { row, field } = open();
  for (const kind of ["format", "lint", "typecheck"]) {
    expect(within(row(kind)).getByRole<HTMLInputElement>("radio", { name: "gx default" }).checked).toBe(true);
    expect(field(kind).value).toBe("");
    expect(field(kind).disabled).toBe(true);
    expect(field(kind).placeholder).toBe("No command found");
    expect(within(row(kind)).getByText("Nothing found")).toBeTruthy();
    expect(row(kind).textContent).toContain(`Runs the gx default: ${defaults[kind]}`);
  }
});

test("rows come in the order format, lint, typecheck, test", () => {
  open();
  expect(screen.getAllByRole("group").map((g) => g.getAttribute("aria-label")).filter((l) => /^[A-Z][a-z]+$/.test(l ?? ""))).toEqual([
    "Format",
    "Lint",
    "Typecheck",
    "Test",
  ]);
});

test("the field offers the other candidates with their sources", () => {
  const { row } = open();
  const options = [...row("test").querySelectorAll("datalist option")].map((o) => [o.getAttribute("value"), o.getAttribute("label")]);
  expect(options).toEqual([["pnpm test", "package.json · test"]]);
});

test("switching away from Use command disables the field but keeps its text", () => {
  const { field, pick } = open();
  pick("test", "gx default");
  expect(field("test").disabled).toBe(true);
  expect(field("test").value).toBe("make test");
  pick("test", "Use command");
  expect(field("test").disabled).toBe(false);
});

test("the preview is collapsed and shows the JSON for a mix of Use command, gx default and Off", () => {
  const { pick, field, preview } = open();
  expect(screen.getByText("Show the file").closest("details")?.open).toBe(false);
  pick("lint", "Off");
  pick("format", "Use command");
  fireEvent.change(field("format"), { target: { value: "make fmt" } });
  expect(preview()).toBe(
    `// .gx/checks.json (committed on main)\n${JSON.stringify({ format: { full: "make fmt" }, lint: false, test: { full: "make test" } }, null, 2)}`,
  );
});

test("Save sends the rows to the repo and reports back", async () => {
  const { save, pick, onSaved, onClose } = open();
  pick("lint", "Off");
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await vi.waitFor(() => expect(onClose).toHaveBeenCalled());
  expect(save).toHaveBeenCalledWith({ project: "demo", target: "repo", checks: { lint: false, test: { full: "make test" } } });
  expect(onSaved).toHaveBeenCalled();
});

test("Keep gx defaults saves {} to the chosen target", async () => {
  const { save, onClose } = open();
  fireEvent.click(screen.getByRole("radio", { name: /Keep it in gx's project.json/ }));
  fireEvent.click(screen.getByRole("button", { name: "Keep gx defaults" }));
  await vi.waitFor(() => expect(onClose).toHaveBeenCalled());
  expect(save).toHaveBeenCalledWith({ project: "demo", target: "project.json", checks: {} });
});

test("the repo option names the trunk and is the default", () => {
  open({ trunk: "trunk" });
  const repo = screen.getByRole<HTMLInputElement>("radio", { name: /Commit \.gx\/checks\.json on trunk/ });
  expect(repo.checked).toBe(true);
  expect(screen.getByText("One commit on trunk. Every clone and worktree sees it.")).toBeTruthy();
  expect(screen.getByText("Nothing goes in the repo. Only this machine's gx sees it.")).toBeTruthy();
});

test("a project with only project.json offers only that option", () => {
  const { preview } = open({ save_targets: ["project.json"] });
  expect(screen.queryByRole("radio", { name: /Commit/ })).toBeNull();
  expect(screen.getByRole<HTMLInputElement>("radio", { name: /project.json/ }).checked).toBe(true);
  expect(preview()).toContain("project.json");
});

test("the close button and Escape close without saving", () => {
  const { save, onClose } = open();
  fireEvent.click(screen.getByRole("button", { name: "Close" }));
  fireEvent.keyDown(window, { key: "Escape" });
  expect(onClose).toHaveBeenCalledTimes(2);
  expect(save).not.toHaveBeenCalled();
});
