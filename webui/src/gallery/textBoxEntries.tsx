import { chakra } from "@chakra-ui/react"
import { TextBox } from "../ui/text-box/TextBox"
import type { GalleryEntry } from "./registry"

const caret = <span aria-hidden="true">▾</span>

const expand = (
  <chakra.button
    type="button"
    aria-label="Expand"
    w="36px"
    h="36px"
    borderRadius="8px"
    fontSize="20px"
    cursor="pointer"
    _hover={{ bg: "surface.hover", color: "text" }}
  >
    ⤢
  </chakra.button>
)

export const textBoxEntries: GalleryEntry[] = [
  {
    name: "TextBox",
    cases: [
      { label: "prose boxed", node: <TextBox kind="prose" label="Note" name="note" rows={3} /> },
      {
        label: "prose bare",
        node: (
          <TextBox
            kind="prose"
            look="bare"
            label="Prompt"
            name="prompt"
            rows={2}
            placeholder="What should happen?"
          />
        ),
      },
      {
        label: "prose bare, page",
        node: (
          <TextBox
            kind="prose"
            look="bare"
            page
            label="Brief"
            name="brief"
            rows={3}
            defaultValue="A page that is one big box, with roomy spacing and taller lines."
          />
        ),
      },
      {
        label: "line, optional",
        node: <TextBox kind="line" label="Title" name="title" optional placeholder="Title" />,
      },
      {
        label: "line verbatim",
        node: (
          <TextBox kind="line" label="Slug" name="slug" verbatim defaultValue="text-box-standard" />
        ),
      },
      {
        label: "line secret",
        node: <TextBox kind="line" label="Token" name="token" secret defaultValue="hunter2" />,
      },
      {
        label: "search, hidden label",
        node: (
          <TextBox kind="search" label="Search epics" name="query" hideLabel placeholder="Search" />
        ),
      },
      {
        label: "search bare",
        node: (
          <TextBox
            kind="search"
            look="bare"
            label="Search history"
            name="history-query"
            hideLabel
            placeholder="Search history"
          />
        ),
      },
      {
        label: "credential with hint",
        node: (
          <TextBox
            kind="credential"
            label="New password"
            name="new-password"
            autoComplete="new-password"
            size="lg"
            hint="At least 12 characters."
          />
        ),
      },
      {
        label: "size sm",
        node: <TextBox kind="line" label="Small" name="small" size="sm" defaultValue="32px" />,
      },
      {
        label: "size md",
        node: <TextBox kind="line" label="Medium" name="medium" size="md" defaultValue="40px" />,
      },
      {
        label: "size lg",
        node: <TextBox kind="line" label="Large" name="large" size="lg" defaultValue="44px" />,
      },
      {
        label: "size title",
        node: (
          <TextBox
            kind="line"
            look="bare"
            size="title"
            label="Title"
            name="page-title"
            hideLabel
            defaultValue="A long page title that is cut with an ellipsis until you focus it"
          />
        ),
      },
      {
        label: "invalid",
        node: (
          <TextBox kind="line" label="Title" name="broken-title" invalid defaultValue="Wrong" />
        ),
      },
      {
        label: "disabled",
        node: <TextBox kind="line" label="Title" name="locked-title" disabled value="Locked" />,
      },
      {
        label: "monospace",
        node: (
          <TextBox kind="line" label="Refs" name="refs" verbatim mono defaultValue="main..HEAD" />
        ),
      },
      {
        label: "end element, caret",
        node: (
          <TextBox
            kind="line"
            label="Folder"
            name="folder"
            verbatim
            defaultValue="~/dev/a/very/long/path/that/never/runs/under/the/caret/at/the/end"
            endElement={caret}
          />
        ),
      },
      {
        label: "end element, button",
        node: (
          <TextBox
            kind="prose"
            label="Details"
            name="details"
            rows={3}
            defaultValue="Text wraps before it reaches the expand button at the top end of this box."
            endElement={expand}
          />
        ),
      },
    ],
  },
]
