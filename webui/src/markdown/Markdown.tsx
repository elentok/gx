import { Box, type BoxProps } from "@chakra-ui/react";
import { memo, useMemo } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

const inlineElements = ["strong", "em", "del", "code", "a"];

const remarkPlugins = [remarkGfm];

const siblingTicketLink = /^(?:issues\/)?(\d+)-[^/]*\.md$/;

function linkComponent(epic: string | undefined): Components["a"] {
  return function MarkdownLink({ href, children }) {
    if (href && /^https?:\/\//i.test(href)) {
      return (
        <a href={href} target="_blank" rel="noopener noreferrer">
          {children}
        </a>
      );
    }
    const id = href?.match(siblingTicketLink)?.[1];
    if (epic && id) return <a href={`#/t/${epic}/${id}`}>{children}</a>;
    return <>{children}</>;
  };
}

// Server text written in markdown, such as an epic summary or a ticket body. react-markdown builds
// React elements and ignores raw HTML, so the text cannot inject markup. `epic` is the "project:epic"
// address that sibling ticket links resolve against. `smallHeadings` shrinks its own `##` headings
// below the page's section headings. `inline` keeps only inline markup inside a span, for text that
// sits in a button or a line, such as an option label. It is memoized: parsing is slow, and a page
// that re-renders on each keystroke (a Grilling round) would re-parse every card.
export const Markdown = memo(function Markdown({
  children,
  epic,
  smallHeadings,
  inline,
  ...box
}: { children: string; epic?: string; smallHeadings?: boolean; inline?: boolean } & BoxProps) {
  const components = useMemo(() => ({ a: linkComponent(epic) }), [epic]);
  return (
    <Box
      as={inline ? "span" : undefined}
      {...box}
      data-small-headings={smallHeadings ? "" : undefined}
      css={{
        "&[data-small-headings] h2": { fontSize: "md", fontWeight: "600" },
        "& table": { borderCollapse: "collapse", marginBlock: "2" },
        "& th, & td": { border: "1px solid {colors.border}", paddingInline: "2", paddingBlock: "1" },
        "& > :first-child": { marginTop: 0 },
        "& > :last-child": { marginBottom: 0 },
        "& p, & ul, & ol": { marginBlock: "2" },
        "& ul, & ol": { paddingInlineStart: "6" },
        "& ul": { listStyleType: "disc" },
        "& ol": { listStyleType: "decimal" },
        "& strong": { fontWeight: "600" },
        "& a": { color: "{colors.tone.done.fg}", textDecoration: "underline" },
        "& code": {
          background: "{colors.surface.raised}",
          borderRadius: "{radii.sm}",
          paddingInline: "1",
          fontSize: "0.9em",
        },
        "& pre": {
          background: "{colors.surface.raised}",
          borderRadius: "{radii.md}",
          paddingInline: "3",
          paddingBlock: "2",
          marginBlock: "2",
          overflowX: "auto",
          lineHeight: "1.5",
        },
        "& pre code": { background: "none", padding: 0, fontSize: "0.85em" },
      }}
    >
      <ReactMarkdown
        remarkPlugins={remarkPlugins}
        components={components}
        allowedElements={inline ? inlineElements : undefined}
        unwrapDisallowed={inline}
      >
        {children}
      </ReactMarkdown>
    </Box>
  );
});
