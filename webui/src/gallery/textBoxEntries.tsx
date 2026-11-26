import { TextBox } from "../ui/text-box/TextBox"
import type { GalleryEntry } from "./registry"

export const textBoxEntries: GalleryEntry[] = [
  {
    name: "TextBox",
    cases: [
      { label: "prose", node: <TextBox kind="prose" label="Note" name="note" rows={3} /> },
      { label: "line", node: <TextBox kind="line" label="Title" name="title" optional /> },
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
        label: "credential with hint",
        node: (
          <TextBox
            kind="credential"
            label="New password"
            name="new-password"
            autoComplete="new-password"
            hint="At least 12 characters."
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
    ],
  },
]
