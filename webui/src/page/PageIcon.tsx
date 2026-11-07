const PATHS = {
  epic: "M3 4.5h10M3 8h10M3 11.5h6",
  design: "M10.5 2.5l3 3-7.5 7.5H3v-3l7.5-7.5z",
} as const;

/** The large icon in a PageHeader for the pages that have a kind. */
export function PageIcon({ kind }: { kind: keyof typeof PATHS }) {
  return (
    <svg
      aria-hidden="true"
      data-page-icon={kind}
      viewBox="0 0 16 16"
      width="28"
      height="28"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d={PATHS[kind]} />
    </svg>
  );
}
