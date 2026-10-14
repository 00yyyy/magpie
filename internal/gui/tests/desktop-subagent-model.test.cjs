// Run with Node's test runner and Playwright on the module path; see README.md.
// Desktop's gateway-applied subagent model shares Codex's square and picker.
// Desktop's routing trace has a subagent pick without a kind. All APIs and
// assets are intercepted; no backend or external network is used.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const models = [
  { value: "fake/m1", label: "M1", ref: "fake/m1", group: "Fake" },
  { value: "fake/m2", label: "M2", ref: "fake/m2", group: "Fake" },
];
const fresh = (wired = true) => ({
  agents: [{
    id: "claude-desktop", name: "Claude Desktop", path: "/test/claude_desktop_config.json", icon: "claude-color", wired,
    fields: [
      { key: "provider", label: "provider", value: wired ? "magpie" : "", options: [{ value: "magpie", label: "magpie" }] },
      ...["opus", "sonnet", "haiku", "fable"].map((tier) => ({ key: tier, label: tier, value: "", options: wired ? models : [] })),
      { key: "subagent_model", label: "subagent model", value: "", options: wired ? models : [] },
    ],
  }],
  profiles: [],
});

const now = new Date();
const day = [now.getFullYear(), now.getMonth() + 1, now.getDate()].map((n) => String(n).padStart(2, "0")).join("-");
const at = (i) => new Date(now.getTime() - (i + 1) * 60e3).toISOString();
const seat = { id: "fake", provider: "fake", name: "Fake", who: "Test key", kind: "key", agent: "claude-desktop", model: "m2" };
// Deliberately no kind: Desktop must explain the pick without a Codex tag.
const routes = [
  { id: 2, seq: 2, time: at(0), agent: "claude-desktop", model: "fake/m1", provider: "fake", subagent: { asked: "fake/m1", to: "fake/m2" },
    order: [seat], tries: [{ id: "fake", model: "m2", start: at(0), done: true, status: 200, ms: 900 }], done: true, status: 200, ms: 900, tokens: 1200 },
  { id: 1, seq: 1, time: at(1), agent: "claude-desktop", model: "fake/m1", provider: "fake", subagent: { asked: "fake/m1", to: "fake/m2", kept: "it is disabled in magpie" },
    order: [{ ...seat, model: "m1" }], tries: [{ id: "fake", model: "m1", start: at(1), done: true, status: 200, ms: 900 }], done: true, status: 200, ms: 900, tokens: 1200 },
];

function server(lang, fixture) {
  return async (route) => {
    const req = route.request(), url = new URL(req.url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ ...fixture.state, settings: { lang, theme: "light" } });
    if (url.pathname === "/api/set") {
      const body = req.postDataJSON();
      fixture.sets.push(body);
      fixture.state = JSON.parse(JSON.stringify(fixture.state));
      fixture.state.agents.find((a) => a.id === body.agent).fields.find((f) => f.key === body.field).value = body.value;
      return json({ ...fixture.state, settings: { lang, theme: "light" } });
    }
    if (url.pathname === "/api/gateway/trace") {
      if (url.searchParams.get("wait")) await new Promise((r) => setTimeout(r, 20e3));
      return json({ mine: true, now: now.toISOString(), seq: 1, totals: { requests: 0, rerouted: 0, errors: 0 }, routes: [] });
    }
    if (url.pathname === "/api/gateway/history") {
      const d = url.searchParams.get("day");
      return json({ cut: false, days: [{ day, requests: routes.length }], routes: d ? routes : [] });
    }
    if (url.pathname === "/api/usage/quotas") return json([]);
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname === "/api/agents/cli") return json({ agents: {}, pending: false });
    if (url.pathname === "/api/providers") return json({ providers: [], presets: [], excluded: [], gateway: { running: true, window: true } });
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await route.fulfill({ body: await fs.readFile(file), contentType });
  };
}

// Expected reader-facing words, independent of the translation lookup.
const W = {
  en: { unset: "subagent model: the model the lead asks for", def: "Default", set: "subagent model: M2",
    note: "each subagent on the model Claude Desktop's lead asks for",
    moved: "Claude Desktop requested a subagent on fake/m1; magpie put it on fake/m2, the model set for its subagents.",
    kept: "Claude Desktop requested a subagent on fake/m1 and it stayed there: its subagents are set to fake/m2, but it is disabled in magpie." },
  zh: { unset: "子代理模型：主代理请求的模型", def: "默认", set: "子代理模型：M2",
    note: "每个子代理都用 Claude Desktop 主代理请求的模型",
    moved: "Claude Desktop 请求了使用 fake/m1 的子代理；magpie 将其改用 fake/m2，即为它的子代理设定的模型。",
    kept: "Claude Desktop 请求了使用 fake/m1 的子代理，并保留在该模型上：它的子代理设为 fake/m2，但该模型在 magpie 中已停用。" },
  "zh-TW": { unset: "子代理模型：主代理請求的模型", def: "預設", set: "子代理模型：M2",
    note: "每個子代理都用 Claude Desktop 主代理請求的模型",
    moved: "Claude Desktop 請求了使用 fake/m1 的子代理；magpie 將其改用 fake/m2，即為它的子代理設定的模型。",
    kept: "Claude Desktop 請求了使用 fake/m1 的子代理，並保留在該模型上：它的子代理設為 fake/m2，但該模型在 magpie 中已停用。" },
  ja: { unset: "サブエージェントのモデル：リードがリクエストしたモデル", def: "デフォルト", set: "サブエージェントのモデル：M2",
    note: "各サブエージェントは Claude Desktop のリードがリクエストしたモデルを使います",
    moved: "Claude Desktop が fake/m1 のサブエージェントをリクエストしました。magpie はサブエージェント用に設定された fake/m2 に切り替えました。",
    kept: "Claude Desktop が fake/m1 のサブエージェントをリクエストし、そのモデルのままになりました。サブエージェントは fake/m2 に設定されていますが、このモデルは magpie で無効になっています。" },
  de: { unset: "Modell der Subagenten: das Modell, das der Haupt-Agent anfordert", def: "Standard", set: "Modell der Subagenten: M2",
    note: "jeder Subagent auf dem Modell, das der Haupt-Agent von Claude Desktop anfordert",
    moved: "Claude Desktop hat einen Subagent mit fake/m1 angefordert; magpie hat ihn auf fake/m2 gesetzt, das für seine Subagenten festgelegte Modell.",
    kept: "Claude Desktop hat einen Subagent mit fake/m1 angefordert, und er blieb dort: Seine Subagenten sind auf fake/m2 gesetzt, aber es ist in magpie deaktiviert." },
};

const cd = '.row.agent[data-id="claude-desktop"]';
const engines = process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"];
const langs = process.env.LANGS ? process.env.LANGS.split(",") : Object.keys(W);

for (const engine of engines) {
  for (const lang of langs) {
    for (const width of [1100, 420]) {
      test(`${engine} ${lang} ${width}px: Desktop's subagent square, translated picker and routing story`, async (t) => {
        const w = W[lang];
        const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
        const page = await browser.newPage({ viewport: { width, height: 760 } });
        page.setDefaultTimeout(5000);
        const errors = [], fixture = { state: fresh(), sets: [] };
        page.on("pageerror", (e) => errors.push(e.message));
        await page.route("**/*", server(lang, fixture));
        t.after(async () => {
          if (process.env.ARTIFACT_DIR) {
            await fs.mkdir(process.env.ARTIFACT_DIR, { recursive: true });
            await page.screenshot({ path: path.join(process.env.ARTIFACT_DIR, `${engine}-${lang}-${width}-desktop-subagent-model.png`) });
          }
          await browser.close();
        });
        await page.goto("http://magpie.test/");
        await page.locator(cd).waitFor();
        await page.locator(`${cd} .ag-link`).click();
        await page.locator(`${cd} .ag-exp`).waitFor();

        const square = page.locator(`${cd} .ag-exp .field.extra[data-key="subagent_model"]`);
        assert.equal(await square.count(), 1, "Desktop's opened row has one subagent square");
        assert.equal(await page.locator(`${cd} .field:not(.extra)[data-key="subagent_model"]`).count(), 0, "no plain subagent picker");
        assert.equal(await square.getAttribute("title"), w.unset, "translated unset title");
        assert.equal(await square.getAttribute("aria-label"), w.unset);
        assert.equal(await square.evaluate((e) => e.classList.contains("set")), false);
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, "no sideways scroll");
        const box = await square.boundingBox();
        assert(box && box.x >= 0 && box.x + box.width <= width, JSON.stringify(box));

        const y = await page.evaluate(() => scrollY);
        await square.click();
        await page.locator("#pop:not([hidden]) #list li").first().waitFor();
        assert.equal(await page.evaluate(() => scrollY), y, "opening the square does not scroll");
        assert.equal(await page.locator("#pop").evaluate((e) => e.classList.contains("model-picker")), true, "app's own model picker");
        assert.equal(await page.locator("select").count(), 0, "no native select");
        const first = await page.locator("#pop #list li").first().evaluate((e) => [e.querySelector(".v")?.textContent, e.querySelector(".n")?.textContent]);
        assert.deepEqual(first, [w.def, w.note], "translated Default first with Desktop's lead note");
        const items = await page.locator("#pop #list li").allInnerTexts();
        for (const model of models) assert(items.some((s) => s.includes(model.label)), `${model.label} offered by magpie`);
        const pop = await page.locator("#pop").boundingBox();
        assert(pop && pop.x >= 0 && pop.x + pop.width <= width, JSON.stringify(pop));
        const left = await page.locator(`${cd} .field.extra[data-key="subagent_model"], #pop #list li`).evaluateAll((es) => es.flatMap((e) => [e, ...e.querySelectorAll("*")]).filter((e) => {
          const s = getComputedStyle(e);
          return parseFloat(s.borderLeftWidth) > 1 && s.borderLeftWidth !== s.borderRightWidth;
        }).length);
        assert.equal(left, 0, "no coloured left border on square or picker rows");

        await page.locator("#pop #list li", { hasText: "M2" }).first().click();
        await page.waitForFunction(() => document.querySelector('.row.agent[data-id="claude-desktop"] .field.extra.set[data-key="subagent_model"]'));
        assert.deepEqual(fixture.sets, [{ agent: "claude-desktop", field: "subagent_model", value: "fake/m2" }], "Desktop pick posts the catalog id");
        assert.equal(await square.getAttribute("title"), w.set);
        assert.equal(await square.getAttribute("aria-label"), w.set);
        assert.equal(await page.evaluate(() => scrollY), y, "picking does not scroll");

        await square.click();
        await page.locator("#pop:not([hidden]) #list li").first().waitFor();
        assert.equal(await page.evaluate(() => scrollY), y, "reopening does not scroll");
        await page.locator("#pop #list li").first().click();
        await page.waitForFunction(() => !document.querySelector('.row.agent[data-id="claude-desktop"] .field.extra.set[data-key="subagent_model"]'));
        assert.deepEqual(fixture.sets, [
          { agent: "claude-desktop", field: "subagent_model", value: "fake/m2" },
          { agent: "claude-desktop", field: "subagent_model", value: "" },
        ], "Default posts an empty value");
        assert.equal(await square.getAttribute("title"), w.unset);
        assert.equal(await page.evaluate(() => scrollY), y, "Default does not scroll");

        fixture.state = fresh(false);
        await page.goto("http://magpie.test/");
        await page.locator(cd).waitFor();
        assert.equal(await page.locator(`${cd} .field[data-key="subagent_model"]`).count(), 0, "unwired Desktop with no options or value has no square");

        // A narrow window covers the list with the story, so reopen it afresh.
        for (const [i, said] of [[0, w.moved], [1, w.kept]]) {
          await page.goto("http://magpie.test/?view=routing");
          await page.locator(".rt-days .rt-day").nth(1).click();
          await page.locator(".rt-req").nth(1).waitFor();
          await page.locator(".rt-req").nth(i).evaluate((e) => e.click());
          await page.waitForFunction((i) => document.querySelectorAll(".rt-req")[i]?.getAttribute("aria-pressed") === "true", i);
          const story = await page.locator(".rt-steps").textContent();
          assert(story.includes(said), `Desktop ${i === 0 ? "moved" : "kept"} story: ${said}\nnot in\n${story}`);
          assert.equal(await page.locator(".rt-steps .aside", { hasText: said }).count(), 1, "one subagent aside without a kind");
          assert.equal(await page.locator(".rt-steps .kind").count(), 0, "Desktop has no Codex purpose tag");
          if (i === 1 && lang !== "en") assert(!story.includes("it is disabled in magpie"), "kept reason must be translated");
        }
        assert.deepEqual(errors, []);
      });
    }
  }
}
