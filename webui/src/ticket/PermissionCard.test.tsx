import { ChakraProvider } from "@chakra-ui/react";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import canned from "../canned-detail.json";
import type { PermissionCard as Card, TicketDetail } from "../generated/server";
import { renderThemed } from "../ui/render";
import { system } from "../ui/theme";
import { PermissionCard } from "./PermissionCard";
import type { DecisionBody, PostDecision } from "./permission";

afterEach(cleanup);

const base = canned as TicketDetail;
const card: Card = { request_id: "req-1", tool: "Bash", description: "Run a command", input: "rm -rf build" };
const withCard = (permission?: Card): TicketDetail => ({ ...base, permission });
const ok: PostDecision = async () => ({});

test("renders the tool, description and input with only Allow once and Deny", () => {
  renderThemed(<PermissionCard detail={withCard(card)} post={ok} />);
  expect(screen.getByText("Bash")).toBeTruthy();
  expect(screen.getByText("Run a command")).toBeTruthy();
  expect(screen.getByLabelText("Tool input").textContent).toBe("rm -rf build");
  const buttons = screen.getAllByRole("button").map((b) => b.textContent);
  expect(buttons).toEqual(["Allow once", "Deny"]);
});

test("renders nothing without a pending prompt", () => {
  const { container } = renderThemed(<PermissionCard detail={withCard()} post={ok} />);
  expect(container.textContent).toBe("");
});

test("Allow posts the request id and drops the note", async () => {
  const post = vi.fn<(b: DecisionBody) => ReturnType<PostDecision>>(ok);
  const onDone = vi.fn();
  renderThemed(<PermissionCard detail={withCard(card)} post={post} onDone={onDone} />);
  fireEvent.change(screen.getByLabelText("Note for Deny (optional)"), { target: { value: "careful" } });
  fireEvent.click(screen.getByRole("button", { name: "Allow once" }));
  await waitFor(() => expect(onDone).toHaveBeenCalled());
  expect(post).toHaveBeenCalledWith({ address: "gx:demo/12", decision: "allow", request_id: "req-1" });
});

test("Deny posts the request id and the note", async () => {
  const post = vi.fn<(b: DecisionBody) => ReturnType<PostDecision>>(ok);
  renderThemed(<PermissionCard detail={withCard(card)} post={post} />);
  fireEvent.change(screen.getByLabelText("Note for Deny (optional)"), { target: { value: "too risky" } });
  fireEvent.click(screen.getByRole("button", { name: "Deny" }));
  await waitFor(() => expect(post).toHaveBeenCalled());
  expect(post).toHaveBeenCalledWith({ address: "gx:demo/12", decision: "deny", request_id: "req-1", text: "too risky" });
});

test.each([
  ["request-changed", "The open request is not req-1"],
  ["not-blocked", "The agent is not waiting on a request"],
  ["agent-gone", "The agent is gone"],
])("a %s refusal shows inline and refetches", async (reason, message) => {
  const post: PostDecision = async () => ({ refused: true, reason, message });
  const onStale = vi.fn();
  const onDone = vi.fn();
  renderThemed(<PermissionCard detail={withCard(card)} post={post} onStale={onStale} onDone={onDone} />);
  fireEvent.click(screen.getByRole("button", { name: "Allow once" }));
  await waitFor(() => expect(screen.getByRole("alert").textContent).toBe(message));
  expect(onStale).toHaveBeenCalled();
  expect(onDone).not.toHaveBeenCalled();
});

test("a changed prompt shows the new card and its request id is the one sent", async () => {
  const post = vi.fn<(b: DecisionBody) => ReturnType<PostDecision>>(ok);
  const { rerender } = renderThemed(<PermissionCard detail={withCard(card)} post={post} />);
  fireEvent.change(screen.getByLabelText("Note for Deny (optional)"), { target: { value: "old" } });
  rerender(
    <ChakraProvider value={system}>
      <PermissionCard detail={withCard({ ...card, request_id: "req-2", tool: "Write", input: "x" })} post={post} />
    </ChakraProvider>,
  );
  expect(screen.getByText("Write")).toBeTruthy();
  expect((screen.getByLabelText("Note for Deny (optional)") as HTMLTextAreaElement).value).toBe("");
  fireEvent.click(screen.getByRole("button", { name: "Deny" }));
  await waitFor(() => expect(post).toHaveBeenCalled());
  expect(post.mock.calls[0][0].request_id).toBe("req-2");
});

test("an unsupported dialog shows the TUI hint with no buttons", () => {
  const hinted: Card = { request_id: "", description: "Plan approval", hint: "Approve it in the TUI" };
  renderThemed(<PermissionCard detail={withCard(hinted)} post={ok} />);
  expect(screen.getByText("Approve it in the TUI")).toBeTruthy();
  expect(screen.queryAllByRole("button")).toHaveLength(0);
});
