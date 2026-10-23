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

it("renders GFM task lists as read-only checkboxes, tables and strikethrough", () => {
  const { container } = renderThemed(
    <Markdown>{"- [x] done\n- [ ] todo\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n~~gone~~"}</Markdown>,
  );
  const boxes = container.querySelectorAll<HTMLInputElement>("input[type=checkbox]");
  expect(boxes).toHaveLength(2);
  expect(boxes[0].checked).toBe(true);
  expect(boxes[0].disabled).toBe(true);
  expect(container.querySelector("table td")?.textContent).toBe("1");
  expect(container.querySelector("del")?.textContent).toBe("gone");
});

it("links sibling ticket files to the ticket page and leaves other relative links as text", () => {
  const { container } = renderThemed(
    <Markdown epic="gx:my-epic">
      {"[a](issues/03-foo.md) [b](04-bar.md) [c](../README.md) [d](https://example.com)"}
    </Markdown>,
  );
  const links = Array.from(container.querySelectorAll("a"));
  expect(links.map((a) => a.getAttribute("href"))).toEqual([
    "#/t/gx:my-epic/03",
    "#/t/gx:my-epic/04",
    "https://example.com",
  ]);
  expect(links[0].hasAttribute("target")).toBe(false);
  expect(links[2].getAttribute("target")).toBe("_blank");
  expect(links[2].getAttribute("rel")).toContain("noopener");
  expect(container.textContent).toContain("c");
});

it("renders relative links as plain text without an epic address", () => {
  const { container } = renderThemed(<Markdown>{"[a](issues/03-foo.md)"}</Markdown>);
  expect(container.querySelector("a")).toBeNull();
  expect(container.textContent).toBe("a");
});

it("renders its h2 headings smaller when asked", () => {
  const { container } = renderThemed(<Markdown smallHeadings>{"## Title"}</Markdown>);
  const h2 = container.querySelector("h2");
  expect(h2?.textContent).toBe("Title");
  expect(h2?.parentElement?.getAttribute("data-small-headings")).toBe("");
});

it("does not turn raw HTML into elements", () => {
  const { container } = renderThemed(<Markdown>{'<img src=x onerror="alert(1)"> hi'}</Markdown>);
  expect(container.querySelector("img")).toBeNull();
});
