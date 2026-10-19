import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { App } from "./App";
import canned from "./canned-snapshot.json";
import type { Snapshot } from "./snapshot";

test("shell renders every ticket of the canned snapshot", () => {
  render(
    <ChakraProvider value={defaultSystem}>
      <App snapshot={canned as Snapshot} />
    </ChakraProvider>,
  );
  expect(screen.getByRole("heading", { name: "gx" })).toBeTruthy();
  for (const t of canned.tickets) {
    expect(screen.getByText(new RegExp(t.title))).toBeTruthy();
  }
});
