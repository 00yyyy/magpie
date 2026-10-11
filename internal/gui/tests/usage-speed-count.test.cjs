// Run with Node's test runner and Playwright on the module path; see README.md.
// One rule for a group's speed, in the Usage page's ranking, its Output speed
// card and the overview's lists (Fallmonkey on Discord): gpt-6.1-sol showed a
// tok/s that was the one reply of its 260 calls that used no tool (the only
// Codex turn its session file times) beside "260 requests", while Claude
// Code, whose rows on the Requests tab say "≥100 tok/s" by the dozen, showed
// none. A group with replies whose speed was measured now says that speed and
// how many replies it is of, "125 tok/s (1)"; one with none, but floors, the
// least they wrote at, "≥100 tok/s (30)"; one with neither, nothing. The
// rows of the Requests tab say each one's the same way. Every language, a
// wide window and a narrow one; no backend, the API is faked.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const reader = require("./reader.cjs");

const assets = path.resolve(__dirname, "../assets");
const now = Date.now();
const at = (i) => new Date(now - (i + 1) * 60e3).toISOString();

const LOG = { in: 9000, cache_read: 0, cache_write: 0, status: 0, cost: 0, priced: false, source: "log", provider: "session-unknown", providerName: "Local session" };
const SOL = { ...LOG, agent: "codex", agentName: "Codex", icon: "codex-color", model: "gpt-6.1-sol", req: "gpt-6.1-sol", served: "gpt-6.1-sol" };
const CC = { ...LOG, agent: "claude", agentName: "Claude Code", icon: "claudecode-color", model: "claude-opus-5-5", req: "claude-opus-5-5", served: "claude-opus-5-5" };
const ROWS = [
  // the one sol reply its session file timed: 500 tokens in the 4 s after its first
  { ...SOL, t: at(0), out: 500, ms: 5000, ttft_ms: 1000 },
  // a sol call in a turn of several: no speed
  { ...SOL, t: at(1), out: 1500, ms: 30000 },
  // a Claude Code reply: at least 2000 tokens in 20 s
  { ...CC, t: at(2), out: 2000, ms: 20000 },
  // a short one tells nothing
  { ...CC, t: at(3), out: 300, ms: 20000 },
];
// the groups as usage.Breakdown sums them (TestGroupSpeedSaysHowManyRepliesItIsOf)
const G = {
  sol: { id: "gpt-6.1-sol", calls: 260, input: 2340000, output: 389000, cache_read: 0, cache_write: 0, cost: 0, timed: 1, ttft_ms: 1000, decode_ms: 4000, decode_out: 500, decode_n: 1 },
  cc: { id: "claude-opus-5-5", calls: 42, input: 210000, output: 64000, cache_read: 0, cache_write: 0, cost: 0, floor_out: 60000, floor_ms: 600000, floor_n: 30 },
  none: { id: "glm-5", calls: 12, input: 1000, output: 100, cache_read: 0, cache_write: 0, cost: 0 },
};
const SUM = ["calls", "input", "output", "timed", "ttft_ms", "decode_ms", "decode_out", "decode_n", "floor_out", "floor_ms", "floor_n"];

function ledger(period, model) {
  const groups = model ? [G[Object.keys(G).find((k) => G[k].id === model)]] : [G.sol, G.cc, G.none];
  const tot = {};
  for (const k of SUM) tot[k] = groups.reduce((a, x) => a + (x[k] || 0), 0);
  const rows = model ? ROWS.filter((r) => r.model === model) : ROWS;
  return {
    period, rows, offset: 0, total: rows.length, errors: 0, cache_read: 0, cache_write: 0, cost: 0, unpriced: 0, ...tot,
    by: { model: groups }, bucket: "hour", series: [],
    agents: [{ id: "claude", name: "Claude Code", icon: "claudecode-color" }, { id: "codex", name: "Codex", icon: "codex-color" }],
  };
}

const USAGE = {
  calls: 262, errors: 0, input: 1, output: 1, reasoning: 0, unpriced: 0, cost: 0, bucket: "day", series: [], path: "",
  agents: [{ ...G.sol, id: "codex", name: "Codex", icon: "codex-color" }, { ...G.none, id: "opencode", name: "OpenCode" }],
  models: [G.sol, G.none],
};

const STATE = (lang) => ({
  agents: [
    { id: "claude", name: "Claude Code", path: "/t/c.json", icon: "claudecode-color", wired: true, fields: [] },
    { id: "codex", name: "Codex", path: "/t/c.toml", icon: "codex-color", wired: true, fields: [] },
  ],
  profiles: [], settings: { lang, theme: "light" }, fx: { rate: 7.2, at: new Date().toISOString() },
});

function server(lang) {
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:false};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json(STATE(lang));
    if (url.pathname === "/api/usage/requests") return json(ledger(url.searchParams.get("period"), url.searchParams.get("model")));
    if (url.pathname === "/api/usage/quotas") return json([]);
    if (url.pathname === "/api/usage") return json(USAGE);
    if (url.pathname === "/api/providers") return json({ providers: [], presets: [], excluded: [], models: [], gateway: { running: false } });
    if (url.pathname === "/api/sessions") return json({ sessions: [], dirs: [] });
    if (url.pathname === "/api/sessions/stats") return json({ from: "", to: "", days: [], agents: {} });
    if (url.pathname === "/api/groups") return json({ groups: [], models: [] });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await route.fulfill({ body: await fs.readFile(file), contentType });
  };
}

const L = {
  en: { speed: "Speed", kpi: "Output speed", sol: "125 tok/s (1)", cc: "≥100 tok/s (30)", row: "125 tok/s", rowFloor: "≥100 tok/s",
    solTip: "Measured on 1 reply, from its first token to its end; the others tell no speed",
    ccTip: "At least: 30 replies read from session files, which have no first token, so the wait for it is counted too" },
  zh: { speed: "速度", kpi: "输出速度", sol: "125 token/秒（1 次）", cc: "≥100 token/秒（30 次）", row: "125 token/秒", rowFloor: "≥100 token/秒",
    solTip: "实测 1 次回复：从首字到结束；其余请求测不出速度",
    ccTip: "下限：30 次读自会话文件的回复。会话文件没有首字时间，等首字的时间也算在里面" },
  "zh-TW": { speed: "速度", kpi: null, sol: "125 token/秒（1 次）", cc: "≥100 token/秒（30 次）", row: "125 token/秒", rowFloor: "≥100 token/秒",
    solTip: "實測 1 次回覆：從首字到結束；其餘請求測不出速度",
    ccTip: "下限：30 次讀自會話檔案的回覆。會話檔案沒有首字時間，等首字的時間也算在裡面" },
  ja: { speed: null, kpi: null, sol: "125 トークン/秒（1 件）", cc: "≥100 トークン/秒（30 件）", row: "125 トークン/秒", rowFloor: "≥100 トークン/秒",
    solTip: "1 件の応答で実測：最初のトークンから終わりまで。ほかのリクエストは速度を計れません",
    ccTip: "下限：セッションファイルから読んだ 30 件の応答。最初のトークンの時刻がないため、それを待つ時間も含まれます" },
  de: { speed: null, kpi: null, sol: "125 Token/s (1)", cc: "≥100 Token/s (30)", row: "125 Token/s", rowFloor: "≥100 Token/s",
    solTip: "Gemessen an 1 Antwort, vom ersten Token bis zum Ende; die übrigen Anfragen zeigen kein Tempo",
    ccTip: "Mindestens: 30 Antworten aus Sitzungsdateien. Sie haben kein erstes Token, also zählt das Warten darauf mit" },
};

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(engine + ": a group's speed says how many replies it is of, and a floor says ≥", async (t) => {
    assert(["chromium", "webkit"].includes(engine), "BROWSER must be chromium or webkit");
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
    t.after(() => browser.close());
    for (const lang of Object.keys(L)) {
      for (const width of [1400, 560]) {
        const w = L[lang];
        await t.test(lang + " " + width, async () => {
          const errors = [];
          const ctx = await browser.newContext({ viewport: { width, height: 900 }, reducedMotion: "reduce" });
          const p = await ctx.newPage();
          p.setDefaultTimeout(5000);
          p.on("pageerror", (e) => errors.push(e.message));
          await p.route("**/*", server(lang));
          await p.goto("http://magpie.test/");
          await p.locator('[data-view="usage"]').first().click();

          // the overview's lists: the measured speed and how many it is of
          const codex = p.locator("#usageAgents .row.stat").first().locator(".sub");
          await codex.waitFor();
          assert((await codex.textContent()).includes(w.sol), "overview: " + (await codex.textContent()));
          assert(!(await p.locator("#usageAgents .row.stat").nth(1).locator(".sub").textContent()).includes("/"), "a group with no speed says none");

          await p.locator("#usageTab .opt").nth(1).click();
          await p.locator("#ledWrap .led tbody tr").first().waitFor();
          await p.waitForTimeout(200);

          // the rows say each one's: measured, and at least
          const cell = (i) => p.locator(".led tbody tr.led-row").nth(i).locator("td.speed");
          assert.equal((await cell(0).textContent()).trim(), w.row);
          assert.equal((await cell(1).textContent()).trim(), "—");
          assert.equal((await cell(2).textContent()).trim(), w.rowFloor);

          // the ranking, by tokens: each group's speed beside its requests
          const rk = (id) => p.locator("#ledRank .rk").filter({ has: p.locator(".rk-nm", { hasText: id }) });
          const speedOf = async (id) => (await rk(id).locator(".rk-speed").count()) ? (await rk(id).locator(".rk-speed").textContent()).trim() : "";
          assert.equal(await speedOf("gpt-6.1-sol"), w.sol);
          assert.equal(await rk("gpt-6.1-sol").locator(".rk-speed").getAttribute("title"), w.solTip);
          assert.equal(await speedOf("claude-opus-5-5"), w.cc);
          assert.equal(await rk("claude-opus-5-5").locator(".rk-speed").getAttribute("title"), w.ccTip);
          assert.equal(await speedOf("glm-5"), "", "no speed, nothing said");

          // the period's speed: sol's measured one, with its count
          const kpi = p.locator("#ledKpi .blk").nth(4);
          assert.equal((await kpi.locator(".v").textContent()).trim(), w.sol);
          assert((await kpi.getAttribute("title")).startsWith(w.solTip), "the card says what it is of");
          if (w.kpi) assert.equal((await kpi.locator(".k").textContent()).trim(), w.kpi);
          assert(await kpi.locator(".v").evaluate((v) => v.scrollWidth <= v.clientWidth + 1), "the card's figure is read whole");

          // the ranking by speed: what each row says, ordered by it, the floor's bar its floor
          await reader.click(p, p.locator("#ledMetric .opt").nth(3));
          await p.waitForTimeout(150);
          const vals = await p.locator("#ledRank .rk").evaluateAll((bs) => bs.map((b) => [b.querySelector(".rk-nm").textContent, b.querySelector(".rk-val").textContent.trim(), b.querySelector(".rk-val").title]));
          assert.deepEqual(vals, [["gpt-6.1-sol", w.sol, w.solTip], ["claude-opus-5-5", w.cc, w.ccTip], ["glm-5", "—", ""]]);
          const bar = async (i) => p.locator("#ledRank .rk").nth(i).locator(".rk-bar i").evaluate((e) => parseFloat(e.style.width));
          assert.equal(await bar(0), 100);
          assert.equal(await bar(1), 80, "≥100 against 125");
          const clipped = await p.locator("#ledRank .rk-val").evaluateAll((vs) => vs.filter((v) => v.scrollWidth > v.clientWidth + 1).length);
          assert.equal(clipped, 0, "every figure is read whole");

          // Claude Code's model alone: only floors, so the period's speed is at least
          await reader.click(p, rk("claude-opus-5-5"));
          await p.waitForFunction(() => document.querySelectorAll(".led tbody tr.led-row").length === 2);
          await p.waitForTimeout(150);
          assert.equal((await p.locator("#ledKpi .blk").nth(4).locator(".v").textContent()).trim(), w.cc);
          assert((await p.locator("#ledKpi .blk").nth(4).getAttribute("title")).startsWith(w.ccTip));

          // no sideways scroll of the page
          assert(await p.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1), "no sideways scroll");
          assert.deepEqual(errors, []);
          await ctx.close();
        });
      }
    }
  });
}
