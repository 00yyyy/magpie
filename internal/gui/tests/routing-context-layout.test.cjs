// The context header is lazy and stable; Routing keeps its full story.
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const { test } = require('node:test');
const { chromium, webkit } = require('playwright');
const assets = process.env.MAGPIE_CONTEXT_ASSETS || path.resolve(__dirname, '../assets');
const now = new Date().toISOString();
const account = { id: 'codex', provider: 'codex', name: 'Codex', who: 'demo@example.com', icon: 'codex-color', kind: 'account', model: 'gpt-6-luna' };
const prompt = { window: 272000, tokens: 43900, counted: true, turns: 1,
  parts: [{ kind: 'system', tokens: 10000, items: [{ name: 'prompt', tokens: 10000 }] },
    { kind: 'tools', tokens: 9500, items: [{ name: 'exec_command', tokens: 9500 }] },
    { kind: 'memory', tokens: 5300, items: [{ name: '/demo/AGENTS.md', tag: 'project', tokens: 5300 }] },
    { kind: 'results', tokens: 14500, items: [{ name: 'exec_command', tag: 'result', n: 4, tokens: 14500 }] },
    { kind: 'chat', tokens: 4600, items: [{ name: '1', tag: 'turn', tokens: 4600 }] }] };
function request(id, extra = {}) {
  return { id, seq: id, time: now, agent: 'codex', model: 'codex/gpt-6-luna', provider: 'codex',
    kind: 'ambient_suggestions', order: [account], tries: [{ id: account.id, model: account.model, effort: 'medium', start: now, done: true, status: 200, ms: 5900, ttft: 5700 }],
    done: true, status: 200, ms: 5900, ttft: 5700, tokens: 44100, out: 200, prompt,
    usage: [{ in: 3200, cache_read: 40700, out: 200 }], ...extra };
}
function serve(lang, feed) {
  return async (r) => {
    const url = new URL(r.request().url()), json = (data) => r.fulfill({ json: data });
    if (url.pathname === '/boot.js') return r.fulfill({ contentType: 'text/javascript', body: `window.bootPrefs={lang:"${lang}",theme:"dark",web:true};` });
    if (url.pathname === '/wails/runtime.js') return r.fulfill({ contentType: 'text/javascript', body: 'export const Window={};' });
    if (url.pathname === '/api/state') return json({ agents: [{ id: 'codex', name: 'Codex', fields: [] }], profiles: [], settings: { lang, theme: 'dark' } });
    if (url.pathname === '/api/gateway/trace') {
      if (!url.searchParams.has('wait')) { feed.count = 1; feed.next = null; }
      const routes = url.searchParams.has('wait') ? await new Promise((resolve) => { feed.next = resolve; }) : [request(100)];
      return json({ mine: true, now, seq: routes[0]?.id || 100, totals: { requests: 1, rerouted: 0, errors: 0 }, routes });
    }
    if (url.pathname === '/api/gateway/history') return json({ days: [], routes: [] });
    if (url.pathname === '/api/providers') return json({ providers: [], presets: [], excluded: [], gateway: { running: true, window: true } });
    if (url.pathname === '/api/groups') return json({ groups: [] });
    if (url.pathname.startsWith('/api/')) return json({});
    const file = path.join(assets, url.pathname === '/' ? 'index.html' : url.pathname);
    await r.fulfill({ body: await fs.readFile(file), contentType: { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.png': 'image/png' }[path.extname(file)] });
  };
}
for (const engine of process.env.BROWSER ? [process.env.BROWSER] : ['chromium', 'webkit']) {
  for (const lang of ['en', 'zh', 'zh-TW', 'ja', 'de']) {
    for (const width of [2048, 1440, 900, 860, 420, 360]) {
      test(`${engine} ${lang} ${width}px: a single optional context header preserves the routing story`, async (t) => {
        const browser = await (engine === 'webkit' ? webkit.launch() : chromium.launch({ channel: 'chromium' }));
        t.after(() => browser.close());
        const page = await browser.newPage({ viewport: { width, height: 1250 }, reducedMotion: 'reduce' });
        page.setDefaultTimeout(5000);
        const feed = {}, errors = [];
        page.on('pageerror', (e) => errors.push(e.message));
        await page.route('**/*', serve(lang, feed));
        await page.goto('http://magpie.test/?view=routing');
        const toggle = page.locator('.rt-ctx-toggle');
        await page.locator('.rt-ctx').waitFor();
        assert.equal(await page.locator('.rt-ctx .ctx-waffle').count(), 0, 'closed context does not build the detail grid');
        assert.equal(await toggle.getAttribute('aria-expanded'), 'false');
        assert.match(await toggle.innerText(), /43\.9K.*272K.*16%.*93%/s);
        assert.equal(await page.locator('.rt-cap').isVisible(), true, 'the stage caption stays visible');
        assert.equal(await page.locator('.rt-steps li.why').isVisible(), true, 'routing reasons stay visible by default');
        const translated = await page.evaluate(() => t('How the last request was routed'));
        assert.ok((await page.locator('.rt-log-head').innerText()).includes(translated), 'the descriptive story heading is preserved');
        await page.waitForTimeout(500);
        await toggle.hover();
        const before = await toggle.evaluate((e) => e.getBoundingClientRect().top);
        await toggle.click();
        await page.locator('.rt-ctx .ctx-waffle').waitFor();
        assert.equal(await page.locator('.rt-ctx .ctx-title').count(), 1, 'one header serves both states');
        assert.equal(await page.locator('.ctx-summary-numbers').count(), 0, 'expanded figures appear only in the body');
        assert.equal(await page.locator('.ctx-waffle > i').count(), 400);
        assert.equal(await toggle.getAttribute('aria-expanded'), 'true');
        assert.ok(Math.abs(await toggle.evaluate((e) => e.getBoundingClientRect().top) - before) < 1, `opening keeps the header under the pointer: ${before} -> ${await toggle.evaluate((e) => e.getBoundingClientRect().top)}`);
        await page.waitForTimeout(100);
        const layout = await page.evaluate(() => {
          const root = document.querySelector('.rt'), log = root.querySelector('.rt-log').getBoundingClientRect();
          const stage = root.querySelector('.rt-stage').getBoundingClientRect(), contents = root.querySelector('.ctx-contents').getBoundingClientRect();
          const head = root.querySelector('.ctx-contents-head');
          return { wide: root.classList.contains('min1100'), pair: root.classList.contains('min800'),
            log: log.left, stage: stage.right, separator: contents.left,
            nowrap: getComputedStyle(head).flexWrap, over: document.documentElement.scrollWidth > innerWidth };
        });
        if (layout.wide) {
          assert.ok(layout.log >= layout.stage, 'stage and request story share a row');
          assert.ok(Math.abs(layout.log - layout.separator) <= 1, 'both column separators align');
        }
        assert.equal(layout.nowrap, 'nowrap', 'content label and filters share a row');
        assert.equal(layout.over, false, 'the page fits the window');
        await toggle.click();
        assert.equal(await page.locator('.ctx-waffle').count(), 0, 'closing releases detail contents');
        await toggle.focus();
        await page.keyboard.press('Enter');
        await page.locator('.ctx-waffle').waitFor();
        assert.equal(await page.evaluate(() => localStorage.getItem('magpie.routingContext')), '1');
        await page.reload();
        await page.locator('.ctx-waffle').waitFor();
        assert.equal(await page.locator('.rt-ctx-toggle').getAttribute('aria-expanded'), 'true', 'reload restores the preference');
        if (width === 900) {
          for (let i = 0; i < 60 && !feed.next; i++) await page.waitForTimeout(50);
          assert.ok(feed.next, 'the next trace request is waiting');
          const next = feed.next; feed.next = null;
          next([request(101, { done: false, prompt: undefined, usage: [], tries: [] })]);
          await page.waitForFunction(() => document.querySelectorAll('.rt-req').length === 2);
          await page.locator('.rt-ctx-toggle').click();
          assert.equal(await page.locator('.rt-ctx').isVisible(), true, 'a disclosure keeps the carried context while the new prompt is being read');
          assert.equal(await page.locator('.ctx-waffle').count(), 0);
          await page.locator('.rt-ctx-toggle').click();
          await page.locator('.ctx-waffle').waitFor();
          assert.equal(await page.locator('.rt-ctx-toggle').getAttribute('aria-expanded'), 'true');
        }
        assert.deepEqual(errors, []);
      });
    }
  }
}
