import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import canned from "../canned-detail.json";
import { ActionResume, type TicketDetail } from "../generated/server";
import { renderThemed } from "../ui/render";
import type { PostResume, ResumeBody } from "./resume";
import { ResumeForm } from "./ResumeForm";

afterEach(cleanup);

const base = canned as TicketDetail;
const resumable = (extra: Partial<TicketDetail> = {}): TicketDetail => ({
  ...base,
  actions: [ActionResume],
  pause_reason: "manual",
  ...extra,
});
const ok: PostResume = async () => ({ data: { ticket: "12", status: "open", message: "reopened" } });

test("shows nothing unless the detail lists resume", () => {
  renderThemed(<ResumeForm detail={{ ...base, actions: [] }} post={vi.fn(ok)} />);
  expect(screen.queryByRole("form")).toBeNull();
});

test.each([
  ["manual", "Resume"],
  ["ambiguous-base", "Choose base"],
  ["zero-commit", "Retry"],
  ["iteration-error", "Retry"],
  ["self-reported", "Retry"],
])("a %s pause shows the %s action", (pause_reason, label) => {
  renderThemed(<ResumeForm detail={resumable({ pause_reason })} post={vi.fn(ok)} />);
  expect(screen.getByRole("button", { name: label })).toBeTruthy();
});

test("a plain resume posts the address and state token and reports the result words", async () => {
  const post = vi.fn<(b: ResumeBody) => ReturnType<PostResume>>(ok);
  const onDone = vi.fn();
  renderThemed(<ResumeForm detail={resumable()} post={post} onDone={onDone} />);
  fireEvent.click(screen.getByRole("button", { name: "Resume" }));
  await waitFor(() => expect(onDone).toHaveBeenCalledWith("reopened"));
  expect(post).toHaveBeenCalledWith({ address: "gx:demo/12", state: "tok-1" });
});

test("a zero-commit adoption says adopted as done", async () => {
  const post: PostResume = async () => ({ data: { ticket: "12", status: "adopted", message: "adopted as done" } });
  const onDone = vi.fn();
  renderThemed(<ResumeForm detail={resumable({ pause_reason: "zero-commit" })} post={post} onDone={onDone} />);
  fireEvent.click(screen.getByRole("button", { name: "Retry" }));
  await waitFor(() => expect(onDone).toHaveBeenCalledWith("adopted as done"));
});

test("an ambiguous-base pause lists the blockers, needs one chosen and posts it as base", async () => {
  const post = vi.fn<(b: ResumeBody) => ReturnType<PostResume>>(ok);
  renderThemed(<ResumeForm detail={resumable({ pause_reason: "ambiguous-base", bases: ["01", "02"] })} post={post} />);
  const choose = screen.getByRole("button", { name: "Choose base" }) as HTMLButtonElement;
  expect(choose.disabled).toBe(true);
  fireEvent.click(choose);
  expect(post).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "02" }));
  fireEvent.click(choose);
  await waitFor(() => expect(post).toHaveBeenCalled());
  expect(post).toHaveBeenCalledWith({ address: "gx:demo/12", state: "tok-1", base: "02" });
});

test("a stale refusal shows inline, refetches and keeps the chosen base", async () => {
  const post: PostResume = async () => ({ refused: true, reason: "stale", message: "The pause changed." });
  const onStale = vi.fn();
  renderThemed(
    <ResumeForm detail={resumable({ pause_reason: "ambiguous-base", bases: ["01", "02"] })} post={post} onStale={onStale} />,
  );
  fireEvent.click(screen.getByRole("button", { name: "01" }));
  fireEvent.click(screen.getByRole("button", { name: "Choose base" }));
  expect((await screen.findByRole("alert")).textContent).toBe("The pause changed.");
  expect(onStale).toHaveBeenCalled();
  expect(screen.getByRole("button", { name: "01" }).getAttribute("aria-pressed")).toBe("true");
});

test("another refusal shows inline without refetching", async () => {
  const post: PostResume = async () => ({ refused: true, reason: "bad-base", message: "Not a competing blocker." });
  const onStale = vi.fn();
  renderThemed(<ResumeForm detail={resumable()} post={post} onStale={onStale} />);
  fireEvent.click(screen.getByRole("button", { name: "Resume" }));
  expect((await screen.findByRole("alert")).textContent).toBe("Not a competing blocker.");
  expect(onStale).not.toHaveBeenCalled();
});
