/** A calendar date like "Nov 11, 2026" in the viewer's locale. */
export const formatDate = (iso: string) => new Date(iso).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
