// The name part of a `project:name` address; everything after the first colon.
export function slugOf(address: string): string {
  return address.slice(address.indexOf(":") + 1);
}
