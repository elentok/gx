import https from 'node:https'; import fs from 'node:fs';
export const log = [];
const COOKIE = '__Host-probe_login';
const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4z8DwHwAFAAH/q842iQAAAABJRU5ErkJggg==', 'base64');
const app = `
const R = (window.__results = {origin: String(location.origin), opaque: location.origin === 'null', scriptLoaded: true, selfOrigin: String(self.origin), isolatedOrigin: String(self.crossOriginIsolated)}); try { localStorage.getItem('x'); R.localStorage = 'allowed'; } catch (e) { R.localStorage = 'blocked: ' + e.name; } try { R.docCookie = document.cookie === '' ? 'empty' : 'visible'; } catch (e) { R.docCookie = 'blocked: ' + e.name; }
window.addEventListener('load', () => { R.cssApplied = getComputedStyle(document.getElementById('box')).color === 'rgb(255, 0, 0)'; R.extScript = !!window.__extLoaded; });
const im = document.getElementById('im');
const done = (async () => {
  async function t(name, url, opts) {
    try { const r = await fetch(url, opts); const body = await r.text(); R[name] = {status: r.status, body: body.slice(0, 60)}; }
    catch (e) { R[name] = {error: String(e)}; }
  }
  await t('fetchRelative', 'data.json');
  await t('fetchApi', '/v1/api.json');
  await t('fetchApiInclude', '/v1/api.json?inc', {credentials: 'include'});
  await t('postWrite', '/v1/write', {method: 'POST', body: '{}'});
  await t('postWriteInclude', '/v1/write?inc', {method: 'POST', body: '{}', credentials: 'include'});
  await new Promise(r => { if (im.complete) r(); else { im.onload = im.onerror = r; setTimeout(r, 1500); } }); R.imgLoaded = im.complete && im.naturalWidth > 0;
  window.__done = true;
})();`;
const page = () => `<!doctype html><meta charset=utf-8><link rel=stylesheet href="style.css"><body><img id=im src="img.png"><div id=box>box</div>
<script src="app.js"></script><script>${app}</script><script type=module src="mod.js"></script>`;
const srv = https.createServer({key: fs.readFileSync('key.pem'), cert: fs.readFileSync('cert.pem')}, (req, res) => {
  let u0 = new URL(req.url, 'https://x'); const tok = u0.pathname.startsWith('/t/abc/'); if (tok) req.url = req.url.slice(6); const hasCookie = tok || (req.headers.cookie || '').includes(COOKIE + '=1'); const viaToken = tok; const url = new URL(req.url, 'https://x');
  const entry = {viaToken: tok, method: req.method, path: req.url, cookie: hasCookie, origin: req.headers.origin ?? null,
    site: req.headers['sec-fetch-site'] ?? null, dest: req.headers['sec-fetch-dest'] ?? null, mode: req.headers['sec-fetch-mode'] ?? null}; log.push(entry); res.on('finish', () => { entry.status = res.statusCode; }); if (tok) res.setHeader('access-control-allow-origin', '*');
  const p = url.pathname;
  if (p === '/login') { res.writeHead(200, {'Set-Cookie': `${COOKIE}=1; Path=/; Secure; HttpOnly; SameSite=Lax`}); return res.end('ok'); }
  if (!hasCookie) { res.writeHead(401); return res.end('no cookie'); }
  if (p === '/v1/files/d/page.html') { res.writeHead(200, {'content-type': 'text/html', 'content-security-policy': 'sandbox allow-scripts'}); return res.end(page()); }
  if (p === '/v1/files/d/style.css') { res.writeHead(200, {'content-type': 'text/css'}); return res.end('#box{color:red}'); }
  if (p === '/v1/files/d/img.png') { res.writeHead(200, {'content-type': 'image/png'}); return res.end(png); }
  if (p === '/v1/files/d/app.js') { res.writeHead(200, {'content-type': 'text/javascript'}); return res.end('window.__extLoaded=true'); }
  if (p === '/v1/files/d/mod.js') { res.writeHead(200, {'content-type': 'text/javascript'}); return res.end('window.__modLoaded=true'); }
  if (p === '/v1/files/d/data.json' || p === '/v1/api.json') { res.writeHead(200, {'content-type': 'application/json'}); return res.end('{"secret":"gx-data"}'); }
  if (p === '/v1/write') {
    const o = req.headers.origin; let ok = false; try { ok = !!o && new URL(o).host === req.headers.host; } catch {}
    res.writeHead(ok ? 200 : 403); return res.end(ok ? 'written' : 'origin check failed');
  }
  if (p === '/parent') { res.writeHead(200, {'content-type': 'text/html'}); return res.end(`<!doctype html><iframe id=f sandbox="allow-scripts" src="${url.searchParams.get('src')}"></iframe>`); }
  res.writeHead(404); res.end('nf');
});
export const start = () => new Promise(r => srv.listen(0, '127.0.0.1', () => r(srv.address().port)));
export const stop = () => srv.close();
