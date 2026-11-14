import {createRequire} from 'node:module'; import {start, stop, log} from './server.mjs';
const require = createRequire('/Users/david/.local/share/fnm/aliases/default/lib/node_modules/@playwright/cli/node_modules/playwright-core/');
const pw = require('./index.js');
const port = await start();
async function run(engineName, mode) {
  const browser = await pw[engineName].launch();
  const ctx = await browser.newContext({ignoreHTTPSErrors: true});
  const pg = await ctx.newPage();
  await pg.goto(`https://localhost:${port}/login`);
  const cookies = (await ctx.cookies()).map(c => `${c.name} secure=${c.secure} httpOnly=${c.httpOnly} sameSite=${c.sameSite}`);
  log.length = 0;
  let target;
  const pre = mode.endsWith('token') ? '/t/abc' : '';
  if (mode.startsWith('top')) { await pg.goto(`https://localhost:${port}${pre}/v1/files/d/page.html`); target = pg; }
  else { await pg.goto(`https://localhost:${port}/parent?src=${pre}/v1/files/d/page.html`); await pg.waitForTimeout(1500); target = pg.frames().find(f => f !== pg.mainFrame()); }
  let results; 
  try { for (let i = 0; i < 60 && !(await target.evaluate(() => !!window.__done)); i++) await pg.waitForTimeout(150); results = await target.evaluate(() => ({...window.__results, modLoaded: !!window.__modLoaded})); } catch (e) { results = {err: String(e)}; }
  await pg.waitForTimeout(300);
  const out = {engine: engineName, mode, version: browser.version(), cookies, results, server: log.slice()};
  await browser.close(); return out;
}
const all = [];
for (const e of ['chromium', 'webkit']) for (const m of ['top', 'iframe', 'top-token', 'iframe-token']) all.push(await run(e, m));
console.log(JSON.stringify(all, null, 1)); stop();
