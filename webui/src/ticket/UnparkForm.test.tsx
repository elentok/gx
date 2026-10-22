import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import canned from "../canned-detail.json";
import { ActionUnpark, type TicketDetail } from "../generated/server";
import { renderThemed } from "../ui/render";
import type { PostUnpark, UnparkBody } from "./unpark";
import { UnparkForm } from "./UnparkForm";

afterEach(cleanup);

const base = canned as TicketDetail;
const unparkable = (extra: Partial<TicketDetail> = {}): TicketDetail => ({
  ...base,
  actions: [ActionUnpark],
  ...extra,
});
const ok: PostUnpark = async () => ({ data: { ticket: "12", status: "open", message: "reopened" } });

test("shows nothing unless the detail lists unpark", () => {
  renderThemed(<UnparkForm detail={{ ...base, actions: [] }} post={vi.fn(ok)} />);
  expect(screen.queryByRole("button", { name: "Resume" })).toBeNull();
});

test("a plain park posts the address and state token and reports the result words", async () => {
  const post = vi.fn<(b: UnparkBody) => ReturnType<PostUnpark>>(ok);
  const onDone = vi.fn();
  renderThemed(<UnparkForm detail={unparkable()} post={post} onDone={onDone} />);
  fireEvent.click(screen.getByRole("button", { name: "Resume" }));
  await waitFor(() => expect(onDone).toHaveBeenCalledWith("reopened"));
  expect(post).toHaveBeenCalledWith({ address: "gx:demo/12", state: "tok-1" });
});

test("a zero-commit adoption says adopted as done", async () => {
  const post: PostUnpark = async () => ({ data: { ticket: "12", status: "adopted", message: "adopted as done" } });
  const onDone = vi.fn();
  renderThemed(<UnparkForm detail={unparkable()} post={post} onDone={onDone} />);
  fireEvent.click(screen.getByRole("button", { name: "Resume" }));
  await waitFor(() => expect(onDone).toHaveBeenCalledWith("adopted as done"));
});

test("an ambiguous-base park lists the blockers, needs one chosen and posts it as base", async () => {
  const post = vi.fn<(b: UnparkBody) => ReturnType<PostUnpark>>(ok);
  renderThemed(<UnparkForm detail={unparkable({ bases: ["01", "02"] })} post={post} />);
  const unpark = screen.getByRole("button", { name: "Resume" }) as HTMLButtonElement;
  expect(unpark.disabled).toBe(true);
  fireEvent.click(unpark);
  expect(post).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "02" }));
  fireEvent.click(unpark);
  await waitFor(() => expect(post).toHaveBeenCalled());
  expect(post).toHaveBeenCalledWith({ address: "gx:demo/12", state: "tok-1", base: "02" });
});

test("a stale refusal shows inline, refetches and keeps the chosen base", async () => {
  const post: PostUnpark = async () => ({ refused: true, reason: "stale", message: "The park changed." });
  const onStale = vi.fn();
  renderThemed(<UnparkForm detail={unparkable({ bases: ["01", "02"] })} post={post} onStale={onStale} />);
  fireEvent.click(screen.getByRole("button", { name: "01" }));
  fireEvent.click(screen.getByRole("button", { name: "Resume" }));
  expect((await screen.findByRole("alert")).textContent).toBe("The park changed.");
  expect(onStale).toHaveBeenCalled();
  expect(screen.getByRole("button", { name: "01" }).getAttribute("aria-pressed")).toBe("true");
});

test("another refusal shows inline without refetching", async () => {
  const post: PostUnpark = async () => ({ refused: true, reason: "bad-base", message: "Not a competing blocker." });
  const onStale = vi.fn();
  renderThemed(<UnparkForm detail={unparkable()} post={post} onStale={onStale} />);
  fireEvent.click(screen.getByRole("button", { name: "Resume" }));
  expect((await screen.findByRole("alert")).textContent).toBe("Not a competing blocker.");
  expect(onStale).not.toHaveBeenCalled();
});
