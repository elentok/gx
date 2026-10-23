import { Box, type BoxProps } from "@chakra-ui/react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

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
// below the page's section headings.
export function Markdown({
  children,
  epic,
  smallHeadings,
  ...box
}: { children: string; epic?: string; smallHeadings?: boolean } & BoxProps) {
  return (
    <Box
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
      }}
    >
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={{ a: linkComponent(epic) }}>
        {children}
      </ReactMarkdown>
    </Box>
  );
}
