async page => {
  const html = await page.evaluate(async () => {
    const rules = [];
    // Chrome drops the var() text of a shorthand that a longhand overrides, so cssText holds `padding-right: ;`.
    // The raw text is gone from the CSSOM, so fill each empty declaration from a matching element's computed value.
    const ser = (r) => {
      if (r.cssRules && !r.style) return r.cssText.slice(0, r.cssText.indexOf('{') + 1) + [...r.cssRules].map(ser).join(' ') + ' }';
      if (!r.style || ![...r.style].some((p) => r.style.getPropertyValue(p) === '')) return r.cssText;
      let el = null;
      try { el = document.querySelector(r.selectorText) } catch {}
      const decls = [...r.style].map((p) => {
        let v = r.style.getPropertyValue(p);
        if (v === '' && el) v = getComputedStyle(el).getPropertyValue(p);
        return v === '' ? '' : p + ': ' + v + (r.style.getPropertyPriority(p) ? ' !important' : '') + ';';
      });
      return r.selectorText + ' { ' + decls.filter(Boolean).join(' ') + ' }';
    };
    for (const ss of document.styleSheets) { try { for (const r of ss.cssRules) rules.push(ser(r)) } catch {} }
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
    // inline every img src as a data: URI, so the saved file shows it when served from another origin
    for (const img of root.querySelectorAll('img[src]')) {
      const src = img.getAttribute('src');
      if (src.startsWith('data:')) continue;
      try {
        const r = await fetch(src); const blob = await r.blob();
        img.setAttribute('src', await new Promise((ok, no) => { const f = new FileReader(); f.onload = () => ok(f.result); f.onerror = no; f.readAsDataURL(blob); }));
      } catch {}
    }
    return '<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>PROTOTYPE B saved</title><style data-mock-inlined>' + css + '</style></head><body class="' + document.body.className + '"><div id="root">' + root.innerHTML + '</div></body></html>';
  });
  return html;
}
