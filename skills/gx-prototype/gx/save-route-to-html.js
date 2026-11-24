async page => {
  const html = await page.evaluate(async () => {
    const rules = [];
    for (const ss of document.styleSheets) { try { for (const r of ss.cssRules) rules.push(r.cssText) } catch {} }
    let css = rules.join('\n');
    // inline every font url as a data: URI
    const urls = [...new Set([...css.matchAll(/url\("?([^")]+\.woff2[^")]*)"?\)/g)].map(m => m[1]))];
    for (const u of urls) {
      const buf = await (await fetch(u)).arrayBuffer();
      let b = ''; const a = new Uint8Array(buf); for (let i = 0; i < a.length; i += 8192) b += String.fromCharCode(...a.subarray(i, i + 8192));
      css = css.split(u).join('data:font/woff2;base64,' + btoa(b));
    }
    const root = document.getElementById('root').cloneNode(true);
    root.querySelectorAll("script").forEach((s) => s.remove());
    return '<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>PROTOTYPE B saved</title><style data-mock-inlined>' + css + '</style></head><body class="' + document.body.className + '"><div id="root">' + root.innerHTML + '</div></body></html>';
  });
  return html;
}
