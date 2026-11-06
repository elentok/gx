/** The shared Design and Epic header's title size; other pages keep `titleSize`. */
export const headerTitleFontSize = (wide: boolean) => (wide ? "34px" : "26px");

/** The page title's size by layout, shared by the plain h1 and the jump menu title. */
export const titleSize = (wide: boolean) =>
  wide
    ? ({ fontSize: "30px", lineHeight: "1.1", overflowWrap: "anywhere" } as const)
    : ({ fontSize: "24px", lineHeight: "1.15", overflowWrap: "anywhere" } as const);
