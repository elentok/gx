import { Box, type BoxProps } from "@chakra-ui/react";
import ReactMarkdown from "react-markdown";

// Server text written in markdown, such as an epic summary. react-markdown builds React elements and
// ignores raw HTML, so the text cannot inject markup.
export function Markdown({ children, ...box }: { children: string } & BoxProps) {
  return (
    <Box
      {...box}
      css={{
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
      <ReactMarkdown>{children}</ReactMarkdown>
    </Box>
  );
}
