const assert = require("node:assert/strict");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { fixture } = require("./fixtures/caller-keys.cjs");

// No gateway key yet (HarriesChen on Discord, 10-25): magpie web on a phone
// drew "No gateway keys yet" in the body's text, 14px and dark, flush
// against its card's edges. It is said in its card the way the same page's
// Recent calls says it has none: muted, small, inset.
const NONE = { en: "No gateway keys yet", zh: "尚无网关密钥", "zh-TW": "尚無閘道金鑰", ja: "ゲートウェイキーはまだありません", de: "Noch keine Zugangsschlüssel" };

const look = (page) => page.evaluate(() => {
  const none = document.querySelector("#gatewayKeys .none");
  const calls = document.querySelector("#activity .none");
  const card = none.parentElement.getBoundingClientRect();
  const r = document.createRange();
  r.selectNodeContents(none);
  const text = r.getBoundingClientRect();
  const cs = getComputedStyle(none), ref = getComputedStyle(calls);
  return {
    inCard: none.parentElement.matches("#gatewayKeys > .accts"),
    text: none.textContent,
    color: cs.color, refColor: ref.color,
    size: cs.fontSize, refSize: ref.fontSize,
    weight: cs.fontWeight,
    inset: Math.min(text.left - card.left, card.right - text.right, text.top - card.top, card.bottom - text.bottom),
    overflow: document.documentElement.scrollWidth - innerWidth,
  };
});

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) {
    for (const theme of ["light", "dark"]) {
      test(`${engine} ${lang} ${theme}: no gateway key is said like the page's other empty lists`, async (t) => {
        const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
        t.after(() => browser.close());
        const page = await (await browser.newContext({ viewport: { width: 390, height: 844 }, reducedMotion: "reduce", colorScheme: theme })).newPage();
        page.setDefaultTimeout(6000);
        const errors = [];
        page.on("pageerror", (e) => errors.push(e.message));
        await page.route("**/*", fixture(lang, theme, [], { lan: true, web: true, keys: [] }));
        await page.goto("http://magpie.test/?view=gateway");
        await page.locator("#gatewayKeys .none").waitFor();
        await page.locator("#activity .none").waitFor();
        const s = await look(page);
        assert.equal(s.inCard, true, "said inside the list's card");
        assert.equal(s.text, NONE[lang]);
        assert.equal(s.color, s.refColor, "muted, as Recent calls' empty line");
        assert.equal(s.size, s.refSize, "the size of Recent calls' empty line");
        assert.equal(s.weight, "400");
        assert(s.inset >= 10, `inset from the card's edges, got ${s.inset}px`);
        assert(s.overflow <= 0, "no sideways scroll at 390px");
        assert.deepEqual(errors, []);
      });
    }
  }
}
