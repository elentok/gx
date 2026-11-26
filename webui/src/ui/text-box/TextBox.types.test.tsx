import { expect, test } from "vitest"
import { TextBox } from "./TextBox"

// Never rendered: tsc alone enforces what each kind refuses.
test("the prop types refuse what TextBox sets itself", () => {
  const line = { kind: "line", label: "Note", name: "note" } as const
  const prose = { kind: "prose", label: "Note", name: "note" } as const
  const search = { kind: "search", label: "Note", name: "note" } as const
  const credential = {
    kind: "credential",
    label: "Note",
    name: "note",
    autoComplete: "username",
  } as const
  const refused = (
    <>
      {/* @ts-expect-error label is required */}
      <TextBox kind="line" name="note" />
      {/* @ts-expect-error name is required */}
      <TextBox kind="line" label="Note" />
      {/* @ts-expect-error the label is the only name */}
      <TextBox {...line} aria-label="Other" />
      {/* @ts-expect-error the label is the only name */}
      <TextBox {...line} aria-labelledby="other" />
      {/* @ts-expect-error TextBox draws its own focus style */}
      <TextBox {...line} data-focus-ring="inset" />
      {/* @ts-expect-error TextBox creates the id */}
      <TextBox {...line} id="other" />
      {/* @ts-expect-error the kind picks the type */}
      <TextBox {...line} type="email" />
      {/* @ts-expect-error the kind sets typing help */}
      <TextBox {...line} autoCapitalize="on" />
      {/* @ts-expect-error the kind sets typing help */}
      <TextBox {...line} autoCorrect="on" />
      {/* @ts-expect-error the kind sets typing help */}
      <TextBox {...line} spellCheck />
      {/* @ts-expect-error the kind sets the Enter key */}
      <TextBox {...search} enterKeyHint="go" />
      {/* @ts-expect-error autoComplete is for credential only */}
      <TextBox {...line} autoComplete="username" />
      {/* @ts-expect-error autoComplete is for credential only */}
      <TextBox {...prose} autoComplete="off" />
      {/* @ts-expect-error autoComplete is for credential only */}
      <TextBox {...search} autoComplete="off" />
      {/* @ts-expect-error a credential box needs autoComplete */}
      <TextBox kind="credential" label="Note" name="note" />
      {/* @ts-expect-error not a credential value */}
      <TextBox {...credential} autoComplete="email" />
      {/* @ts-expect-error inputMode is for line and credential */}
      <TextBox {...prose} inputMode="numeric" />
      {/* @ts-expect-error inputMode is for line and credential */}
      <TextBox {...search} inputMode="numeric" />
      {/* @ts-expect-error verbatim is for line only */}
      <TextBox {...search} verbatim />
      {/* @ts-expect-error verbatim is for line only */}
      <TextBox {...prose} verbatim />
      {/* @ts-expect-error verbatim is for line only */}
      <TextBox {...credential} verbatim />
      {/* @ts-expect-error secret is for line only */}
      <TextBox {...search} secret />
      {/* @ts-expect-error secret is for line only */}
      <TextBox {...prose} secret />
      {/* @ts-expect-error secret is for line only */}
      <TextBox {...credential} secret />
      {/* @ts-expect-error rows is for prose only */}
      <TextBox {...line} rows={3} />
      {/* @ts-expect-error no style props */}
      <TextBox {...line} bg="surface" />
      {/* @ts-expect-error no style props */}
      <TextBox {...line} px="3" />
      {/* @ts-expect-error no style props */}
      <TextBox {...line} h="40px" />
      {/* @ts-expect-error no style props */}
      <TextBox {...prose} fontSize="sm" />
      {/* @ts-expect-error no style props */}
      <TextBox {...line} className="x" />
      {/* @ts-expect-error no style props */}
      <TextBox {...line} style={{}} />
      {/* @ts-expect-error the ref follows the kind */}
      <TextBox {...prose} ref={(_: HTMLInputElement | null) => {}} />
    </>
  )
  expect(refused).toBeTruthy()
})
