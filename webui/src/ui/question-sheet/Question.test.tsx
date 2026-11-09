import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { renderThemed as render } from "../render";
import { Question } from "./Question";

afterEach(cleanup);

test("a card's links show as new-tab links", () => {
  render(
    <Question
      card={{
        label: "1",
        title: "Pick a variant?",
        kind: "yes-no",
        links: [{ label: "Open the mock", url: "https://claude.ai/artifact/abc" }],
      }}
      answer={{}}
      onChange={vi.fn()}
      onPicked={vi.fn()}
    />,
  );
  const link = screen.getByRole("link", { name: /Open the mock/ });
  expect(link.getAttribute("href")).toBe("https://claude.ai/artifact/abc");
  expect(link.getAttribute("target")).toBe("_blank");
});
