/** The jump menu title's size by layout. */
export const titleSize = (wide: boolean) =>
  wide
    ? ({ fontSize: "30px", lineHeight: "1.1", overflowWrap: "anywhere" } as const)
    : ({ fontSize: "24px", lineHeight: "1.15", overflowWrap: "anywhere" } as const);
