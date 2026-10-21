import { expect, it } from "vitest";
import { renderThemed } from "../ui/render";
import { Markdown } from "./Markdown";

it("renders emphasis, code, lists and links as elements", () => {
  const { container } = renderThemed(
    <Markdown>{"A **bold** word with `code`.\n\n- one\n- two\n\n[docs](https://example.com)"}</Markdown>,
  );
  expect(container.querySelector("strong")?.textContent).toBe("bold");
  expect(container.querySelector("code")?.textContent).toBe("code");
  expect(container.querySelectorAll("li")).toHaveLength(2);
  expect(container.querySelector("a")?.getAttribute("href")).toBe("https://example.com");
});

it("does not turn raw HTML into elements", () => {
  const { container } = renderThemed(<Markdown>{'<img src=x onerror="alert(1)"> hi'}</Markdown>);
  expect(container.querySelector("img")).toBeNull();
});
