// Usage: node probe.mjs   (headed Chromium + headed WebKit via Playwright; writes shots/ and prints JSON)
import {createRequire} from 'node:module'; import http from 'node:http'; import fs from 'node:fs';
const require = createRequire('/Users/david/.local/share/fnm/aliases/default/lib/node_modules/@playwright/cli/node_modules/playwright-core/');
const pw = require('./index.js');
const here = new URL('.', import.meta.url).pathname;
fs.mkdirSync(here + 'shots', {recursive: true});
function pdf() { // minimal one-page PDF, big red-free text "PDFPROBE"
  const s = 'BT /F1 72 Tf 50 400 Td (PDFPROBE) Tj ET';
  const o = ['<</Type/Catalog/Pages 2 0 R>>','<</Type/Pages/Kids[3 0 R]/Count 1>>',
   '<</Type/Page/Parent 2 0 R/MediaBox[0 0 400 600]/Contents 4 0 R/Resources<</Font<</F1 5 0 R>>>>>>',
   `<</Length ${s.length}>>\nstream\n${s}\nendstream`,'<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>'];
  let out = '%PDF-1.4\n', off = [];
  o.forEach((b, i) => { off.push(out.length); out += `${i+1} 0 obj\n${b}\nendobj\n`; });
  const x = out.length; out += `xref\n0 6\n0000000000 65535 f \n` + off.map(n => String(n).padStart(10,'0') + ' 00000 n \n').join('');
  out += `trailer\n<</Size 6/Root 1 0 R>>\nstartxref\n${x}\n%%EOF\n`; return Buffer.from(out, 'latin1');
}
const PDF = pdf();
const variants = {
  none: {},
  sandbox: {'Content-Security-Policy': 'sandbox allow-scripts'},
  sandbox_fa: {'Content-Security-Policy': "sandbox allow-scripts; frame-ancestors 'self'"},
  sandbox_dl: {'Content-Security-Policy': 'sandbox allow-scripts allow-downloads'},
  attachment: {'Content-Security-Policy': 'sandbox allow-scripts', 'Content-Disposition': 'attachment; filename="probe.pdf"'},
};
const srv = http.createServer((req, res) => {
  const u = new URL(req.url, 'http://x');
  const m = u.pathname.match(/^\/pdf\/(\w+)$/);
  if (m) { res.writeHead(200, {'Content-Type': 'application/pdf', ...variants[m[1]]}); return res.end(PDF); }
  const mode = u.searchParams.get('mode'), src = u.searchParams.get('src');
  const body = {iframe_nosb: `<iframe src="${src}" width=500 height=700></iframe>`, iframe: `<iframe sandbox="allow-scripts" src="${src}" width=500 height=700></iframe>`,
    embed: `<embed type="application/pdf" src="${src}" width=500 height=700>`,
    object: `<object type="application/pdf" data="${src}" width=500 height=700></object>`}[mode];
  res.writeHead(200, {'Content-Type': 'text/html'}); res.end(`<!doctype html><body>${body}`);
});
await new Promise(r => srv.listen(0, '127.0.0.1', r)); const port = srv.address().port; const base = `http://127.0.0.1:${port}`;

// "dark ink" = fraction of dark pixels in screenshot is unreliable w/o decoder; so judge by: viewer signals + screenshot reviewed by eye.
async function one(engine, variant, kind) {
  const browser = await pw[engine].launch({headless: false});
  const ctx = await browser.newContext({acceptDownloads: true});
  const pg = await ctx.newPage();
  const downloads = []; pg.on('download', d => downloads.push(d.suggestedFilename()));
  const src = `/pdf/${variant}`;
  const url = kind === 'top' ? base + src : `${base}/parent?mode=${kind}&src=${src}`;
  let navErr = null;
  try { await pg.goto(url, {waitUntil: 'load', timeout: 8000}); } catch (e) { navErr = String(e).split('\n')[0]; }
  await pg.waitForTimeout(2500);
  const frames = pg.frames().map(f => f.url());
  const dom = await pg.evaluate(() => ({embeds: document.querySelectorAll('embed,object').length, iframes: document.querySelectorAll('iframe').length, title: document.title, ct: document.contentType})).catch(e => ({err: String(e).split('\n')[0]}));
  const shot = `shots/${engine}-${variant}-${kind}.png`;
  await pg.screenshot({path: here + shot}).catch(() => {});
  const out = {engine, version: browser.version(), variant, kind, navErr, downloads, frames, dom, shot};
  await browser.close(); return out;
}
const all = [];
for (const engine of ['chromium', 'webkit'])
  for (const variant of Object.keys(variants))
    for (const kind of ['top', 'iframe', 'iframe_nosb', 'embed', 'object']) {
      if (kind === 'iframe_nosb' && variant !== 'sandbox' && variant !== 'none') continue;
      if (kind !== 'top' && kind !== 'iframe' && kind !== 'iframe_nosb' && variant !== 'sandbox' && variant !== 'none') continue;
      all.push(await one(engine, variant, kind));
    }
fs.writeFileSync(here + 'results.json', JSON.stringify(all, null, 1)); console.log(JSON.stringify(all.map(r => ({e: r.engine, v: r.variant, k: r.kind, dl: r.downloads, err: r.navErr, frames: r.frames.map(f => f.slice(0, 70)), dom: r.dom})), null, 0).replace(/\},\{/g, '},\n{'));
srv.close();
