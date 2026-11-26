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
      {/* @ts-expect-error a title is a bare line box */}
      <TextBox {...line} size="title" />
      {/* @ts-expect-error a title is a bare line box */}
      <TextBox {...line} look="boxed" size="title" />
      {/* @ts-expect-error a title is a bare line box */}
      <TextBox {...search} look="bare" size="title" />
      {/* @ts-expect-error a title is a bare line box */}
      <TextBox {...prose} look="bare" size="title" />
      {/* @ts-expect-error a title is a bare line box */}
      <TextBox {...credential} look="bare" size="title" />
      {/* @ts-expect-error page spacing is for bare prose */}
      <TextBox {...prose} page />
      {/* @ts-expect-error page spacing is for bare prose */}
      <TextBox {...line} look="bare" page />
      {/* @ts-expect-error page spacing is size md */}
      <TextBox {...prose} look="bare" page size="lg" />
      {/* @ts-expect-error textarea heights are for prose */}
      <TextBox {...line} minH="0" />
      {/* @ts-expect-error textarea heights are for prose */}
      <TextBox {...search} maxH="40px" />
      {/* @ts-expect-error textarea heights are for prose */}
      <TextBox {...credential} maxHeight="40px" />
      {/* @ts-expect-error resize is for prose */}
      <TextBox {...line} resize="none" />
      {/* @ts-expect-error no style props */}
      <TextBox {...prose} h="40px" />
      {/* @ts-expect-error no style props */}
      <TextBox {...line} borderRadius="0" />
      {/* @ts-expect-error mono is the only font choice */}
      <TextBox {...line} fontFamily="mono" />
    </>
  )
  const accepted = (
    <>
      <TextBox
        {...line}
        w={{ base: "100%", md: "88px" }}
        minW="0"
        maxW="400px"
        m="0"
        my="3"
        marginTop="1"
        flex="1"
        flexGrow={1}
        flexShrink={0}
        flexBasis="auto"
        gridArea="a"
        gridColumn="1 / 3"
        gridRow="1"
        alignSelf="start"
        justifySelf="end"
        order={2}
        mono
        endElement="▾"
      />
      <TextBox {...line} look="bare" size="title" />
      <TextBox {...prose} flex="1" minH="0" maxH="300px" resize="none" />
      <TextBox {...prose} look="bare" page minHeight="0" maxHeight="9" />
      <TextBox {...prose} look="bare" page size="md" />
    </>
  )
  expect(refused).toBeTruthy()
  expect(accepted).toBeTruthy()
})
