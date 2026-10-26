function copyViaTextarea(text: string): boolean {
  const area = document.createElement("textarea");
  area.value = text;
  area.style.position = "fixed";
  area.style.opacity = "0";
  document.body.appendChild(area);
  area.select();
  try {
    return document.execCommand("copy");
  } finally {
    area.remove();
  }
}

function selectNode(node: Node | null) {
  if (!node) return;
  const range = document.createRange();
  range.selectNodeContents(node);
  const selection = window.getSelection();
  selection?.removeAllRanges();
  selection?.addRange(range);
}

/** Copies text; when every way fails, selects `fallback` so the person can copy by hand. Returns whether it copied. */
export async function copyText(text: string, fallback: Node | null): Promise<boolean> {
  try {
    // navigator.clipboard only exists in a secure context; plain-HTTP LAN addresses lack it.
    if (navigator.clipboard) await navigator.clipboard.writeText(text);
    else if (!copyViaTextarea(text)) throw new Error("copy command rejected");
    return true;
  } catch {
    selectNode(fallback);
    return false;
  }
}
